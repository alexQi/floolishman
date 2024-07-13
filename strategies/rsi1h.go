package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
	"reflect"
)

type Rsi1h struct {
	BaseStrategy
}

func (s Rsi1h) SortScore() int {
	return 90
}

func (s Rsi1h) Timeframe() string {
	return "1h"
}

func (s Rsi1h) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s Rsi1h) Indicators(df *model.Dataframe) {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	bbRsiZero := []float64{}
	for _, val := range df.Close {
		if val > 0 {
			bbRsiZero = append(bbRsiZero, val)
		}
	}
	df.Metadata["rsi"] = indicator.RSI(bbRsiZero, 6)

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

func (s *Rsi1h) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}
	rsi := df.Metadata["rsi"].Last(0)
	bbUpper := df.Metadata["bb_upper"].Last(0)
	bbLower := df.Metadata["bb_lower"].Last(0)
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar, _ := s.checkPinBar(
		1.2,
		df.Open.Last(0),
		df.Close.Last(0),
		df.High.Last(0),
		df.Low.Last(0),
	)
	// 趋势判断
	if rsi >= 80 && isUpperPinBar && calc.Abs(df.Close.Last(0)-bbUpper)/bbUpper > 0.005 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	// RSI 小于30，买入信号
	if rsi <= 20 && isLowerPinBar && calc.Abs(df.Close.Last(0)-bbLower)/bbLower > 0.005 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	return strategyPosition
}
