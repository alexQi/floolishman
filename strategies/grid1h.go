package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Grid1h struct {
	BaseStrategy
}

func (s Grid1h) SortScore() float64 {
	return 90.0
}

func (s Grid1h) Timeframe() string {
	return "1h"
}

func (s Grid1h) WarmupPeriod() int {
	return 128 // RSI的预热期设定为14个数据点
}

func (s Grid1h) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}

	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth
	df.Metadata["volume"] = df.Volume

	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 7)
	df.Metadata["basePrice"] = indicator.EMA(df.Close, 5)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
	df.Metadata["discrepancy"] = indicator.Discrepancy(bbMiddle, 2)
}

func (s *Grid1h) OnCandle(df *model.Dataframe) model.Strategy {
	return model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
}
