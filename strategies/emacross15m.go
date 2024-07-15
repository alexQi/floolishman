package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Emacross15m struct {
	BaseStrategy
}

func (s Emacross15m) SortScore() int {
	return 80
}

func (s Emacross15m) Timeframe() string {
	return "15m"
}

func (s Emacross15m) WarmupPeriod() int {
	return 90
}

func (s Emacross15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Emacross15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	ema8 := df.Metadata["ema8"]
	ema21 := df.Metadata["ema21"]
	adx := df.Metadata["adx"].Last(1)
	// 判断量价关系
	if ema8.Crossover(ema21) && adx > 25 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	if ema8.Crossunder(ema21) && adx > 25 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
