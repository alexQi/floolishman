package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils/calc"
	"reflect"
)

type Test15m struct {
	BaseStrategy
}

func (s Test15m) SortScore() int {
	return 90
}

func (s Test15m) Timeframe() string {
	return "15m"
}

func (s Test15m) WarmupPeriod() int {
	return 90
}

func (s Test15m) Indicators(df *model.Dataframe) {
	// 计算布林带指标
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 2.0)
	df.Metadata["bb_upper"] = bbUpper
	df.Metadata["bb_middle"] = bbMiddle
	df.Metadata["bb_lower"] = bbLower

	// 计算动量和交易量指标
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["avgVolume"] = indicator.EMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume

	// 计算ATR指标
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)

	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 12, 26, 9)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
}

func (s *Test15m) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
		LastAtr:      df.Metadata["atr"].Last(1),
	}

	// 获取指标数据
	momentums := df.Metadata["momentum"].LastValues(2)
	volume := df.Metadata["volume"].LastValues(3)
	avgVolume := df.Metadata["avgVolume"].LastValues(3)
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]

	// 判断是否有足够的数据
	if len(macd) < 2 || len(signal) < 2 || len(momentums) < 2 || len(volume) < 3 || len(avgVolume) < 3 {
		return strategyPosition
	}

	// 计算动量和交易量信号
	openPrice := df.Open.Last(0)
	closePrice := df.Close.Last(0)
	momentumsDistance := momentums[1] - momentums[0]
	isCross, _ := s.bactchCheckVolume(volume, avgVolume, 2)
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 1.2)

	// 获取MACD信号
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

	// 趋势判断和交易信号
	if macdCrossedAboveZero && isGoldenCross && momentumsDistance > 7 && momentums[1] < 35 && isCross && closePrice > openPrice && !isUpperPinBar {
		// 多单
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	if macdCrossedBelowZero && isDeathCross && momentumsDistance < 0 && calc.Abs(momentumsDistance) > 7 && calc.Abs(momentums[1]) < 18 && isCross && openPrice > closePrice && !isLowerPinBar {
		// 空单
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
