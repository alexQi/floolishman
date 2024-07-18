package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
	"reflect"
)

type Range15m struct {
	BaseStrategy
}

func (s Range15m) SortScore() int {
	return 65
}

func (s Range15m) Timeframe() string {
	return "15m"
}

func (s Range15m) WarmupPeriod() int {
	return 90
}

func (s Range15m) Indicators(df *model.Dataframe) {

	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}

	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
	df.Metadata["bb_width"] = bbWidth
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
}

func (s *Range15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	bbMiddle := df.Metadata["bb_middle"].Last(0)
	bbWidth := df.Metadata["bb_width"].Last(0)
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
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeBuy
		}

		if rsi < 65 && currentPrice > (bbMiddle+bbWaveDistance*6) && volume < avgVolume*2 {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeSell
		}
	}

	return strategyPosition
}
