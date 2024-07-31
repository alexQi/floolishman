package caller

import (
	"errors"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

var (
	InitMoreRatio = 0.5
)

type CallerDual struct {
	CallerCommon
}

func (c *CallerDual) Start() {
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
		tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
		for {
			select {
			case <-tickerCheck.C:
				for _, option := range c.pairOptions {
					c.openDualPosition(option)
				}
			case <-tickerClose.C:
				for _, option := range c.pairOptions {
					c.closePosition(option)
				}
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()
	//c.Listen()
}

func (cc *CallerDual) Listen() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			cc.checkOrderTimeout()
		}
	}
}

func (cc *CallerDual) checkOrderTimeout() {
	// 判断当前是否已有同向挂单未成交，有则不在开单
	// todo
}

// 当前模式下仓位比例
//"fullSapceRatio": 0.05,
//"stopSpaceRatio": 0.20,
//"baseLossRatio": 0.002,
//"profitableScale": 0.12,
//"initProfitRatioLimit": 0.20

func (s *CallerDual) openDualPosition(option model.PairOption) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	currentPrice := s.pairPrices[option.Pair]
	// 判断当前资产
	_, quotePosition, err := s.broker.PairAsset(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	// 判断当前是否已有同向挂单未成交，有则不在开单
	existUnfilledOrderMap, err := s.broker.GetPositionOrdersForPairUnfilled(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	unfillePositionOrders, ok := existUnfilledOrderMap["position"]
	if ok == true && len(unfillePositionOrders) > 0 {
		for _, unfillePositionOrder := range unfillePositionOrders {
			utils.Log.Infof(
				"[POSITION - EXSIT %s] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v, CandleTime: %s | (UNFILLED)",
				unfillePositionOrder.PositionSide,
				unfillePositionOrder.OrderFlag,
				unfillePositionOrder.Pair,
				unfillePositionOrder.Quantity,
				unfillePositionOrder.Price,
				currentPrice,
				s.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		return
	}
	// 重置当前交易对止损比例
	s.profitRatioLimit[option.Pair] = 0
	// 计算仓位大小
	amount := calc.OpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPrice, 1, s.setting.FullSpaceRatio)
	utils.Log.Infof(
		"[POSITION OPENING] Pair: %s | Quantity: %v | Price: %v | CandleTime: %s",
		option.Pair,
		amount,
		currentPrice,
		s.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
	)
	// 批量下单
	_, err = s.broker.BatchCreateOrderLimit([]*model.OrderParam{
		{
			Side:         model.SideTypeSell,
			PositionSide: model.PositionSideTypeShort,
			Pair:         option.Pair,
			Quantity:     amount,
			Limit:        currentPrice,
			Extra: model.OrderExtra{
				Leverage: option.Leverage,
			},
		},
		{
			Side:         model.SideTypeBuy,
			PositionSide: model.PositionSideTypeLong,
			Pair:         option.Pair,
			Quantity:     amount,
			Limit:        currentPrice,
			Extra: model.OrderExtra{
				Leverage: option.Leverage,
			},
		},
	})
	if err != nil {
		utils.Log.Error(err)
	}
}

func (cc *CallerDual) closePosition(option model.PairOption) {
	cc.mu.Lock()         // 加锁
	defer cc.mu.Unlock() // 解锁
	// 获取当前已存在的仓位
	openedPositions, err := cc.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) < 2 {
		return
	}

	openedPositionMap := map[model.PositionSideType]*model.Position{}
	for _, position := range openedPositions {
		openedPositionMap[model.PositionSideType(position.PositionSide)] = position
	}
	_, longOk := openedPositionMap[model.PositionSideTypeLong]
	_, shortOk := openedPositionMap[model.PositionSideTypeShort]
	if !longOk || !shortOk {
		utils.Log.Error(errors.New("Position error,some position is nil"))
	}

	currentPrice := cc.pairPrices[option.Pair]
	profitValue, mainPosition, subPosition, breakEvenPrice := cc.CalculateProfit(openedPositionMap)
	// 判断当前利润是否时正数
	if mainPosition.Quantity == subPosition.Quantity {
		// 多头头寸加仓操作
		if currentPrice > breakEvenPrice {
			_, err := cc.broker.CreateOrderMarket(
				model.SideType(mainPosition.Side),
				model.PositionSideType(mainPosition.PositionSide),
				mainPosition.Pair,
				mainPosition.Quantity*InitMoreRatio,
				model.OrderExtra{
					Leverage:  mainPosition.Leverage,
					OrderFlag: mainPosition.OrderFlag,
				},
			)
			if err != nil {
				utils.Log.Error(err)
			}
		} else {
			// 空头头寸加仓操作
			_, err := cc.broker.CreateOrderMarket(
				model.SideType(subPosition.Side),
				model.PositionSideType(subPosition.PositionSide),
				subPosition.Pair,
				subPosition.Quantity*InitMoreRatio,
				model.OrderExtra{
					Leverage:  subPosition.Leverage,
					OrderFlag: subPosition.OrderFlag,
				},
			)
			if err != nil {
				utils.Log.Error(err)
			}
		}
	} else {
		// 判断亏损利润比达到最大亏损利润比，则平掉双向仓位
		profitRatio := calc.CalculateDualProfitRatio(
			model.SideType(mainPosition.Side),
			mainPosition.Quantity,
			mainPosition.AvgPrice,
			subPosition.Quantity,
			subPosition.AvgPrice,
			float64(mainPosition.Leverage),
		)
		if profitValue > 0 {
			// 判断当前盈亏比是否大于触发盈亏比
			if profitRatio < cc.setting.InitProfitRatioLimit {
				utils.Log.Infof(
					"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < InitProfitRatioLimit",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
				)
				return
			}
			// 判断利润比小于等于上次设置的利润比，则平仓
			if profitRatio <= cc.profitRatioLimit[option.Pair] {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < StopLossRatio: %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", cc.profitRatioLimit[option.Pair]*100),
				)
				cc.finishPosition(mainPosition, subPosition)
				return
			}
			// 利润比大于上次设置的利润比，重设利润比
			if profitRatio > (cc.profitRatioLimit[option.Pair] + cc.setting.ProfitableScale + 0.01) {
				cc.profitRatioLimit[option.Pair] = profitRatio - cc.setting.ProfitableScale
				utils.Log.Infof(
					"[POSITION - PROFIT] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > NewProfitRatio: %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", cc.profitRatioLimit[option.Pair]*100),
				)
			} else {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > LastProfitRatio: %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", cc.profitRatioLimit[option.Pair]*100),
				)
			}
		} else {
			// 亏损盈利比已大于最大
			if profitRatio < 0 && calc.Abs(profitRatio) > cc.setting.BaseLossRatio {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", cc.setting.BaseLossRatio*100),
				)
				cc.finishPosition(mainPosition, subPosition)
				return
			}
			// 判断当前仓位比例，仓位比例超过一定程度不在加仓
			_, quotePosition, err := cc.broker.PairAsset(option.Pair)
			if err != nil {
				utils.Log.Error(err)
				return
			}
			// 仓位比例已达到加仓上线，则不在加仓
			stopPositionRatio := calc.StopPositionSizeRatio(quotePosition, float64(mainPosition.Leverage), mainPosition.AvgPrice, mainPosition.Quantity)
			if stopPositionRatio >= cc.setting.StopSpaceRatio {
				utils.Log.Infof(
					"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (Position Size In Max: %s)",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					subPosition.OrderFlag,
					subPosition.Quantity,
					subPosition.AvgPrice,
					subPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", stopPositionRatio*100),
				)
				return
			}
			// 判断当前是否已有同向挂单未成交，有则不在开单
			existUnfilledOrderMap, err := cc.broker.GetPositionOrdersForPairUnfilled(option.Pair)
			if err != nil {
				utils.Log.Error(err)
				return
			}
			unfillePositionOrders, ok := existUnfilledOrderMap["position"]
			// 当前有未成交的加仓订单
			if ok == true {
				// 未成交的加仓订单方向与要加仓的方向一致,不在加仓
				if _, exsit := unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)]; exsit {
					utils.Log.Infof(
						"[POSITION - EXSIT] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v | (UNFILLED MORE)",
						unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)].OrderFlag,
						unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)].Pair,
						unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)].Quantity,
						unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)].Price,
						cc.pairPrices[option.Pair],
					)
					return
				}
			}
			utils.Log.Infof(
				"[POSITION - MORE] Pair: %s | Main OrderFlag: %s, Quantity: %v ( +%v ), Price: %v | Current: %v | PR.%%: %s",
				subPosition.Pair,
				subPosition.OrderFlag,
				subPosition.Quantity,
				subPosition.Quantity*InitMoreRatio,
				subPosition.AvgPrice,
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			// 亏损状态下给副仓位加仓
			_, err = cc.broker.CreateOrderMarket(
				model.SideType(subPosition.Side),
				model.PositionSideType(subPosition.PositionSide),
				subPosition.Pair,
				subPosition.Quantity*InitMoreRatio,
				model.OrderExtra{
					Leverage:  subPosition.Leverage,
					OrderFlag: subPosition.OrderFlag,
				},
			)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		}
	}
}

func (cc *CallerDual) finishPosition(mainPosition *model.Position, subPosition *model.Position) {
	// 批量下单
	_, err := cc.broker.BatchCreateOrderMarket([]*model.OrderParam{
		{
			// 平掉副仓位
			Side:         model.SideType(mainPosition.Side),
			PositionSide: model.PositionSideType(subPosition.PositionSide),
			Pair:         subPosition.Pair,
			Quantity:     subPosition.Quantity,
			Extra: model.OrderExtra{
				Leverage:  subPosition.Leverage,
				OrderFlag: subPosition.OrderFlag,
			},
		},
		{
			// 平掉主仓位
			Side:         model.SideType(subPosition.Side),
			PositionSide: model.PositionSideType(mainPosition.PositionSide),
			Pair:         mainPosition.Pair,
			Quantity:     mainPosition.Quantity,
			Extra: model.OrderExtra{
				Leverage:  mainPosition.Leverage,
				OrderFlag: mainPosition.OrderFlag,
			},
		},
	})
	if err != nil {
		utils.Log.Error(err)
	}
}

func (cc *CallerDual) CalculateProfit(positionMap map[model.PositionSideType]*model.Position) (float64, *model.Position, *model.Position, float64) {
	var mainProfitValue, subProfitValue float64
	var mainPosition, subPosition *model.Position
	var breakEvenPrice, breakEvenQuantity float64

	_, longOk := positionMap[model.PositionSideTypeLong]
	_, shortOk := positionMap[model.PositionSideTypeShort]
	if !longOk || !shortOk {
		return mainProfitValue + subProfitValue, mainPosition, subPosition, breakEvenPrice
	}
	if positionMap[model.PositionSideTypeLong].Quantity == positionMap[model.PositionSideTypeShort].Quantity {
		mainPosition = positionMap[model.PositionSideTypeLong]
		subPosition = positionMap[model.PositionSideTypeShort]
	} else {
		if positionMap[model.PositionSideTypeLong].Quantity > positionMap[model.PositionSideTypeShort].Quantity {
			mainPosition = positionMap[model.PositionSideTypeLong]
			subPosition = positionMap[model.PositionSideTypeShort]
			// 主仓位利润
			mainProfitValue = (cc.pairPrices[mainPosition.Pair] - mainPosition.AvgPrice) * mainPosition.Quantity
			// 副仓位利润
			subProfitValue = (subPosition.AvgPrice - cc.pairPrices[mainPosition.Pair]) * subPosition.Quantity
		} else {
			mainPosition = positionMap[model.PositionSideTypeShort]
			subPosition = positionMap[model.PositionSideTypeLong]
			// 主仓位利润
			mainProfitValue = (mainPosition.AvgPrice - cc.pairPrices[mainPosition.Pair]) * mainPosition.Quantity
			// 副仓位利润
			subProfitValue = (cc.pairPrices[mainPosition.Pair] - subPosition.AvgPrice) * subPosition.Quantity
		}
	}
	// 计算盈亏平衡价格和数量
	if mainPosition.Quantity == subPosition.Quantity {
		breakEvenPrice = mainPosition.AvgPrice
	} else if mainPosition.Quantity > subPosition.Quantity {
		breakEvenQuantity = mainPosition.Quantity - subPosition.Quantity
		breakEvenPrice = (mainPosition.AvgPrice*mainPosition.Quantity - subPosition.AvgPrice*subPosition.Quantity) / breakEvenQuantity
	} else {
		breakEvenQuantity = subPosition.Quantity - mainPosition.Quantity
		breakEvenPrice = (subPosition.AvgPrice*subPosition.Quantity - mainPosition.AvgPrice*mainPosition.Quantity) / breakEvenQuantity
	}

	return mainProfitValue + subProfitValue, mainPosition, subPosition, breakEvenPrice

}
