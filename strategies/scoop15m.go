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
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower
	df.Metadata["bbWidth"] = bbWidth
	// 检查插针
	upperPinRates, lowPinRates, upperShadows, lowShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowPinRates"] = lowPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowShadows"] = lowShadows
	// 计算MACD指标
	macdLine, signalLine, hist := indicator.MACD(df.Close, 8, 17, 5)
	df.Metadata["macd"] = macdLine
	df.Metadata["signal"] = signalLine
	df.Metadata["hist"] = hist
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Scoop15m) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
	}

	//prevBbWidth := df.Metadata["bbWidth"].Last(1)
	//prevBbMiddle := df.Metadata["bbMiddle"].Last(1)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	upperPinRates := df.Metadata["upperPinRates"]
	lowPinRates := df.Metadata["lowPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowShadows := df.Metadata["lowShadows"]

	prevUpperPinRate := upperPinRates.Last(1)
	prevLowPinRate := lowPinRates.Last(1)
	upperShadowChangeRate := upperShadows.Last(0) / upperShadows.Last(1)
	lowerShadowChangeRate := lowShadows.Last(0) / lowShadows.Last(1)

	//prevBollWidthRatio := 100 * prevBbWidth / prevBbMiddle
	//var lastIsUpperPinBar, lastIsLowerPinBar bool

	amplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 0.85, false)
	//lastIsUpperPinBar, lastIsLowerPinBar, _, _ = calc.CheckPinBar(0.5, 4, 0, df.Open.Last(0), df.Close.Last(0), df.High.Last(0), df.Low.Last(0))

	if isUpperPinBar && (lastRsi-prevRsi) > 10 && amplitude > 0.5 && prevUpperPinRate > 0.1 {
		if prevRsi >= 75 && prevRsi < 80 {
			if amplitude < 1.0 {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else {
				strategyPosition.Useable = 1
				strategyPosition.Side = string(model.SideTypeSell)
				strategyPosition.Score = lastRsi
			}
		}
		if prevRsi >= 80 && prevRsi < 90 {
			if amplitude < 1.0 {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else {
				strategyPosition.Useable = 1
				strategyPosition.Side = string(model.SideTypeSell)
				strategyPosition.Score = lastRsi
			}
		}
		if prevRsi >= 90 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = lastRsi
		}
	}

	if isLowerPinBar && (lastRsi-prevRsi) > 10 && amplitude > 0.5 && prevLowPinRate > 0.1 {
		if prevRsi >= 20 && prevRsi < 25 {
			if amplitude < 1.0 {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else {
				strategyPosition.Useable = 1
				strategyPosition.Side = string(model.SideTypeBuy)
				strategyPosition.Score = lastRsi
			}
		}
		if prevRsi >= 12 && prevRsi < 20 {
			if amplitude < 1.0 {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = lastRsi
				}
			} else {
				strategyPosition.Useable = 1
				strategyPosition.Side = string(model.SideTypeBuy)
				strategyPosition.Score = lastRsi
			}
		}
		if prevRsi < 12 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = lastRsi
		}
		// 先根据rsi划分区间 12-25、12
		//if prevRsi >= 12 && prevRsi < 22 && lastIsLowerPinBar {
		//	strategyPosition.Useable = 1
		//	strategyPosition.Side = string(model.SideTypeBuy)
		//	strategyPosition.Score = lastRsi
		//}
		//if prevRsi < 12 {
		//	// 先根据rsi划分区间 12-25、12
		//	if amplitude < 2.2 {
		//		strategyPosition.Useable = 1
		//		strategyPosition.Side = string(model.SideTypeBuy)
		//		strategyPosition.Score = lastRsi
		//	}
		//	if amplitude > 2.2 && lastIsLowerPinBar {
		//		strategyPosition.Useable = 1
		//		strategyPosition.Side = string(model.SideTypeBuy)
		//		strategyPosition.Score = lastRsi
		//	}
		//}
	}

	return strategyPosition
}
