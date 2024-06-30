package handler

import (
	"errors"
	"floolisher/types"
	log "github.com/sirupsen/logrus"

	"floolisher/model"
	"floolisher/service"
)

type Controller struct {
	volatilityThreshold float64
	strategy            types.CompositesStrategy
	dataframes          map[string]*model.Dataframe
	broker              service.Broker
	started             bool
	currentOrders       map[string]model.Order
	currentLossOrders   map[string]model.Order
	initLossRatio       float64
	profitableScale     float64
	profitableRatio     float64
}

func NewStrategyController(pair string, strategy types.CompositesStrategy, broker service.Broker) *Controller {
	dataframes := map[string]*model.Dataframe{}
	for _, s := range strategy.Strategies {
		dataframes[s.Timeframe()] = &model.Dataframe{
			Pair:     pair,
			Metadata: make(map[string]model.Series[float64]),
		}
	}

	return &Controller{
		dataframes:          dataframes,
		strategy:            strategy,
		broker:              broker,
		volatilityThreshold: 0.002,
		currentOrders:       map[string]model.Order{},
		currentLossOrders:   map[string]model.Order{},
		initLossRatio:       0.5,
		profitableScale:     0.1,
		profitableRatio:     0.3,
	}
}

func (s *Controller) Start() {
	s.started = true
}

func (s *Controller) OnPartialCandle(candle model.Candle) {
	for _, strategy := range s.strategy.Strategies {
		if !candle.Complete && len(s.dataframes[strategy.Timeframe()].Close) >= strategy.WarmupPeriod() {
			if str, ok := strategy.(types.HighFrequencyStrategy); ok {
				s.updateDataFrame(strategy.Timeframe(), candle)
				str.Indicators(s.dataframes[strategy.Timeframe()])
				str.OnPartialCandle(s.dataframes[strategy.Timeframe()], s.broker)
			}
		}
	}
}

func (s *Controller) updateDataFrame(timeframe string, candle model.Candle) {
	if len(s.dataframes[timeframe].Time) > 0 && candle.Time.Equal(s.dataframes[timeframe].Time[len(s.dataframes[timeframe].Time)-1]) {
		last := len(s.dataframes[timeframe].Time) - 1
		s.dataframes[timeframe].Close[last] = candle.Close
		s.dataframes[timeframe].Open[last] = candle.Open
		s.dataframes[timeframe].High[last] = candle.High
		s.dataframes[timeframe].Low[last] = candle.Low
		s.dataframes[timeframe].Volume[last] = candle.Volume
		s.dataframes[timeframe].Time[last] = candle.Time
		for k, v := range candle.Metadata {
			s.dataframes[timeframe].Metadata[k][last] = v
		}
	} else {
		s.dataframes[timeframe].Close = append(s.dataframes[timeframe].Close, candle.Close)
		s.dataframes[timeframe].Open = append(s.dataframes[timeframe].Open, candle.Open)
		s.dataframes[timeframe].High = append(s.dataframes[timeframe].High, candle.High)
		s.dataframes[timeframe].Low = append(s.dataframes[timeframe].Low, candle.Low)
		s.dataframes[timeframe].Volume = append(s.dataframes[timeframe].Volume, candle.Volume)
		s.dataframes[timeframe].Time = append(s.dataframes[timeframe].Time, candle.Time)
		s.dataframes[timeframe].LastUpdate = candle.Time
		for k, v := range candle.Metadata {
			s.dataframes[timeframe].Metadata[k] = append(s.dataframes[timeframe].Metadata[k], v)
		}
	}
}

func (s *Controller) OnCandle(timeframe string, candle model.Candle) {
	if len(s.dataframes[timeframe].Time) > 0 && candle.Time.Before(s.dataframes[timeframe].Time[len(s.dataframes[timeframe].Time)-1]) {
		log.Errorf("late candle received: %#v", candle)
		return
	}

	s.updateDataFrame(timeframe, candle)
	for _, strategy := range s.strategy.Strategies {
		if len(s.dataframes[timeframe].Close) >= strategy.WarmupPeriod() {
			sample := s.dataframes[timeframe].Sample(strategy.WarmupPeriod())
			strategy.Indicators(&sample)
			if s.started {
				strategy.OnCandle(&sample)
			}
		}
	}
	if s.started {
		s.strategy.OnDecision(s.dataframes[timeframe], s.broker, s.OnDecision)
	}
}

func (s *Controller) OnDecision(matchers []types.StrategyPosition, df *model.Dataframe, broker service.Broker) {
	assetPosition, quotePosition, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}
	openPrice := df.Open.Last(0)
	closePrice := df.Close.Last(0)

	if s.abs(assetPosition) > 0 {
		s.OnWatchPosition(assetPosition, openPrice, closePrice, df, broker)
	} else {
		s.OnOpenPosition(matchers, quotePosition, closePrice, df, broker)
	}
}

func (s *Controller) OnWatchPosition(
	assetPosition float64,
	openPrice float64,
	closePrice float64,
	df *model.Dataframe,
	broker service.Broker,
) {
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
	// 查询当前已挂订单
	currentOrder, ok := s.currentOrders[df.Pair]
	if ok == false || currentOrder.ID == 0 {
		log.Error(errors.New("unkown order for this pair"))
		return
	}
	// 查询止损单
	currentLossOrder, ok := s.currentLossOrders[df.Pair]
	if ok == false || currentLossOrder.ID == 0 {
		// 未查询到止损单时，重设止损单
		// todo 根据配置获取当前交易对杠杆倍数
		stopLossDistance := s.calculateStopLossDistancee(s.initLossRatio, currentOrder.Price, 100, s.abs(assetPosition))
		if assetPosition > 0 {
			stopLossPrice = currentOrder.Price - stopLossDistance
		} else {
			stopLossPrice = currentOrder.Price + stopLossDistance
		}
		lossOrder, err := broker.CreateOrderStopLimit(tempSideType, df.Pair, s.abs(assetPosition), stopLossPrice)
		if err != nil {
			log.Error(err)
		}
		s.currentLossOrders[df.Pair] = lossOrder
	} else {
		// 已查询到止损单
		isProfitable, isWithoutVolatility := s.checkIsRetracement(openPrice, closePrice, currentOrder.Price, currSideType)
		// 判断是否盈利，盈利中则处理平仓及移动止损，反之则保持之前的止损单
		if isProfitable {
			// 利润大幅回撤时，市价平仓
			if isWithoutVolatility {
				// 取消之前的止损单
				err := broker.Cancel(currentLossOrder)
				if err != nil {
					log.Error(err)
					return
				}
				delete(s.currentLossOrders, df.Pair)
				// 市价平仓
				_, err = broker.CreateOrderStop(df.Pair, s.abs(assetPosition), closePrice)
				if err != nil {
					log.Error(err)
				}
			} else {
				// l利润未大幅回撤，判断当前利润比是否大于预设值
				// todo 根据配置获取当前交易对杠杆倍数
				profitRatio := s.calculateProfitRatio(currentOrder.Price, closePrice, 100, currentOrder.Quantity)
				if profitRatio < s.profitableRatio {
					return
				}
				// todo 根据配置获取当前交易对杠杆倍数
				stopLossDistance = s.calculateStopLossDistancee(s.profitableRatio-s.profitableScale, currentOrder.Price, 100, s.abs(assetPosition))
				// 递增利润比
				s.profitableRatio = s.profitableRatio + s.profitableScale
				// 重新计算止损价格
				if currSideType == model.SideTypeSell {
					stopLossPrice = currentOrder.Price - stopLossDistance
				} else {
					stopLossPrice = currentOrder.Price + stopLossDistance
				}
				// 如果止损单是BUY，判断新的止损价格是否小于之前的止损价格
				if tempSideType == model.SideTypeBuy && stopLossPrice < currentLossOrder.Price {
					// 取消之前的止损单
					err := broker.Cancel(currentLossOrder)
					if err != nil {
						log.Error(err)
						return
					}
					// 设置新的止损单
					lossOrder, err := broker.CreateOrderStopLimit(tempSideType, df.Pair, s.abs(assetPosition), stopLossPrice)
					if err != nil {
						log.Error(err)
					}
					s.currentLossOrders[df.Pair] = lossOrder
				}
				// 如果止损单是SELL，判断新的止损价格是否小于之前的止损价格
				if tempSideType == model.SideTypeBuy && stopLossPrice > currentLossOrder.Price {
					// 取消之前的止损单
					err := broker.Cancel(currentLossOrder)
					if err != nil {
						log.Error(err)
						return
					}
					// 设置新的止损单
					lossOrder, err := broker.CreateOrderStopLimit(tempSideType, df.Pair, s.abs(assetPosition), stopLossPrice)
					if err != nil {
						log.Error(err)
					}
					s.currentLossOrders[df.Pair] = lossOrder
				}
			}
		}
	}
}

func (s *Controller) OnOpenPosition(
	matchers []types.StrategyPosition,
	quotePosition float64,
	closePrice float64,
	df *model.Dataframe,
	broker service.Broker,
) {
	// 无资产
	if quotePosition <= 0 {
		return
	}
	finalPosition := s.judgeStrategyForScore(matchers)
	if len(finalPosition) == 0 {
		log.Infof("Pair: %s has %d strategy, Got %d matchers", df.Pair, len(s.strategy.Strategies), len(matchers))
		return
	}
	log.Infof("Pair: %s has %d strategy, Got %d matchers: %s, Most Side: %s", df.Pair, len(s.strategy.Strategies), len(matchers), matchers, finalPosition)

	amount := s.calculatePositionSize(quotePosition, 100, closePrice) * 0.08
	// 挂单
	order, err := broker.CreateOrderMarket(finalPosition, df.Pair, amount)
	if err != nil {
		log.Error(err)
		return
	}
	s.currentOrders[df.Pair] = order
	// 设置止损订单
	// 获取杠杆倍数
	var tempSideType model.SideType
	// TODO 杠杆倍数根据配置决定
	stopLossDistance := s.calculateStopLossDistancee(s.initLossRatio, order.Price, 100, amount)
	var stopLossPrice float64
	if finalPosition == model.SideTypeBuy {
		tempSideType = model.SideTypeSell
		stopLossPrice = order.Price - stopLossDistance
	} else {
		tempSideType = model.SideTypeBuy
		stopLossPrice = order.Price + stopLossDistance
	}
	lossOrder, err := broker.CreateOrderStopLimit(tempSideType, df.Pair, amount, stopLossPrice)
	if err != nil {
		log.Error(err)
		return
	}
	s.currentLossOrders[df.Pair] = lossOrder
}

func (s *Controller) judgeStrategyForScore(matchers []types.StrategyPosition) model.SideType {
	// check side max
	var finalPosition model.SideType
	if len(matchers) == 0 {
		return finalPosition
	}
	var totalScore int
	// 如果没有匹配的策略位置，直接返回空方向
	for _, strategy := range s.strategy.Strategies {
		totalScore += strategy.SortScore()
	}
	if totalScore == 0 {
		return finalPosition
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
		}
	}
	// 如果没有策略得分比例超过一半，则选择得分最高的策略
	var highestScore int
	for _, pos := range matchers {
		if pos.Score > highestScore {
			highestScore = pos.Score
			finalPosition = pos.Side
		}
	}

	return finalPosition
}

func (s *Controller) judgeStrategyForFrequency(matchers []types.StrategyPosition) model.SideType {
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

func (s *Controller) calculatePositionSize(balance, leverage, currentPrice float64) float64 {
	return (balance * leverage) / currentPrice
}

func (s *Controller) calculateProfitRatio(entryPrice float64, currentPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据当前价格计算利润
	profit := (currentPrice - entryPrice) * quantity
	// 计算利润比
	return profit / margin
}

// profitRatio = profile/margin
// profitRatio*margin = profile
// profile/quantity = (currentPrice - entryPrice)
func (s *Controller) calculateStopLossDistancee(profitRatio float64, entryPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据保证金，利润比计算利润
	profit := profitRatio * margin
	// 根据利润 计算价差
	return s.abs(profit / quantity)
}

func (s *Controller) abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func (s *Controller) checkIsRetracement(openPrice float64, currentPrice float64, positionPrice float64, side model.SideType) (bool, bool) {
	// 判断是否盈利中
	isProfitable := false
	isWithoutVolatility := false
	if side == model.SideTypeBuy && currentPrice > positionPrice {
		isProfitable = true
	} else if side == model.SideTypeSell && currentPrice < positionPrice {
		isProfitable = true
	}
	// 获取环比
	priceChange := (currentPrice - openPrice) / openPrice
	volatility := s.abs(priceChange) > s.volatilityThreshold
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
	return isProfitable, isWithoutVolatility
}
