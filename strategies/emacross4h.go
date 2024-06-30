package strategies

import (
	"floolisher/constants"
	"floolisher/indicator"
	"floolisher/model"
	"floolisher/types"
	"github.com/markcheno/go-talib"
	"reflect"
)

type Emacross4h struct {
	BaseStrategy
}

func (s Emacross4h) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Emacross4h) GetPosition() types.StrategyPosition {
	return s.StrategyPosition
}

func (s Emacross4h) Timeframe() string {
	return "4h"
}

func (s Emacross4h) WarmupPeriod() int {
	return 42
}

func (s Emacross4h) Indicators(df *model.Dataframe) []types.ChartIndicator {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["sma21"] = indicator.SMA(df.Close, 21)
	df.Metadata["obv"] = indicator.OBV(df.Close, df.Volume)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)

	return []types.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "MA's",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["ema8"],
					Name:   "EMA 8",
					Color:  "red",
					Style:  constants.StyleLine,
				},
				{
					Values: df.Metadata["sma21"],
					Name:   "SMA 21",
					Color:  "blue",
					Style:  constants.StyleLine,
				},
			},
		},
		{
			Overlay:   false,
			GroupName: "ATR",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["atr"],
					Name:   "ATR 14",
					Color:  "green",
					Style:  constants.StyleLine,
				},
			},
		},
		{
			Overlay:   false,
			GroupName: "OBV",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["obv"],
					Name:   "On Balance Volume",
					Color:  "purple",
					Style:  constants.StyleLine,
				},
			},
		},
	}
}

func (s *Emacross4h) OnCandle(df *model.Dataframe) {
	ema8 := df.Metadata["ema8"]
	sma21 := df.Metadata["sma21"]
	obv := df.Metadata["obv"]

	// 判断量价关系
	if ema8.Crossover(sma21) && obv[len(obv)-1] > talib.Sma(obv, 20)[len(talib.Sma(obv, 20))-1] {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}

	if ema8.Crossunder(sma21) && obv[len(obv)-1] > talib.Sma(obv, 20)[len(talib.Sma(obv, 20))-1] {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
}
