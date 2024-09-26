package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Rsi1h struct {
	BaseStrategy
}

func (s Rsi1h) SortScore() float64 {
	return 90
}

func (s Rsi1h) Timeframe() string {
	return "1h"
}

func (s Rsi1h) WarmupPeriod() int {
	return 36 // RSI的预热期设定为14个数据点
}

func (s Rsi1h) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	// 检查插针
	upperPinRates, lowerPinRates, upperShadows, lowerShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowerPinRates"] = lowerPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowerShadows"] = lowerShadows

	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Rsi1h) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
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
	isUpperPinBar, isLowerPinBar := s.batchCheckPinBar(df, 3, 1.5, true)
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
