package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Test15m struct {
	BaseStrategy
}

func (s Test15m) SortScore() int {
	return 90
}

func (s Test15m) Timeframe() string {
	return "15m"
}

func (s Test15m) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Test15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	kcUpper, kcMiddle, kcLower := indicator.KeltnerChannel(df.Close, df.High, df.Low, 20, 2.0)
	df.Metadata["kc_upper"] = kcUpper
	df.Metadata["kc_middle"] = kcMiddle
	df.Metadata["kc_lower"] = kcLower

	df.Metadata["rsi"] = indicator.RSI(df.Close, 14)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Test15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	// 使用前一帧的收盘价作为判断依据，避免未完成的K线对策略的影响
	previousPrice := df.Close.Last(1)
	currentPrice := df.Close.Last(0)
	kcUpper := df.Metadata["kc_upper"].Last(1)   // 使用已完成的肯特纳通道上轨
	kcLower := df.Metadata["kc_lower"].Last(1)   // 使用已完成的肯特纳通道下轨
	rsi := df.Metadata["rsi"].Last(1)            // 使用已完成的RSI
	bbUpper := df.Metadata["bb_upper"].Last(1)   // 使用已完成的布林带上轨
	bbLower := df.Metadata["bb_lower"].Last(1)   // 使用已完成的布林带下轨
	bbMiddle := df.Metadata["bb_middle"].Last(1) // 使用已完成的布林带中轨

	// 求稳的多单进场逻辑
	if previousPrice < kcLower && currentPrice > kcLower && rsi < 30 && (bbUpper-bbLower)/bbMiddle < 0.1 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	// 求稳的空单进场逻辑
	if previousPrice > kcUpper && currentPrice < kcUpper && rsi > 70 && (bbUpper-bbLower)/bbMiddle < 0.1 {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
