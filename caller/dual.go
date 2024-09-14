package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

type Dual struct {
	Common
	pairTriggerPrice map[string]float64
}

func (c *Dual) Start() {
	c.pairTriggerPrice = make(map[string]float64)
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Millisecond)
		tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
		for {
			select {
			case <-tickerCheck.C:
				for _, option := range c.pairOptions {
					if option.Status == false {
						continue
					}
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

func (c *Dual) Listen() {
	go c.tickCheckOrderTimeout()
}

// 当前模式下仓位比例
//"MaxMarginRatio": 0.20,
//"MaxMarginLossRatio": 0.002,

func (c *Dual) openDualPosition(option *model.PairOption) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁
	currentPrice, _ := c.pairPrices.Get(option.Pair)
	// 判断当前资产
	_, quotePosition, err := c.broker.PairAsset(option.Pair)
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
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) > 0 {
		for _, openedPosition := range openedPositions {
			utils.Log.Infof("[POSITION - EXSIT] %s | Current: %v", openedPosition.String(), currentPrice)
		}
		return
	}
	// 判断当前是否已有挂单未成交，有则不在开单
	existUnfilledOrderMap, err := c.broker.GetPositionOrdersForPairUnfilled(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	unfillePositionOrders, ok := existUnfilledOrderMap["position"]
	if ok == true && len(unfillePositionOrders) > 0 {
		for _, unfillePositionOrder := range unfillePositionOrders {
			utils.Log.Infof("[POSITION - EXSIT] %s | Current: %v | (UNFILLED)", unfillePositionOrder.String(), currentPrice)
		}
		return
	}
	unfilleLossOrders, ok := existUnfilledOrderMap["lossLimit"]
	if ok == true && len(unfilleLossOrders) > 0 {
		for _, unfilleLossOrder := range unfilleLossOrders {
			utils.Log.Infof("[POSITION - EXSIT] %s | Current: %v | (CLOSE PENDING)", unfilleLossOrder.String(), currentPrice)
		}
		return
	}
	if _, ok := c.pairTriggerPrice[option.Pair]; !ok {
		c.pairTriggerPrice[option.Pair] = currentPrice
		return
	}
	// 重置当前交易对止损比例
	c.resetPairProfit(option.Pair)
	// 计算仓位大小
	amount := c.getPositionMargin(quotePosition, currentPrice, option)
	utils.Log.Infof(
		"[POSITION OPENING] Pair: %s | Quantity: %v | Price: %v",
		option.Pair,
		amount,
		currentPrice,
	)
	var finalSide model.SideType
	var postionSide model.PositionSideType

	if currentPrice > c.pairTriggerPrice[option.Pair] {
		finalSide = model.SideTypeBuy
		postionSide = model.PositionSideTypeLong
	} else {
		finalSide = model.SideTypeSell
		postionSide = model.PositionSideTypeShort
	}
	// 根据最新价格创建限价单
	_, err = c.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
		Leverage: option.Leverage,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	delete(c.pairTriggerPrice, option.Pair)
}

func (c *Dual) closePosition(option *model.PairOption) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁
	// 获取当前已存在的仓位
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
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

	currentPrice, _ := c.pairPrices.Get(option.Pair)
	mainPosition, subPosition := c.judePosition(option, currentPrice, openedPositionMap)

	// 判断当前是否已有同向挂单未成交，有则不在开单
	existUnfilledOrderMap, err := c.broker.GetPositionOrdersForPairUnfilled(option.Pair)
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
					"[POSITION - EXSIT] %s, Current: %v | (UNFILLED MORE)",
					unfillePositionOrders[model.PositionSideType(subPosition.PositionSide)].String(),
					currentPrice,
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
	_, quotePosition, err := c.broker.PairAsset(option.Pair)
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
		pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
		// ---------------------
		// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
		if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.Close > 0 {
			utils.Log.Infof(
				"[POSITION - CLOSE] Main %s | Sub %s | Current: %v | PR.%%: %.2f%% < ProfitClose: %s",
				mainPosition.String(),
				subPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.Close*100,
			)
			// 重置交易对盈利
			c.resetPairProfit(option.Pair)
			if subPosition.Quantity > 0 {
				go c.CloseOrder(false)
			}
			c.finishAllPosition(mainPosition, subPosition)
			return
		}
		profitTriggerRatio := pairCurrentProfit.Floor
		// 判断是否已锁定利润比
		if pairCurrentProfit.Close == 0 {
			// 保守出局，利润比稍微为正即可
			if subPosition.Quantity > 0 || (subPosition.Quantity == 0 && mainPosition.MoreCount >= option.MaxAddPosition) {
				profitTriggerRatio = pairCurrentProfit.Decrease
			}
			// 小于触发值时，记录当前利润比
			if profitRatio < profitTriggerRatio {
				utils.Log.Infof(
					"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitTriggerRatio: %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", profitTriggerRatio*100),
				)
				return
			}
		} else {
			if profitRatio < profitTriggerRatio {
				// 当前利润比触发值，之前已经有Close时，判断当前利润比是否比上次设置的利润比大
				if profitRatio <= pairCurrentProfit.Close+pairCurrentProfit.Decrease {
					utils.Log.Infof(
						"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitCloseRatio: %s",
						mainPosition.Pair,
						mainPosition.OrderFlag,
						mainPosition.Quantity,
						mainPosition.AvgPrice,
						mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
						fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
					)
					return
				}
			}
		}
		// 当前利润比大于等于触发利润比，
		profitLevel := c.findProfitLevel(option.Pair, profitRatio)
		if profitLevel != nil {
			pairCurrentProfit.Floor = profitLevel.NextTriggerRatio
			pairCurrentProfit.Decrease = profitLevel.DrawdownRatio
		}

		pairCurrentProfit.Close = profitRatio - pairCurrentProfit.Decrease
		c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
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
			fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
		)
	} else {
		// 亏损盈利比已大于最大
		if calc.Abs(profitRatio) > option.MaxMarginLossRatio {
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
				fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
			)
			c.resetPairProfit(option.Pair)
			if subPosition.Quantity > 0 {
				go c.CloseOrder(false)
			}
			c.finishAllPosition(mainPosition, subPosition)
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
					currentPrice,
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
		_, err = c.broker.CreateOrderMarket(
			model.SideType(morePosition.Side),
			model.PositionSideType(morePosition.PositionSide),
			morePosition.Pair,
			addAmount,
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

func (cc *Dual) judePosition(option *model.PairOption, price float64, positionMap map[model.PositionSideType]*model.Position) (*model.Position, *model.Position) {
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
