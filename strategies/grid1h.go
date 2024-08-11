package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Grid1h struct {
	BaseStrategy
}

func (s Grid1h) SortScore() int {
	return 90
}

func (s Grid1h) Timeframe() string {
	return "1h"
}

func (s Grid1h) WarmupPeriod() int {
	return 36 // RSI的预热期设定为14个数据点
}

func (s Grid1h) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
	df.Metadata["bb_width"] = bbWidth

	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["ema7"] = indicator.EMA(df.Close, 7)
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
