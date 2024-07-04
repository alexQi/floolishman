package service

import (
	"context"
	"floolishman/model"
	"floolishman/process"
	"floolishman/reference"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/calc"
	"reflect"
	"sync"
)

type StrategyService struct {
	ctx                 context.Context
	volatilityThreshold float64
	strategy            types.CompositesStrategy
	dataframes          map[string]map[string]*model.Dataframe
	samples             map[string]map[string]map[string]*model.Dataframe
	realCandles         map[string]map[string]*model.Candle
	pairPrices          map[string]float64
	pairOptions         map[string]model.PairOption
	broker              reference.Broker
	started             bool
	fullSpaceRadio      float64
	initLossRatio       float64
	profitableScale     float64
	profitableRatio     float64
	mu                  sync.Mutex
}

func NewStrategyService(ctx context.Context, strategy types.CompositesStrategy, broker reference.Broker) *StrategyService {
	return &StrategyService{
		ctx:                 ctx,
		dataframes:          make(map[string]map[string]*model.Dataframe),
		samples:             make(map[string]map[string]map[string]*model.Dataframe),
		realCandles:         make(map[string]map[string]*model.Candle),
		pairPrices:          make(map[string]float64),
		pairOptions:         make(map[string]model.PairOption),
		strategy:            strategy,
		broker:              broker,
		volatilityThreshold: 0.002,
		fullSpaceRadio:      0.1,
		initLossRatio:       0.45,
		profitableScale:     0.1,
		profitableRatio:     0.25,
	}
}

func (s *StrategyService) Start() {
	go process.CheckOpenPoistion(s.pairOptions, s.broker, s.openPosition)
	go process.CheckClosePoistion(s.pairOptions, s.broker, s.closeOption)
	s.started = true
}

func (s *StrategyService) SetPairDataframe(option model.PairOption) {
	s.pairOptions[option.Pair] = option
	s.pairPrices[option.Pair] = 0
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
	if ok && oldCandle.Time.Before(candle.Time) == false {
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
	// 有仓位时，不再开仓
	if calc.Abs(assetPosition) > 0 {
		return
	}
	// 无资产
	if quotePosition <= 0 {
		return
	}
	matchers := s.strategy.CallMatchers(s.realCandles[option.Pair], s.samples[option.Pair])
	// 判断策略结果
	totalScore, currentScore, finalPosition := s.judgeStrategyForScore(matchers)
	if totalScore == 0 {
		utils.Log.Infof("[WAIT] Pair: %s Count %d | %d Matchers | Price: %v", option.Pair, len(s.strategy.Strategies), len(matchers), s.pairPrices[option.Pair])
		return
	}
	utils.Log.Infof("[OPEN] Pair: %s Count %d | %d Matchers %s | Price: %v | Side: %s | Total Score: %d | Final Score: %d", option.Pair, len(s.strategy.Strategies), len(matchers), matchers, s.pairPrices[option.Pair], finalPosition, totalScore, currentScore)

	// 根据分数动态计算仓位大小
	scoreRadio := float64(currentScore / totalScore)
	amount := s.calculateOpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), s.pairPrices[option.Pair], scoreRadio)
	utils.Log.Infof("Position size: %v, Pair:%s", amount, option.Pair)
	// 根据最新价格创建限价单
	order, err := broker.CreateOrderLimit(finalPosition, option.Pair, amount, s.pairPrices[option.Pair])
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var tempSideType model.SideType
	stopLossDistance := s.calculateStopLossDistancee(s.initLossRatio, order.Price, float64(s.pairOptions[option.Pair].Leverage), amount)
	var stopLossPrice float64
	if finalPosition == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLossPrice = order.Price - stopLossDistance
	} else {
		tempSideType = model.SideTypeBuy
		stopLossPrice = order.Price + stopLossDistance
	}
	_, err = broker.CreateOrderStopLimit(tempSideType, option.Pair, amount, stopLossPrice)
	if err != nil {
		utils.Log.Error(err)
		return
	}
}

func (s *StrategyService) closeOption(option model.PairOption, broker reference.Broker) {
	if s.started == false {
		return
	}
	assetPosition, _, err := broker.Position(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if calc.Abs(assetPosition) == 0 {
		return
	}
	positionOrders, err := broker.GetCurrentPositionOrders(option.Pair)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	if len(positionOrders) == 0 {
		utils.Log.Errorf("Pair %s has no Position and Order", option.Pair)
	}

	currentOrder := model.Order{}
	currentLossOrder := model.Order{}
	for _, order := range positionOrders {
		if order.Type == model.OrderTypeLimit {
			currentOrder = *order
		}
		if order.Type == model.OrderTypeStop {
			currentLossOrder = *order
		}
	}
	if currentOrder.ExchangeID == 0 {
		utils.Log.Errorf("No order for this pair: %s", option.Pair)
		return
	}

	// 定义基础信息
	var currSideType model.SideType
	var tempSideType model.SideType
	var stopLossDistance float64
	var stopLossPrice float64

	// 获取仓位方向，计算移动止损价格
	if assetPosition > 0 {
		currSideType = model.SideTypeBuy
		tempSideType = model.SideTypeSell
	} else {
		currSideType = model.SideTypeSell
		tempSideType = model.SideTypeBuy
	}
	if currentLossOrder.ExchangeID == 0 {
		// 未查询到止损单时，重设止损单
		stopLossDistance := s.calculateStopLossDistancee(s.initLossRatio, currentOrder.Price, float64(option.Leverage), calc.Abs(assetPosition))
		if assetPosition > 0 {
			stopLossPrice = currentOrder.Price - stopLossDistance
		} else {
			stopLossPrice = currentOrder.Price + stopLossDistance
		}
		_, err := broker.CreateOrderStopLimit(tempSideType, option.Pair, calc.Abs(assetPosition), stopLossPrice)
		if err != nil {
			utils.Log.Error(err)
		}
	} else {
		// 已查询到止损单
		isProfitable := s.checkIsProfitable(s.pairPrices[option.Pair], currentOrder.Price, currSideType)
		// 判断是否盈利，盈利中则处理平仓及移动止损，反之则保持之前的止损单
		if isProfitable {
			// 判断当前利润比是否大于预设值
			profitRatio := s.calculateProfitRatio(currentOrder.Price, s.pairPrices[option.Pair], float64(option.Leverage), currentOrder.Quantity)
			if profitRatio < s.profitableRatio {
				utils.Log.Infof("Pair: %s is profiting, ratio: %v, loss ratio: %v", option.Pair, profitRatio, s.profitableRatio)
				return
			}
			stopLossDistance = s.calculateStopLossDistancee(s.profitableRatio-s.profitableScale, currentOrder.Price, float64(option.Leverage), calc.Abs(assetPosition))
			// 递增利润比
			s.profitableRatio = s.profitableRatio + s.profitableScale
			// 重新计算止损价格
			if currSideType == model.SideTypeSell {
				stopLossPrice = currentOrder.Price - stopLossDistance
			} else {
				stopLossPrice = currentOrder.Price + stopLossDistance
			}
			utils.Log.Infof("Pair: %s profit is growing, ratio: %v, new loss ratio: %v, new loss price: %v", option.Pair, profitRatio, s.profitableRatio, stopLossPrice)

			// 如果止损单是BUY，判断新的止损价格是否小于之前的止损价格
			if tempSideType == model.SideTypeBuy && stopLossPrice < currentLossOrder.Price {
				// 设置新的止损单
				_, err := broker.CreateOrderStopLimit(tempSideType, option.Pair, calc.Abs(assetPosition), stopLossPrice)
				if err != nil {
					utils.Log.Error(err)
				}
				// 取消之前的止损单
				err = broker.Cancel(currentLossOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
			// 如果止损单是SELL，判断新的止损价格是否小于之前的止损价格
			if tempSideType == model.SideTypeBuy && stopLossPrice > currentLossOrder.Price {
				// 设置新的止损单
				_, err := broker.CreateOrderStopLimit(tempSideType, option.Pair, calc.Abs(assetPosition), stopLossPrice)
				if err != nil {
					utils.Log.Error(err)
				}
				// 取消之前的止损单
				err = broker.Cancel(currentLossOrder)
				if err != nil {
					utils.Log.Error(err)
					return
				}
			}
		}
	}
}

func (s *StrategyService) judgeStrategyForScore(matchers []types.StrategyPosition) (int, int, model.SideType) {
	// check side max
	var totalScore int
	var currentScore int
	var finalPosition model.SideType
	if len(matchers) == 0 {
		return totalScore, currentScore, finalPosition
	}
	// 如果没有匹配的策略位置，直接返回空方向
	for _, strategy := range s.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	if totalScore == 0 {
		return totalScore, currentScore, finalPosition
	}
	// 初始化计数器
	counts := map[model.SideType]int{model.SideTypeBuy: 0, model.SideTypeSell: 0}
	// 统计各个方向的得分总和
	for _, pos := range matchers {
		counts[pos.Side] += pos.Score
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
		if pos.Score > currentScore {
			currentScore = pos.Score
			finalPosition = pos.Side
		}
	}

	return totalScore, currentScore, finalPosition
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

func (s *StrategyService) calculateProfitRatio(entryPrice float64, currentPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据当前价格计算利润
	profit := (currentPrice - entryPrice) * quantity
	// 计算利润比
	return profit / margin
}

func (s *StrategyService) calculateStopLossDistancee(profitRatio float64, entryPrice float64, leverage float64, quantity float64) float64 {
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
