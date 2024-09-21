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

type Radicalization struct {
	BaseStrategy
}

func (s Radicalization) SortScore() float64 {
	return 90
}

func (s Radicalization) Timeframe() string {
	return "30m"
}

func (s Radicalization) WarmupPeriod() int {
	return 96 // 预热期设定为50个数据点
}

func (s Radicalization) Indicators(df *model.Dataframe) {
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
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Radicalization) OnCandle(df *model.Dataframe) model.PositionStrategy {
	lastPrice := df.Close.Last(0)
	prevPrice := df.Close.Last(1)

	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    lastPrice,
	}

	//prevBbWidth := df.Metadata["bbWidth"].Last(1)
	//prevBbMiddle := df.Metadata["bbMiddle"].Last(1)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	prevPriceRate := calc.Abs(df.Metadata["priceRate"].Last(1))

	upperPinRates := df.Metadata["upperPinRates"]
	lowPinRates := df.Metadata["lowPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowShadows := df.Metadata["lowShadows"]

	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowPinRates.Last(1)

	var upperShadowChangeRate, lowerShadowChangeRate float64
	prevUpperShadow := upperShadows.Last(1)
	prevLowerShadow := lowShadows.Last(1)
	if prevUpperShadow == 0 {
		upperShadowChangeRate = 0
	} else {
		upperShadowChangeRate = upperShadows.Last(0) / prevUpperShadow
	}
	if prevLowerShadow == 0 {
		lowerShadowChangeRate = 0
	} else {
		lowerShadowChangeRate = lowShadows.Last(0) / prevLowerShadow
	}

	amplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))
	isUpperPinBar, isLowerPinBar := s.batchCheckPinBar(df, 2, 1.021, false)

	openParams := map[string]interface{}{
		"prevPriceRate":         prevPriceRate,
		"prevPrice":             prevPrice,
		"lastPrice":             lastPrice,
		"isUpperPinBar":         isUpperPinBar,
		"isLowerPinBar":         isLowerPinBar,
		"lastRsi":               lastRsi,
		"prevRsi":               prevRsi,
		"amplitude":             amplitude,
		"prevUpperPinRate":      prevUpperPinRate,
		"prevLowerPinRate":      prevLowerPinRate,
		"upperShadowChangeRate": upperShadowChangeRate,
		"lowerShadowChangeRate": lowerShadowChangeRate,
		"openAt":                df.LastUpdate.In(Loc).Format("2006-01-02 15:04:05"),
	}

	//scoreParams := map[string]float64{
	//	"prevPriceRate":         prevPriceRate / 0.01,
	//	"amplitude":             amplitude / 4,
	//	"lastRsi":               lastRsi - 50,
	//	"prevRsi":               prevRsi - 50,
	//	"prevUpperPinRate":      prevUpperPinRate,
	//	"prevLowerPinRate":      prevLowerPinRate,
	//	"upperShadowChangeRate": upperShadowChangeRate,
	//	"lowerShadowChangeRate": lowerShadowChangeRate,
	//}

	// 设置评分机制
	//var longScore, shortScore float64
	//if isUpperPinBar && !isLowerPinBar && (prevRsi-lastRsi) > 8.2 && prevRsi > 55 && amplitude > 0.65 && upperShadowChangeRate > 0.5 && calc.Abs(prevPriceRate) > 0.001 {
	//	scoreParams["lastRsi"] = (lastRsi - 50) / 50
	//	scoreParams["prevRsi"] = (prevRsi - 50) / 50
	//	shortScore = s.calculateScoreForWeight(model.SideTypeSell, scoreParams)
	//	if shortScore > 16 {
	//		strategyPosition.Score = shortScore
	//		strategyPosition.Side = string(model.SideTypeSell)
	//		strategyPosition.Useable = 1
	//	}
	//	openParams["openScore"] = shortScore
	//}
	//
	//if isLowerPinBar && !isUpperPinBar && (lastRsi-prevRsi) > 8.5 && prevRsi < 47 && amplitude > 0.66 && lowerShadowChangeRate > 0.45 && prevPriceRate > 0.001 {
	//	scoreParams["lastRsi"] = (50 - lastRsi) / 50
	//	scoreParams["prevRsi"] = (50 - prevRsi) / 50
	//	longScore = s.calculateScoreForWeight(model.SideTypeBuy, scoreParams)
	//	if longScore > 0 {
	//		strategyPosition.Side = string(model.SideTypeBuy)
	//		strategyPosition.Score = longScore
	//		strategyPosition.Useable = 1
	//	}
	//	openParams["openScore"] = longScore
	//}

	scoreParams := map[string]float64{
		//"amplitude":             amplitude / 4,
	}

	var paramScore, lastRsiRate, prevRsiRate, fronRsiRate float64
	if isUpperPinBar && (prevRsi-lastRsi) > 6.2 && prevUpperPinRate > 0.25 {
		lastRsiRate = (calc.Max(lastRsi, 50) - 50) / 42
		prevRsiRate = (calc.Max(prevRsi, 50) - 50) / 42
		if lastRsiRate > 0 {
			scoreParams["rsiChangeRate"] = lastRsiRate * prevRsiRate * fronRsiRate
		} else {
			scoreParams["rsiChangeRate"] = prevRsiRate * fronRsiRate
		}
		scoreParams["lastRsiRate"] = lastRsiRate
		scoreParams["prevRsiRate"] = prevRsiRate

		//scoreParams["pinRate"] = ((prevUpperPinRate - prevLowerPinRate) / prevUpperPinRate) * (prevPriceRate / 0.02)
		//scoreParams["lastRsiChangeRate"] = lastRsiRate * prevUpperPinRate * (prevPriceRate / 0.02)
		//scoreParams["prevRsiChangeRate"] = prevRsiRate * prevUpperPinRate * (prevPriceRate / 0.02)

		scoreParams["upperPinChangeRate"] = prevUpperPinRate * (prevPriceRate / 0.02) * (amplitude / 4)
		scoreParams["lowerPinChangeRate"] = prevLowerPinRate * (prevPriceRate / 0.02) * (amplitude / 4)

		paramScore = s.calculateScoreForWeight(model.SideTypeSell, scoreParams)

		openParams["openScore"] = paramScore

		strategyPosition.Side = string(model.SideTypeSell)
		strategyPosition.Score = paramScore

		if prevRsi >= 66 && prevRsi < 74 {
			//strategyPosition.Useable = 1
			if paramScore > 0.31 && upperShadowChangeRate > 1.0 {
				//strategyPosition.Useable = 1
			}
		} else if prevRsi >= 74 && prevRsi < 80 {
			//strategyPosition.Useable = 1
			if paramScore*upperShadowChangeRate > 0.7 {
				strategyPosition.Useable = 1
			}
		} else if prevRsi >= 80 && prevRsi < 86 {
			//strategyPosition.Useable = 1
			if paramScore*upperShadowChangeRate > 0.5 {
				//strategyPosition.Useable = 1
			}
		} else if prevRsi >= 86 && prevRsi < 92 {
			//strategyPosition.Useable = 1
			if paramScore > 0.60 && upperShadowChangeRate > 0.25 {
				//strategyPosition.Useable = 1
			}
		} else if prevRsi >= 92 {
			if upperShadowChangeRate > 0.90 {
				strategyPosition.Useable = 1
			}
		}
		//if prevRsi >= 74 && prevRsi < 80 {
		//	if amplitude < 1.4 {
		//		if upperShadowChangeRate > 0.6 && prevUpperPinRate > 2.8 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 1.4 && amplitude < 2.4 {
		//		if upperShadowChangeRate > 0.55 && prevUpperPinRate > 1.73 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2.4 && amplitude < 4.0 {
		//		if upperShadowChangeRate > 0.5 && prevUpperPinRate > 1.06 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if upperShadowChangeRate > 0.45 && prevUpperPinRate > 0.66 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//}
		//if prevRsi >= 80 && prevRsi < 86 {
		//	if amplitude < 1.2 {
		//		if upperShadowChangeRate > 0.55 && prevUpperPinRate > 1.73 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 1.2 && amplitude < 2.2 {
		//		if upperShadowChangeRate > 0.5 && prevUpperPinRate > 1.06 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2.2 && amplitude < 4.0 {
		//		if upperShadowChangeRate > 0.45 && prevUpperPinRate > 0.66 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if upperShadowChangeRate > 0.4 && prevUpperPinRate > 0.4 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//}
		//if prevRsi >= 86 && prevRsi < 92 {
		//	if amplitude < 1 {
		//		if upperShadowChangeRate > 0.55 && prevUpperPinRate > 1.06 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude >= 1 && amplitude < 2 {
		//		if upperShadowChangeRate > 0.45 && prevUpperPinRate > 0.66 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2 && amplitude < 4.0 {
		//		if upperShadowChangeRate > 0.4 && prevUpperPinRate > 0.4 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if upperShadowChangeRate > 0.35 && prevUpperPinRate > 0.25 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//}
		//if prevRsi >= 92 && upperShadowChangeRate > 0.3 && prevUpperPinRate > 0.15 {
		//	strategyPosition.Useable = 1
		//}
	}

	if isLowerPinBar && (lastRsi-prevRsi) > 10008.5 && amplitude > 0.65 && prevLowerPinRate > 0.25 && prevPriceRate > 0.001 {
		lastRsiRate = (50 - calc.Min(lastRsi, 50)) / 42
		prevRsiRate = (50 - calc.Min(prevRsi, 50)) / 42
		if lastRsiRate > 0 {
			scoreParams["rsiChangeRate"] = lastRsiRate * prevRsiRate
		} else {
			scoreParams["rsiChangeRate"] = prevRsiRate
		}

		scoreParams["pinRate"] = ((prevLowerPinRate - prevUpperPinRate) / prevLowerPinRate) * (prevPriceRate / 0.02)
		scoreParams["upperPinChangeRate"] = prevUpperPinRate * (prevPriceRate / 0.02) * (amplitude / 4)
		scoreParams["lowerPinChangeRate"] = prevLowerPinRate * (prevPriceRate / 0.02) * (amplitude / 4)
		scoreParams["lowerShadowChangeRate"] = lowerShadowChangeRate

		paramScore = s.calculateScoreForWeight(model.SideTypeBuy, scoreParams)

		openParams["openScore"] = paramScore

		strategyPosition.Side = string(model.SideTypeBuy)
		strategyPosition.Score = 100 - lastRsi

		if prevRsi >= 24 && prevRsi < 30 {
			strategyPosition.Useable = 1
		} else if prevRsi >= 18 && prevRsi < 24 {
			strategyPosition.Useable = 1
		} else if prevRsi >= 10 && prevRsi < 18 {
			strategyPosition.Useable = 1
		} else if prevRsi < 10 {
			strategyPosition.Useable = 1
		}

		//if prevRsi >= 24 && prevRsi < 30 {
		//	if amplitude < 1.6 {
		//		if lowerShadowChangeRate > 0.6 && prevLowerPinRate > 2.61 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 1.6 && amplitude < 2.8 {
		//		if lowerShadowChangeRate > 0.55 && prevLowerPinRate > 1.61 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2.8 && amplitude < 4.0 {
		//		if lowerShadowChangeRate > 0.5 && prevLowerPinRate > 1 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if lowerShadowChangeRate > 0.45 && prevLowerPinRate > 0.618 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//
		//	//if amplitude < 1.6 {
		//	//	if lowerShadowChangeRate > 0.6 && prevLowerPinRate > 2.61 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 1.6 && amplitude < 2.8 {
		//	//	if lowerShadowChangeRate > 0.55 && prevLowerPinRate > 1.61 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 2.8 && amplitude < 4.0 {
		//	//	if lowerShadowChangeRate > 0.5 && prevLowerPinRate > 1 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else {
		//	//	if lowerShadowChangeRate > 0.45 && prevLowerPinRate > 0.618 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//}
		//}
		//if prevRsi >= 18 && prevRsi < 24 {
		//	if amplitude < 1.2 {
		//		if lowerShadowChangeRate > 0.5 && prevLowerPinRate > 1.61 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 1.2 && amplitude < 2.4 {
		//		if lowerShadowChangeRate > 0.5 && prevLowerPinRate > 1 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2.4 && amplitude < 4.0 {
		//		if prevLowerPinRate > 0.618 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if prevLowerPinRate > 0.38 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//
		//	//if amplitude < 1.2 {
		//	//	if lowerShadowChangeRate > 0.55 && prevLowerPinRate > 1.61 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 1.2 && amplitude < 2.4 {
		//	//	if lowerShadowChangeRate > 0.5 && prevLowerPinRate > 1 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 2.4 && amplitude < 4.0 {
		//	//	if lowerShadowChangeRate > 0.45 && prevLowerPinRate > 0.618 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else {
		//	//	if lowerShadowChangeRate > 0.4 && prevLowerPinRate > 0.38 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//}
		//}
		//if prevRsi >= 10 && prevRsi < 18 {
		//	if amplitude < 1 {
		//		if prevLowerPinRate > 1 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 1 && amplitude < 2.2 {
		//		if prevLowerPinRate > 0.618 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else if amplitude > 2.2 && amplitude < 4.0 {
		//		if prevLowerPinRate > 0.38 {
		//			strategyPosition.Useable = 1
		//		}
		//	} else {
		//		if prevLowerPinRate > 0.234 {
		//			strategyPosition.Useable = 1
		//		}
		//	}
		//	//if amplitude < 1 {
		//	//	if lowerShadowChangeRate > 0.50 && prevLowerPinRate > 1 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 1 && amplitude < 2.2 {
		//	//	if lowerShadowChangeRate > 0.45 && prevLowerPinRate > 0.618 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else if amplitude > 2.2 && amplitude < 4.0 {
		//	//	if lowerShadowChangeRate > 0.4 && prevLowerPinRate > 0.38 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//} else {
		//	//	if lowerShadowChangeRate > 0.35 && prevLowerPinRate > 0.234 {
		//	//		strategyPosition.Useable = 1
		//	//	}
		//	//}
		//}
		//if prevRsi < 10 && lowerShadowChangeRate > 0.3 && prevLowerPinRate > 0.15 {
		//	strategyPosition.Useable = 1
		//}
	}

	// 将 map 转换为 JSON 字符串
	openParamsBytes, err := json.Marshal(openParams)
	if err != nil {
		fmt.Println("错误：", err)
	}

	strategyPosition.OpenParams = string(openParamsBytes)
	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(0.04, strategyPosition.OpenPrice, 20)
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		utils.Log.Tracef("[PARAMS] PositionSide:%s, openParams:%v,  weightParams: %v", strategyPosition.Side, strategyPosition.OpenParams, scoreParams)
	}

	return strategyPosition
}

func (s Radicalization) calculateScoreForWeight(side model.SideType, params map[string]float64) float64 {
	paramWeightMap := map[model.SideType]map[string]float64{
		model.SideTypeBuy: {
			"prevRsi":            28, // 上根蜡烛rsi
			"lowerPinChangeRate": 24, // 上根蜡烛下影线与蜡烛实体比
			//"amplitude":             22, // 振幅
			"lowerShadowChangeRate": 12, // 上根蜡烛下影线与当前蜡烛下影线比例
			"lastRsi":               8,  // 当前RSI

			"upperPinChangeRate":    -26, // 上根蜡烛上影线与蜡烛实体比
			"upperShadowChangeRate": -20, // 上根蜡烛上影线与当前蜡烛上影线比例
		},
		model.SideTypeSell: {
			"lastRsiRate":        0.36,
			"prevRsiRate":        0.31,
			"upperPinChangeRate": 0.25,
			"pinRate":            0.23,
			"lowerPinChangeRate": -0.21,
			//"upperShadowChangeRate": 0.13,

			//"lastRsiRate":           0.42,
			//"prevRsiRate":           0.35,
			//"upperPinChangeRate":    0.27,
			//"pinRate":               0.20,
			//"upperShadowChangeRate": 0.11,
			//"lowerShadowChangeRate": -0.08,
			//"lowerPinChangeRate":    -0.28,
			//"rsiChangeRate":         0.35, // 上根蜡烛rsi
			//"amplitude":             16, // 振幅
		},
	}
	var score float64
	for key, val := range paramWeightMap[side] {
		if _, ok := params[key]; !ok {
			continue
		}
		score += params[key] * val
		//utils.Log.Tracef("%s: %v * %v = %v, total: %v", key, params[key], val, params[key]*val, score)
	}
	//utils.Log.Tracef("------------\n")
	return score
}
