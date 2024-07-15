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
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Rsi15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	rsis := df.Metadata["rsi"].LastValues(3)
	volume := df.Metadata["volume"].Last(1)
	avgVolume := df.Metadata["avgVolume"].Last(1)

	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 3, 1.2)

	// 趋势判断
	if strategyPosition.Tendency != "range" && rsis[0] > rsis[1] && rsis[0] > 90 && isUpperPinBar && volume > avgVolume*2 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}
	// RSI 小于30，买入信号
	if strategyPosition.Tendency != "range" && rsis[0] < 10 && rsis[1] > rsis[0] && isLowerPinBar && volume > avgVolume*2 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	return strategyPosition
}
