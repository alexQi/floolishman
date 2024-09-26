package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Macd15m struct {
	BaseStrategy
}

func (s Macd15m) SortScore() float64 {
	return 90
}

func (s Macd15m) Timeframe() string {
	return "15m"
}

func (s Macd15m) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Macd15m) Indicators(df *model.Dataframe) {
	_, bbMiddle, _ := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 12, 26, 9)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Macd15m) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	adx := df.Metadata["adx"].Last(0)

	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	previousMACD := macd.Last(1)
	currentMACD := macd.Last(0)
	previousSignal := signal.Last(1)
	currentSignal := signal.Last(0)

	// 判断MACD是否穿越0轴
	macdCrossedAboveZero := previousMACD < 0 && currentMACD > 0
	macdCrossedBelowZero := previousMACD > 0 && currentMACD < 0

	// 判断金叉和死叉
	isGoldenCross := previousMACD <= previousSignal && currentMACD > currentSignal
	isDeathCross := previousMACD >= previousSignal && currentMACD < currentSignal

	// 仅在0轴附近进行交易
	if macdCrossedAboveZero && adx > 28 {
		if isGoldenCross {
			// 多单
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
		}
	}

	if macdCrossedBelowZero && adx > 28 {
		if isDeathCross {
			// 空单
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
		}
	}

	return strategyPosition
}
