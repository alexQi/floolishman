package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
	"reflect"
)

type Resonance15m struct {
	BaseStrategy
}

func (s Resonance15m) SortScore() float64 {
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
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 8, 17, 5)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
	df.Metadata["macdAngle"] = indicator.TendencyAngles(macdLine, 3)
	df.Metadata["signalAngle"] = indicator.TendencyAngles(signalLine, 3)
}

func (s *Resonance15m) OnCandle(df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	price := df.Close.Last(0)
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	//tendencyAngle := df.Metadata["macdAngle"]
	tendencyAngle := df.Metadata["signalAngle"]
	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	lastRsi := df.Metadata["rsi"].Last(0)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastMacd := macd.Last(0)
	lastSignal := signal.Last(0)
	historyOpens := df.Open.GetLastValues(3, 1)
	historyCloses := df.Close.GetLastValues(3, 1)

	macdPriceRatio := (lastMacd / price) * 100
	historyTendency := s.checkCandleTendency(historyOpens, historyCloses, 3, 1)
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 0.85, false)

	lastTendencyAngle := tendencyAngle.Last(0)
	prevTendencyAngle := tendencyAngle.Last(1)

	// 判断当前macd和信号线是否处于0轴上方
	if lastMacd > 0 && lastSignal > 0 {
		// 金叉信号，追多
		//if macd.Crossover(signal) && rsi < 60 && lastTendencyAngle > 80.0 && prevTendencyAngle > 0 && historyTendency != "bullish" && !isUpperPinBar {
		//	strategyPosition.Useable = 1
		//	strategyPosition.Side = string(model.SideTypeBuy)
		//	strategyPosition.Score = calc.Abs(lastTendencyAngle)
		//}
		// 死叉信号，博反转
		if macd.Crossunder(signal, 1) &&
			prevRsi > lastRsi &&
			prevRsi > 60 &&
			calc.Abs(lastTendencyAngle) > 80.0 &&
			lastTendencyAngle < 0 &&
			historyTendency != "bearish" &&
			calc.Abs(macdPriceRatio) > 0.80 &&
			!isLowerPinBar {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = calc.Abs(lastTendencyAngle) * (calc.Abs(macdPriceRatio) + 1)
		}
	}
	// 判断当前macd和信号线是否处于0轴下方
	if lastMacd < 0 && lastSignal < 0 {
		// 金叉信号，博反转
		if macd.Crossover(signal, 1) &&
			prevRsi < lastRsi &&
			prevRsi < 30 &&
			lastTendencyAngle > 80.0 &&
			historyTendency != "bullish" &&
			calc.Abs(macdPriceRatio) > 0.80 &&
			!isUpperPinBar {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = calc.Abs(lastTendencyAngle) * (calc.Abs(macdPriceRatio) + 1)
		}
		// 死叉信号，追空
		if macd.Crossunder(signal, 0) &&
			prevRsi > lastRsi &&
			prevRsi > 35 &&
			calc.Abs(lastTendencyAngle) > 80.0 &&
			lastTendencyAngle < 0 &&
			prevTendencyAngle < 0 &&
			lastTendencyAngle > prevTendencyAngle &&
			historyTendency != "bearish" &&
			!isLowerPinBar {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = calc.Abs(lastTendencyAngle)
		}
	}

	//if calc.Abs(lastTendencyAngle) > 80 && calc.Abs(macdPriceRatio) > 0.75 {
	//	strategyPosition.Score = calc.Abs(lastTendencyAngle) * calc.Abs(macdPriceRatio)
	//
	//	if macd.Crossover(signal) && lastMacd < 0 && lastSignal < 0 && rsi < 35 && historyTendency != "bullish" && !isUpperPinBar {
	//		strategyPosition.Useable = 1
	//		strategyPosition.Side = string(model.SideTypeBuy)
	//	}
	//
	//	if macd.Crossunder(signal) && lastMacd > 0 && lastSignal > 0 && rsi > 60 && historyTendency != "bearish" && !isLowerPinBar {
	//		strategyPosition.Useable = 1
	//		strategyPosition.Side = string(model.SideTypeSell)
	//	}
	//}

	return strategyPosition
}
