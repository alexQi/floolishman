package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Kc15m struct {
	BaseStrategy
}

func (s Kc15m) SortScore() int {
	return 90
}

func (s Kc15m) Timeframe() string {
	return "15m"
}

func (s Kc15m) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Kc15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	kcUpper, kcMiddle, kcLower := indicator.KeltnerChannel(df.Close, df.High, df.Low, 20, 2.0)
	df.Metadata["kc_upper"] = kcUpper
	df.Metadata["kc_middle"] = kcMiddle
	df.Metadata["kc_lower"] = kcLower

	df.Metadata["rsi"] = indicator.RSI(df.Close, 14)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Kc15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	// 使用前一帧的收盘价作为判断依据，避免未完成的K线对策略的影响
	previousPrice := df.Close.Last(1)
	currentPrice := df.Close.Last(0)
	kcUpper := df.Metadata["kc_upper"].Last(1)  // 使用已完成的肯特纳通道上轨
	kcLower := df.Metadata["kc_lower"].Last(1)  // 使用已完成的肯特纳通道下轨
	rsi := df.Metadata["rsi"].Last(1)           // 使用已完成的RSI
	bbUpper := df.Metadata["bbUpper"].Last(1)   // 使用已完成的布林带上轨
	bbLower := df.Metadata["bbLower"].Last(1)   // 使用已完成的布林带下轨
	bbMiddle := df.Metadata["bbMiddle"].Last(1) // 使用已完成的布林带中轨

	// 求稳的多单进场逻辑
	if previousPrice < kcLower && currentPrice > kcLower && rsi < 30 && (bbUpper-bbLower)/bbMiddle < 0.1 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}

	// 求稳的空单进场逻辑
	if previousPrice > kcUpper && currentPrice < kcUpper && rsi > 85 && (bbUpper-bbLower)/bbMiddle < 0.1 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}

	return strategyPosition
}
