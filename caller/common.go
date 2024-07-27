package caller

import (
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

type CallerCommon struct {
	CallerBase
	samples              map[string]map[string]map[string]*model.Dataframe
	positionJudgers      map[string]*PositionJudger
	backtest             bool
	fullSpaceRadio       float64
	lossTimeDuration     int
	baseLossRatio        float64
	profitableScale      float64
	initProfitRatioLimit float64
	profitRatioLimit     map[string]float64
	lossLimitTimes       map[string]time.Time
}

func (cc *CallerCommon) Listen() {
	// 监听仓位关闭信号重置judger
	go cc.RegisterOrderSignal()
	// 非回溯测试时，执行检查仓位关闭
	if cc.backtest == false {
		// 执行超时检查
		go cc.tickCheckOrderTimeout()
		// 非回溯测试模式且不是看门狗方式下监听平仓
		if cc.setting.followSymbol == false {
			go cc.tickerCheckForClose(cc.pairOptions)
		}
	}
}

func (cc *CallerCommon) tickerCheckForClose(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Millisecond):
			for _, option := range options {
				cc.EventCallClose(option.Pair)
			}
		}
	}
}

func (cc *CallerCommon) RegisterOrderSignal() {
	for {
		select {
		case orderClose := <-types.OrderCloseChan:
			cc.ResetJudger(orderClose.Pair)
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func (cc *CallerCommon) ResetJudger(pair string) {
	cc.positionJudgers[pair] = &PositionJudger{
		Pair:          pair,
		Matchers:      []model.Strategy{},
		TendencyCount: make(map[string]int),
		Count:         0,
		CreatedAt:     time.Now(),
	}
}

func (s *CallerCommon) EventCallOpen(pair string) {
	assetPosition, quotePosition, longShortRatio, currentMatchers, matcherStrategy := s.checkPosition(s.pairOptions[pair])
	if longShortRatio >= 0 {
		s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy, currentMatchers)
	}
}

func (s *CallerCommon) EventCallClose(pair string) {
	_, _, longShortRatio, currentMatchers, _ := s.checkPosition(s.pairOptions[pair])
	if longShortRatio >= 0 {
		s.closePosition(s.pairOptions[pair], longShortRatio, currentMatchers)
	}
}

func (s *CallerCommon) checkPosition(option model.PairOption) (float64, float64, float64, []model.Strategy, map[string]int) {
	if _, ok := s.samples[option.Pair]; !ok {
		return 0, 0, -1, []model.Strategy{}, map[string]int{}
	}
	matchers := s.strategy.CallMatchers(s.samples[option.Pair])
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(finalTendency, currentMatchers)
	// 判断策略结果
	if s.backtest == true && len(currentMatchers) > 0 {
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

func (cc *CallerCommon) openPosition(option model.PairOption, assetPosition, quotePosition, longShortRatio float64, matcherStrategy map[string]int, strategies []model.Strategy) {
	cc.mu.Lock()         // 加锁
	defer cc.mu.Unlock() // 解锁
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

	openedPositions, err := cc.broker.GetPositionsForPair(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	currentPrice := cc.pairPrices[option.Pair]
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
		if cc.backtest == false {
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
				cc.finishPosition(SeasonTypeReverse, reversePosition)
				return
			}
			if model.PositionSideType(reversePosition.PositionSide) == model.PositionSideTypeShort &&
				(0.5-longShortRatio) < 0 &&
				calc.Abs(0.5-longShortRatio) <= (0.5-reversePosition.LongShortRatio) {
				cc.finishPosition(SeasonTypeReverse, reversePosition)
				return
			}
		}
	}

	// 获取最新仓位positionSide
	if finalSide == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
	} else {
		postionSide = model.PositionSideTypeShort
	}
	// ******************* 执行反手开仓操作 *****************//
	// 根据多空比动态计算仓位大小
	scoreRadio := calc.Abs(0.5-longShortRatio) / 0.5
	amount := calc.OpenPositionSize(quotePosition, float64(cc.pairOptions[option.Pair].Leverage), currentPrice, scoreRadio, cc.fullSpaceRadio)
	if cc.backtest == false {
		utils.Log.Infof(
			"[POSITION OPENING] Pair: %s | P.Side: %s | Quantity: %v | Price: %v",
			option.Pair,
			postionSide,
			amount,
			currentPrice,
		)
	}

	// 重置当前交易对止损比例
	cc.profitRatioLimit[option.Pair] = 0
	// 根据最新价格创建限价单
	order, err := cc.broker.CreateOrderLimit(finalSide, postionSide, option.Pair, amount, currentPrice, model.OrderExtra{
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

	var lossRatio = cc.baseLossRatio * float64(option.Leverage)
	if scoreRadio < 0.5 {
		lossRatio = lossRatio * 0.5
	} else {
		lossRatio = lossRatio * scoreRadio
	}
	// 计算止损距离
	stopLossDistance := calc.StopLossDistance(lossRatio, order.Price, float64(cc.pairOptions[option.Pair].Leverage), amount)
	if finalSide == model.SideTypeBuy {
		closeSideType = model.SideTypeSell
		stopLimitPrice = order.Price - stopLossDistance
		stopTrigerPrice = order.Price - stopLossDistance*StopLossDistanceRatio
	} else {
		closeSideType = model.SideTypeBuy
		stopLimitPrice = order.Price + stopLossDistance
		stopTrigerPrice = order.Price + stopLossDistance*StopLossDistanceRatio
	}
	_, err = cc.broker.CreateOrderStopLimit(
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
	cc.ResetJudger(option.Pair)
}

func (cc *CallerCommon) closePosition(option model.PairOption, longShortRatio float64, strategies []model.Strategy) {
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

	currentPrice := cc.pairPrices[option.Pair]
	var closeSideType model.SideType
	var currentTime time.Time
	var stopLossDistance float64
	var stopLimitPrice float64
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
				cc.finishPosition(SeasonTypeReverse, openedPosition)
				continue
			}
			if model.PositionSideType(openedPosition.PositionSide) == model.PositionSideTypeShort &&
				(0.5-longShortRatio) < 0 &&
				calc.Abs(0.5-longShortRatio) <= (0.5-openedPosition.LongShortRatio) {
				cc.finishPosition(SeasonTypeReverse, openedPosition)
				continue
			}
		}
		// 监控已成交仓位，记录订单成交时间+指定时间作为时间止损
		if _, ok := cc.lossLimitTimes[openedPosition.OrderFlag]; !ok {
			cc.lossLimitTimes[openedPosition.OrderFlag] = openedPosition.UpdatedAt.Add(time.Duration(cc.lossTimeDuration) * time.Minute)
		}
		// 记录利润比
		profitRatio := calc.ProfitRatio(
			model.SideType(openedPosition.Side),
			openedPosition.AvgPrice,
			currentPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		if cc.backtest == false {
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
				cc.lossLimitTimes[openedPosition.OrderFlag].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		if model.SideType(openedPosition.Side) == model.SideTypeBuy {
			closeSideType = model.SideTypeSell
		} else {
			closeSideType = model.SideTypeBuy
		}
		currentTime = time.Now()
		if cc.setting.checkMode == "candle" {
			currentTime = cc.lastUpdate
		}
		// 如果利润比大于预设值，则使用计算出得利润比 - 指定步进的利润比 得到新的止损利润比
		// 小于预设值，判断止损时间
		// 此处处理时间止损
		// 获取当前时间使用
		if profitRatio < cc.initProfitRatioLimit || profitRatio <= (cc.profitRatioLimit[option.Pair]+cc.profitableScale+0.01) {
			// 时间未达到新的止损限制时间
			if currentTime.Before(cc.lossLimitTimes[openedPosition.OrderFlag]) {
				continue
			}
			// 时间超过限制时间，执行时间止损 市价平单
			cc.finishPosition(SeasonTypeTimeout, openedPosition)
			continue
		}
		// 盈利时更新止损终止时间
		cc.lossLimitTimes[openedPosition.OrderFlag] = currentTime.Add(time.Duration(cc.lossTimeDuration) * time.Minute)
		// 递增利润比
		currentLossLimitProfit := profitRatio - cc.profitableScale
		// 使用新的止损利润比计算止损点数
		stopLossDistance = calc.StopLossDistance(
			currentLossLimitProfit,
			openedPosition.AvgPrice,
			float64(option.Leverage),
			openedPosition.Quantity,
		)
		// 重新计算止损价格
		if model.SideType(openedPosition.Side) == model.SideTypeSell {
			stopLimitPrice = openedPosition.AvgPrice - stopLossDistance
		} else {
			stopLimitPrice = openedPosition.AvgPrice + stopLossDistance
		}
		if cc.backtest == false {
			utils.Log.Infof(
				"[POSITION - PROFIT] OrderFlag: %s | Pair: %s | P.Side: %s | Quantity: %v | Price: %v, Current: %v, Stop: %v | PR.%%: %s, SPR.%%: %s | Create: %s | Stop Cut-off: %s",
				openedPosition.OrderFlag,
				openedPosition.Pair,
				openedPosition.PositionSide,
				openedPosition.Quantity,
				openedPosition.AvgPrice,
				currentPrice,
				stopLimitPrice,
				fmt.Sprintf("%.2f%%", profitRatio*100),
				fmt.Sprintf("%.2f%%", currentLossLimitProfit*100),
				openedPosition.UpdatedAt.In(Loc).Format("2006-01-02 15:04:05"),
				cc.lossLimitTimes[openedPosition.OrderFlag].In(Loc).Format("2006-01-02 15:04:05"),
			)
		}
		// 获取原始止损单
		lossOrders, err := cc.broker.GetOrdersForPostionLossUnfilled(openedPosition.OrderFlag)
		if err != nil {
			utils.Log.Error(err)
			continue
		}
		// 设置新的止损单
		_, err = cc.broker.CreateOrderStopMarket(closeSideType, model.PositionSideType(openedPosition.PositionSide), option.Pair, openedPosition.Quantity, stopLimitPrice, model.OrderExtra{
			Leverage:             option.Leverage,
			OrderFlag:            openedPosition.OrderFlag,
			LongShortRatio:       openedPosition.LongShortRatio,
			MatcherStrategyCount: openedPosition.MatcherStrategyCount,
		})
		if err != nil {
			// 如果重新挂限价止损失败则不在取消
			utils.Log.Error(err)
			continue
		}
		cc.profitRatioLimit[option.Pair] = profitRatio - cc.profitableScale
		for _, lossOrder := range lossOrders {
			// 取消之前的止损单
			err = cc.broker.Cancel(*lossOrder)
			if err != nil {
				utils.Log.Error(err)
				return
			}
		}
	}
}

func (cc *CallerCommon) finishPosition(seasonType SeasonType, position *model.Position) {
	var closeSideType model.SideType
	if model.PositionSideType(position.PositionSide) == model.PositionSideTypeLong {
		closeSideType = model.SideTypeSell
	} else {
		closeSideType = model.SideTypeBuy
	}
	// 判断仓位方向为反方向，平掉现有仓位
	_, err := cc.broker.CreateOrderMarket(
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
	delete(cc.lossLimitTimes, position.OrderFlag)

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
	lossOrders, err := cc.broker.GetOrdersForPostionLossUnfilled(position.OrderFlag)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	for _, lossOrder := range lossOrders {
		// 取消之前的止损单
		err = cc.broker.Cancel(*lossOrder)
		if err != nil {
			utils.Log.Error(err)
			return
		}
	}
}

func (bcc *CallerCommon) Sanitizer(matchers []model.Strategy) (string, []model.Strategy) {
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

func (cc *CallerCommon) getStrategyLongShortRatio(finalTendency string, currentMatchers []model.Strategy) (float64, map[string]int) {
	longShortRatio := -1.0
	totalScore := 0
	matcherMapScore := make(map[string]int)
	matcherStrategy := make(map[string]int)
	// 无检查结果
	if len(currentMatchers) == 0 || finalTendency == "ambiguity" {
		return longShortRatio, matcherStrategy
	}
	// 计算总得分
	for _, strategy := range cc.strategy.Strategies {
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
