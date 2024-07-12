package service

import (
	"context"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"reflect"
	"sync"
	"time"
)

type StrategySetting struct {
	CheckMode            string
	FullSpaceRadio       float64
	BaseLossRatio        float64
	ProfitableScale      float64
	InitProfitRatioLimit float64
}

type ServiceStrategy struct {
	ctx                  context.Context
	strategy             types.CompositesStrategy
	dataframes           map[string]map[string]*model.Dataframe
	samples              map[string]map[string]map[string]*model.Dataframe
	realCandles          map[string]map[string]*model.Candle
	pairPrices           map[string]float64
	pairOptions          map[string]model.PairOption
	broker               reference.Broker
	started              bool
	backtest             bool
	checkMode            string
	fullSpaceRadio       float64
	baseLossRatio        float64
	profitableScale      float64
	initProfitRatioLimit float64
	profitRatioLimit     map[string]float64
	mu                   sync.Mutex
	// 仓位检查员
	positionJudgers map[string]*types.PositionJudger
}

var (
	CheckOpenInterval     time.Duration = 10
	CheckCloseInterval    time.Duration = 3
	CheckStrategyInterval time.Duration = 1
	ResetStrategyInterval time.Duration = 120
	StopLossDistanceRatio float64       = 0.9
	OpenPassCountLimit                  = 10
)

func NewServiceStrategy(
	ctx context.Context,
	strategySetting StrategySetting,
	strategy types.CompositesStrategy,
	broker reference.Broker,
	backtest bool,
) *ServiceStrategy {
	return &ServiceStrategy{
		ctx:                  ctx,
		dataframes:           make(map[string]map[string]*model.Dataframe),
		samples:              make(map[string]map[string]map[string]*model.Dataframe),
		realCandles:          make(map[string]map[string]*model.Candle),
		pairPrices:           make(map[string]float64),
		pairOptions:          make(map[string]model.PairOption),
		strategy:             strategy,
		broker:               broker,
		backtest:             backtest,
		checkMode:            strategySetting.CheckMode,
		fullSpaceRadio:       strategySetting.FullSpaceRadio,
		baseLossRatio:        strategySetting.BaseLossRatio,
		profitableScale:      strategySetting.ProfitableScale,
		initProfitRatioLimit: strategySetting.InitProfitRatioLimit,
		profitRatioLimit:     make(map[string]float64),
		positionJudgers:      make(map[string]*types.PositionJudger),
	}
}

func (s *ServiceStrategy) Start() {
	s.started = true
	switch s.checkMode {
	case "interval":
		s.PeriodCall()
		break
	case "frequency":
		s.CheckForFrequency()
	case "candle":
		utils.Log.Infof("On Candle will processed")
		break
	default:
		s.PeriodCall()
		utils.Log.Infof("Use default check mode: <interval>")
	}
}

func (s *ServiceStrategy) CandleCall(pair string) {
	if s.started {
		assetPosition, quotePosition, longShortRatio, matcherStrategy := s.checkPosition(s.pairOptions[pair])
		if longShortRatio >= 0 {
			s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy)
		}
		s.closeOption(s.pairOptions[pair])
	}
}

// 普通周期性开仓（单次判断策略通过）
func (s *ServiceStrategy) PeriodCall() {
	if s.started && s.backtest == false {
		go s.TickerCheckForOpen(s.pairOptions)
		go s.TickerCheckForClose(s.pairOptions)
	}
}

func (s *ServiceStrategy) TickerCheckForOpen(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询数据是否满足开仓条件
		case <-time.After(CheckOpenInterval * time.Second):
			for _, option := range options {
				assetPosition, quotePosition, longShortRatio, matcherStrategy := s.checkPosition(option)
				if longShortRatio >= 0 {
					s.openPosition(option, assetPosition, quotePosition, longShortRatio, matcherStrategy)
				}
			}
		}
	}
}

func (s *ServiceStrategy) TickerCheckForClose(options map[string]model.PairOption) {
	for {
		select {
		// 定时查询当前是否有仓位
		case <-time.After(CheckCloseInterval * time.Second):
			for _, option := range options {
				s.closeOption(option)
			}
		}
	}
}

func (s *ServiceStrategy) CheckForFrequency() {
	if s.started && s.backtest == false {
		for _, option := range s.pairOptions {
			go s.StartJudger(option.Pair)
		}
	}
}

func (s *ServiceStrategy) StartJudger(pair string) {
	tickerCheck := time.NewTicker(CheckStrategyInterval * time.Second)
	tickerClose := time.NewTicker(CheckCloseInterval * time.Second)
	tickerReset := time.NewTicker(ResetStrategyInterval * time.Second)
	for {
		select {
		case <-tickerCheck.C:
			// 执行策略
			s.Process(pair)
			// 获取多空比
			longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(s.positionJudgers[pair].Matchers)
			utils.Log.Infof(
				"[JUDGE] Pair: %s | LongShortRatio: %.2f | TendencyCount: %v | MatcherStrategy:【%s】",
				pair,
				longShortRatio,
				s.positionJudgers[pair].TendencyCount,
				matcherStrategy,
			)
			// 多空比不满足开仓条件
			if longShortRatio < 0 {
				continue
			}
			// 计算当前方向通过总数
			passCount := 0
			for _, i := range matcherStrategy {
				passCount += i
			}
			// 当前方向通过次数少于阈值 不开仓
			if passCount < OpenPassCountLimit {
				continue
			}
			// 执行开仓检查
			assetPosition, quotePosition, err := s.broker.Position(pair)
			if err != nil {
				utils.Log.Error(err)
			}
			s.openPosition(s.pairOptions[pair], assetPosition, quotePosition, longShortRatio, matcherStrategy)
		case <-tickerClose.C:
			// 执行移动止损平仓
			s.closeOption(s.pairOptions[pair])
		case <-tickerReset.C:
			s.ResetJudger(pair)
		}
	}
}

func (s *ServiceStrategy) ResetJudger(pair string) {
	s.positionJudgers[pair] = &types.PositionJudger{
		Pair:          pair,
		Matchers:      []types.StrategyPosition{},
		TendencyCount: make(map[string]int),
		Count:         0,
		CreatedAt:     time.Now(),
	}
}

func (s *ServiceStrategy) Process(pair string) {
	// 如果 pair 在 positionJudgers 中不存在，则初始化
	if _, ok := s.positionJudgers[pair]; !ok {
		s.ResetJudger(pair)
	}
	// 执行计数器+1
	s.positionJudgers[pair].Count++
	// 执行策略检查
	matchers := s.strategy.CallMatchers(s.samples[pair])
	// 清洗策略结果
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	// 重组匹配策略数据
	s.positionJudgers[pair].Matchers = append(s.positionJudgers[pair].Matchers, currentMatchers...)
	// 更新趋势计数
	s.positionJudgers[pair].TendencyCount[finalTendency]++
}

func (s *ServiceStrategy) checkPosition(option model.PairOption) (float64, float64, float64, map[string]int) {
	if _, ok := s.realCandles[option.Pair]; !ok {
		return 0, 0, -1, map[string]int{}
	}
	matchers := s.strategy.CallMatchers(s.samples[option.Pair])
	finalTendency, currentMatchers := s.Sanitizer(matchers)
	longShortRatio, matcherStrategy := s.getStrategyLongShortRatio(currentMatchers)
	// 判断策略结果
	if s.backtest == false {
		utils.Log.Infof(
			"[JUDGE] Tendency: %s | Pair: %s | LongShortRatio: %.2f | Matchers:【%s】",
			finalTendency,
			option.Pair,
			longShortRatio,
			matcherStrategy,
		)
	}
	if longShortRatio < 0 {
		return 0, 0, longShortRatio, matcherStrategy
	}
	assetPosition, quotePosition, err := s.broker.Position(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return 0, 0, longShortRatio, matcherStrategy
	}
	return assetPosition, quotePosition, longShortRatio, matcherStrategy
}

// openPosition 开仓方法
func (s *ServiceStrategy) openPosition(option model.PairOption, assetPosition, quotePosition, longShortRatio float64, matcherStrategy map[string]int) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	var finalPosition model.SideType
	if longShortRatio > 0.5 {
		finalPosition = model.SideTypeBuy
	} else {
		finalPosition = model.SideTypeSell
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && finalPosition == model.SideTypeBuy {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && finalPosition == model.SideTypeSell {
		return
	}
	var tempSideType model.SideType
	var postionSide model.PositionSideType
	// 策略通过，判断当前是否已有未成交的限价单
	// 判断之前是否已有未成交的限价单
	// 直接获取当前交易对订单
	existOrders, err := s.getPositionOrders(option, s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 原始为空 止损为多  当前为多
	// 判断当前是否已有限价止损单
	// 有限价止损单时，判断止损方向和当前方向一致说明反向了
	// 在判断新的多空比和开仓多空比的大小，新的多空比绝对值比旧的小，需要继续持仓
	// 反之取消所有的限价止损单
	if _, ok := existOrders[model.OrderTypeStop]; ok {
		// 取消限价止损单
		stopLimitOrders, ok := existOrders[model.OrderTypeStop][model.OrderStatusTypeNew]
		if ok && len(stopLimitOrders) > 0 {
			for _, stopLimitOrder := range stopLimitOrders {
				// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
				if stopLimitOrder.Side != finalPosition {
					continue
				}
				// 计算相对分界线距离
				if calc.Abs(0.5-longShortRatio) < calc.Abs(0.5-stopLimitOrder.LongShortRatio) {
					continue
				}
				// 取消之前的止损单
				err = s.broker.Cancel(*stopLimitOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
		// 取消市价止损单
		stopLimitMarketOrders, ok := existOrders[model.OrderTypeStopMarket][model.OrderStatusTypeNew]
		if ok && len(stopLimitOrders) > 0 {
			for _, stopLimitMarketOrder := range stopLimitMarketOrders {
				// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
				if stopLimitMarketOrder.Side != finalPosition {
					continue
				}
				// 计算相对分界线距离
				if calc.Abs(0.5-longShortRatio) < calc.Abs(0.5-stopLimitMarketOrder.LongShortRatio) {
					continue
				}
				// 取消之前的止损单
				err = s.broker.Cancel(*stopLimitMarketOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
	currentPirce := s.pairPrices[option.Pair]
	// 判断当前是否已有仓位
	// 仓位类型双向持仓 下单时根据类型可下对冲单。通过协程提升平仓在开仓的效率
	// 有仓位时，判断当前持仓方向是否与策略相同
	holdedOrder := model.Order{}
	if _, ok := existOrders[model.OrderTypeLimit]; ok {
		// 判断是否已有仓位
		positionOrders, ok := existOrders[model.OrderTypeLimit][model.OrderStatusTypeFilled]
		if ok && len(positionOrders) > 0 {
			for _, positionOrder := range positionOrders {
				// 原始单方向和当前策略判断方向相同，保留原始单
				if positionOrder.Side == finalPosition {
					holdedOrder = *positionOrder
					continue
				}
				// 计算相对分界线距离
				if calc.Abs(0.5-longShortRatio) < calc.Abs(0.5-positionOrder.LongShortRatio) {
					holdedOrder = *positionOrder
					continue
				}
				if positionOrder.Side == model.SideTypeBuy {
					tempSideType = model.SideTypeSell
					postionSide = model.PositionSideTypeLong
				} else {
					tempSideType = model.SideTypeBuy
					postionSide = model.PositionSideTypeShort
				}
				// 判断仓位方向为反方向，平掉现有仓位
				_, err := s.broker.CreateOrderStopMarket(tempSideType, postionSide, option.Pair, positionOrder.Quantity, currentPirce, positionOrder.OrderFlag, positionOrder.LongShortRatio, positionOrder.MatchStrategy)
				if err != nil {
					utils.Log.Error(err)
				}
			}
		}
		if holdedOrder.ExchangeID == 0 {
			positionNewOrders, ok := existOrders[model.OrderTypeLimit][model.OrderStatusTypeNew]
			if ok && len(positionNewOrders) > 0 {
				for _, positionNewOrder := range positionNewOrders {
					// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
					if positionNewOrder.Side == finalPosition {
						holdedOrder = *positionNewOrder
						continue
					}
					// 计算相对分界线距离
					if calc.Abs(0.5-longShortRatio) < calc.Abs(0.5-positionNewOrder.LongShortRatio) {
						holdedOrder = *positionNewOrder
						continue
					}
					// 取消之前的限价单
					err = s.broker.Cancel(*positionNewOrder)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				}
			}
		}
	}
	if holdedOrder.ExchangeID > 0 {
		if s.backtest == false {
			utils.Log.Infof(
				"[ODER EXIST - %s] Pair: %s | Price: %v | Quantity: %v  | Side: %s |  OrderFlag: %s",
				holdedOrder.Status,
				option.Pair,
				holdedOrder.Price,
				holdedOrder.Quantity,
				holdedOrder.Side,
				holdedOrder.OrderFlag,
			)
		}
		return
	}

	// 根据多空比动态计算仓位大小
	scoreRadio := calc.Abs(0.5-longShortRatio) / 0.5
	amount := calc.OpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPirce, scoreRadio, s.fullSpaceRadio)
	if s.backtest == false {
		utils.Log.Infof(
			"[OPEN] Pair: %s | Price: %v | Quantity: %v | Side: %s",
			option.Pair,
			currentPirce,
			amount,
			finalPosition,
		)
	}

	// 重置当前交易对止损比例
	s.profitRatioLimit[option.Pair] = 0
	// 获取最新仓位positionSide
	if finalPosition == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
	} else {
		postionSide = model.PositionSideTypeShort
	}
	// 根据最新价格创建限价单
	order, err := s.broker.CreateOrderLimit(finalPosition, postionSide, option.Pair, amount, currentPirce, longShortRatio, matcherStrategy)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var stopLimitPrice float64
	var stopTrigerPrice float64

	var lossRatio = s.baseLossRatio * float64(option.Leverage)
	if scoreRadio < 0.5 {
		lossRatio = lossRatio * 0.5
	} else {
		lossRatio = lossRatio * scoreRadio
	}
	// 计算止损距离
	stopLossDistance := calc.StopLossDistance(lossRatio, order.Price, float64(s.pairOptions[option.Pair].Leverage), amount)
	if finalPosition == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLimitPrice = order.Price - stopLossDistance
		stopTrigerPrice = order.Price - stopLossDistance*StopLossDistanceRatio
	} else {
		tempSideType = model.SideTypeBuy
		stopLimitPrice = order.Price + stopLossDistance
		stopTrigerPrice = order.Price + stopLossDistance*StopLossDistanceRatio
	}
	_, err = s.broker.CreateOrderStopLimit(tempSideType, postionSide, option.Pair, amount, stopLimitPrice, stopTrigerPrice, order.OrderFlag, longShortRatio, order.MatchStrategy)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 重置开仓检查条件
	s.ResetJudger(option.Pair)
}

func (s *ServiceStrategy) closeOption(option model.PairOption) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	existOrderMap, err := s.getExistOrders(option, s.broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var tempSideType model.SideType
	var stopLossDistance float64
	var stopLimitPrice float64

	currentPirce := s.pairPrices[option.Pair]

	for orderFlag, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		for _, positionOrder := range positionOrders {
			profitRatio := calc.ProfitRatio(positionOrder.Side, positionOrder.Price, currentPirce, float64(option.Leverage), positionOrder.Quantity)
			if s.backtest == false {
				utils.Log.Infof(
					"[WATCH] Pair: %s | Price Order: %v, Current: %v | Quantity: %v | Profit Ratio: %s",
					option.Pair,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					fmt.Sprintf("%.2f%%", profitRatio*100),
				)
			}
			// 判断是否盈利，盈利中则处理平仓及移动止损，反之则保持之前的止损单
			if profitRatio <= 0 {
				return
			}
			// 如果利润比大于预设值，则使用计算出得利润比 - 指定步进的利润比 得到新的止损利润比
			// 0.22
			if profitRatio < s.initProfitRatioLimit || profitRatio <= (s.profitRatioLimit[option.Pair]+s.profitableScale) {
				return
			}
			// 递增利润比
			currentLossLimitProfit := profitRatio - s.profitableScale
			// 使用新的止损利润比计算止损点数
			stopLossDistance = calc.StopLossDistance(currentLossLimitProfit, positionOrder.Price, float64(option.Leverage), positionOrder.Quantity)
			// 重新计算止损价格
			if positionOrder.Side == model.SideTypeSell {
				stopLimitPrice = positionOrder.Price - stopLossDistance
			} else {
				stopLimitPrice = positionOrder.Price + stopLossDistance
			}
			if s.backtest == false {
				utils.Log.Infof(
					"[PROFIT] Pair: %s | Side: %s | Order Price: %v, Current: %v | Quantity: %v | Profit Ratio: %s | Stop Loss: %v, %s",
					option.Pair,
					positionOrder.Side,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					stopLimitPrice,
					fmt.Sprintf("%.2f%%", currentLossLimitProfit*100),
				)
			}

			if positionOrder.Side == model.SideTypeBuy {
				tempSideType = model.SideTypeSell
			} else {
				tempSideType = model.SideTypeBuy
			}
			// 设置新的止损单
			// 使用滚动利润比保证该止损利润是递增的
			// 不再判断新的止损价格是否小于之前的止损价格
			_, err := s.broker.CreateOrderStopMarket(tempSideType, positionOrder.PositionSide, option.Pair, positionOrder.Quantity, stopLimitPrice, positionOrder.OrderFlag, positionOrder.LongShortRatio, positionOrder.MatchStrategy)
			if err != nil {
				// 如果重新挂限价止损失败则不在取消
				utils.Log.Error(err)
				continue
			}
			s.profitRatioLimit[option.Pair] = profitRatio - s.profitableScale
			lossLimitOrders, ok := existOrderMap[orderFlag]["lossLimit"]
			if !ok {
				continue
			}
			for _, lossLimitOrder := range lossLimitOrders {
				// 取消之前的止损单
				err = s.broker.Cancel(*lossLimitOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
}

func (s *ServiceStrategy) getPositionOrders(option model.PairOption, broker reference.Broker) (map[model.OrderType]map[model.OrderStatusType][]*model.Order, error) {
	// 存储当前存在的仓位和限价单
	existOrders := map[model.OrderType]map[model.OrderStatusType][]*model.Order{}
	positionOrders, err := broker.GetCurrentPositionOrders(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return existOrders, err
	}
	if len(positionOrders) > 0 {
		for _, order := range positionOrders {
			// 判断当前订单状态
			if _, ok := existOrders[order.Type]; !ok {
				existOrders[order.Type] = make(map[model.OrderStatusType][]*model.Order)
			}
			if _, ok := existOrders[order.Type][order.Status]; !ok {
				existOrders[order.Type][order.Status] = []*model.Order{}
			}
			existOrders[order.Type][order.Status] = append(existOrders[order.Type][order.Status], order)
		}
	}
	return existOrders, nil
}

func (s *ServiceStrategy) getExistOrders(option model.PairOption, broker reference.Broker) (map[string]map[string][]*model.Order, error) {
	// 存储当前存在的仓位和限价单
	existOrders := map[string]map[string][]*model.Order{}
	positionOrders, err := broker.GetCurrentPositionOrders(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return existOrders, err
	}
	if len(positionOrders) > 0 {
		for _, order := range positionOrders {
			if _, ok := existOrders[order.OrderFlag]; !ok {
				existOrders[order.OrderFlag] = make(map[string][]*model.Order)
			}
			if order.Type == model.OrderTypeLimit && order.Status == model.OrderStatusTypeFilled {
				if _, ok := existOrders[order.OrderFlag]["position"]; !ok {
					existOrders[order.OrderFlag]["position"] = []*model.Order{}
				}
				existOrders[order.OrderFlag]["position"] = append(existOrders[order.OrderFlag]["position"], order)
			}
			if (order.Type == model.OrderTypeStop || order.Type == model.OrderTypeStopMarket) && order.Status == model.OrderStatusTypeNew {
				if _, ok := existOrders[order.OrderFlag]["lossLimit"]; !ok {
					existOrders[order.OrderFlag]["lossLimit"] = []*model.Order{}
				}
				existOrders[order.OrderFlag]["lossLimit"] = append(existOrders[order.OrderFlag]["lossLimit"], order)
			}
		}
	}
	return existOrders, nil
}

func (s *ServiceStrategy) Sanitizer(matchers []types.StrategyPosition) (string, []types.StrategyPosition) {
	var finalTendency string
	// 初始化变量
	currentMatchers := []types.StrategyPosition{}
	// 调用策略执行器
	// 如果没有匹配的策略位置，直接返回空方向
	if len(matchers) == 0 {
		return finalTendency, currentMatchers
	}
	// 初始化本次趋势计数器
	tendencyCounts := make(map[string]int)
	// 更新计数器和得分
	for _, pos := range matchers {
		// 趋势判断 不需要判断当前是否可用
		tendencyCounts[pos.Tendency] += 1
		// 跳过不可用的策略
		if pos.Useable == false {
			continue
		}
		// 统计通过的策略
		currentMatchers = append(currentMatchers, pos)
	}

	// 获取最终趋势
	initTendency := 0
	for tendency, tc := range tendencyCounts {
		if tc > initTendency {
			finalTendency = tendency
			initTendency = tc
		}
	}
	// 返回结果
	return finalTendency, currentMatchers
}

func (s *ServiceStrategy) getStrategyLongShortRatio(currentMatchers []types.StrategyPosition) (float64, map[string]int) {
	longShortRatio := -1.0
	totalScore := 0
	matcherMapScore := make(map[string]int)
	matcherStrategy := make(map[string]int)
	// 无检查结果
	if len(currentMatchers) == 0 {
		return longShortRatio, matcherStrategy
	}
	// 计算总得分
	for _, strategy := range s.strategy.Strategies {
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
		result[pos.Side][pos.StrategyName]++
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
			}
			longShortRatio = 0
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

func (s *ServiceStrategy) SetPairDataframe(option model.PairOption) {
	s.pairOptions[option.Pair] = option
	s.pairPrices[option.Pair] = 0
	s.profitRatioLimit[option.Pair] = 0
	if s.dataframes[option.Pair] == nil {
		s.dataframes[option.Pair] = make(map[string]*model.Dataframe)
	}
	if s.samples[option.Pair] == nil {
		s.samples[option.Pair] = make(map[string]map[string]*model.Dataframe)
	}
	if s.realCandles[option.Pair] == nil {
		s.realCandles[option.Pair] = make(map[string]*model.Candle)
	}
	// 初始化不同时间周期的dataframe 及 samples
	for _, strategy := range s.strategy.Strategies {
		s.dataframes[option.Pair][strategy.Timeframe()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
		if _, ok := s.samples[option.Pair][strategy.Timeframe()]; !ok {
			s.samples[option.Pair][strategy.Timeframe()] = make(map[string]*model.Dataframe)
		}
		s.samples[option.Pair][strategy.Timeframe()][reflect.TypeOf(strategy).Elem().Name()] = &model.Dataframe{
			Pair:     option.Pair,
			Metadata: make(map[string]model.Series[float64]),
		}
	}
}

func (s *ServiceStrategy) setDataFrame(dataframe model.Dataframe, candle model.Candle) model.Dataframe {
	if len(dataframe.Time) > 0 && candle.Time.Equal(dataframe.Time[len(dataframe.Time)-1]) {
		last := len(dataframe.Time) - 1
		dataframe.Close[last] = candle.Close
		dataframe.Open[last] = candle.Open
		dataframe.High[last] = candle.High
		dataframe.Low[last] = candle.Low
		dataframe.Volume[last] = candle.Volume
		dataframe.Time[last] = candle.Time
		for k, v := range candle.Metadata {
			dataframe.Metadata[k][last] = v
		}
	} else {
		dataframe.Close = append(dataframe.Close, candle.Close)
		dataframe.Open = append(dataframe.Open, candle.Open)
		dataframe.High = append(dataframe.High, candle.High)
		dataframe.Low = append(dataframe.Low, candle.Low)
		dataframe.Volume = append(dataframe.Volume, candle.Volume)
		dataframe.Time = append(dataframe.Time, candle.Time)
		dataframe.LastUpdate = candle.Time
		for k, v := range candle.Metadata {
			dataframe.Metadata[k] = append(dataframe.Metadata[k], v)
		}
	}
	return dataframe
}

func (s *ServiceStrategy) updateDataFrame(timeframe string, candle model.Candle) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	tempDataframe := s.setDataFrame(*s.dataframes[candle.Pair][timeframe], candle)
	s.dataframes[candle.Pair][timeframe] = &tempDataframe
}

func (s *ServiceStrategy) OnRealCandle(timeframe string, candle model.Candle) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	oldCandle, ok := s.realCandles[candle.Pair][timeframe]
	if ok && oldCandle.UpdatedAt.Before(candle.UpdatedAt) == false {
		return
	}
	s.realCandles[candle.Pair][timeframe] = &candle
	s.pairPrices[candle.Pair] = candle.Close
	// 采样数据转换指标
	for _, str := range s.strategy.Strategies {
		if len(s.dataframes[candle.Pair][timeframe].Close) < str.WarmupPeriod() {
			continue
		}
		// 执行数据采样
		sample := s.dataframes[candle.Pair][timeframe].Sample(str.WarmupPeriod())
		// 加入最新指标
		sample = s.setDataFrame(sample, candle)
		str.Indicators(&sample)
		// 在向samples添加之前，确保对应的键存在
		if timeframe == str.Timeframe() {
			s.samples[candle.Pair][timeframe][reflect.TypeOf(str).Elem().Name()] = &sample
		}
	}
}

func (s *ServiceStrategy) OnCandle(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	// 更新Dataframe
	s.updateDataFrame(timeframe, candle)
	s.OnRealCandle(timeframe, candle)
	if s.checkMode == "candle" {
		s.CandleCall(candle.Pair)
	}
}
