package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type MomentumVolume15m struct {
	BaseStrategy
}

func (s MomentumVolume15m) SortScore() int {
	return 80
}

func (s MomentumVolume15m) Timeframe() string {
	return "15m"
}

func (s MomentumVolume15m) WarmupPeriod() int {
	return 90
}

func (s MomentumVolume15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *MomentumVolume15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	momentums := df.Metadata["momentum"].LastValues(2)
	volume := df.Metadata["volume"].Last(0)
	avgVolume := df.Metadata["avgVolume"].Last(1)

	openPrice := df.Open.Last(0)
	closePrice := df.Close.Last(0)

	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 3, 1.2)
	// 趋势判断
	if (momentums[1]-momentums[0]) > 5 &&
		momentums[0] < momentums[1] &&
		volume > (avgVolume*2) &&
		closePrice > openPrice &&
		!isUpperPinBar {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}
	// 动量递减向下 且未下方插针
	if (momentums[0]-momentums[1]) > 5 &&
		momentums[0] > momentums[1] &&
		volume > (avgVolume*2) &&
		openPrice > closePrice &&
		!isLowerPinBar {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	return strategyPosition
}
