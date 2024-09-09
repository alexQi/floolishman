package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Scoop15m struct {
	BaseStrategy
}

func (s Scoop15m) SortScore() float64 {
	return 90
}

func (s Scoop15m) Timeframe() string {
	return "15m"
}

func (s Scoop15m) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Scoop15m) Indicators(df *model.Dataframe) {
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

func (s *Scoop15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
	}
	lastRsi := df.Metadata["rsi"].Last(0)
	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 0.85, false)

	// 判断实时数据死叉
	if macd.Crossunder(signal, 0) {
		// 获取上轨突破情况，反转做单
		hasCross, hasBack := s.checkBollingCross(df, 3, 1, "up")
		if hasCross && hasBack && !isLowerPinBar {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = lastRsi
		}
	}
	// 判断实时数据金叉
	if macd.Crossover(signal, 0) {
		// 获取下轨突破情况，反转做单
		hasCross, hasBack := s.checkBollingCross(df, 3, 1, "down")
		if hasCross && hasBack && !isUpperPinBar {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = 100 - lastRsi
		}
	}

	return strategyPosition
}
