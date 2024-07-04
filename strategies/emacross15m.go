package strategies

import (
	"floolishman/constants"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Emacross15m struct {
	BaseStrategy
}

func (s Emacross15m) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Emacross15m) Timeframe() string {
	return "15m"
}

func (s Emacross15m) WarmupPeriod() int {
	return 30
}

func (s Emacross15m) Indicators(df *model.Dataframe) []types.ChartIndicator {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["ova"] = indicator.SMA(df.Volume, 14)

	return []types.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "MA's",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["ema8"],
					Name:   "EMA 7",
					Color:  "red",
					Style:  constants.StyleLine,
				},
				{
					Values: df.Metadata["ema21"],
					Name:   "EMA 25",
					Color:  "blue",
					Style:  constants.StyleLine,
				},
			},
		},
		{
			Overlay:   false,
			GroupName: "OV",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Volume,
					Name:   "Volume",
					Color:  "pink",
					Style:  constants.StyleLine,
				},
			},
		},
		{
			Overlay:   false,
			GroupName: "OVA",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["ova"],
					Name:   "Volume Avg",
					Color:  "green",
					Style:  constants.StyleLine,
				},
			},
		},
	}
}

func (s *Emacross15m) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
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
			Price:        realCandle.Close,
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
	return strategyPosition
}
