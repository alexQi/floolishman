package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Rsi1m struct {
	BaseStrategy
}

func (s Rsi1m) SortScore() int {
	return 80
}

func (s Rsi1m) Timeframe() string {
	return "1m"
}

func (s Rsi1m) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi1m) Indicators(df *model.Dataframe) {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	// 计算布林带（Bollinger Bands）
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
}

func (s *Rsi1m) OnCandle(_ *model.Candle, df *model.Dataframe) types.StrategyPosition {
	var strategyPosition types.StrategyPosition

	rsi := df.Metadata["rsi"].Last(0)

	// 趋势判断
	if rsi >= 55 {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	if rsi < 45 {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	strategyPosition.Tendency = s.checkMarketTendency(df)

	return strategyPosition
}
