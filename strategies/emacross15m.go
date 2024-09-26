package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Emacross15m struct {
	BaseStrategy
}

func (s Emacross15m) SortScore() float64 {
	return 70
}

func (s Emacross15m) Timeframe() string {
	return "15m"
}

func (s Emacross15m) WarmupPeriod() int {
	return 90
}

func (s Emacross15m) Indicators(df *model.Dataframe) {
	dif, dea, _ := indicator.MACD(df.Close, 12, 26, 9)
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.EMA(df.Close, 21)
	df.Metadata["dea"] = dea
	df.Metadata["dif"] = dif
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Emacross15m) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	ema8 := df.Metadata["ema8"]
	ema21 := df.Metadata["ema21"]
	dea := df.Metadata["dea"]
	dif := df.Metadata["dif"]
	// 判断量价关系
	if ema8.Crossover(ema21, 0) && dif.Crossover(dea, 0) {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}

	if ema8.Crossunder(ema21, 0) && dif.Crossunder(dea, 0) {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}

	return strategyPosition
}
