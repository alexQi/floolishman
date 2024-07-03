package strategies

import (
	"floolishman/constants"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Rsi1m struct {
	BaseStrategy
}

func (s Rsi1m) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Rsi1m) Timeframe() string {
	return "1m"
}

func (s Rsi1m) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi1m) Indicators(df *model.Dataframe) []types.ChartIndicator {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	// 计算布林带（Bollinger Bands）
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

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

func (s *Rsi1m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	var strategyPosition types.StrategyPosition

	rsis := df.Metadata["rsi"].LastValues(2)
	bbUpper := df.Metadata["bb_upper"].Last(0)
	bbLower := df.Metadata["bb_lower"].Last(0)

	// 判断是否换线
	tendency := s.checkCandleTendency(df, 3)
	// 趋势判断
	if rsis[1] >= 70 && rsis[0] >= 70 && df.Close.Last(0) > bbUpper && tendency == "bullish" {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	if rsis[1] <= 30 && rsis[0] <= 30 && df.Close.Last(0) < bbLower && tendency == "bearish" {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	return strategyPosition
}
