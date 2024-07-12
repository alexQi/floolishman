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
	return 60
}

func (s Emacross15m) Timeframe() string {
	return "15m"
}

func (s Emacross15m) WarmupPeriod() int {
	return 30
}

func (s Emacross15m) Indicators(df *model.Dataframe) {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	changeRates := make([]float64, len(bbWidth)-1)
	for i := 1; i < len(bbWidth); i++ {
		changeRates[i-1] = (bbWidth[i] - bbWidth[i-1]) / bbWidth[i-1]
	}
	df.Metadata["bb_width"] = bbWidth
	df.Metadata["bb_change_rate"] = changeRates
}

func (s *Emacross15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	ema8 := df.Metadata["ema8"]
	ema21 := df.Metadata["ema21"]
	volume := df.Metadata["volume"].Last(0)
	ova := df.Metadata["avgVolume"].Last(0)
	rsi := df.Metadata["rsi"].Last(0)

	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	//_, _, isRise := s.checkPinBar(
	//	df.Open.Last(0),
	//	df.Close.Last(0),
	//	df.High.Last(0),
	//	df.Low.Last(0),
	//)
	// 判断量价关系
	if strategyPosition.Tendency == "rise" && ema8.Crossover(ema21) && volume > ova && rsi < 70 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	if strategyPosition.Tendency == "down" && ema8.Crossunder(ema21) && volume > ova && rsi > 30 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
