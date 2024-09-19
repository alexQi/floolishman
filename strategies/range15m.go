package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
	"reflect"
)

type Range15m struct {
	BaseStrategy
}

func (s Range15m) SortScore() float64 {
	return 65
}

func (s Range15m) Timeframe() string {
	return "15m"
}

func (s Range15m) WarmupPeriod() int {
	return 90
}

func (s Range15m) Indicators(df *model.Dataframe) {

	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}

	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Range15m) OnCandle(df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	bbMiddle := df.Metadata["bbMiddle"].Last(0)
	bbWidth := df.Metadata["bbWidth"].Last(0)
	volume := df.Metadata["volume"].Last(0)
	avgVolume := df.Metadata["avgVolume"].Last(0)
	momentums := df.Metadata["momentum"].LastValues(2)
	rsi := df.Metadata["rsi"].Last(1)

	bbWaveDistance := bbWidth * 0.05
	currentPrice := df.Close.Last(0)
	momentumsDistance := momentums[1] - momentums[0]
	momentumsAvg := calc.Abs(momentums[1]+momentums[0]) / 2

	if strategyPosition.Tendency == "range" && calc.Abs(momentumsDistance) < 10 && momentumsAvg < 10 {
		if rsi > 35 && currentPrice < (bbMiddle-bbWaveDistance*6) && volume < avgVolume*2 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
		}

		if rsi < 65 && currentPrice > (bbMiddle+bbWaveDistance*6) && volume < avgVolume*2 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
		}
	}

	return strategyPosition
}
