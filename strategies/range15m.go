package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Range15m struct {
	BaseStrategy
}

func (s Range15m) SortScore() int {
	return 85
}

func (s Range15m) Timeframe() string {
	return "15m"
}

func (s Range15m) WarmupPeriod() int {
	return 90
}

func (s Range15m) Indicators(df *model.Dataframe) {

	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}

	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower
	df.Metadata["bb_width"] = bbWidth
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Range15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	bbMiddle := df.Metadata["bb_middle"].Last(0)
	bbWidth := df.Metadata["bb_width"].Last(0)

	bbWaveDistance := bbWidth * 0.05
	currentPrice := df.Close.Last(0)

	if strategyPosition.Tendency == "range" {
		if currentPrice < (bbMiddle - bbWaveDistance*6) {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeBuy
		}

		if currentPrice > (bbMiddle + bbWaveDistance*6) {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeSell
		}
	}

	return strategyPosition
}
