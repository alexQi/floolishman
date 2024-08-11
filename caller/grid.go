package caller

import (
	"floolishman/constants"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

type CallerGrid struct {
	CallerCommon
}

func (c *CallerGrid) Start() {
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Millisecond)
		tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
		for {
			select {
			case <-tickerCheck.C:
				for _, option := range c.pairOptions {
					c.openGridPosition(option)
				}
			case <-tickerClose.C:
				for _, option := range c.pairOptions {
					c.closeGridPosition(option)
				}
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()
	go c.Listen()
	go c.WatchPriceChange()
}

func (c *CallerGrid) WatchPriceChange() {
	for {
		select {
		case <-time.After(CHeckPriceUndulateInterval * time.Second):
			for _, option := range c.pairOptions {
				c.pairOriginPrices[option.Pair].Add(c.pairPrices[option.Pair])
				if c.pairOriginPrices[option.Pair].Count() < PriceChangeRingCount {
					c.pairChangeRatio[option.Pair] = 0
				} else {
					c.pairChangeRatio[option.Pair] = (c.pairOriginPrices[option.Pair].GetFirst() - c.pairOriginPrices[option.Pair].GetLast()) / option.UndulatePriceLimit
				}
			}
		}
	}
}

func (c *CallerGrid) Listen() {
	for {
		select {
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			c.CheckOrderTimeout()
		}
	}
}

func (c *CallerGrid) openGridPosition(option model.PairOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 判断当前网格是否存在，不存在则不开仓
	if _, ok := c.positionGridMap[option.Pair]; !ok {
		return
	}
	currentPrice := c.pairPrices[option.Pair]
	openIndex, err := c.getOpenGrid(option.Pair, currentPrice)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if openIndex < 0 {
		utils.Log.Errorf("[POSITION] Price out of grid, switch mode to OUT ....")
		c.pairGirdStatus[option.Pair] = constants.GridStatusOut
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
			utils.Log.Infof(
				"[POSITION - EXSIT %s] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v, CandleTime: %s | (UNFILLED)",
				unfillePositionOrder.PositionSide,
				unfillePositionOrder.OrderFlag,
				unfillePositionOrder.Pair,
				unfillePositionOrder.Quantity,
				unfillePositionOrder.Price,
				currentPrice,
				c.lastUpdate[option.Pair].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		return
	}
	openPositionGrid := c.positionGridMap[option.Pair].GridItems[openIndex]
	// 获取当前已存在的仓位
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var amount float64
	var sampleSidePosition, hedgeSidePosition *model.Position
	if len(openedPositions) > 0 {
		for _, openedPosition := range openedPositions {
			if model.PositionSideType(openedPosition.PositionSide) != openPositionGrid.PositionSide {
				hedgeSidePosition = openedPosition
			} else {
				sampleSidePosition = openedPosition
			}
		}
	}
	// 判断当前资产
	_, quotePosition, err := c.broker.PairAsset(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("[EXCHANGE] Balance is not enough to create order")
		return
	}
	orderExtra := model.OrderExtra{
		Leverage: option.Leverage,
	}
	if sampleSidePosition != nil && hedgeSidePosition != nil {
		utils.Log.Infof(
			"[POSITION - HEDGE EXSIT] Pair: %s | Quantity: %v, Price: %v | Hedge Quantity: %v, Price: %v | Current: %v",
			sampleSidePosition.Pair,
			sampleSidePosition.Quantity,
			sampleSidePosition.AvgPrice,
			hedgeSidePosition.Quantity,
			hedgeSidePosition.AvgPrice,
			currentPrice,
		)
		return
	}
	if hedgeSidePosition != nil {
		utils.Log.Infof(
			"[POSITION - EXSIT REVERSE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v",
			hedgeSidePosition.Pair,
			hedgeSidePosition.Side,
			hedgeSidePosition.PositionSide,
			hedgeSidePosition.Quantity,
			hedgeSidePosition.AvgPrice,
			hedgeSidePosition.MoreCount,
			currentPrice,
		)
		return
	}

	if sampleSidePosition != nil {
		// 判断当前价格不动是否在快速变化
		if calc.Abs(c.pairChangeRatio[option.Pair]) >= 1 {
			// 当前仓位为空，价格快速拉升时开启对冲
			// 当前仓位为多，价格快速下降时开启对冲
			if (c.pairChangeRatio[option.Pair] < 0 && model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeShort) ||
				(c.pairChangeRatio[option.Pair] > 0 && model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeLong) {
				utils.Log.Infof(
					"[POSITION - UNDULATE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v, PriceChangeRatio: %.2f%% (start hedge mode)",
					sampleSidePosition.Pair,
					sampleSidePosition.Side,
					sampleSidePosition.PositionSide,
					sampleSidePosition.Quantity,
					sampleSidePosition.AvgPrice,
					sampleSidePosition.MoreCount,
					currentPrice,
					c.pairChangeRatio[option.Pair]*100,
				)
				c.pairHedgeMode[option.Pair] = true
				return
			}
		}
		// 判断加仓次数已达上线，不在加仓
		gridItems := c.positionGridMap[option.Pair].GridItems
		// 判断仓位加仓次数是否达到上线
		if sampleSidePosition.MoreCount >= c.setting.MaxAddPostion {
			if c.positionGridIndex[option.Pair] >= 0 &&
				// 多单
				((openPositionGrid.PositionSide == model.PositionSideTypeLong &&
					(((c.positionGridIndex[option.Pair]-1) >= 0 && gridItems[c.positionGridIndex[option.Pair]-1].Price >= currentPrice) ||
						(c.positionGridIndex[option.Pair]-1) < 0)) ||
					// 空单
					(openPositionGrid.PositionSide == model.PositionSideTypeShort &&
						(((c.positionGridIndex[option.Pair]+1) < len(gridItems) && gridItems[c.positionGridIndex[option.Pair]+1].Price <= currentPrice) ||
							c.positionGridIndex[option.Pair]+1 >= len(gridItems)))) {
				// 当前价格达到指定上下后再次触达下一个网格
				c.pairGirdStatus[option.Pair] = constants.GridStatusMoreLimit
				utils.Log.Infof(
					"[POSITION - MORE LIMIT] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v (start hedge mode)",
					sampleSidePosition.Pair,
					sampleSidePosition.Side,
					sampleSidePosition.PositionSide,
					sampleSidePosition.Quantity,
					sampleSidePosition.AvgPrice,
					sampleSidePosition.MoreCount,
					currentPrice,
				)
				return
			}
			utils.Log.Infof(
				"[POSITION - MORE LIMIT] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v",
				sampleSidePosition.Pair,
				sampleSidePosition.Side,
				sampleSidePosition.PositionSide,
				sampleSidePosition.Quantity,
				sampleSidePosition.AvgPrice,
				sampleSidePosition.MoreCount,
				currentPrice,
			)
			return
		}
		orderExtra.OrderFlag = sampleSidePosition.OrderFlag
		amount = sampleSidePosition.UnitQuantity
	} else {
		// 判断开仓时，保证后方有足够的加仓次数才开仓否则，可能会只加一次仓后突破仓位限制导致平仓
		if (openPositionGrid.PositionSide == model.PositionSideTypeLong && int64(openIndex) < c.setting.MinAddPostion) ||
			(openPositionGrid.PositionSide == model.PositionSideTypeShort && (int64(len(c.positionGridMap[option.Pair].GridItems))-int64(openIndex)) < c.setting.MinAddPostion) {
			utils.Log.Infof(
				"[POSITION - IGNORE] Pair: %s | Side: %v, PositionSide: %s | Grid Count: %v, Price: %v, Min: %v, Max: %v | Current Price: %v, Index: %v",
				option.Pair,
				openPositionGrid.Side,
				openPositionGrid.PositionSide,
				len(c.positionGridMap[option.Pair].GridItems),
				openPositionGrid.Price,
				c.positionGridMap[option.Pair].GridItems[0].Price,
				c.positionGridMap[option.Pair].GridItems[len(c.positionGridMap[option.Pair].GridItems)-1].Price,
				currentPrice,
				openIndex,
			)
			return
		}

		amount = calc.OpenPositionSize(
			quotePosition,
			float64(c.pairOptions[option.Pair].Leverage),
			openPositionGrid.Price,
			1,
			c.setting.FullSpaceRatio,
		)
	}
	utils.Log.Infof(
		"[POSITION OPENING] Pair: %s | Quantity: %v | Main Price: %v, PositionSide: %s",
		option.Pair,
		amount,
		openPositionGrid.Price,
		openPositionGrid.PositionSide,
	)
	// 根据最新价格创建限价单
	_, err = c.broker.CreateOrderLimit(openPositionGrid.Side, openPositionGrid.PositionSide, option.Pair, amount, openPositionGrid.Price, orderExtra)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	c.pairGirdStatus[option.Pair] = constants.GridStatusInside
	c.pairHedgeMode[option.Pair] = false
	c.profitRatioLimit[option.Pair] = 0
	c.positionGridIndex[option.Pair] = openIndex
	c.positionGridMap[option.Pair].GridItems[openIndex].Lock = true
}

func (c *CallerGrid) closeGridPosition(option model.PairOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 获取当前已存在的仓位
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(openedPositions) == 0 {
		// 当前存在仓位时
		if _, ok := c.positionGridMap[option.Pair]; ok {
			// 重新初始化网格
			c.BuildGird(option.Pair, "1h", true)
		}
		return
	}

	openedPositionMap := map[model.PositionSideType]*model.Position{}
	for _, position := range openedPositions {
		openedPositionMap[model.PositionSideType(position.PositionSide)] = position
	}
	currentPrice := c.pairPrices[option.Pair]
	// 判断当前是否已有同向挂单未成交，有则不在开单
	existUnfilledOrderMap, err := c.broker.GetPositionOrdersForPairUnfilled(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	mainPosition, subPosition := c.judePosition(option, currentPrice, openedPositionMap)
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
	if profitRatio > 0 {
		// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
		if profitRatio <= c.profitRatioLimit[option.Pair] && c.profitRatioLimit[option.Pair] > 0 {
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
				fmt.Sprintf("%.2f%%", c.profitRatioLimit[option.Pair]*100),
			)

			c.finishAllPosition(mainPosition, subPosition)
			if subPosition.Quantity > 0 {
				delete(c.positionGridMap, option.Pair)
			} else {
				c.ResetGrid(option.Pair)
			}
			// 重置当前交易对止损比例
			c.profitRatioLimit[option.Pair] = 0
			return
		}
		// 判断当前盈亏比是否大于触发盈亏比
		if profitRatio < c.setting.InitProfitRatioLimit || profitRatio < (c.profitRatioLimit[option.Pair]+c.setting.ProfitableScale+0.01) {
			utils.Log.Infof(
				"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < InitProfitRatioLimit: %s",
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
				fmt.Sprintf("%.2f%%", c.setting.InitProfitRatioLimit*100),
			)
			if subPosition.Quantity > 0 && profitRatio >= c.setting.ProfitableScale {
				c.profitRatioLimit[option.Pair] = profitRatio - c.setting.ProfitableScale
			}
			return
		}
		// 重设利润比
		c.profitRatioLimit[option.Pair] = profitRatio - c.setting.ProfitableScale
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
			fmt.Sprintf("%.2f%%", c.profitRatioLimit[option.Pair]*100),
		)
	} else {
		lossRatio := c.setting.BaseLossRatio * float64(option.Leverage)
		// 判断是否开启对冲模式，未开启对冲模式时判断是否直接平仓
		if c.pairHedgeMode[option.Pair] == false {
			// 判断在网格内时不做处理
			if c.pairGirdStatus[option.Pair] == constants.GridStatusInside {
				return
			}
			// 超出网格时判断当当前亏损比例
			if c.pairGirdStatus[option.Pair] == constants.GridStatusOut && calc.Abs(profitRatio) <= lossRatio {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s <= MaxLoseRatio: %s (price out of grid)",
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
				return
			}
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
				c.pairGirdStatus[option.Pair],
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", lossRatio*100),
			)
			c.finishAllPosition(mainPosition, subPosition)
			delete(c.positionGridMap, option.Pair)
			// 重置当前交易对止损比例
			c.profitRatioLimit[option.Pair] = 0
			return
		}
		// ********** 对冲开启 **********
		// 判断当前是否是超出亏损限制
		if c.pairGirdStatus[option.Pair] == constants.GridStatusOut {
			// 超出网格后判断当前亏损是否超过比例，判断是否要加对冲仓
			if calc.Abs(profitRatio) < StepMoreRatio {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < LossStepRatio: %s (OUT GRID)",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", StepMoreRatio*100),
				)
				return
			}
		}

		// 判断对冲仓位是否已存在,存在则不在加仓
		if subPosition.Quantity > 0 {
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
				c.finishAllPosition(mainPosition, subPosition)
				delete(c.positionGridMap, option.Pair)
				// 重置当前交易对止损比例
				c.profitRatioLimit[option.Pair] = 0
			} else {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < LossRatio: %s",
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
			}
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
		var addAmount float64
		if c.pairGirdStatus[option.Pair] == constants.GridStatusMoreLimit {
			// 根据当前联合仓位保证金亏损比例计算加仓数量
			addAmount = calc.CalculateAddQuantity(
				model.SideType(mainPosition.Side),
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				subPosition.Quantity,
				subPosition.AvgPrice,
				currentPrice,
				float64(option.Leverage),
				c.setting.MaxPositionLossRatio,
			)
		} else {
			// 判断当前亏损比例是否小于要拉回的亏损比例 类似-18%
			if calc.Abs(lossRatio) < c.setting.MaxPositionLossRatio {
				addAmount = mainPosition.Quantity * c.pairChangeRatio[option.Pair]
			} else {
				// 根据当前联合仓位保证金亏损比例计算加仓数量
				addAmount = calc.CalculateAddQuantity(
					model.SideType(mainPosition.Side),
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					subPosition.Quantity,
					subPosition.AvgPrice,
					currentPrice,
					float64(option.Leverage),
					c.setting.MaxPositionLossRatio,
				)
			}
		}

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
		// 亏损状态下给副仓位加仓 -- 对冲亏损
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

func (c *CallerGrid) judePosition(option model.PairOption, price float64, positionMap map[model.PositionSideType]*model.Position) (*model.Position, *model.Position) {
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
