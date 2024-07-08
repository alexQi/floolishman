package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Momentum15m struct {
	BaseStrategy
}

func (s Momentum15m) SortScore() int {
	return 80
}

func (s Momentum15m) Timeframe() string {
	return "15m"
}

func (s Momentum15m) WarmupPeriod() int {
	return 24 // 预热期设定为24个数据点
}

func (s Momentum15m) Indicators(df *model.Dataframe) {
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
}

func (s *Momentum15m) OnCandle(_ *model.Candle, df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	momentums := df.Metadata["momentum"].LastValues(2)

	// 判断是否换线
	tendency := s.checkCandleTendency(df, 3)
	// 趋势判断
	if momentums[1] > 0 && momentums[0] > momentums[1] && tendency == "bullish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}
	if momentums[1] < 0 && momentums[0] < momentums[1] && tendency == "bearish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
