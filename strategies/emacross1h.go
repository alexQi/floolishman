package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Emacross1h struct {
	BaseStrategy
}

func (s Emacross1h) SortScore() float64 {
	return 60
}

func (s Emacross1h) Timeframe() string {
	return "1h"
}

func (s Emacross1h) WarmupPeriod() int {
	return 36
}

func (s Emacross1h) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Emacross1h) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	ema8 := df.Metadata["ema8"]
	ema21 := df.Metadata["ema21"]
	avgVolume := df.Metadata["avgVolume"].Last(1)
	volume := df.Metadata["volume"].Last(0)

	// 判断量价关系
	if strategyPosition.Tendency == "rise" && ema8.Crossover(ema21) && volume > avgVolume*2 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}

	if strategyPosition.Tendency == "down" && ema8.Crossunder(ema21) && volume > avgVolume*2 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}

	return strategyPosition
}
