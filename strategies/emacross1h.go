package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Emacross1h struct {
	BaseStrategy
}

func (s Emacross1h) SortScore() int {
	return 70
}

func (s Emacross1h) Timeframe() string {
	return "1h"
}

func (s Emacross1h) WarmupPeriod() int {
	return 36
}

func (s Emacross1h) Indicators(df *model.Dataframe) {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["ova"] = indicator.SMA(df.Volume, 14)
}

func (s *Emacross1h) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	ema8 := df.Metadata["ema8"]
	ema21 := df.Metadata["ema21"]
	ova := df.Metadata["ova"]

	var strategyPosition types.StrategyPosition
	// 判断量价关系
	if ema8.Crossover(ema21) && df.Volume[len(df.Volume)-1] > ova[len(ova)-1] && realCandle.Close > df.Close.Last(0) {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}

	if ema8.Crossunder(ema21) && df.Volume[len(df.Volume)-1] > ova[len(ova)-1] && realCandle.Close < df.Close.Last(0) {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	strategyPosition.Tendency = s.checkMarketTendency(df)
	return strategyPosition
}
