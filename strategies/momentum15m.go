package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Momentum15m struct {
	BaseStrategy
}

func (s Momentum15m) SortScore() int {
	return 80
}

func (s Momentum15m) Timeframe() string {
	return "15m"
}

func (s Momentum15m) WarmupPeriod() int {
	return 24 // 预热期设定为24个数据点
}

func (s Momentum15m) Indicators(df *model.Dataframe) {
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
}

func (s *Momentum15m) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	var strategyPosition types.StrategyPosition

	momentums := df.Metadata["momentum"].LastValues(2)

	// 判断是否换线
	tendency := s.checkCandleTendency(df, 3)
	// 趋势判断
	if momentums[1] > 0 && momentums[0] > momentums[1] && realCandle.Low > df.Close.Last(0) && tendency == "bullish" {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	if momentums[1] < 0 && momentums[0] < momentums[1] && realCandle.Low < df.Close.Last(0) && tendency == "bearish" {
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
