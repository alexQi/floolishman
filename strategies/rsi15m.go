package strategies

import (
	"floolisher/constants"
	"floolisher/indicator"
	"floolisher/model"
	"floolisher/types"
	"reflect"
)

type Rsi15m struct {
	BaseStrategy
}

func (s Rsi15m) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Rsi15m) GetPosition() types.StrategyPosition {
	return s.StrategyPosition
}

func (s Rsi15m) Timeframe() string {
	return "15m"
}

func (s Rsi15m) WarmupPeriod() int {
	return 30 // RSI的预热期设定为14个数据点
}

func (s Rsi15m) Indicators(df *model.Dataframe) []types.ChartIndicator {
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

func (s *Rsi15m) OnCandle(df *model.Dataframe) {
	rsi := df.Metadata["rsi"].Last(0)
	if rsi >= 60 {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	// RSI 小于30，买入信号
	if rsi <= 60 {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
}
