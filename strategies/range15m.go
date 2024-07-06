package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
	"reflect"
)

type Range15m struct {
	BaseStrategy
}

func (s Range15m) SortScore() int {
	return 85
}

func (s Range15m) Timeframe() string {
	return "15m"
}

func (s Range15m) WarmupPeriod() int {
	return 30
}

func (s Range15m) Indicators(df *model.Dataframe) {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	// 计算布林带（Bollinger Bands）
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	df.Metadata["max"] = indicator.Max(df.Close, 21)
	df.Metadata["min"] = indicator.Min(df.Close, 21)
}

func (s *Range15m) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	rsi := df.Metadata["rsi"].Last(0)
	bbUpper := df.Metadata["bb_upper"]
	bbLower := df.Metadata["bb_lower"]
	max := df.Metadata["max"]
	min := df.Metadata["min"]

	topCount := 0
	bottomCount := 0

	for i := 0; i < len(max)-1; i++ {
		if max[i] > bbUpper[i] {
			topCount++
		}
	}
	for i := 0; i < len(min)-1; i++ {
		if min[i] < bbLower[i] {
			bottomCount++
		}
	}

	const limitBreak = 3

	var strategyPosition types.StrategyPosition

	// 判断量价关系
	if rsi < 30 && bottomCount <= limitBreak && calc.Abs(realCandle.Close-df.Low.Last(0))/bbLower.Last(0) < 0.003 {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
			Price:        realCandle.Close,
		}
	}

	if rsi > 70 && topCount <= limitBreak && calc.Abs(realCandle.Close-df.High.Last(0))/bbUpper.Last(0) < 0.003 {
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
