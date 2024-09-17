package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"reflect"
)

type Scoop struct {
	BaseStrategy
}

func (s Scoop) SortScore() float64 {
	return 90
}

func (s Scoop) Timeframe() string {
	return "30m"
}

func (s Scoop) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Scoop) Indicators(df *model.Dataframe) {
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

func (s *Scoop) OnCandle(df *model.Dataframe) model.Strategy {
	strategyPosition := model.Strategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    df.Close.Last(1),
	}

	lastPrice := df.Close.Last(0)
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

	amplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 1, false)

	if isUpperPinBar && (lastRsi-prevRsi) > 10 && amplitude > 0.65 && prevUpperPinRate > 0.25 {
		if prevRsi >= 75 && prevRsi < 80 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.5 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			} else {
				if upperShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 85 && prevRsi < 92.5 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
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
					strategyPosition.OpenPrice = lastPrice
				}
			} else {
				if upperShadowChangeRate > 0.15 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 92.5 && upperShadowChangeRate > 0.1 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = lastRsi
			strategyPosition.OpenPrice = lastPrice
		}
	}

	if isLowerPinBar && (lastRsi-prevRsi) > 10 && amplitude > 0.65 && prevLowPinRate > 0.25 {
		if prevRsi >= 18 && prevRsi < 24 {
			if amplitude < 1.0 {
				if lowerShadowChangeRate > 0.5 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.5 {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 2.5 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			} else {
				if lowerShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 10 && prevRsi < 17 {
			if amplitude < 1.0 {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 1 && amplitude < 2.5 {
				if lowerShadowChangeRate > 0.3 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 2.5 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.2 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			} else {
				if lowerShadowChangeRate > 0.15 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi < 10 && lowerShadowChangeRate > 0.1 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = 100 - lastRsi
			strategyPosition.OpenPrice = lastPrice
		}
	}

	return strategyPosition
}
