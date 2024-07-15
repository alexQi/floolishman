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
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

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
	avgVolume := df.Metadata["avgVolume"].Last(1)
	volume := df.Metadata["volume"].Last(0)

	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	_, _, isRise := s.checkPinBar(
		1.2,
		df.Open.Last(0),
		df.Close.Last(0),
		df.High.Last(0),
		df.Low.Last(0),
	)
	// 判断量价关系
	if strategyPosition.Tendency == "rise" && ema8.Crossover(ema21) && volume > avgVolume*2 && isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	if strategyPosition.Tendency == "down" && ema8.Crossunder(ema21) && volume > avgVolume*2 && !isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
