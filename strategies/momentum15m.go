package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
	"reflect"
)

type Momentum15m struct {
	BaseStrategy
}

func (s Momentum15m) SortScore() float64 {
	return 80
}

func (s Momentum15m) Timeframe() string {
	return "15m"
}

func (s Momentum15m) WarmupPeriod() int {
	return 30 // 预热期设定为24个数据点
}

func (s Momentum15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth

	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Momentum15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
		ChaseMode:    1,
	}
	momentums := df.Metadata["momentum"].LastValues(2)
	momentumsDistance := momentums[1] - momentums[0]
	// 动量向上
	if momentumsDistance > 8 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}
	// 动量向下
	if momentumsDistance < 0 && calc.Abs(momentumsDistance) > 7 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}
	return strategyPosition
}
