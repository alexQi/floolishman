package caller

import (
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"floolishman/utils/strutil"
	"fmt"
	"sort"
	"time"
)

var ExecInterval time.Duration = 4

type Scoop struct {
	Common
}

type ScoopCheckItem struct {
	PairOption     *model.PairOption
	Score          float64
	LongShortRatio float64
	Matchers       []model.Strategy
}

func (c *Scoop) Start() {
	now := time.Now()
	next := now.Truncate(time.Minute * ExecInterval).Add(time.Minute * ExecInterval)
	duration := time.Until(next)
	// 在下一个5分钟倍数时执行任务
	time.AfterFunc(duration, func() {
		c.tickCheckForOpen()
	})
	go c.Listen()
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
	ticker := time.NewTicker(ExecInterval * time.Minute)
	for {
		select {
		case <-ticker.C:
			utils.Log.Infof("[CALLER] tick check for all pair to open position ...")
			// 判断总仓位数量
			totalOpenedPositions, err := c.broker.GetPositionsForOpened()
			if err != nil {
				utils.Log.Error(err)
				continue
			}
			if len(totalOpenedPositions) >= MaxPairPositions {
				utils.Log.Infof("[POSITION - MAX PAIR] pair position reach to max, waiting...")
				continue
			}
			// 检查所有币种,获取可以开仓的币种
			var tempMatcherScore float64
			openAliablePairs := []ScoopCheckItem{}
			scoopCheckSlice := []ScoopCheckItem{}
			currentHour := time.Now().Hour()
			for _, option := range c.pairOptions {
				if option.Status == false {
					continue
				}
				if strutil.IsInArray(option.IgnoreHours, currentHour) {
					continue
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
				// 当前开单币种暂停防止在同一根蜡烛线内再次开单
				if _, ok := opendPositionSide[positionSide]; ok {
					c.PausePairCall(openItem.PairOption.Pair)
					continue
				}
				opendPositionSide[positionSide] = openItem.LongShortRatio
				// 执行开仓
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
	if c.pairOptions[pair].Status == false {
		return
	}
	longShortRatio, currentMatchers := c.checkScoopPosition(c.pairOptions[pair])
	if longShortRatio >= 0 {
		go c.openScoopPosition(c.pairOptions[pair], longShortRatio, currentMatchers)
	}
}

func (c *Scoop) EventCallClose(pair string) {
	c.closeScoopPosition(c.pairOptions[pair])
}

func (s *Scoop) checkScoopPosition(option *model.PairOption) (float64, []model.Strategy) {
	s.mu[option.Pair].Lock()         // 加锁
	defer s.mu[option.Pair].Unlock() // 解锁
	if _, ok := s.samples[option.Pair]; !ok {
		return -1, []model.Strategy{}
	}
	matchers := s.strategy.CallMatchers(s.samples[option.Pair])
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

func (c *Scoop) openScoopPosition(option *model.PairOption, longShortRatio float64, strategies []model.Strategy) {
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
			utils.Log.Infof(
				"[POSITION - EXSIT] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v",
				existPosition.OrderFlag,
				existPosition.Pair,
				existPosition.PositionSide,
				existPosition.Quantity,
				existPosition.AvgPrice,
				currentPrice,
			)
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
	var stopLossPrice, avgAtr, atrSum float64
	for _, strategy := range strategies {
		atrSum += strategy.LastAtr
	}
	avgAtr = atrSum / float64(len(strategies))
	// 获取最新仓位positionSide
	if finalSide == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
		stopLossPrice = currentPrice - avgAtr
	} else {
		postionSide = model.PositionSideTypeShort
		stopLossPrice = currentPrice + avgAtr
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
	// ******************* 执行反手开仓操作 *****************//
	// 根据多空比动态计算仓位大小
	amount := c.getPositionMargin(quotePosition, currentPrice, option)
	lastTime, _ := c.lastUpdate.Get(option.Pair)
	if c.setting.Backtest == false {
		utils.Log.Infof(
			"[POSITION OPENING] Pair: %s | P.Side: %s | Quantity: %v | Price: %v ｜ Time: %s",
			option.Pair,
			postionSide,
			amount,
			currentPrice,
			lastTime.In(Loc).Format("2006-01-02 15:04:05"),
		)
	}
	// 根据最新价格创建限价单
	_, err = c.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
		Leverage:       option.Leverage,
		LongShortRatio: longShortRatio,
		StopLossPrice:  stopLossPrice,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 重置当前交易对止损比例
	c.resetPairProfit(option.Pair)
	// 重置开仓检查条件
	c.ResetJudger(option.Pair)
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
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		lossLimitTime, ok := c.lossLimitTimes.Get(openedPosition.OrderFlag)
		if !ok {
			lossLimitTime = openedPosition.UpdatedAt.Add(time.Duration(c.setting.LossTimeDuration) * time.Minute)
			c.lossLimitTimes.Set(openedPosition.OrderFlag, lossLimitTime)
		}
		if c.setting.Backtest == false {
			utils.Log.Infof(
				"[POSITION - WATCH] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v | PR.%%: %s | Create: %s | Stop Cut-off: %s",
				openedPosition.OrderFlag,
				openedPosition.Pair,
				openedPosition.PositionSide,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				lossLimitTime.In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		// 时间未达到新的止损限制时间
		if currentTime.After(lossLimitTime) {
			utils.Log.Infof(
				"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s (time out)",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
			)
			c.resetPairProfit(option.Pair)
			c.PausePairCall(option.Pair)
			c.finishPosition(SeasonTypeTimeout, openedPosition)
			continue
		}
		// ****************
		if profitRatio > 0 {
			pairCurrentProfit, _ := c.pairCurrentProfit.Get(option.Pair)
			// ---------------------
			// 判断利润比小于等于上次设置的利润比，则平仓 初始时为0
			if profitRatio <= pairCurrentProfit.Close && pairCurrentProfit.Close > 0 {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitClose: %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
				)
				// 重置交易对盈利
				c.resetPairProfit(option.Pair)
				c.PausePairCall(option.Pair)
				c.finishPosition(SeasonTypeProfitBack, openedPosition)
				return
			}
			profitTriggerRatio := pairCurrentProfit.Floor
			// 判断是否已锁定利润比
			if pairCurrentProfit.Close == 0 {
				// 小于触发值时，记录当前利润比
				if profitRatio < profitTriggerRatio {
					utils.Log.Infof(
						"[POSITION - WATCH] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < ProfitTriggerRatio: %s",
						openedPosition.Pair,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
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
							openedPosition.Pair,
							openedPosition.OrderFlag,
							openedPosition.Quantity,
							openedPosition.AvgPrice,
							openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
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
			// 盈利递增时修改时间止损结束时间
			c.lossLimitTimes.Set(openedPosition.OrderFlag, currentTime.Add(time.Duration(c.setting.LossTimeDuration)*time.Minute))

			utils.Log.Infof(
				"[POSITION - PROFIT] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > NewProfitRatio: %s",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", pairCurrentProfit.Close*100),
			)
		} else {
			if option.MaxMarginLossRatio > 0 {
				// 亏损盈利比已大于最大
				if calc.Abs(profitRatio) > option.MaxMarginLossRatio {
					utils.Log.Infof(
						"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
						openedPosition.Pair,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
						fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
					)
					c.resetPairProfit(option.Pair)
					c.PausePairCall(option.Pair)
					c.finishPosition(SeasonTypeLossMax, openedPosition)
					return
				}
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < MaxLoseRatio: %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", option.MaxMarginLossRatio*100),
				)
			} else {
				if (openedPosition.PositionSide == string(model.PositionSideTypeLong) && currentPrice <= openedPosition.StopLossPrice) ||
					(openedPosition.PositionSide == string(model.PositionSideTypeShort) && currentPrice >= openedPosition.StopLossPrice) {
					utils.Log.Infof(
						"[POSITION - CLOSE] Pair: %s | PositionSide: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v, StopLoss:%v | PR.%%: %s",
						openedPosition.Pair,
						openedPosition.PositionSide,
						openedPosition.OrderFlag,
						openedPosition.Quantity,
						openedPosition.AvgPrice,
						openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
						currentPrice,
						openedPosition.StopLossPrice,
						fmt.Sprintf("%.2f%%", profitRatio*100),
					)
					c.resetPairProfit(option.Pair)
					c.PausePairCall(option.Pair)
					c.finishPosition(SeasonTypeLossMax, openedPosition)
					return
				}
				utils.Log.Infof(
					"[POSITION - HOLD] Pair: %s | PositionSide: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v, StopLoss:%v | PR.%%: %s",
					openedPosition.Pair,
					openedPosition.PositionSide,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					openedPosition.StopLossPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
				)
			}
		}
	}
}
