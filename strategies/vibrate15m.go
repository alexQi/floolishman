package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
	"reflect"
)

type Vibrate15m struct {
	BaseStrategy
}

func (s Vibrate15m) SortScore() int {
	return 85
}

func (s Vibrate15m) Timeframe() string {
	return "15m"
}

func (s Vibrate15m) WarmupPeriod() int {
	return 30 // 预热期设定为30个数据点
}

func (s Vibrate15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 20, 2.0, 2.0)
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
	df.Metadata["bb_width"] = bbWidth
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Vibrate15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	// 使用前一帧的收盘价作为判断依据，避免未完成的K线对策略的影响
	currentPrice := df.Close.Last(0)
	previousPrice := df.Close.Last(1)
	bbUpper := df.Metadata["bb_upper"].Last(1)   // 使用已完成的布林带上轨
	bbLower := df.Metadata["bb_lower"].Last(1)   // 使用已完成的布林带下轨
	bbMiddle := df.Metadata["bb_middle"].Last(1) // 使用已完成的布林带中轨
	bbWidth := df.Metadata["bb_width"].Last(1)
	rsi := df.Metadata["rsi"].Last(1) // 使用已完成的RSI
	ema8 := df.Metadata["ema8"].Last(0)

	bbWaveDistance := bbWidth * 0.05
	// 判断当前是否处于箱体震荡
	// 如果布林带上轨和下轨在一定范围内波动，并且RSI在35到65之间，且ADX小于25，则认为是箱体震荡
	isInBoxRange := (bbUpper-bbLower)/bbMiddle < 0.05 && rsi >= 35 && rsi <= 60
	emaPriceRatio := calc.Abs(ema8-currentPrice) / ema8

	if isInBoxRange {
		// 高点做空
		if previousPrice > (bbMiddle-bbWaveDistance*8) && emaPriceRatio >= 0.005 {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeSell
		}

		// 低点做多
		if previousPrice < (bbMiddle-bbWaveDistance*8) && emaPriceRatio >= 0.005 {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeBuy
		}
	}

	return strategyPosition
}
