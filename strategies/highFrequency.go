package strategies

import (
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type HighFrequency struct {
	BaseStrategy
}

func (s HighFrequency) SortScore() int {
	return 100
}

func (s HighFrequency) Timeframe() string {
	return "1m"
}

func (s HighFrequency) WarmupPeriod() int {
	return 30
}

func (s HighFrequency) Indicators(_ *model.Dataframe) {

}

func (s *HighFrequency) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}
	candleColor := s.getCandleColor(df.Open.Last(0), df.Close.Last(0))
	// 判断插针情况，排除动量数据滞后导致反弹趋势还继续开单
	_, _, isRise := s.checkPinBar(
		df.Open.Last(0),
		df.Close.Last(0),
		df.High.Last(0),
		df.Low.Last(0),
	)
	if candleColor == "bullish" && isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}
	if candleColor == "bearish" && !isRise {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
