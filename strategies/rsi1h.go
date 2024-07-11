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
}

func (s *Rsi1h) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}
	rsi := df.Metadata["rsi"].Last(0)
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar, isRise := s.checkPinBar(
		df.Open.Last(0),
		df.Close.Last(0),
		df.High.Last(0),
		df.Low.Last(0),
	)
	// 趋势判断
	if rsi >= 80 && isUpperPinBar && !isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	// RSI 小于30，买入信号
	if rsi <= 20 && isLowerPinBar && isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	return strategyPosition
}
