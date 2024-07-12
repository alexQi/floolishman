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

func (s Rsi15m) Indicators(df *model.Dataframe) {
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

func (s Rsi15m) SortScore() int {
	return 90
}

func (s Rsi15m) Timeframe() string {
	return "15m"
}

func (s Rsi15m) WarmupPeriod() int {
	return 24 // RSI的预热期设定为14个数据点
}

func (s *Rsi15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	rsis := df.Metadata["rsi"].LastValues(2)
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar, isRise := s.checkPinBar(
		df.Open.Last(0),
		df.Close.Last(0),
		df.High.Last(0),
		df.Low.Last(0),
	)
	// 趋势判断
	if rsis[0] >= 70 && rsis[1] > rsis[0] && isUpperPinBar && !isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	// RSI 小于30，买入信号
	if rsis[1] < 30 && rsis[1] < rsis[0] && isLowerPinBar && isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	return strategyPosition
}
