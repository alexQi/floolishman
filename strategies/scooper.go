package strategies

import (
	"encoding/json"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"fmt"
	"reflect"
)

type Scooper struct {
	BaseStrategy
}

// SortScore
func (s Scooper) SortScore() float64 {
	return 90
}

// Timeframe
func (s Scooper) Timeframe() string {
	return "30m"
}

func (s Scooper) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Scooper) Indicators(df *model.Dataframe) {
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
	upperPinRates, lowerPinRates, upperShadows, lowerShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowerPinRates"] = lowerPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowerShadows"] = lowerShadows
	// 计算MACD指标
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Scooper) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	lastPrice := df.Close.Last(0)
	prevPrice := df.Close.Last(1)

	prevHigh := df.High.Last(1)
	lastHigh := df.High.Last(0)

	prevLow := df.Low.Last(1)
	lastLow := df.Low.Last(0)

	prevBbUpper := df.Metadata["bbUpper"].Last(1)
	lastBbUpper := df.Metadata["bbUpper"].Last(0)
	prevBbLower := df.Metadata["bbLower"].Last(1)
	lastBbLower := df.Metadata["bbLower"].Last(0)

	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    lastPrice,
	}

	penuRsi := df.Metadata["rsi"].Last(2)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	prevPriceRate := calc.Abs(df.Metadata["priceRate"].Last(1))

	upperPinRates := df.Metadata["upperPinRates"]
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

	penuUpperPinRate := upperPinRates.Last(2)
	penuLowerPinRate := lowerPinRates.Last(2)
	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowerPinRates.Last(1)

	var upperShadowChangeRate, lowerShadowChangeRate float64
	lastUpperShadow := upperShadows.Last(0)
	lastLowerShadow := lowerShadows.Last(0)
	prevUpperShadow := upperShadows.Last(1)
	prevLowerShadow := lowerShadows.Last(1)
	if prevUpperShadow == 0 {
		upperShadowChangeRate = 0
	} else {
		upperShadowChangeRate = lastUpperShadow / prevUpperShadow
	}
	if prevLowerShadow == 0 {
		lowerShadowChangeRate = 0
	} else {
		lowerShadowChangeRate = lastLowerShadow / prevLowerShadow
	}

	penuAmplitude := indicator.AMP(df.Open.Last(2), df.High.Last(2), df.Low.Last(2))
	prevAmplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))

	isUpperPinBar, isLowerPinBar := s.batchCheckPinBar(df, 3, 0.65, false)

	openParams := map[string]interface{}{
		"prevPriceRate":         prevPriceRate,
		"prevPrice":             prevPrice,
		"lastPrice":             lastPrice,
		"isUpperPinBar":         isUpperPinBar,
		"isLowerPinBar":         isLowerPinBar,
		"lastRsi":               lastRsi,
		"prevRsi":               prevRsi,
		"penuRsi":               penuRsi,
		"penuAmplitude":         penuAmplitude,
		"prevAmplitude":         prevAmplitude,
		"penuUpperPinRate":      penuUpperPinRate,
		"penuLowerPinRate":      penuLowerPinRate,
		"prevUpperPinRate":      prevUpperPinRate,
		"prevLowerPinRate":      prevLowerPinRate,
		"upperShadowChangeRate": upperShadowChangeRate,
		"lowerShadowChangeRate": lowerShadowChangeRate,
		"openAt":                df.LastUpdate.In(Loc).Format("2006-01-02 15:04:05"),
	}

	var distanceRate float64
	if isUpperPinBar && prevAmplitude > 0.65 && prevRsi > 60 && prevUpperPinRate > 0.25 && prevPriceRate > 0.001 {
		lastRsiChange := prevRsi - lastRsi

		openParams["positionSide"] = string(model.SideTypeSell)
		// 上根蜡烛布林带突破情况 收盘，高点
		// 大于1，突破，反之没突破
		openParams["prevBollingCrossRate"] = prevHigh / prevBbUpper
		openParams["prevCloseCrossRate"] = prevPrice / prevBbUpper
		// 本根蜡烛布林带突破情况 收盘，高点
		openParams["lastBollingCrossRate"] = lastHigh / lastBbUpper
		openParams["lastCloseCrossRate"] = lastPrice / lastBbUpper
		// 前一个rsi增幅
		openParams["prevRsiChange"] = prevRsi - penuRsi
		// lastRsiChange
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeSell)
		strategyPosition.Score = lastRsi

		if prevRsi >= 74 && prevRsi < 80 && lastRsiChange > 9.4 && lastRsiChange < 11.2 {
			if prevAmplitude < 1.2 {
				if upperShadowChangeRate > 0.6 && prevAmplitude*prevUpperPinRate > 1.5 {
					distanceRate = 0.165
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if upperShadowChangeRate > 0.45 && prevAmplitude*prevUpperPinRate > 2.0 {
					distanceRate = 0.155
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if upperShadowChangeRate > 0.38 && prevAmplitude*prevUpperPinRate > 2.5 {
					distanceRate = 0.145
					strategyPosition.Useable = 1
				}
			} else {
				if upperShadowChangeRate > 0.30 && prevAmplitude*prevUpperPinRate > 3 {
					distanceRate = 0.135
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 80 && prevRsi < 86 && lastRsiChange > 7.6 && lastRsiChange < 9.4 {
			if prevAmplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && prevAmplitude*prevUpperPinRate > 0.42 {
					distanceRate = 0.135
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && prevAmplitude*prevUpperPinRate > 0.68 {
					distanceRate = 0.125
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && prevAmplitude*prevUpperPinRate > 1.2 {
					distanceRate = 0.115
					strategyPosition.Useable = 1
				}
			} else {
				if upperShadowChangeRate > 0.30 && prevAmplitude*prevUpperPinRate > 2 {
					distanceRate = 0.105
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 86 && prevRsi < 92 && lastRsiChange > 5.8 && lastRsiChange < 7.6 {
			if prevAmplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && prevAmplitude*prevUpperPinRate > 0.42 {
					distanceRate = 0.105
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && prevAmplitude*prevUpperPinRate > 0.68 {
					distanceRate = 0.095
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && prevAmplitude*prevUpperPinRate > 1.2 {
					distanceRate = 0.085
					strategyPosition.Useable = 1
				}
			} else {
				if upperShadowChangeRate > 0.30 && prevAmplitude*prevUpperPinRate > 2 {
					distanceRate = 0.075
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 92 && lastRsiChange > 4 && lastRsiChange < 5.8 {
			if prevAmplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && prevAmplitude*prevUpperPinRate > 0.91 {
					distanceRate = 0.075
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && prevAmplitude*prevUpperPinRate > 1.8 {
					distanceRate = 0.065
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && prevAmplitude*prevUpperPinRate > 2.0 {
					distanceRate = 0.055
					strategyPosition.Useable = 1
				}
			} else {
				if upperShadowChangeRate > 0.30 && prevAmplitude*prevUpperPinRate > 2.2 {
					distanceRate = 0.045
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
	}

	if isLowerPinBar && prevAmplitude > 0.65 && prevRsi < 40 && prevLowerPinRate > 0.25 && prevPriceRate > 0.001 {
		lastRsiChange := lastRsi - prevRsi

		openParams["positionSide"] = string(model.SideTypeBuy)
		// 上根蜡烛布林带突破情况 收盘，高点
		// 大于1，突破，反之没突破
		openParams["prevBollingCrossRate"] = prevBbLower / prevLow
		openParams["prevCloseCrossRate"] = prevBbLower / prevPrice
		// 本根蜡烛布林带突破情况 收盘，高点
		openParams["lastBollingCrossRate"] = lastBbLower / lastLow
		openParams["lastCloseCrossRate"] = lastBbLower / lastPrice
		// 前一个rsi增幅
		openParams["prevRsiChange"] = penuRsi - prevRsi
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeBuy)
		strategyPosition.Score = 100 - lastRsi

		if prevRsi >= 20 && prevRsi < 26 && lastRsiChange > 9.4 && lastRsiChange < 11.2 {
			if prevAmplitude < 1.2 {
				if lowerShadowChangeRate > 0.8 && prevAmplitude*prevLowerPinRate > 1.5 {
					distanceRate = 0.165
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if lowerShadowChangeRate > 0.75 && prevAmplitude*prevLowerPinRate > 2.0 {
					distanceRate = 0.155
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if lowerShadowChangeRate > 0.7 && prevAmplitude*prevLowerPinRate > 2.0 {
					distanceRate = 0.145
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.65 && prevAmplitude*prevLowerPinRate > 3 {
					distanceRate = 0.135
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 14 && prevRsi < 20 && lastRsiChange > 7.6 && lastRsiChange < 9.4 {
			if prevAmplitude < 1.2 {
				if lowerShadowChangeRate > 0.7 && prevAmplitude*prevLowerPinRate > 0.42 {
					distanceRate = 0.135
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if lowerShadowChangeRate > 0.65 && prevAmplitude*prevLowerPinRate > 0.68 {
					distanceRate = 0.125
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if lowerShadowChangeRate > 0.6 && prevAmplitude*prevLowerPinRate > 1.2 {
					distanceRate = 0.115
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.55 && prevAmplitude*prevLowerPinRate > 2 {
					distanceRate = 0.105
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 8 && prevRsi < 14 && lastRsiChange > 5.8 && lastRsiChange < 7.6 {
			if prevAmplitude < 1.2 {
				if lowerShadowChangeRate > 0.6 && prevAmplitude*prevLowerPinRate > 0.42 {
					distanceRate = 0.105
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if lowerShadowChangeRate > 0.55 && prevAmplitude*prevLowerPinRate > 0.68 {
					distanceRate = 0.095
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if lowerShadowChangeRate > 0.5 && prevAmplitude*prevLowerPinRate > 1.2 {
					distanceRate = 0.085
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.45 && prevAmplitude*prevLowerPinRate > 2 {
					distanceRate = 0.075
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi < 8 && lastRsiChange > 4 && lastRsiChange < 5.8 {
			if prevAmplitude < 1.2 {
				if lowerShadowChangeRate > 0.72 && prevAmplitude*prevLowerPinRate > 0.91 {
					distanceRate = 0.075
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 1.2 && prevAmplitude < 2.1 {
				if lowerShadowChangeRate > 0.66 && prevAmplitude*prevLowerPinRate > 1.8 {
					distanceRate = 0.065
					strategyPosition.Useable = 1
				}
			} else if prevAmplitude > 2.1 && prevAmplitude < 4.0 {
				if lowerShadowChangeRate > 0.60 && prevAmplitude*prevLowerPinRate > 2.0 {
					distanceRate = 0.055
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.30 && prevAmplitude*prevLowerPinRate > 2.2 {
					distanceRate = 0.045
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
	}

	// 将 map 转换为 JSON 字符串
	openParamsBytes, err := json.Marshal(openParams)
	if err != nil {
		fmt.Println("错误：", err)
	}

	strategyPosition.OpenParams = string(openParamsBytes)
	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(distanceRate, strategyPosition.OpenPrice, float64(option.Leverage))
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		utils.Log.Infof("[PARAMS] %s", strategyPosition.OpenParams)
	}

	return strategyPosition
}
