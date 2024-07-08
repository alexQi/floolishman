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
	maxPrices := df.Metadata["max"]
	minPrices := df.Metadata["min"]

	topCount := 0
	bottomCount := 0

	for i := 0; i < len(maxPrices)-1; i++ {
		if maxPrices[i] > bbUpper[i] {
			topCount++
		}
	}
	for i := 0; i < len(minPrices)-1; i++ {
		if minPrices[i] < bbLower[i] {
			bottomCount++
		}
	}

	const limitBreak = 3

	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}
	// 判断是否换线
	tendency := s.checkCandleTendency(df, 1)
	// 判断量价关系
	if rsi < 30 && bottomCount <= limitBreak && calc.Abs(realCandle.Close-df.Close.Last(0))/bbLower.Last(0) < 0.003 && tendency == "bullish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	if rsi > 70 && topCount <= limitBreak && calc.Abs(realCandle.Close-df.Close.Last(0))/bbUpper.Last(0) < 0.003 && tendency == "bearish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
