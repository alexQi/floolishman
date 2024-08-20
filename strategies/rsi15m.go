package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Rsi15m struct {
	BaseStrategy
}

func (s Rsi15m) SortScore() int {
	return 80
}

func (s Rsi15m) Timeframe() string {
	return "15m"
}

func (s Rsi15m) WarmupPeriod() int {
	return 90 // RSI的预热期设定为14个数据点
}

func (s Rsi15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Rsi15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	rsis := df.Metadata["rsi"].LastValues(2)
	volume := df.Metadata["volume"].LastValues(3)
	avgVolume := df.Metadata["avgVolume"].LastValues(3)

	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 3, 1)
	isCross, direction := s.bactchCheckVolume(volume[:len(volume)-1], avgVolume[:len(avgVolume)-1], 2.2)

	// 趋势判断 85 84
	if strategyPosition.Tendency != "range" && rsis[0] > rsis[1] && rsis[1] > 90 && isUpperPinBar && isCross {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}
	// RSI 小于30，买入信号
	if strategyPosition.Tendency != "range" && rsis[0] < rsis[1] && rsis[0] <= 8 && isLowerPinBar && isCross && direction == "fall" {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}
	return strategyPosition
}
