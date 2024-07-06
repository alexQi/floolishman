package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Rsi1h struct {
	BaseStrategy
}

func (s Rsi1h) SortScore() int {
	return 70
}

func (s Rsi1h) Timeframe() string {
	return "1h"
}

func (s Rsi1h) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi1h) Indicators(df *model.Dataframe) {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	// 计算布林带（Bollinger Bands）
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
}

func (s *Rsi1h) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	var strategyPosition types.StrategyPosition
	rsi := df.Metadata["rsi"].Last(0)
	bbUpper := df.Metadata["bb_upper"].Last(0)
	bbLower := df.Metadata["bb_lower"].Last(0)
	// 判断是否换线
	tendency := s.checkCandleTendency(df, 2)
	// 趋势判断
	if rsi >= 85 && df.Close.Last(0) > bbUpper && realCandle.Close < df.Close.Last(0) && tendency == "bullish" {
		strategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	// RSI 小于30，买入信号
	if rsi <= 15 && df.Close.Last(0) < bbLower && realCandle.Close > df.Close.Last(0) && tendency == "bearish" {
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
