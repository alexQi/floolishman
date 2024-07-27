package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Macd4h struct {
	BaseStrategy
}

func (s Macd4h) SortScore() int {
	return 90
}

func (s Macd4h) Timeframe() string {
	return "4h"
}

func (s Macd4h) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Macd4h) Indicators(df *model.Dataframe) {
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 12, 26, 9)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
}

func (s *Macd4h) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]

	if len(macd) < 2 || len(signal) < 2 {
		return strategyPosition
	}

	previousMACD := macd[len(macd)-2]
	currentMACD := macd[len(macd)-1]
	previousSignal := signal[len(signal)-2]
	currentSignal := signal[len(signal)-1]

	// 判断MACD是否穿越0轴
	macdCrossedAboveZero := previousMACD < 0 && currentMACD > 0
	macdCrossedBelowZero := previousMACD > 0 && currentMACD < 0

	// 判断金叉和死叉
	isGoldenCross := previousMACD <= previousSignal && currentMACD > currentSignal
	isDeathCross := previousMACD >= previousSignal && currentMACD < currentSignal

	// 仅在0轴附近进行交易
	if macdCrossedAboveZero {
		if isGoldenCross {
			// 多单
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
		}
	}

	if macdCrossedBelowZero {
		if isDeathCross {
			// 空单
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
		}
	}

	return strategyPosition
}
