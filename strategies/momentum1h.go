package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Momentum1h struct {
	BaseStrategy
}

func (s Momentum1h) SortScore() int {
	return 100
}

func (s Momentum1h) Timeframe() string {
	return "1h"
}

func (s Momentum1h) WarmupPeriod() int {
	return 24 // 预热期设定为24个数据点
}

func (s Momentum1h) Indicators(df *model.Dataframe) {
	// 计算动量指标
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
}

func (s *Momentum1h) OnCandle(realCandle *model.Candle, df *model.Dataframe) types.StrategyPosition {
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
	if momentums[1] > 0 && momentums[0] > momentums[1] && realCandle.Low > df.Close.Last(0) && tendency == "bullish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}
	if momentums[1] < 0 && momentums[0] < momentums[1] && realCandle.Low < df.Close.Last(0) && tendency == "bearish" {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
