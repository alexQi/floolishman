package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"fmt"
	"reflect"
)

type Resonance15m struct {
	BaseStrategy
}

func (s Resonance15m) SortScore() int {
	return 90
}

func (s Resonance15m) Timeframe() string {
	return "15m"
}

func (s Resonance15m) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Resonance15m) Indicators(df *model.Dataframe) {
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
}

func (s *Resonance15m) OnCandle(df *model.Dataframe) model.Strategy {
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
	adx := df.Metadata["adx"].Last(0)

	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	// 移动平均线上穿&&金叉
	if ema5.Crossover(ema10) && macd.Crossover(signal) && adx > 40 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
		fmt.Printf("-------------- ADX : %v \n", adx)
	}
	// 移动平均线下穿&&死叉
	if ema5.Crossunder(ema10) && macd.Crossunder(signal) && adx > 40 {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
		fmt.Printf("-------------- ADX : %v \n", adx)
	}

	return strategyPosition
}
