package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"fmt"
	"reflect"
)

type Test15m struct {
	BaseStrategy
}

func (s Test15m) SortScore() float64 {
	return 90
}

func (s Test15m) Timeframe() string {
	return "15m"
}

func (s Test15m) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Test15m) Indicators(df *model.Dataframe) {
	_, bbMiddle, _ := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算ema
	df.Metadata["ema5"] = indicator.EMA(df.Close, 5)
	df.Metadata["ema10"] = indicator.EMA(df.Close, 10)
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 12, 26, 9)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Test15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	ema5 := df.Metadata["ema5"]
	ema10 := df.Metadata["ema10"]
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	adx := df.Metadata["adx"].Last(1)

	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	// 移动平均线上穿&&金叉
	if ema5.Crossover(ema10, 0) && macd.Crossover(signal, 0) && adx > 40 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
		fmt.Printf("-------------- ADX : %v \n", adx)
	}
	// 移动平均线下穿&&死叉
	if ema5.Crossunder(ema10, 0) && macd.Crossunder(signal, 0) && adx > 40 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
		fmt.Printf("-------------- ADX : %v \n", adx)
	}

	return strategyPosition
}
