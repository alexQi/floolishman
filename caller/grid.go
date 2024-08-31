package caller

import (
	"floolishman/constants"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"time"
)

var (
	VolumeChangeLimitRatio   = 10.0
	VolumeReversalLimitRatio = 30.0
	VolumeGrowLimitRatio     = 3.0
)

type Grid struct {
	Common
}

func (c *Grid) Start() {
	go func() {
		tickerCheck := time.NewTicker(CheckStrategyInterval * time.Millisecond)
		for {
			select {
			case <-tickerCheck.C:
				for _, option := range c.pairOptions {
					if option.Status == false {
						continue
					}
					go c.openGridPosition(option)
				}
			}
		}
	}()
	go func() {
		tickerClose := time.NewTicker(CheckCloseInterval * time.Millisecond)
		for {
			select {
			case <-tickerClose.C:
				for _, option := range c.pairOptions {
					go c.closeGridPosition(option)
				}
			}
		}
	}()
	go c.Listen()
	go c.ListenIndicator()
}

func (c *Grid) Listen() {
	for {
		select {
		case <-time.After(CheckTimeoutInterval * time.Millisecond):
			c.CloseOrder(true)
		}
	}
}

func (c *Grid) openGridPosition(option *model.PairOption) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁
	lastAvgVolume, _ := c.lastAvgVolume.Get(option.Pair)
	pairOriginVolumes, _ := c.pairOriginVolumes.Get(option.Pair)
	pairVolumeGrowRatio, _ := c.pairVolumeGrowRatio.Get(option.Pair)
	pairVolumeChangeRatio, _ := c.pairVolumeChangeRatio.Get(option.Pair)
	pairPriceChangeRatio, _ := c.pairPriceChangeRatio.Get(option.Pair)
	// 当前量能与上个蜡烛的平均量能比例
	volAvgChangeLimit := pairOriginVolumes.Last(0)/lastAvgVolume > AvgVolumeLimitRatio
	// 量能增长率
	volGrowHasSurmountLimit := pairVolumeGrowRatio > VolumeGrowLimitRatio
	// 量能变化率
	volChangeHasSurmountLimit := pairVolumeChangeRatio > VolumeChangeLimitRatio
	// 量能反转限制
	volChangeHasReversalLimit := pairVolumeChangeRatio < VolumeReversalLimitRatio
	// 判断当前网格是否存在，获取当前价格网格
	currentPrice, _ := c.pairPrices.Get(option.Pair)
	openIndex, err := c.getOpenGrid(option.Pair, currentPrice)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if openIndex < 0 {
		utils.Log.Errorf("[POSITION] Price out of grid, switch mode to OUT ....")
		c.pairGirdStatus.Set(option.Pair, constants.GridStatusOut)
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
				"[POSITION - EXSIT %s] OrderFlag: %s | Pair: %s | Quantity: %v | Price: %v, Current: %v | (UNFILLED)",
				unfillePositionOrder.PositionSide,
				unfillePositionOrder.OrderFlag,
				unfillePositionOrder.Pair,
				unfillePositionOrder.Quantity,
				unfillePositionOrder.Price,
				currentPrice,
			)
		}
		return
	}
	pairGrid, ok := c.pairGridMap.Get(option.Pair)
	if !ok {
		utils.Log.Error(fmt.Sprintf("[POSITION] Grid for %s not exsit, waiting ....", option.Pair))
		return
	}
	openPositionGrid := pairGrid.GridItems[openIndex]
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
		// 量能增长量突破限制，量能增长率突破限制，价格波动突破限制
		if volChangeHasSurmountLimit && volGrowHasSurmountLimit {
			// 量能价格都超过限制,执行对冲操作，否则继续持仓
			if volChangeHasReversalLimit && calc.Abs(pairPriceChangeRatio) > 1 {
				// 跟当前仓位相反
				if (pairPriceChangeRatio < 0 && model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeLong) ||
					(pairPriceChangeRatio > 0 && model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeShort) {
					// 如果仓位是头仓时,直接平仓
					if sampleSidePosition.MoreCount < option.MaxAddPosition/2 {
						c.pairHedgeMode.Set(option.Pair, constants.PositionModeHedge)
						utils.Log.Infof(
							"[POSITION - UNDULATE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v , %.2f%% (Change) | Volume: %.2f%% (Change), %.2f%% (Grow) (start hedge mode)",
							sampleSidePosition.Pair,
							sampleSidePosition.Side,
							sampleSidePosition.PositionSide,
							sampleSidePosition.Quantity,
							sampleSidePosition.AvgPrice,
							sampleSidePosition.MoreCount,
							currentPrice,
							pairPriceChangeRatio*100,
							pairVolumeChangeRatio*100,
							pairVolumeGrowRatio*100,
						)
					} else {
						c.pairHedgeMode.Set(option.Pair, constants.PositionModeClose)
						utils.Log.Infof(
							"[POSITION - UNDULATE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v , %.2f%% (Change) | Volume: %.2f%% (Change), %.2f%% (Grow) (start close mode - volume in ReversalLimit)",
							sampleSidePosition.Pair,
							sampleSidePosition.Side,
							sampleSidePosition.PositionSide,
							sampleSidePosition.Quantity,
							sampleSidePosition.AvgPrice,
							sampleSidePosition.MoreCount,
							currentPrice,
							pairPriceChangeRatio*100,
							pairVolumeChangeRatio*100,
							pairVolumeGrowRatio*100,
						)
					}
					return
				}
			}
		}
		// 判断当前价格是否优于仓位价格
		if (model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeLong && currentPrice > sampleSidePosition.AvgPrice) || (model.PositionSideType(sampleSidePosition.PositionSide) == model.PositionSideTypeShort && currentPrice < sampleSidePosition.AvgPrice) {
			utils.Log.Infof(
				"[POSITION - IGNORE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v (position profiting)",
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
		// 判断加仓次数已达上限，不在加仓
		pairGridIndex, ok := c.pairGridMapIndex.Get(option.Pair)
		if !ok {
			utils.Log.Error(fmt.Sprintf("[POSITION] Grid for %s Index not exsit, waiting ....", option.Pair))
			return
		}
		// 判断仓位加仓次数是否达到上线
		if sampleSidePosition.MoreCount >= option.MaxAddPosition {
			if pairGridIndex >= 0 &&
				// 多单
				((openPositionGrid.PositionSide == model.PositionSideTypeLong &&
					(((pairGridIndex-1) >= 0 && pairGrid.GridItems[pairGridIndex-1].Price >= currentPrice) ||
						(pairGridIndex-1) < 0)) ||
					// 空单
					(openPositionGrid.PositionSide == model.PositionSideTypeShort &&
						(((pairGridIndex+1) < len(pairGrid.GridItems) && pairGrid.GridItems[pairGridIndex+1].Price <= currentPrice) ||
							pairGridIndex+1 >= len(pairGrid.GridItems)))) {
				// 当前价格达到指定上下后再次触达下一个网格
				c.pairGirdStatus.Set(option.Pair, constants.GridStatusMoreLimit)
				utils.Log.Infof(
					"[POSITION - MORE LIMIT] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v (start more limit mode)",
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
				"[POSITION - MORE LIMIT] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v, MoreCount:%v | Current: %v (watching ...)",
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
		// 判断当前量能是否变化当前无仓位，暂停caller
		if volChangeHasSurmountLimit && volGrowHasSurmountLimit {
			// 暂停该交易对新的仓位请求
			c.PausePairCall(option.Pair)
			// 取消所有挂单
			go c.CloseOrder(false)
			// 日志
			utils.Log.Infof(
				"[POSITION PAUSE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v | Current: %v , %.2f%% (Change) | Volume: %.2f%% (Change), %.2f%% (Grow)",
				option.Pair,
				openPositionGrid.Side,
				openPositionGrid.PositionSide,
				amount,
				openPositionGrid.Price,
				currentPrice,
				pairPriceChangeRatio*100,
				pairVolumeChangeRatio*100,
				pairVolumeGrowRatio*100,
			)
			return
		}
		// 当前量能超过平均量能
		if volAvgChangeLimit {
			utils.Log.Infof(
				"[POSITION PAUSE] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v | Current: %v , %.2f%% (Change) | Volume: %.2f%% (Change), %.2f%% (Grow) (Volume surmount avg volume limit)",
				option.Pair,
				openPositionGrid.Side,
				openPositionGrid.PositionSide,
				amount,
				openPositionGrid.Price,
				currentPrice,
				pairPriceChangeRatio*100,
				pairVolumeChangeRatio*100,
				pairVolumeGrowRatio*100,
			)
			// 暂停该交易对新的仓位请求
			c.PausePairCall(option.Pair)
			// 取消所有挂单
			go c.CloseOrder(false)
			return
		}
		// 判断开仓时，保证后方有足够的加仓次数才开仓否则，可能会只加一次仓后突破仓位限制导致平仓
		if (openPositionGrid.PositionSide == model.PositionSideTypeLong && int64(openIndex) < option.MinAddPosition) ||
			(openPositionGrid.PositionSide == model.PositionSideTypeShort && (int64(len(pairGrid.GridItems))-int64(openIndex)) < option.MinAddPosition) {
			utils.Log.Infof(
				"[POSITION - IGNORE] Pair: %s | Side: %v, PositionSide: %s | Grid Count: %v, Price: %v, Min: %v, Max: %v | Current Price: %v, Index: %v (grid Insufficient)",
				option.Pair,
				openPositionGrid.Side,
				openPositionGrid.PositionSide,
				len(pairGrid.GridItems),
				openPositionGrid.Price,
				pairGrid.GridItems[0].Price,
				pairGrid.GridItems[len(pairGrid.GridItems)-1].Price,
				currentPrice,
				openIndex,
			)
			return
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
		amount = c.getPositionMargin(quotePosition, openPositionGrid.Price, option)
	}
	utils.Log.Infof(
		"[POSITION - OPENING] Pair: %s | Side: %v, PositionSide: %s | Quantity: %v, Price: %v | Current: %v , %.2f%% (Change) | Volume: %.2f%% (Change), %.2f%% (Grow)",
		option.Pair,
		openPositionGrid.Side,
		openPositionGrid.PositionSide,
		amount,
		openPositionGrid.Price,
		currentPrice,
		pairPriceChangeRatio*100,
		pairVolumeChangeRatio*100,
		pairVolumeGrowRatio*100,
	)
	// 根据最新价格创建限价单
	_, err = c.broker.CreateOrderLimit(openPositionGrid.Side, openPositionGrid.PositionSide, option.Pair, amount, openPositionGrid.Price, orderExtra)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 处理网格锁定状态及当前网格索引
	pairGrid.GridItems[openIndex].Lock = true
	// 修改map
	c.pairGirdStatus.Set(option.Pair, constants.GridStatusInside)
	c.pairHedgeMode.Set(option.Pair, constants.PositionModeNormal)
	c.pairGridMapIndex.Set(option.Pair, openIndex)
	c.pairGridMap.Set(option.Pair, pairGrid)
}

func (c *Grid) closeGridPosition(option *model.PairOption) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁
	// 获取当前已存在的仓位
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 当前没有仓位 重新生成网格
	if len(openedPositions) == 0 {
		types.PairGridBuilderParamChan <- types.PairGridBuilderParam{
			Pair:      option.Pair,
			Timeframe: "1h",
			IsForce:   true,
		}
		return
	}

	openedPositionMap := map[model.PositionSideType]*model.Position{}
	for _, position := range openedPositions {
		openedPositionMap[model.PositionSideType(position.PositionSide)] = position
	}
	currentPrice, _ := c.pairPrices.Get(option.Pair)
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
	lastAvgVolume, _ := c.lastAvgVolume.Get(option.Pair)
	pairOriginVolumes, _ := c.pairOriginVolumes.Get(option.Pair)
	// 当前量能与上个蜡烛的平均量能比例
	volAvgChangeLimit := pairOriginVolumes.Last(0)/lastAvgVolume > AvgVolumeLimitRatio
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
	hedgeMode, _ := c.pairHedgeMode.Get(option.Pair)
	gridStatus, _ := c.pairGirdStatus.Get(option.Pair)
	if profitRatio > 0 {
		// 判断是否是平仓模式
		if hedgeMode == constants.PositionModeClose {
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (hedge mode in close)",
				gridStatus,
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			// 平仓
			c.finishAllPosition(mainPosition, subPosition)
			// 重置利润比
			c.resetPairProfit(option.Pair)
			// 暂停交易
			c.PausePairCall(option.Pair)
			// 取消所有挂单
			go c.CloseOrder(false)
			return
		}
		pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
		// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
		if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.Close > 0 {
			utils.Log.Infof(
				"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitClose: %s",
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
			// 关闭仓位
			c.finishAllPosition(mainPosition, subPosition)
			// 重置交易对盈利
			c.resetPairProfit(option.Pair)
			// 副仓位存在||当前量能呢超过平均量能时暂停caller
			if subPosition.Quantity > 0 || volAvgChangeLimit {
				// 暂停交易
				c.PausePairCall(option.Pair)
			} else {
				// 重置网格锁定状态
				c.ResetGrid(option.Pair)
			}
			// 取消挂单
			go c.CloseOrder(false)
			return
		}
		profitTriggerRatio := pairCurrentProfit.Floor
		// 判断是否已锁定利润比
		if pairCurrentProfit.Close == 0 {
			// 保守出局，利润比稍微为正即可
			if subPosition.Quantity > 0 || (subPosition.Quantity == 0 && mainPosition.MoreCount >= option.MaxAddPosition) {
				profitTriggerRatio = pairCurrentProfit.Decrease
			}
			// 计算持仓时间周期倍数，获取盈利触发百分比
			holdPeriod := int(time.Now().Sub(mainPosition.UpdatedAt).Minutes() / float64(option.HoldPositionPeriod))
			if pairCurrentProfit.Close == 0 && holdPeriod >= 1 {
				profitTriggerRatio = profitTriggerRatio - option.HoldPositionPeriodDecrStep*float64(holdPeriod)
				if profitTriggerRatio <= option.HoldPositionPeriodDecrStep {
					profitTriggerRatio = option.HoldPositionPeriodDecrStep
				}
			}
			// 小于触发值时，记录当前利润比
			if profitRatio < profitTriggerRatio {
				utils.Log.Infof(
					"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < NextTrigger: %s, CurrentScale: %s",
					mainPosition.Pair,
					mainPosition.OrderFlag,
					mainPosition.Quantity,
					mainPosition.AvgPrice,
					mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", profitTriggerRatio*100),
					fmt.Sprintf("%.2f%%", pairCurrentProfit.Decrease*100),
				)
				return
			}
		} else {
			if profitRatio < profitTriggerRatio {
				// 当前利润比触发值，之前已经有Close时，判断当前利润比是否比上次设置的利润比大
				if profitRatio <= (pairCurrentProfit.Close + pairCurrentProfit.Decrease) {
					utils.Log.Infof(
						"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitCloseRatio: %s, NextTrigger: %s, CurrentScale: %s",
						mainPosition.Pair,
						mainPosition.OrderFlag,
						mainPosition.Quantity,
						mainPosition.AvgPrice,
						mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
						fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
						fmt.Sprintf("%.2f%%", profitTriggerRatio*100),
						fmt.Sprintf("%.2f%%", pairCurrentProfit.Decrease*100),
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
			"[POSITION - PROFIT] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > NewProfitRatio: %s NextTrigger: %s, CurrentScale: %s",
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
			fmt.Sprintf("%.2f%%", pairCurrentProfit.Floor*100),
			fmt.Sprintf("%.2f%%", pairCurrentProfit.Decrease*100),
		)
	} else {
		// 判断是否是普通模式
		if hedgeMode == constants.PositionModeNormal {
			// 判断在网格内时不做处理
			if gridStatus == constants.GridStatusInside {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s <= MaxLoseRatio: %s (price inside of grid)",
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
				return
			}
			// 超出网格时判断当当前亏损比例
			if gridStatus == constants.GridStatusOut && calc.Abs(profitRatio) <= option.MaxMarginLossRatio {
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
					fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
				)
				return
			}
			if gridStatus == constants.GridStatusGreaterAvgVol && calc.Abs(profitRatio) <= option.MaxMarginLossRatio {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s <= MaxLoseRatio: %s (volume greater than avgVolume)",
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
				return
			}
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
				gridStatus,
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
			)
			// 平仓操作
			c.finishAllPosition(mainPosition, subPosition)
			// 重置利润比
			c.resetPairProfit(option.Pair)
			// 暂停交易
			c.PausePairCall(option.Pair)
			// 取消挂单
			go c.CloseOrder(false)
			return
		}
		// 判断是否是平仓模式
		if hedgeMode == constants.PositionModeClose {
			// 超出网格时判断当当前亏损比例
			if calc.Abs(profitRatio) <= option.MaxMarginLossRatio {
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s <= MaxLoseRatio: %s (hedge mode in close)",
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
				return
			}
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (hedge mode in close)",
				gridStatus,
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			// 平仓
			c.finishAllPosition(mainPosition, subPosition)
			// 重置利润比
			c.resetPairProfit(option.Pair)
			// 暂停交易
			c.PausePairCall(option.Pair)
			// 取消所有挂单
			go c.CloseOrder(false)
			return
		}
		// ********** 对冲开启 **********
		// 判断对冲仓位是否已存在,存在则不在加仓
		if subPosition.Quantity > 0 || option.PullMarginLossRatio <= 0 {
			// 亏损盈利比已大于最大
			if calc.Abs(profitRatio) >= option.MaxMarginLossRatio {
				utils.Log.Infof(
					"[POSITION - CLOSE: HEDGE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Sub OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
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
				// 关闭仓位
				c.finishAllPosition(mainPosition, subPosition)
				// 重设利润比
				c.resetPairProfit(option.Pair)
				// 暂停交易
				c.PausePairCall(option.Pair)
				// 取消挂单
				go c.CloseOrder(false)
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
					fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
				)
			}
			return
		}
		// 当加仓次数达到限制时直接损减小风险
		if mainPosition.MoreCount >= option.MaxAddPosition {
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (ignore hedge: position times in max)",
				gridStatus,
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			// 关闭仓位
			c.finishAllPosition(mainPosition, subPosition)
			// 重设利润比
			c.resetPairProfit(option.Pair)
			// 暂停交易
			c.PausePairCall(option.Pair)
			// 取消挂单
			go c.CloseOrder(false)
			return
		}
		// 仓位亏损比例过大取消对冲直接平仓
		if calc.Abs(profitRatio) >= option.MaxMarginLossRatio/2 {
			// 超出网格或仓位最大时直接平仓
			utils.Log.Infof(
				"[POSITION - CLOSE：%s] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (ignore hedge: position loss ratio more than in max %v)",
				gridStatus,
				mainPosition.Pair,
				mainPosition.OrderFlag,
				mainPosition.Quantity,
				mainPosition.AvgPrice,
				mainPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
			)
			// 关闭仓位
			c.finishAllPosition(mainPosition, subPosition)
			// 重设利润比
			c.resetPairProfit(option.Pair)
			// 暂停交易
			c.PausePairCall(option.Pair)
			// 取消挂单
			go c.CloseOrder(false)
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

		pairPriceChangeRatio, _ := c.pairPriceChangeRatio.Get(option.Pair)
		var addAmount float64
		// 判断当前亏损比例是否小于要拉回的亏损比例 类似-18%
		if calc.Abs(profitRatio) < option.PullMarginLossRatio {
			addAmount = mainPosition.Quantity * pairPriceChangeRatio
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
				option.PullMarginLossRatio,
			)
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

func (c *Grid) judePosition(option *model.PairOption, price float64, positionMap map[model.PositionSideType]*model.Position) (*model.Position, *model.Position) {
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
