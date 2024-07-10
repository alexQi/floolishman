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
	CheckMode            string
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
	backtest             bool
	checkMode            string
	volatilityThreshold  float64
	fullSpaceRadio       float64
	initLossRatio        float64
	profitableScale      float64
	initProfitRatioLimit float64
	profitRatioLimit     map[string]float64
	mu                   sync.Mutex
}

func NewStrategyService(
	ctx context.Context,
	tradingSetting StrategyServiceSetting,
	strategy types.CompositesStrategy,
	broker reference.Broker,
	backtest bool,
) *StrategyService {
	return &StrategyService{
		ctx:                  ctx,
		dataframes:           make(map[string]map[string]*model.Dataframe),
		samples:              make(map[string]map[string]map[string]*model.Dataframe),
		realCandles:          make(map[string]map[string]*model.Candle),
		pairPrices:           make(map[string]float64),
		pairOptions:          make(map[string]model.PairOption),
		strategy:             strategy,
		broker:               broker,
		backtest:             backtest,
		checkMode:            tradingSetting.CheckMode,
		volatilityThreshold:  tradingSetting.VolatilityThreshold,
		fullSpaceRadio:       tradingSetting.FullSpaceRadio,
		initLossRatio:        tradingSetting.InitLossRatio,
		profitableScale:      tradingSetting.ProfitableScale,
		initProfitRatioLimit: tradingSetting.InitProfitRatioLimit,
		profitRatioLimit:     make(map[string]float64),
	}
}

func (s *StrategyService) Start() {
	// 是否定时检查蜡烛
	if s.checkMode == "interval" {
		go process.CheckOpenPoistion(s.pairOptions, s.broker, s.openPosition)
	}
	if s.backtest == false {
		go process.CheckClosePoistion(s.pairOptions, s.broker, s.closeOption)
	}
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

func (s *StrategyService) setDataFrame(dataframe model.Dataframe, candle model.Candle) model.Dataframe {
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

func (s *StrategyService) updateDataFrame(timeframe string, candle model.Candle) {
	tempDataframe := s.setDataFrame(*s.dataframes[candle.Pair][timeframe], candle)
	s.dataframes[candle.Pair][timeframe] = &tempDataframe
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

func (s *StrategyService) OnCandle(timeframe string, candle model.Candle) {
	if len(s.dataframes[candle.Pair][timeframe].Time) > 0 && candle.Time.Before(s.dataframes[candle.Pair][timeframe].Time[len(s.dataframes[candle.Pair][timeframe].Time)-1]) {
		utils.Log.Errorf("late candle received: %#v", candle)
		return
	}
	// 更新Dataframe
	s.updateDataFrame(timeframe, candle)
	s.OnRealCandle(timeframe, candle)
	// 如果是蜡烛结束的时候检查
	if s.checkMode == "candle" {
		s.openPosition(s.pairOptions[candle.Pair], s.broker)
	}
	if s.backtest {
		s.closeOption(s.pairOptions[candle.Pair], s.broker)
	}
}

// openPosition 开仓方法
func (s *StrategyService) openPosition(option model.PairOption, broker reference.Broker) {
	if !s.started {
		return
	}
	if _, ok := s.realCandles[option.Pair]; !ok {
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
	matchers := s.strategy.CallMatchers(s.samples[option.Pair])
	// 判断策略结果
	currentPirce := s.pairPrices[option.Pair]
	totalScore, currentScore, finalTendency, finalPosition, currentMatchers := s.judgeStrategyForScore(matchers)
	if s.backtest == false {
		utils.Log.Infof(
			"[JUDGE] Tendency: %s | Pair: %s | Price: %v | Side: %s | Total Score: %d | Final Score: %d | Matchers:【%s】",
			finalTendency,
			option.Pair,
			currentPirce,
			finalPosition,
			totalScore,
			currentScore,
			currentMatchers,
		)
	}
	if currentScore == 0 {
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
	holdedOrder := model.Order{}
	if _, ok := existOrders[model.OrderTypeLimit]; ok {
		// 判断是否已有仓位
		positionOrders, ok := existOrders[model.OrderTypeLimit][model.OrderStatusTypeFilled]
		if ok && len(positionOrders) > 0 {
			for _, positionOrder := range positionOrders {
				// 原始止损单方向和当前策略判断方向相同，则取消原始止损单
				if positionOrder.Side == finalPosition {
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
				// 当前分数小于仓位分数，不要在开仓
				if currentScore < positionOrder.Score {
					holdedOrder = *positionOrder
					continue
				}
				// 判断仓位方向为反方向，平掉现有仓位
				_, err := broker.CreateOrderStopMarket(tempSideType, postionSide, option.Pair, positionOrder.Quantity, currentPirce, positionOrder.OrderFlag, positionOrder.Score, positionOrder.Strategy)
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
					// 当前分数小于仓位分数，不要在开仓
					if currentScore < positionNewOrder.Score {
						holdedOrder = *positionNewOrder
						continue
					}
					// 取消之前的限价单
					err = broker.Cancel(*positionNewOrder)
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

	// 根据分数动态计算仓位大小
	scoreRadio := float64(currentScore) / float64(totalScore)
	amount := s.calculateOpenPositionSize(quotePosition, float64(s.pairOptions[option.Pair].Leverage), currentPirce, scoreRadio)
	if s.backtest == false {
		utils.Log.Infof(
			"[OPEN] Tendency: %s | Pair: %s | Price: %v | Quantity: %v | Side: %s",
			finalTendency,
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
	order, err := broker.CreateOrderLimit(finalPosition, postionSide, option.Pair, amount, currentPirce, currentScore, currentMatchers[0].StrategyName)
	if err != nil {
		utils.Log.Error(err)
		return
	}
	// 设置止损订单
	var stopLimitPrice float64
	var stopTrigerPrice float64
	// 计算止损距离
	stopLossDistance := s.calculateStopLossDistance(s.initLossRatio, order.Price, float64(s.pairOptions[option.Pair].Leverage), amount)
	if finalPosition == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLimitPrice = order.Price - stopLossDistance
		stopTrigerPrice = order.Price - stopLossDistance*0.9
	} else {
		tempSideType = model.SideTypeBuy
		stopLimitPrice = order.Price + stopLossDistance
		stopTrigerPrice = order.Price + stopLossDistance*0.9
	}
	_, err = broker.CreateOrderStopLimit(tempSideType, postionSide, option.Pair, amount, stopLimitPrice, stopTrigerPrice, order.OrderFlag, currentScore, order.Strategy)
	if err != nil {
		utils.Log.Error(err)
		return
	}
}

func (s *StrategyService) closeOption(option model.PairOption, broker reference.Broker) {
	if s.started == false {
		return
	}
	existOrderMap, err := s.getExistOrders(option, broker)
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
			profitRatio := s.calculateProfitRatio(positionOrder.Side, positionOrder.Price, currentPirce, float64(option.Leverage), positionOrder.Quantity)
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
			if profitRatio < s.initProfitRatioLimit || profitRatio <= s.profitRatioLimit[option.Pair]+s.profitableScale {
				return
			}
			// 递增利润比
			s.profitRatioLimit[option.Pair] = profitRatio - s.profitableScale
			// 使用新的止损利润比计算止损点数
			stopLossDistance = s.calculateStopLossDistance(s.profitRatioLimit[option.Pair], positionOrder.Price, float64(option.Leverage), positionOrder.Quantity)
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
					fmt.Sprintf("%.2f%%", s.profitRatioLimit[option.Pair]*100),
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
			_, err := broker.CreateOrderStopMarket(tempSideType, positionOrder.PositionSide, option.Pair, positionOrder.Quantity, stopLimitPrice, positionOrder.OrderFlag, positionOrder.Score, positionOrder.Strategy)
			if err != nil {
				utils.Log.Error(err)
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

func (s *StrategyService) judgeStrategyForScore(matchers []types.StrategyPosition) (int, int, string, model.SideType, []types.StrategyPosition) {
	// check side max
	var totalScore int
	var currentScore int
	var finalPosition model.SideType
	var finalTendency string

	currentMatchers := []types.StrategyPosition{}
	if len(matchers) == 0 {
		return totalScore, currentScore, finalTendency, finalPosition, currentMatchers
	}

	// 如果没有匹配的策略位置，直接返回空方向
	for _, strategy := range s.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	if totalScore == 0 {
		return totalScore, currentScore, finalTendency, finalPosition, currentMatchers
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
		currentMatchers = append(currentMatchers, pos)
		counts[pos.Side] += pos.Score
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

	return totalScore, currentScore, finalTendency, finalPosition, currentMatchers
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
	if profit == 0 {
		return 0
	}
	return calc.Abs(profit / quantity)
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
