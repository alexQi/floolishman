package caller

import (
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"floolishman/utils/strutil"
	"sort"
	"time"
)

var ExecStart time.Duration = 1
var ExecInterval time.Duration = 5

type Scoop struct {
	Common
}

type ScoopCheckItem struct {
	PairOption     *model.PairOption
	Score          float64
	LongShortRatio float64
	Matchers       []model.PositionStrategy
}

func (c *Scoop) Start() {
	now := time.Now()
	next := now.Truncate(time.Minute * ExecStart).Add(time.Minute * ExecStart)
	duration := time.Until(next)
	// 在下一个n分钟倍数时执行任务
	if c.setting.Backtest == false {
		time.AfterFunc(duration, func() {
			c.tickCheckForOpen()
		})
		go c.Listen()
	}
}

func (c *Scoop) Listen() {
	// 非回溯测试时，执行检查仓位关闭
	if c.setting.Backtest == false {
		// 非回溯测试模式且不是看门狗方式下监听平仓
		go c.tickerCheckForClose()
		// 执行超时检查
		go c.tickCheckOrderTimeout()
	}
}

func (c *Scoop) tickCheckForOpen() {
	ticker := time.NewTicker(ExecInterval * time.Second)
	for {
		select {
		case <-ticker.C:
			if c.status == false {
				continue
			}
			// 判断总仓位数量
			totalOpenedPositions, err := c.broker.GetPositionsForOpened()
			if err != nil {
				utils.Log.Error(err)
				continue
			}
			if len(totalOpenedPositions) >= MaxPairPositions {
				utils.Log.Infof("[POSITION - MAX PAIR] Pair position reach to max, waiting...")
				continue
			}
			unfilledOrderCount := 0
			totalUnfilledOrders, err := c.broker.GetOrdersForUnfilled()
			if err != nil {
				utils.Log.Error(err)
				continue
			}
			for _, existOrders := range totalUnfilledOrders {
				_, ok := existOrders["position"]
				if !ok {
					continue
				}
				unfilledOrderCount += 1
			}
			if len(totalOpenedPositions)+unfilledOrderCount >= MaxPairPositions {
				utils.Log.Infof("[POSITION - MAX PAIR] Pair position (%v) or order (%v) reach to max, waiting...", len(totalOpenedPositions), unfilledOrderCount)
				continue
			}
			mapOpenedPosition := map[string][]string{
				string(model.PositionSideTypeLong):  {},
				string(model.PositionSideTypeShort): {},
			}
			if len(totalOpenedPositions) > 0 {
				for _, openedPosition := range totalOpenedPositions {
					mapOpenedPosition[openedPosition.PositionSide] = append(mapOpenedPosition[openedPosition.PositionSide], openedPosition.Pair)
				}
			}

			// 检查所有币种,获取可以开仓的币种
			var tempMatcherScore float64
			openAliablePairs := []ScoopCheckItem{}
			scoopCheckSlice := []ScoopCheckItem{}
			currentHour := time.Now().Hour()
			currentWeek := time.Now().Weekday()
			for _, option := range c.pairOptions {
				if option.Status == false {
					continue
				}
				// 如果今天不是周六或周日，且当前时间在 IgnoreHours 中，则跳过
				if currentWeek != time.Saturday && currentWeek != time.Sunday {
					if strutil.IsInArray(option.IgnoreHours, currentHour) {
						continue
					}
				}
				tempMatcherScore = 0
				longShortRatio, currentMatchers := c.checkScoopPosition(option)
				if longShortRatio < 0 {
					continue
				}
				for _, matcher := range currentMatchers {
					tempMatcherScore += matcher.Score
				}
				scoopCheckSlice = append(scoopCheckSlice, ScoopCheckItem{
					PairOption:     option,
					Score:          tempMatcherScore,
					LongShortRatio: longShortRatio,
					Matchers:       currentMatchers,
				})
			}
			if len(scoopCheckSlice) == 0 {
				utils.Log.Infof("[POSITION - SCOOP NONE] No trading pair was selected, waiting...")
				continue
			}
			utils.Log.Infof("[POSITION SCOOP TICK] Check for all pair to open position ...")
			// 根据评分排序，倒叙排列
			sort.Slice(scoopCheckSlice, func(i, j int) bool {
				return scoopCheckSlice[i].Score > scoopCheckSlice[j].Score
			})
			openAliableCount := MaxPairPositions - len(totalOpenedPositions)
			// 判断当前获取的币种是否大于可开的仓位，大于：截断，小于等于时直接使用
			if len(scoopCheckSlice) > openAliableCount {
				openAliablePairs = scoopCheckSlice[:openAliableCount]
			} else {
				openAliablePairs = scoopCheckSlice
			}
			// 本次同向单只开一个
			var positionSide model.PositionSideType
			opendPositionSide := map[model.PositionSideType]float64{}
			for _, openItem := range openAliablePairs {
				if openItem.LongShortRatio > 0.5 {
					positionSide = model.PositionSideTypeLong
				} else {
					positionSide = model.PositionSideTypeShort
				}
				// 判断当前开单方向是否已达到当前仓位方向最多
				// 已有同向仓位已达到总仓位的一半以上，则不在开仓
				if float64(len(mapOpenedPosition[string(positionSide)])) >= float64(MaxPairPositions/2) {
					continue
				}
				// 当前开单币种暂停防止在同一根蜡烛线内再次开单
				if _, ok := opendPositionSide[positionSide]; ok {
					types.CallerPauserChan <- types.CallerStatus{
						Status: true,
						PairStatuses: []types.PairStatus{
							{Pair: openItem.PairOption.Pair, Status: false},
						},
					}
					continue
				}
				opendPositionSide[positionSide] = openItem.LongShortRatio
				// 执行
				go c.openScoopPosition(openItem.PairOption, openItem.LongShortRatio, openItem.Matchers)
			}
		}
	}
}

func (c *Scoop) tickerCheckForClose() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Millisecond):
			for _, option := range c.pairOptions {
				go c.closeScoopPosition(option)
			}
		}
	}
}

func (c *Scoop) EventCallOpen(pair string) {
	longShortRatio, currentMatchers := c.checkScoopPosition(c.pairOptions[pair])
	if longShortRatio >= 0 {
		go c.openScoopPosition(c.pairOptions[pair], longShortRatio, currentMatchers)
	}
}

func (c *Scoop) EventCallClose(pair string) {
	c.closeScoopPosition(c.pairOptions[pair])
}

func (s *Scoop) checkScoopPosition(option *model.PairOption) (float64, []model.PositionStrategy) {
	s.mu[option.Pair].Lock()         // 加锁
	defer s.mu[option.Pair].Unlock() // 解锁
	if _, ok := s.samples[option.Pair]; !ok {
		return -1, []model.PositionStrategy{}
	}
	matchers := s.strategy.CallMatchers(option, s.samples[option.Pair])
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(finalTendency, currentMatchers)
	// 判断策略结果
	if s.setting.Backtest == false && len(currentMatchers) > 0 {
		utils.Log.Infof(
			"[JUDGE] Tendency: %s | Pair: %s | LongShortRatio: %.2f | Matchers:【%v】",
			finalTendency,
			option.Pair,
			longShortRatio,
			matcherStrategy,
		)
	}
	return longShortRatio, currentMatchers
}

func (c *Scoop) openScoopPosition(option *model.PairOption, longShortRatio float64, strategies []model.PositionStrategy) {
	c.mu[option.Pair].Lock()         // 加锁
	defer c.mu[option.Pair].Unlock() // 解锁

	var finalSide model.SideType
	var postionSide model.PositionSideType

	if longShortRatio > 0.5 {
		finalSide = model.SideTypeBuy
		postionSide = model.PositionSideTypeLong
	} else {
		finalSide = model.SideTypeSell
		postionSide = model.PositionSideTypeShort
	}
	openedPositions, err := c.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	currentPrice, _ := c.pairPrices.Get(option.Pair)
	var existPosition *model.Position
	var reversePosition *model.Position
	for _, openedPosition := range openedPositions {
		if model.PositionSideType(openedPosition.PositionSide) == postionSide {
			existPosition = openedPosition
		} else {
			reversePosition = openedPosition
		}
	}
	// 当前方向已存在仓位，不在开仓
	if existPosition != nil {
		if c.setting.Backtest == false {
			utils.Log.Infof("[POSITION - EXSIT] %s | Current: %v", existPosition.String(), currentPrice)
		}
		return
	}
	// 策略通过，判断当前是否已有未成交的限价单
	// 判断之前是否已有未成交的限价单
	// 直接获取当前交易对订单
	// 原始为空 止损为多  当前为多
	// 判断当前是否已有限价止损单
	// 有限价止损单时，判断止损方向和当前方向一致说明反向了
	// 在判断新的多空比和开仓多空比的大小，新的多空比绝对值比旧的小，需要继续持仓
	// 反之取消所有的限价止损单
	// ----------------

	if reversePosition != nil {
		if model.PositionSideType(reversePosition.PositionSide) != postionSide {
			if model.PositionSideType(reversePosition.PositionSide) == model.PositionSideTypeLong &&
				(0.5-longShortRatio) > 0 &&
				(0.5-longShortRatio) >= calc.Abs(0.5-reversePosition.LongShortRatio) {
				c.finishPosition(SeasonTypeReverse, reversePosition)
			}
			if model.PositionSideType(reversePosition.PositionSide) == model.PositionSideTypeShort &&
				(0.5-longShortRatio) < 0 &&
				calc.Abs(0.5-longShortRatio) <= (0.5-reversePosition.LongShortRatio) {
				c.finishPosition(SeasonTypeReverse, reversePosition)
			}
		}
	}
	var atrSum, sumOpenPrice, avgOpenPrice float64
	// 获取平均开仓价格
	for _, strategy := range strategies {
		atrSum += strategy.LastAtr
		sumOpenPrice += strategy.OpenPrice
	}
	avgOpenPrice = sumOpenPrice / float64(len(strategies))
	// 设置止损订单
	var stopLimitPrice, stopTrigerPrice, stopLossDistance float64
	var closeSideType model.SideType
	// 计算止损距离
	if option.MaxMarginLossRatio > 0 {
		stopLossDistance = calc.StopLossDistance(option.MaxMarginLossRatio, avgOpenPrice, float64(option.Leverage))
	} else {
		stopLossDistance = atrSum / float64(len(strategies))
	}
	if finalSide == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
		closeSideType = model.SideTypeSell
		stopLimitPrice = avgOpenPrice - stopLossDistance
		stopTrigerPrice = avgOpenPrice - stopLossDistance*0.85
	} else {
		postionSide = model.PositionSideTypeShort
		closeSideType = model.SideTypeBuy
		stopLimitPrice = avgOpenPrice + stopLossDistance
		stopTrigerPrice = avgOpenPrice + stopLossDistance*0.85
	}
	// 判断是否有当前方向未成交的订单
	hasOrder, err := c.CheckHasUnfilledPositionOrders(option.Pair, finalSide, postionSide)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if hasOrder {
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
	// 计算仓位大小
	amount := c.getPositionMargin(quotePosition, avgOpenPrice, option)
	lastTime, _ := c.lastUpdate.Get(option.Pair)
	if c.setting.Backtest == false {
		utils.Log.Infof(
			"[POSITION OPENING] Pair: %s | P.Side: %s | Quantity: %v | Price: %v ｜ Time: %s",
			option.Pair,
			postionSide,
			amount,
			avgOpenPrice,
			lastTime.In(Loc).Format("2006-01-02 15:04:05"),
		)
	}
	// 根据最新价格创建限价单
	order, err := c.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, avgOpenPrice, model.OrderExtra{
		Leverage:        option.Leverage,
		LongShortRatio:  longShortRatio,
		StopLossPrice:   stopLimitPrice,
		MatcherStrategy: strategies,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	_, err = c.broker.CreateOrderStopLimit(
		closeSideType,
		postionSide,
		option.Pair,
		order.Quantity,
		stopLimitPrice,
		stopTrigerPrice,
		model.OrderExtra{
			Leverage:        option.Leverage,
			OrderFlag:       order.OrderFlag,
			LongShortRatio:  longShortRatio,
			MatcherStrategy: strategies,
		},
	)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 重置当前交易对止损比例
	c.resetPairProfit(option.Pair)
}

func (c *Scoop) closeScoopPosition(option *model.PairOption) {
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

	currentPrice, _ := c.pairPrices.Get(option.Pair)
	currentTime := time.Now()
	if c.setting.Backtest {
		currentTime, _ = c.lastUpdate.Get(option.Pair)
	}
	// 与当前方向相反有仓位,计算相对分界线距离，多空比达到反手标准平仓
	// ***********************
	for _, openedPosition := range openedPositions {
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		positionTimeout, ok := c.positionTimeouts.Get(openedPosition.OrderFlag)
		if !ok {
			positionTimeout = openedPosition.UpdatedAt.Add(time.Duration(c.setting.PositionTimeOut) * time.Minute)
			c.positionTimeouts.Set(openedPosition.OrderFlag, positionTimeout)
		}
		// 时间未达到新的止损限制时间
		if currentTime.After(positionTimeout) {
			utils.Log.Infof(
				"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%%, MaxProfit: %.2f%% (Position Timeout)",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.MaxProfit*100,
			)
			c.ba.Reset()
			c.resetPairProfit(option.Pair)
			c.finishPosition(SeasonTypeTimeout, openedPosition)
			return
		}
		// ---------------------
		// 判断当前利润比是否小于锁定利润比，小于则平仓
		if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.IsLock {
			utils.Log.Infof(
				"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%% < ProfitClose: %.2f%%, MaxProfit: %.2f%%",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.Close*100,
				pairCurrentProfit.MaxProfit*100,
			)
			// 重置交易对盈利
			c.ba.Reset()
			c.resetPairProfit(option.Pair)
			c.finishPosition(SeasonTypeProfitBack, openedPosition)
			return
		}
		// ****************
		if profitRatio > 0 {
			// 判断是否已锁定利润比
			if pairCurrentProfit.IsLock == false {
				// 当前未锁定利润比，且利润比未达到触发利润比---直接返回
				if profitRatio < pairCurrentProfit.Floor {
					utils.Log.Infof(
						"[POSITION - WATCH] %s | Current: %v | PR.%%: %.2f%% < ProfitTriggerRatio: %.2f%%, MaxProfit: %.2f%% | StopAt: %s",
						openedPosition.String(),
						currentPrice,
						profitRatio*100,
						pairCurrentProfit.Floor*100,
						pairCurrentProfit.MaxProfit*100,
						positionTimeout.In(Loc).Format("2006-01-02 15:04:05"),
					)
					if profitRatio > pairCurrentProfit.MaxProfit {
						pairCurrentProfit.MaxProfit = profitRatio
						c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
					}
					return
				}
			} else {
				if profitRatio < pairCurrentProfit.Floor {
					// 当前利润比触发值，之前已经有Close时，判断当前利润比是否比上次设置的利润比大
					if profitRatio <= pairCurrentProfit.Close+pairCurrentProfit.Decrease {
						utils.Log.Infof(
							"[POSITION - WATCH] %s | Current: %v | PR.%%: %.2f%% < ProfitFloorRatio: %.2f%%, LockProfit: %.2f%%, MaxProfit: %.2f%% | StopAt: %s",
							openedPosition.String(),
							currentPrice,
							profitRatio*100,
							pairCurrentProfit.Floor*100,
							pairCurrentProfit.Close*100,
							pairCurrentProfit.MaxProfit*100,
							positionTimeout.In(Loc).Format("2006-01-02 15:04:05"),
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
			pairCurrentProfit.IsLock = true
			pairCurrentProfit.MaxProfit = profitRatio
			pairCurrentProfit.Close = profitRatio - pairCurrentProfit.Decrease
			c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
			// 盈利递增时修改时间止损结束时间
			c.positionTimeouts.Set(openedPosition.OrderFlag, currentTime.Add(time.Duration(c.setting.PositionTimeOut)*time.Minute))

			utils.Log.Infof(
				"[POSITION - PROFIT] %s | Current: %v | PR.%%: %.2f%% > NewProfitRatio: %.2f%% | StopAt: %s",
				openedPosition.String(),
				currentPrice,
				profitRatio*100,
				pairCurrentProfit.Close*100,
				positionTimeout.In(Loc).Format("2006-01-02 15:04:05"),
			)
		} else {
			if pairCurrentProfit.IsLock {
				utils.Log.Infof(
					"[POSITION - HOLD] %s | Current: %v | PR.%%: %.2f%% < ProfitCloseRatio: %.2f%%, MaxProfit: %.2f%% | StopAt: %s",
					openedPosition.String(),
					currentPrice,
					profitRatio*100,
					pairCurrentProfit.Close*100,
					pairCurrentProfit.MaxProfit*100,
					positionTimeout.In(Loc).Format("2006-01-02 15:04:05"),
				)
			} else {
				// 判断价格已经超过止损价格，直接平仓
				if (openedPosition.PositionSide == string(model.PositionSideTypeLong) && currentPrice <= openedPosition.StopLossPrice) ||
					(openedPosition.PositionSide == string(model.PositionSideTypeShort) && currentPrice >= openedPosition.StopLossPrice) {
					// 判断当前亏损是否超过10次检查
					if pairCurrentProfit.LossCount > 10 {
						utils.Log.Infof(
							"[POSITION - CLOSE] %s | Current: %v | PR.%%: %.2f%%, MaxProfit: %.2f%%",
							openedPosition.String(),
							currentPrice,
							profitRatio*100,
							pairCurrentProfit.MaxProfit*100,
						)
						c.resetPairProfit(option.Pair)
						c.finishPosition(SeasonTypeLossMax, openedPosition)
						return
					}
					pairCurrentProfit.LossCount = pairCurrentProfit.LossCount + 1
					c.pairCurrentProfit.Set(option.Pair, pairCurrentProfit)
				}
				utils.Log.Infof(
					"[POSITION - HOLD] %s | Current: %v | PR.%%: %.2f%% < MaxLoseRatio: %.2f%%, MaxProfit: %.2f%% | StopAt: %s",
					openedPosition.String(),
					currentPrice,
					profitRatio*100,
					option.MaxMarginLossRatio*100,
					pairCurrentProfit.MaxProfit*100,
					positionTimeout.In(Loc).Format("2006-01-02 15:04:05"),
				)
			}
		}
	}
}
