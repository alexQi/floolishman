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
	CheckOpenInterval     time.Duration = 10
	ResetStrategyInterval time.Duration = 120
	StopLossDistanceRatio               = 0.9
)

type Common struct {
	Base
}

//"lossTimeDuration": 40,

func (c *Common) Listen() {
	// 监听仓位关闭信号重置judger
	go c.RegisterOrderSignal()
	// 非回溯测试时，执行检查仓位关闭
	if c.setting.Backtest == false {
		// 执行超时检查
		go c.tickCheckOrderTimeout()
		// 非回溯测试模式且不是看门狗方式下监听平仓
		if c.setting.FollowSymbol == false {
			go c.tickerCheckForClose()
		}
	}
}

func (c *Common) tickerCheckForClose() {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Millisecond):
			for _, option := range c.pairOptions {
				c.EventCallClose(option.Pair)
			}
		}
	}
}

func (c *Common) RegisterOrderSignal() {
	for {
		select {
		case orderClose := <-types.OrderCloseChan:
			c.ResetJudger(orderClose.Pair)
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *Common) ResetJudger(pair string) {
	c.positionJudgers[pair] = &PositionJudger{
		Pair:          pair,
		Matchers:      []model.Strategy{},
		TendencyCount: make(map[string]int),
		Count:         0,
		CreatedAt:     time.Now(),
	}
}

func (s *Common) EventCallOpen(pair string) {
	assetPosition, quotePosition, longShortRatio, currentMatchers, matcherStrategy := s.checkPosition(s.pairOptions[pair])
	if longShortRatio >= 0 {
		// todo 反向明灯
		//if len(currentMatchers) < 2 {
		//	return
		//}
		//if longShortRatio > 0.5 {
		//	longShortRatio = 0
		//} else {
		//	longShortRatio = 1
		//}
		s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy, currentMatchers)
	}
}

func (s *Common) EventCallClose(pair string) {
	_, _, longShortRatio, currentMatchers, _ := s.checkPosition(s.pairOptions[pair])
	if longShortRatio >= 0 {
		s.closePosition(s.pairOptions[pair], longShortRatio, currentMatchers)
	}
}

func (s *Common) checkPosition(option *model.PairOption) (float64, float64, float64, []model.Strategy, map[string]int) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	if _, ok := s.samples[option.Pair]; !ok {
		return 0, 0, -1, []model.Strategy{}, map[string]int{}
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
	if longShortRatio < 0 {
		return 0, 0, longShortRatio, currentMatchers, matcherStrategy
	}
	assetPosition, quotePosition, err := s.broker.PairAsset(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return 0, 0, longShortRatio, currentMatchers, matcherStrategy
	}
	return assetPosition, quotePosition, longShortRatio, currentMatchers, matcherStrategy
}

func (c *Common) openPosition(option *model.PairOption, assetPosition, quotePosition, longShortRatio float64, matcherStrategy map[string]int, strategies []model.Strategy) {
	c.mu.Lock()         // 加锁
	defer c.mu.Unlock() // 解锁
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	var finalSide model.SideType
	var closeSideType model.SideType
	var postionSide model.PositionSideType

	if longShortRatio > 0.5 {
		finalSide = model.SideTypeBuy
		closeSideType = model.SideTypeSell
		postionSide = model.PositionSideTypeLong

	} else {
		finalSide = model.SideTypeSell
		closeSideType = model.SideTypeBuy
		postionSide = model.PositionSideTypeShort
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && finalSide == model.SideTypeBuy {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && finalSide == model.SideTypeSell {
		return
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
	// 获取最新仓位positionSide
	if finalSide == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
	} else {
		postionSide = model.PositionSideTypeShort
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
	// ******************* 执行反手开仓操作 *****************//
	// 根据多空比动态计算仓位大小
	scoreRadio := calc.Abs(0.5-longShortRatio) / 0.5
	var amount float64
	if option.MarginMode == constants.MarginModeRoll {
		amount = calc.OpenPositionSize(quotePosition, float64(option.Leverage), currentPrice, scoreRadio, option.MarginSize)
	} else {
		amount = option.MarginSize
	}
	if c.setting.Backtest == false {
		utils.Log.Infof(
			"[POSITION OPENING] Pair: %s | P.Side: %s | Quantity: %v | Price: %v",
			option.Pair,
			postionSide,
			amount,
			currentPrice,
		)
	}

	// 重置当前交易对止损比例
	c.resetPairProfit(option.Pair)
	// 根据最新价格创建限价单
	order, err := c.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
		Leverage:             option.Leverage,
		LongShortRatio:       longShortRatio,
		MatcherStrategyCount: matcherStrategy,
		MatcherStrategy:      strategies,
	})
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var stopLimitPrice float64
	var stopTrigerPrice float64

	var lossRatio = option.MaxMarginLossRatio * float64(option.Leverage)
	if scoreRadio < 0.5 {
		lossRatio = lossRatio * 0.5
	} else {
		lossRatio = lossRatio * scoreRadio
	}
	// 计算止损距离
	stopLossDistance := calc.StopLossDistance(lossRatio, order.Price, float64(option.Leverage), amount)
	if finalSide == model.SideTypeBuy {
		closeSideType = model.SideTypeSell
		stopLimitPrice = order.Price - stopLossDistance
		stopTrigerPrice = order.Price - stopLossDistance*StopLossDistanceRatio
	} else {
		closeSideType = model.SideTypeBuy
		stopLimitPrice = order.Price + stopLossDistance
		stopTrigerPrice = order.Price + stopLossDistance*StopLossDistanceRatio
	}
	_, err = c.broker.CreateOrderStopLimit(
		closeSideType,
		postionSide,
		option.Pair,
		order.Quantity,
		stopLimitPrice,
		stopTrigerPrice,
		model.OrderExtra{
			Leverage:             option.Leverage,
			OrderFlag:            order.OrderFlag,
			LongShortRatio:       longShortRatio,
			MatcherStrategyCount: order.MatcherStrategyCount,
		},
	)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 重置开仓检查条件
	c.ResetJudger(option.Pair)
}

func (c *Common) closePosition(option *model.PairOption, longShortRatio float64, strategies []model.Strategy) {
	c.mu.Lock()         // 加锁
	defer c.mu.Unlock() // 解锁
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
	var currentTime time.Time
	// ***********************
	var checkPostionSide model.PositionSideType
	if longShortRatio > 0.5 {
		checkPostionSide = model.PositionSideTypeLong
	} else {
		checkPostionSide = model.PositionSideTypeShort
	}
	// 与当前方向相反有仓位,计算相对分界线距离，多空比达到反手标准平仓
	// ***********************
	for _, openedPosition := range openedPositions {
		// 判断多空比已反转的仓位平仓
		if model.PositionSideType(openedPosition.PositionSide) != checkPostionSide {
			//calc.Abs(0.5-longShortRatio) >= calc.Abs(0.5-openedPosition.LongShortRatio)
			if model.PositionSideType(openedPosition.PositionSide) == model.PositionSideTypeLong &&
				(0.5-longShortRatio) > 0 &&
				(0.5-longShortRatio) >= calc.Abs(0.5-openedPosition.LongShortRatio) {
				c.finishPosition(SeasonTypeReverse, openedPosition)
				continue
			}
			if model.PositionSideType(openedPosition.PositionSide) == model.PositionSideTypeShort &&
				(0.5-longShortRatio) < 0 &&
				calc.Abs(0.5-longShortRatio) <= (0.5-openedPosition.LongShortRatio) {
				c.finishPosition(SeasonTypeReverse, openedPosition)
				continue
			}
		}
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		_, ok := c.lossLimitTimes.Get(openedPosition.OrderFlag)
		if !ok {
			c.lossLimitTimes.Set(openedPosition.OrderFlag, openedPosition.UpdatedAt.Add(time.Duration(c.setting.LossTimeDuration)*time.Minute))
		}
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		lossLimitTime, _ := c.lossLimitTimes.Get(openedPosition.OrderFlag)
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
		currentTime = time.Now()
		if c.setting.CheckMode == "candle" {
			currentTime, _ = c.lastUpdate.Get(option.Pair)
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
			c.finishAllPosition(openedPosition, &model.Position{})
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
				c.finishAllPosition(openedPosition, &model.Position{})
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
			lossRatio := option.MaxMarginLossRatio * float64(option.Leverage)
			// 亏损盈利比已大于最大
			if calc.Abs(profitRatio) > lossRatio {
				utils.Log.Infof(
					"[POSITION - CLOSE] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s > MaxLoseRatio %s",
					openedPosition.Pair,
					openedPosition.OrderFlag,
					openedPosition.Quantity,
					openedPosition.AvgPrice,
					openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
					currentPrice,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					fmt.Sprintf("%.2f%%", lossRatio*100),
				)
				c.resetPairProfit(option.Pair)
				c.finishAllPosition(openedPosition, &model.Position{})
				return
			}
			utils.Log.Infof(
				"[POSITION - HOLD] Pair: %s | Main OrderFlag: %s, Quantity: %v, Price: %v, Time: %s | Current: %v | PR.%%: %s < LossStepRatio: %s",
				openedPosition.Pair,
				openedPosition.OrderFlag,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				currentPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", StepMoreRatio*100),
			)
		}
	}
}

func (c *Common) finishPosition(seasonType SeasonType, position *model.Position) {
	var closeSideType model.SideType
	if model.PositionSideType(position.PositionSide) == model.PositionSideTypeLong {
		closeSideType = model.SideTypeSell
	} else {
		closeSideType = model.SideTypeBuy
	}
	// 判断仓位方向为反方向，平掉现有仓位
	_, err := c.broker.CreateOrderMarket(
		closeSideType,
		model.PositionSideType(position.PositionSide),
		position.Pair,
		position.Quantity,
		model.OrderExtra{
			Leverage:             position.Leverage,
			OrderFlag:            position.OrderFlag,
			LongShortRatio:       position.LongShortRatio,
			MatcherStrategyCount: position.MatcherStrategyCount,
		},
	)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 删除止损时间限制配置
	c.lossLimitTimes.Delete(position.OrderFlag)
	utils.Log.Infof(
		"[POSITION - %s] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v",
		seasonType,
		position.OrderFlag,
		position.Pair,
		position.PositionSide,
		position.Quantity,
		position.AvgPrice,
		position,
	)
	// 查询当前orderFlag所有的止损单，全部取消
	lossOrders, err := c.broker.GetOrdersForPostionLossUnfilled(position.OrderFlag)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for _, lossOrder := range lossOrders {
		// 取消之前的止损单
		err = c.broker.Cancel(*lossOrder)
		if err != nil {
			utils.Log.Error(err)
			return
		}
	}
}

func (bcc *Common) Sanitizer(matchers []model.Strategy) (string, []model.Strategy) {
	var finalTendency string
	// 初始化变量
	currentMatchers := []model.Strategy{}
	// 调用策略执行器
	// 如果没有匹配的策略位置，直接返回空方向
	if len(matchers) == 0 {
		return finalTendency, currentMatchers
	}
	totalScore := 0
	matcherMapScore := make(map[string]int)
	// 初始化本次趋势计数器
	// 初始化多空双方map
	tendencyCounts := make(map[string]map[string]int)
	// 更新计数器和得分
	for _, pos := range matchers {
		// 计算总得分
		totalScore += pos.Score
		// 统计当前所有得分
		if _, ok := matcherMapScore[pos.StrategyName]; !ok {
			matcherMapScore[pos.StrategyName] = pos.Score
		}
		// 趋势判断 不需要判断当前是否可用
		if _, ok := tendencyCounts[pos.Tendency]; !ok {
			tendencyCounts[pos.Tendency] = make(map[string]int)
		}
		tendencyCounts[pos.Tendency][pos.StrategyName]++
		// 跳过不可用的策略
		if pos.Useable == 0 {
			continue
		}
		// 统计通过的策略
		currentMatchers = append(currentMatchers, pos)
	}

	currentTendency := map[string]float64{}
	// 外层循环方向
	for td, sm := range tendencyCounts {
		for sn, count := range sm {
			currentTendency[td] += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
		}
	}
	// 获取最终趋势
	var initTendency float64
	for tendency, tc := range currentTendency {
		if tc > initTendency {
			finalTendency = tendency
			initTendency = tc
		}
	}
	// 返回结果
	return finalTendency, currentMatchers
}

func (c *Common) getStrategyLongShortRatio(finalTendency string, currentMatchers []model.Strategy) (float64, map[string]int) {
	longShortRatio := -1.0
	totalScore := 0
	matcherMapScore := make(map[string]int)
	matcherStrategy := make(map[string]int)
	// 无检查结果
	if len(currentMatchers) == 0 || finalTendency == "ambiguity" {
		return longShortRatio, matcherStrategy
	}
	// 计算总得分
	for _, strategy := range c.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	// 初始化多空双方map
	result := map[model.SideType]map[string]int{
		model.SideTypeBuy:  make(map[string]int),
		model.SideTypeSell: make(map[string]int),
	}
	// 统计多空双方出现次数
	for _, pos := range currentMatchers {
		// 获取策略权重评分
		if _, ok := matcherMapScore[pos.StrategyName]; !ok {
			matcherMapScore[pos.StrategyName] = pos.Score
		}
		// 统计出现次数
		result[model.SideType(pos.Side)][pos.StrategyName]++
	}
	var buyDivisor, sellDivisor float64
	// 外层循环方向
	for sideType, sm := range result {
		for sn, count := range sm {
			// 加权计算最终得分因子
			if sideType == model.SideTypeBuy {
				buyDivisor += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
			} else {
				sellDivisor += float64(count) * float64(matcherMapScore[sn]) / float64(totalScore)
			}
		}
	}

	if buyDivisor == sellDivisor {
		longShortRatio = -1
	} else {
		if sellDivisor == 0 {
			if buyDivisor > 0 {
				longShortRatio = 1
			} else {
				longShortRatio = -1
			}
		} else {
			if buyDivisor > 0 {
				longShortRatio = buyDivisor / (buyDivisor + sellDivisor)
			} else {
				longShortRatio = 0
			}
		}
	}
	if longShortRatio < 0 {
		return longShortRatio, matcherStrategy
	} else {
		if longShortRatio > 0.5 {
			return longShortRatio, result[model.SideTypeBuy]
		} else {
			return longShortRatio, result[model.SideTypeSell]
		}
	}
}
