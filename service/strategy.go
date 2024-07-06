package service

import (
	"context"
	"floolishman/model"
	"floolishman/process"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"reflect"
	"sync"
)

type StrategyServiceSetting struct {
	VolatilityThreshold  float64
	FullSpaceRadio       float64
	InitLossRatio        float64
	ProfitableScale      float64
	InitProfitRatioLimit float64
}

type StrategyService struct {
	ctx                  context.Context
	strategy             types.CompositesStrategy
	dataframes           map[string]map[string]*model.Dataframe
	samples              map[string]map[string]map[string]*model.Dataframe
	realCandles          map[string]map[string]*model.Candle
	pairPrices           map[string]float64
	pairOptions          map[string]model.PairOption
	broker               reference.Broker
	started              bool
	volatilityThreshold  float64
	fullSpaceRadio       float64
	initLossRatio        float64
	profitableScale      float64
	initProfitRatioLimit float64
	profitRatioLimit     map[string]float64
	mu                   sync.Mutex
}

func NewStrategyService(ctx context.Context, tradingSetting StrategyServiceSetting, strategy types.CompositesStrategy, broker reference.Broker) *StrategyService {
	return &StrategyService{
		ctx:                  ctx,
		dataframes:           make(map[string]map[string]*model.Dataframe),
		samples:              make(map[string]map[string]map[string]*model.Dataframe),
		realCandles:          make(map[string]map[string]*model.Candle),
		pairPrices:           make(map[string]float64),
		pairOptions:          make(map[string]model.PairOption),
		strategy:             strategy,
		broker:               broker,
		volatilityThreshold:  tradingSetting.VolatilityThreshold,
		fullSpaceRadio:       tradingSetting.FullSpaceRadio,
		initLossRatio:        tradingSetting.InitLossRatio,
		profitableScale:      tradingSetting.ProfitableScale,
		initProfitRatioLimit: tradingSetting.InitProfitRatioLimit,
		profitRatioLimit:     make(map[string]float64),
	}
}

func (s *StrategyService) Start() {
	go process.CheckOpenPoistion(s.pairOptions, s.broker, s.openPosition)
	go process.CheckClosePoistion(s.pairOptions, s.broker, s.closeOption)
	s.started = true
}

func (s *StrategyService) SetPairDataframe(option model.PairOption) {
	utils.Log.Info(option.String())
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

func (s *StrategyService) updateDataFrame(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Equal(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		last := len(s.dataframes[candle.Pair][timeframe].Time) - 1
		s.dataframes[candle.Pair][timeframe].Close[last] = candle.Close
		s.dataframes[candle.Pair][timeframe].Open[last] = candle.Open
		s.dataframes[candle.Pair][timeframe].High[last] = candle.High
		s.dataframes[candle.Pair][timeframe].Low[last] = candle.Low
		s.dataframes[candle.Pair][timeframe].Volume[last] = candle.Volume
		s.dataframes[candle.Pair][timeframe].Time[last] = candle.Time
		for k, v := range candle.Metadata {
			s.dataframes[candle.Pair][timeframe].Metadata[k][last] = v
		}
	} else {
		s.dataframes[candle.Pair][timeframe].Close = append(s.dataframes[candle.Pair][timeframe].Close, candle.Close)
		s.dataframes[candle.Pair][timeframe].Open = append(s.dataframes[candle.Pair][timeframe].Open, candle.Open)
		s.dataframes[candle.Pair][timeframe].High = append(s.dataframes[candle.Pair][timeframe].High, candle.High)
		s.dataframes[candle.Pair][timeframe].Low = append(s.dataframes[candle.Pair][timeframe].Low, candle.Low)
		s.dataframes[candle.Pair][timeframe].Volume = append(s.dataframes[candle.Pair][timeframe].Volume, candle.Volume)
		s.dataframes[candle.Pair][timeframe].Time = append(s.dataframes[candle.Pair][timeframe].Time, candle.Time)
		s.dataframes[candle.Pair][timeframe].LastUpdate = candle.Time
		for k, v := range candle.Metadata {
			s.dataframes[candle.Pair][timeframe].Metadata[k] = append(s.dataframes[candle.Pair][timeframe].Metadata[k], v)
		}
	}
}

func (s *StrategyService) OnRealCandle(timeframe string, candle model.Candle) {
	s.mu.Lock()         // 加锁
	defer s.mu.Unlock() // 解锁
	oldCandle, ok := s.realCandles[candle.Pair][timeframe]
	if ok && oldCandle.UpdatedAt.Before(candle.UpdatedAt) == false {
		return
	}
	s.realCandles[candle.Pair][timeframe] = &candle
	s.pairPrices[candle.Pair] = candle.Close
}

func (s *StrategyService) OnCandle(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	s.updateDataFrame(timeframe, candle)

	for _, str := range s.strategy.Strategies {
		if len(s.dataframes[candle.Pair][timeframe].Close) >= str.WarmupPeriod() {
			sample := s.dataframes[candle.Pair][timeframe].Sample(str.WarmupPeriod())
			str.Indicators(&sample)
			// 在向samples添加之前，确保对应的键存在
			if timeframe == str.Timeframe() {
				s.samples[candle.Pair][timeframe][reflect.TypeOf(str).Elem().Name()] = &sample
			}
		}
	}
}

// openPosition 开仓方法
func (s *StrategyService) openPosition(option model.PairOption, broker reference.Broker) {
	if !s.started {
		return
	}
	assetPosition, quotePosition, err := s.broker.Position(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 无资产
	if quotePosition <= 0 {
		utils.Log.Errorf("Balance is not enough to create order")
		return
	}
	matchers := s.strategy.CallMatchers(s.realCandles[option.Pair], s.samples[option.Pair])
	// 判断策略结果
	currentPirce := s.pairPrices[option.Pair]
	matcherCount, totalScore, currentScore, finalTendency, finalPosition := s.judgeStrategyForScore(matchers)
	if currentScore == 0 {
		utils.Log.Infof("[WAIT] Tendency: %s | Pair: %s | Price: %v | Strategy Count: %d, %d Matchers ", finalTendency, option.Pair, currentPirce, len(s.strategy.Strategies), matcherCount)
		return
	}
	// 当前仓位为多，最近策略为多，保持仓位
	if assetPosition > 0 && finalPosition == model.SideTypeBuy {
		return
	}
	// 当前仓位为空，最近策略为空，保持仓位
	if assetPosition < 0 && finalPosition == model.SideTypeSell {
		return
	}
	// 当前分数小于总分数/策略总数,保持仓位
	if currentScore < totalScore/len(s.strategy.Strategies) {
		return
	}
	var tempSideType model.SideType
	var postionSide model.PositionSideType
	// 策略通过，判断当前是否已有未成交的限价单
	// 判断之前是否已有未成交的限价单
	// 直接获取当前交易对订单
	existOrders, err := s.getPositionOrders(option, broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 原始为空 止损为多  当前为多
	// 判断当前是否已有限价止损单
	if _, ok := existOrders[model.OrderTypeStop]; ok {
		stopLimitOrders, ok := existOrders[model.OrderTypeStop][model.OrderStatusTypeNew]
		if ok && len(stopLimitOrders) > 0 {
			for _, stopLimitOrder := range stopLimitOrders {
				// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
				if stopLimitOrder.Side != finalPosition {
					continue
				}
				// 取消之前的止损单
				err = broker.Cancel(*stopLimitOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
	// 判断当前是否已有仓位
	// 仓位类型双向持仓 下单时根据类型可下对冲单。通过协程提升平仓在开仓的效率
	// 有仓位时，判断当前持仓方向是否与策略相同
	// 开多单: side=BUY&positionSide=LONG
	// 平多单: side=SELL&positionSide=LONG
	// 开空单: side=SELL&positionSide=SHORT
	// 平空单: side=BUY&positionSide=SHORT
	if _, ok := existOrders[model.OrderTypeLimit]; ok {
		positionOrders, ok := existOrders[model.OrderTypeLimit][model.OrderStatusTypeFilled]
		if ok && len(positionOrders) > 0 {
			for _, positionOrder := range positionOrders {
				// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
				if positionOrder.Side == finalPosition {
					continue
				}
				if positionOrder.Side == model.SideTypeBuy {
					tempSideType = model.SideTypeSell
					postionSide = model.PositionSideTypeLong
				} else {
					tempSideType = model.SideTypeSell
					postionSide = model.PositionSideTypeShort
				}
				// 判断仓位方向为反方向，平掉现有仓位
				_, err := broker.CreateOrderStopMarket(tempSideType, postionSide, option.Pair, positionOrder.Quantity, currentPirce, positionOrder.OrderFlag)
				if err != nil {
					utils.Log.Error(err)
				}
			}
		}
	}

	// 根据分数动态计算仓位大小
	scoreRadio := float64(currentScore) / float64(totalScore)
	amount := s.calculateOpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPirce, scoreRadio)
	utils.Log.Infof(
		"[OPEN] Tendency: %s | Pair: %s | Price: %v | Quantity: %v | Strategy Count %d, %d Matchers: %s  | Side: %s | Total Score: %d | Final Score: %d",
		finalTendency,
		option.Pair,
		currentPirce,
		amount,
		len(s.strategy.Strategies),
		len(matchers),
		matchers,
		finalPosition,
		totalScore,
		currentScore,
	)
	// 重置当前交易对止损比例
	s.profitRatioLimit[option.Pair] = 0
	// 获取最新仓位positionSide
	if finalPosition == model.SideTypeBuy {
		postionSide = model.PositionSideTypeLong
	} else {
		postionSide = model.PositionSideTypeShort
	}
	// 根据最新价格创建限价单
	order, err := broker.CreateOrderLimit(finalPosition, postionSide, option.Pair, amount, currentPirce)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var stopLossPrice float64
	// 计算止损距离
	stopLossDistance := s.calculateStopLossDistance(s.initLossRatio, order.Price, float64(s.pairOptions[option.Pair].Leverage), amount)
	if finalPosition == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLossPrice = order.Price - stopLossDistance
	} else {
		tempSideType = model.SideTypeBuy
		stopLossPrice = order.Price + stopLossDistance
	}
	_, err = broker.CreateOrderStopLimit(tempSideType, postionSide, option.Pair, amount, stopLossPrice, order.OrderFlag)
	if err != nil {
		utils.Log.Error(err)
		return
	}
}

func (s *StrategyService) closeOption(option model.PairOption, broker reference.Broker) {
	if s.started == false {
		return
	}
	// TODO 考虑后续去掉这个网络检查 减少网络IO
	assetPosition, _, err := broker.Position(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if calc.Abs(assetPosition) == 0 {
		return
	}

	existOrderMap, err := s.getExistOrders(option, broker)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	var tempSideType model.SideType
	var stopLossDistance float64
	var stopLossPrice float64

	currentPirce := s.pairPrices[option.Pair]

	for orderFlag, existOrders := range existOrderMap {
		positionOrders, ok := existOrders["position"]
		if !ok {
			continue
		}
		for _, positionOrder := range positionOrders {
			// 已查询到止损单
			isProfitable := s.checkIsProfitable(currentPirce, positionOrder.Price, positionOrder.Side)
			// 判断是否盈利，盈利中则处理平仓及移动止损，反之则保持之前的止损单
			if isProfitable {
				// 判断当前利润比是否大于预设值
				// 如果利润比大于预设值，则使用计算出得利润比 - 指定步进的利润比 得到新的止损利润比
				profitRatio := s.calculateProfitRatio(positionOrder.Side, positionOrder.Price, currentPirce, float64(option.Leverage), positionOrder.Quantity)
				if profitRatio < s.initProfitRatioLimit || profitRatio <= s.profitRatioLimit[option.Pair]+s.profitableScale {
					utils.Log.Infof(
						"[WATCH] Pair: %s | Price Order: %v, Current: %v | Quantity: %v | Profit Ratio: %s",
						option.Pair,
						positionOrder.Price,
						currentPirce,
						positionOrder.Quantity,
						fmt.Sprintf("%.2f%%", profitRatio*100),
					)
					return
				}
				// 递增利润比
				s.profitRatioLimit[option.Pair] = profitRatio - s.profitableScale
				// 使用新的止损利润比计算止损点数
				stopLossDistance = s.calculateStopLossDistance(s.profitRatioLimit[option.Pair], positionOrder.Price, float64(option.Leverage), calc.Abs(assetPosition))
				// 重新计算止损价格
				if positionOrder.Side == model.SideTypeSell {
					stopLossPrice = positionOrder.Price - stopLossDistance
				} else {
					stopLossPrice = positionOrder.Price + stopLossDistance
				}
				utils.Log.Infof(
					"[GROW] Pair: %s | Price Order: %v, Current: %v | Quantity: %v | Profit Ratio: %s | Loss Price: %v, Ratio:%s",
					option.Pair,
					positionOrder.Price,
					currentPirce,
					positionOrder.Quantity,
					fmt.Sprintf("%.2f%%", profitRatio*100),
					stopLossPrice,
					fmt.Sprintf("%.2f%%", s.profitRatioLimit[option.Pair]*100),
				)
				if positionOrder.Side == model.SideTypeBuy {
					tempSideType = model.SideTypeSell
				} else {
					tempSideType = model.SideTypeBuy
				}
				// 设置新的止损单
				// 使用滚动利润比保证该止损利润是递增的
				// 不再判断新的止损价格是否小于之前的止损价格
				_, err := broker.CreateOrderStopLimit(tempSideType, positionOrder.PositionSide, option.Pair, calc.Abs(assetPosition), stopLossPrice, positionOrder.OrderFlag)
				if err != nil {
					utils.Log.Error(err)
				}

				if _, ok := existOrderMap[orderFlag]; !ok {
					continue
				}
				lossLimitOrders, ok := existOrderMap[orderFlag]["lossLimit"]
				if !ok {
					continue
				}
				for _, lossLimitOrder := range lossLimitOrders {
					// 取消之前的止损单
					err = broker.Cancel(*lossLimitOrder)
					if err != nil {
						utils.Log.Error(err)
						return
					}
				}
			}
		}
	}
}

func (s *StrategyService) getPositionOrders(option model.PairOption, broker reference.Broker) (map[model.OrderType]map[model.OrderStatusType][]*model.Order, error) {
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

func (s *StrategyService) getExistOrders(option model.PairOption, broker reference.Broker) (map[string]map[string][]*model.Order, error) {
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
			if order.Type == model.OrderTypeStop && order.Status == model.OrderStatusTypeNew {
				if _, ok := existOrders[order.OrderFlag]["lossLimit"]; !ok {
					existOrders[order.OrderFlag]["lossLimit"] = []*model.Order{}
				}
				existOrders[order.OrderFlag]["lossLimit"] = append(existOrders[order.OrderFlag]["lossLimit"], order)
			}
		}
	}
	return existOrders, nil
}

func (s *StrategyService) judgeStrategyForScore(matchers []types.StrategyPosition) (int, int, int, string, model.SideType) {
	// check side max
	var matcherCount int
	var totalScore int
	var currentScore int
	var finalPosition model.SideType
	var finalTendency string
	if len(matchers) == 0 {
		return matcherCount, totalScore, currentScore, finalTendency, finalPosition
	}

	// 如果没有匹配的策略位置，直接返回空方向
	for _, strategy := range s.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	if totalScore == 0 {
		return matcherCount, totalScore, currentScore, finalTendency, finalPosition
	}
	// 初始化计数器
	counts := map[model.SideType]int{model.SideTypeBuy: 0, model.SideTypeSell: 0}
	// 初始化趋势计数器
	tendencyCounts := make(map[string]int)
	// 统计各个方向的得分总和
	for _, pos := range matchers {
		tendencyCounts[pos.Tendency] += 1
		if pos.Useable == false {
			continue
		}
		counts[pos.Side] += pos.Score
		matcherCount++
	}

	initTendency := 0
	for tendency, tc := range tendencyCounts {
		if tc > initTendency {
			finalTendency = tendency
			initTendency = tc
		}
	}
	// 计算当前得分与总分的比例
	for side, score := range counts {
		// 比较当前得分与总分的比例
		scoreRatio := float64(score) / float64(totalScore)
		// 根据需要的逻辑进行判断，这里是示例逻辑
		if scoreRatio >= 0.5 {
			finalPosition = side // 如果当前得分占总分超过50%，选择这个方向
			currentScore = score
		}
	}
	// 如果没有策略得分比例超过一半，则选择得分最高的策略
	for _, pos := range matchers {
		if pos.Useable == false {
			continue
		}
		if pos.Score > currentScore {
			currentScore = pos.Score
			finalPosition = pos.Side
		}
	}

	return matcherCount, totalScore, currentScore, finalTendency, finalPosition
}

func (s *StrategyService) judgeStrategyForFrequency(matchers []types.StrategyPosition) model.SideType {
	// check side max
	var finalPosition model.SideType
	var maxCount int
	// 判断策略
	if len(matchers) <= len(s.strategy.Strategies)/2 {
		return finalPosition
	}
	// get side count
	counts := make(map[model.SideType]int)
	for _, pos := range matchers {
		counts[pos.Side]++
	}

	for side, count := range counts {
		if count > maxCount {
			finalPosition = side
			maxCount = count
		}
	}
	if maxCount == 0 || maxCount < len(matchers)/2 {
		return ""
	}
	return finalPosition
}

func (s *StrategyService) calculatePositionSize(balance, leverage, currentPrice float64) float64 {
	return (balance * leverage) / currentPrice
}

func (s *StrategyService) calculateOpenPositionSize(balance, leverage, currentPrice float64, scoreRadio float64) float64 {
	var amount float64
	fullPositionSize := (balance * leverage) / currentPrice
	if scoreRadio >= 0.5 {
		amount = fullPositionSize * s.fullSpaceRadio
	} else {
		if scoreRadio < 0.2 {
			amount = fullPositionSize * s.fullSpaceRadio * 0.4
		} else {
			amount = fullPositionSize * s.fullSpaceRadio * scoreRadio * 2
		}
	}
	return amount
}

func (s *StrategyService) calculateProfitRatio(side model.SideType, entryPrice float64, currentPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据当前价格计算利润
	var profit float64
	if side == model.SideTypeSell {
		profit = (entryPrice - currentPrice) * quantity
	} else {
		profit = (currentPrice - entryPrice) * quantity
	}

	// 计算利润比
	return profit / margin
}

func (s *StrategyService) calculateStopLossDistance(profitRatio float64, entryPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据保证金，利润比计算利润
	profit := profitRatio * margin
	// 根据利润 计算价差
	return calc.Abs(profit / quantity)
}

func (s *StrategyService) checkIsProfitable(currentPrice float64, positionPrice float64, side model.SideType) bool {
	// 判断是否盈利中
	isProfitable := false
	if side == model.SideTypeBuy && currentPrice > positionPrice {
		isProfitable = true
	} else if side == model.SideTypeSell && currentPrice < positionPrice {
		isProfitable = true
	}
	return isProfitable
}

func (s *StrategyService) checkIsRetracement(openPrice float64, currentPrice float64, side model.SideType) bool {
	// 判断是否盈利中
	isWithoutVolatility := false
	// 获取环比
	priceChange := (currentPrice - openPrice) / openPrice
	volatility := calc.Abs(priceChange) > s.volatilityThreshold
	if side == model.SideTypeBuy {
		if volatility && priceChange < 0 {
			isWithoutVolatility = true
		}
	}
	if side == model.SideTypeSell {
		if volatility && priceChange > 0 {
			isWithoutVolatility = true
		}
	}
	// 只有在盈利中且波动在合理范围内时，返回 true
	return isWithoutVolatility
}
