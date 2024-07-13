package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Momentum15m struct {
	BaseStrategy
}

func (s Momentum15m) SortScore() int {
	return 80
}

func (s Momentum15m) Timeframe() string {
	return "15m"
}

func (s Momentum15m) WarmupPeriod() int {
	return 30 // 预热期设定为24个数据点
}

func (s Momentum15m) Indicators(df *model.Dataframe) {
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

func (s *Momentum15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	momentums := df.Metadata["momentum"].LastValues(2)
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 3, 1.2)
	// 趋势判断
	if strategyPosition.Tendency == "rise" && (momentums[1]-momentums[0]) > 5 && momentums[0] < momentums[1] && !isUpperPinBar {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}
	// 动量递减向下 且未下方插针
	if strategyPosition.Tendency == "down" && (momentums[0]-momentums[1]) > 5 && momentums[0] > momentums[1] && !isLowerPinBar {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	return strategyPosition
}
