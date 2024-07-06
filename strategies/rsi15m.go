package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Rsi15m struct {
	BaseStrategy
}

func (s Rsi15m) SortScore() int {
	return 60
}

func (s Rsi15m) Timeframe() string {
	return "15m"
}

func (s Rsi15m) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi15m) Indicators(df *model.Dataframe) {
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	// 计算布林带（Bollinger Bands）
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
}

func (s *Rsi15m) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	rsis := df.Metadata["rsi"].LastValues(2)
	bbUpper := df.Metadata["bb_upper"].Last(0)
	bbLower := df.Metadata["bb_lower"].Last(0)

	// 判断是否换线
	tendency := s.checkCandleTendency(df, 3)
	// 趋势判断
	if rsis[1] >= 70 && rsis[0] >= 70 && df.Close.Last(0) > bbUpper && realCandle.Close < df.Close.Last(0) && tendency == "bullish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	if rsis[1] <= 30 && rsis[0] <= 30 && df.Close.Last(0) < bbLower && realCandle.Close > df.Close.Last(0) && tendency == "bearish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	return strategyPosition
}
