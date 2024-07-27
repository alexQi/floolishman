package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
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
	return 36 // RSI的预热期设定为14个数据点
}

func (s Rsi1h) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Rsi1h) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	rsi := df.Metadata["rsi"].Last(1)
	volume := df.Metadata["volume"].Last(1)
	avgVolume := df.Metadata["avgVolume"].Last(1)
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 3, 1.5)
	// 趋势判断
	if strategyPosition.Tendency != "range" && rsi >= 80 && isUpperPinBar && volume > avgVolume*1.2 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}
	// RSI 小于30，买入信号
	if strategyPosition.Tendency != "range" && rsi <= 20 && isLowerPinBar && volume > avgVolume*1.2 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}

	return strategyPosition
}
