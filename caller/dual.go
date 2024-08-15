package caller

import (
	"floolishman/constants"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

type CallerDual struct {
	CallerCommon
	pairTriggerPrice map[string]float64
}

func (c *CallerDual) Start() {
	c.pairTriggerPrice = make(map[string]float64)
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Millisecond)
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
	go c.Listen()
}

func (cc *CallerDual) Listen() {
	go cc.tickCheckOrderTimeout()
}

// 当前模式下仓位比例
//"MaxMarginRatio": 0.20,
//"MaxMarginLossRatio": 0.002,

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
	// 获取当前已存在的仓位
	openedPositions, err := s.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) > 0 {
		for _, openedPosition := range openedPositions {
			utils.Log.Infof(
				"[POSITION - EXSIT %s] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v, CandleTime: %s",
				openedPosition.PositionSide,
				openedPosition.OrderFlag,
				openedPosition.Pair,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				currentPrice,
				s.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		return
	}
	// 判断当前是否已有挂单未成交，有则不在开单
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
	unfilleLossOrders, ok := existUnfilledOrderMap["lossLimit"]
	if ok == true && len(unfilleLossOrders) > 0 {
		for _, unfilleLossOrder := range unfilleLossOrders {
			utils.Log.Infof(
				"[POSITION - EXSIT %s] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v, CandleTime: %s | (CLOSE PENDING)",
				unfilleLossOrder.PositionSide,
				unfilleLossOrder.OrderFlag,
				unfilleLossOrder.Pair,
				unfilleLossOrder.Quantity,
				unfilleLossOrder.Price,
				currentPrice,
				s.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		return
	}
	if _, ok := s.pairTriggerPrice[option.Pair]; !ok {
		s.pairTriggerPrice[option.Pair] = currentPrice
		return
	}
	// 重置当前交易对止损比例
	s.profitRatioLimit[option.Pair] = 0
	// 计算仓位大小
	var amount float64
	if option.MarginMode == constants.MarginModeRoll {
		amount = calc.OpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPrice, 1, option.MarginSize)
	} else {
		amount = option.MarginSize
	}
	utils.Log.Infof(
		"[POSITION OPENING] Pair: %s | Quantity: %v | Price: %v | CandleTime: %s",
		option.Pair,
		amount,
		currentPrice,
		s.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
	)
	var finalSide model.SideType
	var postionSide model.PositionSideType

	if currentPrice > s.pairTriggerPrice[option.Pair] {
		finalSide = model.SideTypeBuy
		postionSide = model.PositionSideTypeLong
	} else {
		finalSide = model.SideTypeSell
		postionSide = model.PositionSideTypeShort
	}
	// 根据最新价格创建限价单
	_, err = s.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
		Leverage: option.Leverage,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	delete(s.pairTriggerPrice, option.Pair)
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
	if len(openedPositions) == 0 {
		return
	}

	openedPositionMap := map[model.PositionSideType]*model.Position{}
	for _, position := range openedPositions {
		openedPositionMap[model.PositionSideType(position.PositionSide)] = position
	}

	lossRatio := option.MaxMarginLossRatio * float64(option.Leverage)
	currentPrice := cc.pairPrices[option.Pair]
	mainPosition, subPosition := cc.judePosition(option, currentPrice, openedPositionMap)

	// 判断当前是否已有同向挂单未成交，有则不在开单
	existUnfilledOrderMap, err := cc.broker.GetPositionOrdersForPairUnfilled(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if subPosition.Quantity == 0 {
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
	}
	// 定义需要加仓仓位
	var morePosition *model.Position
	// 判断亏损利润比达到最大亏损利润比，则平掉双向仓位
	profitRatio := calc.CalculateDualProfitRatio(
		model.SideType(mainPosition.Side),
		mainPosition.Quantity,
		mainPosition.AvgPrice,
		subPosition.Quantity,
		subPosition.AvgPrice,
		currentPrice,
		float64(mainPosition.Leverage),
	)
	// 判断当前仓位比例，仓位比例超过一定程度不在加仓
	_, quotePosition, err := cc.broker.PairAsset(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 仓位比例已达到加仓上线，则不在加仓
	stopPositionRatio := calc.StopPositionSizeRatio(
		quotePosition,
		float64(mainPosition.Leverage),
		mainPosition.AvgPrice,
		mainPosition.Quantity,
	)
	if profitRatio > 0 {
		// 已经达到最大加仓时，盈利超过设置止盈的一半就平仓防止亏损扩大
		if stopPositionRatio >= option.MaxMarginRatio && profitRatio > option.ProfitableTrigger/2 {
			utils.Log.Infof(
				"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (Position Size In Max: %s [%s] With Profit)",
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
				fmt.Sprintf("%.2f%%", option.MaxMarginRatio*100),
			)
			cc.finishAllPosition(mainPosition, subPosition)
			// 当仓位无法增加
			return
		}
		// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
		if profitRatio <= cc.profitRatioLimit[option.Pair] && cc.profitRatioLimit[option.Pair] > 0 {
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
			cc.finishAllPosition(mainPosition, subPosition)
			return
		}
		// 判断当前盈亏比是否大于触发盈亏比
		if profitRatio < option.ProfitableTrigger || profitRatio < (cc.profitRatioLimit[option.Pair]+option.ProfitableScale+0.01) {
			utils.Log.Infof(
				"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitableTrigger: %s",
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
				fmt.Sprintf("%.2f%%", option.ProfitableTrigger*100),
			)
			return
		}
		// 重设利润比
		cc.profitRatioLimit[option.Pair] = profitRatio - option.ProfitableScale
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
		// 亏损盈利比已大于最大
		if calc.Abs(profitRatio) > lossRatio {
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
				fmt.Sprintf("%.2f%%", lossRatio*100),
			)
			cc.finishAllPosition(mainPosition, subPosition)
			return
		}
		// 判断是否要加仓
		if calc.Abs(profitRatio) < StepMoreRatio {
			utils.Log.Infof(
				"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < LossStepRatio: %s",
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
				fmt.Sprintf("%.2f%%", StepMoreRatio*100),
			)
			return
		}
		// 判断能否加仓
		if stopPositionRatio >= option.MaxMarginRatio {
			utils.Log.Infof(
				"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (Position Size In Max: %s [%s])",
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
				fmt.Sprintf("%.2f%%", option.MaxMarginRatio*100),
			)
			// 当仓位无法增加
			return
		}
		morePosition = subPosition
		unfillePositionOrders, ok := existUnfilledOrderMap["position"]
		// 当前有未成交的加仓订单
		if ok == true {
			// 未成交的加仓订单方向与要加仓的方向一致,不在加仓
			if _, exsit := unfillePositionOrders[model.PositionSideType(morePosition.PositionSide)]; exsit {
				utils.Log.Infof(
					"[POSITION - EXSIT] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v | (UNFILLED MORE)",
					unfillePositionOrders[model.PositionSideType(morePosition.PositionSide)].OrderFlag,
					unfillePositionOrders[model.PositionSideType(morePosition.PositionSide)].Pair,
					unfillePositionOrders[model.PositionSideType(morePosition.PositionSide)].Quantity,
					unfillePositionOrders[model.PositionSideType(morePosition.PositionSide)].Price,
					cc.pairPrices[option.Pair],
				)
				return
			}
		}
		// 根据当前联合仓位保证金亏损比例计算加仓数量
		addAmount := calc.CalculateAddQuantity(
			model.SideType(mainPosition.Side),
			mainPosition.Quantity,
			mainPosition.AvgPrice,
			subPosition.Quantity,
			subPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			ProfitMoreHedgeRatio,
		)
		utils.Log.Infof(
			"[POSITION - MORE] Pair: %s | Main OrderFlag: %s, Quantity: %v ( +%v ), Price: %v | Current: %v | PR.%%: %s",
			morePosition.Pair,
			morePosition.OrderFlag,
			morePosition.Quantity,
			addAmount,
			morePosition.AvgPrice,
			currentPrice,
			fmt.Sprintf("%.2f%%", profitRatio*100),
		)
		// 亏损状态下给副仓位加仓
		_, err = cc.broker.CreateOrderLimit(
			model.SideType(morePosition.Side),
			model.PositionSideType(morePosition.PositionSide),
			morePosition.Pair,
			addAmount,
			currentPrice,
			model.OrderExtra{
				Leverage:  morePosition.Leverage,
				OrderFlag: morePosition.OrderFlag,
			},
		)
		if err != nil {
			utils.Log.Error(err)
			return
		}
	}
}

func (cc *CallerDual) judePosition(option model.PairOption, price float64, positionMap map[model.PositionSideType]*model.Position) (*model.Position, *model.Position) {
	var mainPosition, subPosition *model.Position
	if _, ok := positionMap[model.PositionSideTypeLong]; !ok {
		positionMap[model.PositionSideTypeLong] = &model.Position{
			Pair:         option.Pair,
			Leverage:     option.Leverage,
			Side:         string(model.SideTypeBuy),
			PositionSide: string(model.PositionSideTypeLong),
			MarginType:   string(option.MarginType),
			Quantity:     0,
			AvgPrice:     price,
		}
	}
	if _, ok := positionMap[model.PositionSideTypeShort]; !ok {
		positionMap[model.PositionSideTypeShort] = &model.Position{
			Pair:         option.Pair,
			Leverage:     option.Leverage,
			Side:         string(model.SideTypeSell),
			PositionSide: string(model.PositionSideTypeShort),
			MarginType:   string(option.MarginType),
			Quantity:     0,
			AvgPrice:     price,
		}
	}
	if positionMap[model.PositionSideTypeLong].Quantity == positionMap[model.PositionSideTypeShort].Quantity {
		mainPosition = positionMap[model.PositionSideTypeLong]
		subPosition = positionMap[model.PositionSideTypeShort]
	} else {
		if positionMap[model.PositionSideTypeLong].Quantity > positionMap[model.PositionSideTypeShort].Quantity {
			mainPosition = positionMap[model.PositionSideTypeLong]
			subPosition = positionMap[model.PositionSideTypeShort]
		} else {
			mainPosition = positionMap[model.PositionSideTypeShort]
			subPosition = positionMap[model.PositionSideTypeLong]
		}
	}
	return mainPosition, subPosition

}
