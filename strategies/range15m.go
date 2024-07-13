package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
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
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	bbRsiZero := []float64{}
	for _, val := range df.Close {
		if val > 0 {
			bbRsiZero = append(bbRsiZero, val)
		}
	}
	df.Metadata["rsi"] = indicator.RSI(bbRsiZero, 6)

	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)

	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	changeRates := make([]float64, len(bbWidth)-1)
	for i := 1; i < len(bbWidth); i++ {
		changeRates[i-1] = (bbWidth[i] - bbWidth[i-1]) / bbWidth[i-1]
	}
	df.Metadata["bb_width"] = bbWidth
	df.Metadata["bb_change_rate"] = changeRates
}

func (s *Range15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	rsis := df.Metadata["rsi"].LastValues(2)
	bbUpper := df.Metadata["bb_upper"].Last(0)
	bbLower := df.Metadata["bb_lower"].Last(0)

	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	if strategyPosition.Tendency == "range" {
		if rsis[0] < rsis[1] && calc.Abs(bbLower-df.Close.Last(0))/bbLower < 0.005 {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeBuy
		}

		if rsis[0] > rsis[1] && calc.Abs(bbUpper-df.Close.Last(0))/bbUpper < 0.005 {
			strategyPosition.Useable = true
			strategyPosition.Side = model.SideTypeSell
		}
	}

	return strategyPosition
}
