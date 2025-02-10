package strategies

import (
	"encoding/json"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"math"
	"reflect"
)

type Test struct {
	BaseStrategy
}

// SortScore
func (s Test) SortScore() float64 {
	return 90
}

// Timeframe
func (s Test) Timeframe() string {
	return "30m"
}

func (s Test) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Test) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth
	// 检查插针
	upperPinRates, lowerPinRates, upperShadows, lowerShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowerPinRates"] = lowerPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowerShadows"] = lowerShadows
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 8, 17, 5)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	// 其他指标
	df.Metadata["avgVolume"] = indicator.EMA(df.Volume, 7)
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Test) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	feature := s.extraFeatures(df)
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    feature.LastPrice,
	}
	var decayFactorFloor, decayFactorAmplitude, decayFactorDistance, datum, shadowK float64

	floorK := 1.077993
	distanceK := 1.445
	amplitudeK := 1.1601
	deltaRsiRatio := 0.1
	baseFloor := 10.0
	baseDistanceRate := 0.275

	if feature.PrevRsi > feature.LastRsi {
		datum = 1.2
		shadowK = 0.4405

		strategyPosition.Side = string(model.SideTypeSell)

		decayFactorFloor = calc.CalculateFactor(feature.PrevRsiExtreme, floorK)
		decayFactorAmplitude = calc.CalculateFactor(feature.PrevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(feature.PrevRsiExtreme, distanceK-decayFactorAmplitude)

		feature.RsiFloor = decayFactorFloor * baseFloor
		feature.RsiUpper = math.Exp(floorK*deltaRsiRatio) * feature.RsiFloor
		feature.DistanceRate = decayFactorDistance * baseDistanceRate
		feature.LimitShadowChangeRate = calc.CalculateRate(feature.PrevAmplitude*feature.PrevRsiExtreme, datum, shadowK)

		if calc.Abs(feature.LastRsiDiff) > feature.RsiFloor && calc.Abs(feature.LastRsiDiff) < feature.RsiUpper {
			strategyPosition.Useable = 1
			strategyPosition.Score = 100 * feature.PrevRsiExtreme
		}
	} else {
		datum = 2.6
		shadowK = 0.26

		strategyPosition.Side = string(model.SideTypeBuy)

		decayFactorFloor = calc.CalculateFactor(feature.PrevRsiExtreme, floorK)
		decayFactorAmplitude = calc.CalculateFactor(feature.PrevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(feature.PrevRsiExtreme, distanceK-decayFactorAmplitude)

		feature.RsiFloor = decayFactorFloor * baseFloor
		feature.RsiUpper = math.Exp(floorK*deltaRsiRatio) * feature.RsiFloor
		feature.DistanceRate = decayFactorDistance * baseDistanceRate
		feature.LimitShadowChangeRate = calc.CalculateRate(feature.PrevAmplitude*feature.PrevRsiExtreme, datum, shadowK)

		if feature.LastRsiDiff > feature.RsiFloor && feature.LastRsiDiff < feature.RsiUpper {
			strategyPosition.Useable = 1
			strategyPosition.Score = 100 * feature.PrevRsiExtreme
		}
	}

	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(feature.DistanceRate, strategyPosition.OpenPrice, float64(option.Leverage))
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		feature.PositionSide = strategyPosition.Side
		feature.OpenPrice = strategyPosition.OpenPrice
		// 将 map 转换为 JSON 字符串
		openParamsBytes, err := json.Marshal(feature)
		if err != nil {
			utils.Log.Error("错误：", err)
		}
		strategyPosition.OpenParams = string(openParamsBytes)
		utils.Log.Tracef("[PARAMS] %s", strategyPosition.OpenParams)
	}

	return strategyPosition
}
