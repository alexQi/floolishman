package strategies

import (
	"floolisher/constants"
	"floolisher/indicator"
	"floolisher/model"
	"floolisher/types"
	"reflect"
)

type Rsi1h struct {
	BaseStrategy
}

func (s Rsi1h) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Rsi1h) GetPosition() types.StrategyPosition {
	return s.StrategyPosition
}

func (s Rsi1h) Timeframe() string {
	return "1h"
}

func (s Rsi1h) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi1h) Indicators(df *model.Dataframe) []types.ChartIndicator {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 14)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 7)
	df.Metadata["prev_close"] = df.Open

	return []types.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "RSI Indicator",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["rsi"],
					Name:   "RSI (14)",
					Color:  "blue",
					Style:  constants.StyleLine,
				},
			},
		},
	}
}

func (s *Rsi1h) OnCandle(df *model.Dataframe) {
	rsi := df.Metadata["rsi"].Last(0)
	if rsi >= 85 {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	// RSI 小于30，买入信号
	if rsi <= 15 {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
}
