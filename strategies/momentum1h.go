package strategies

import (
	"floolishman/constants"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Momentum1h struct {
	BaseStrategy
}

func (s Momentum1h) SortScore() int {
	return 100
}

func (s Momentum1h) Timeframe() string {
	return "1h"
}

func (s Momentum1h) WarmupPeriod() int {
	return 24 // 预热期设定为24个数据点
}

func (s Momentum1h) Indicators(df *model.Dataframe) []types.ChartIndicator {
	// 计算动量指标
	momentum := indicator.Momentum(df.Close, 14)

	df.Metadata["momentum"] = momentum

	return []types.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "Momentum Indicator",
			Time:      df.Time,
			Metrics: []types.IndicatorMetric{
				{
					Values: df.Metadata["momentum"],
					Name:   "Momentum (14)",
					Color:  "orange",
					Style:  constants.StyleLine,
				},
			},
		},
	}
}

func (s *Momentum1h) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	var strategyPosition types.StrategyPosition

	momentums := df.Metadata["momentum"].LastValues(2)

	// 判断是否换线
	tendency := s.checkCandleTendency(df, 3)
	// 趋势判断
	if momentums[1] > 0 && momentums[0] > momentums[1] && realCandle.Close > df.Close.Last(0) && tendency == "bullish" {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	if momentums[1] < 0 && momentums[0] < momentums[1] && realCandle.Close < df.Close.Last(0) && tendency == "bearish" {
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
