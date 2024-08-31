package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
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
	return 96 // 预热期设定为50个数据点
}

func (s Resonance15m) Indicators(df *model.Dataframe) {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	// 计算ema
	df.Metadata["ema5"] = indicator.EMA(df.Close, 5)
	df.Metadata["ema10"] = indicator.EMA(df.Close, 10)
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 12, 26, 9)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Resonance15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	rsi := df.Metadata["rsi"].Last(0)
	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	lastMacd := macd.Last(0)
	lastSignal := signal.Last(0)

	historyOpens := df.Open.LastValues(4)
	historyCloses := df.Close.LastValues(4)

	historyTendency := s.checkCandleTendency(historyOpens[:len(historyOpens)-1], historyCloses[:len(historyCloses)-1], 3, 1)
	if macd.Crossover(signal) && lastMacd < 0 && lastSignal < 0 && rsi < 50 && historyTendency != "bullish" {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeBuy)
	}

	if macd.Crossunder(signal) && lastMacd > 0 && lastSignal > 0 && rsi > 50 && historyTendency != "bearish" {
		strategyPosition.Useable = 1
		strategyPosition.Side = string(model.SideTypeSell)
	}

	return strategyPosition
}
