package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
	"reflect"
)

type MomentumVolume15m struct {
	BaseStrategy
}

func (s MomentumVolume15m) SortScore() int {
	return 90
}

func (s MomentumVolume15m) Timeframe() string {
	return "15m"
}

func (s MomentumVolume15m) WarmupPeriod() int {
	return 90
}

func (s MomentumVolume15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["avgVolume"] = indicator.EMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *MomentumVolume15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
		ChaseMode:    1,
	}

	momentums := df.Metadata["momentum"].LastValues(2)
	volume := df.Metadata["volume"].LastValues(3)
	avgVolume := df.Metadata["avgVolume"].LastValues(3)

	momentumsDistance := momentums[1] - momentums[0]

	isCross, _ := s.bactchCheckVolume(volume, avgVolume, 2)

	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 1)
	// 趋势判断
	// 动量正向增长
	// 7 35
	if momentumsDistance > 8 &&
		momentums[1] < 35 &&
		isCross &&
		!isUpperPinBar {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}
	// 动量负向增长
	// 7 18
	if momentumsDistance < 0 &&
		calc.Abs(momentumsDistance) > 7 &&
		calc.Abs(momentums[1]) < 18 &&
		isCross &&
		!isLowerPinBar {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}

	return strategyPosition
}
