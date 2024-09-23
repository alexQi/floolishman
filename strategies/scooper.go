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

func (s Scooper) SortScore() float64 {
	return 90
}

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

func (s *Scooper) OnCandle(df *model.Dataframe) model.PositionStrategy {
	lastPrice := df.Close.Last(0)
	prevPrice := df.Close.Last(1)

	strategyPosition := model.PositionStrategy{
		Tendency:     s.checkMarketTendency(df),
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		LastAtr:      df.Metadata["atr"].Last(1) * 1.5,
		OpenPrice:    lastPrice,
	}

	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	prevPriceRate := calc.Abs(df.Metadata["priceRate"].Last(1))

	upperPinRates := df.Metadata["upperPinRates"]
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

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

	amplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))
	isUpperPinBar, isLowerPinBar := s.batchCheckPinBar(df, 3, 0.65, false)

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

	if isUpperPinBar && amplitude > 0.65 && prevUpperPinRate > 0.25 && prevPriceRate > 0.001 {
		rsiChange := prevRsi - lastRsi
		strategyPosition.Side = string(model.SideTypeSell)
		strategyPosition.Score = lastRsi

		if prevRsi >= 74 && prevRsi < 80 && rsiChange > 8.5 && rsiChange < 12.5 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.6 && amplitude*prevUpperPinRate > 1.5 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.45 && amplitude*prevUpperPinRate > 2.0 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.38 && amplitude*prevUpperPinRate > 2.5 {
					strategyPosition.Useable = 1
				}
			} else {
				if amplitude*prevUpperPinRate > 3 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 80 && prevRsi < 86 && rsiChange > 7 && rsiChange < 11 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && amplitude*prevUpperPinRate > 0.42 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && amplitude*prevUpperPinRate > 0.68 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && amplitude*prevUpperPinRate > 1.2 {
					strategyPosition.Useable = 1
				}
			} else {
				if amplitude*prevUpperPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 86 && prevRsi < 92 && rsiChange > 5.5 && rsiChange < 9.5 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && amplitude*prevUpperPinRate > 0.42 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && amplitude*prevUpperPinRate > 0.68 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && amplitude*prevUpperPinRate > 1.2 {
					strategyPosition.Useable = 1
				}
			} else {
				if amplitude*prevUpperPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 92 && rsiChange > 4 && rsiChange < 8 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.72 && amplitude*prevUpperPinRate > 0.91 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.66 && amplitude*prevUpperPinRate > 1.8 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.60 && amplitude*prevUpperPinRate > 2.0 {
					strategyPosition.Useable = 1
				}
			} else {
				if upperShadowChangeRate > 0.30 && amplitude*prevUpperPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
	}

	if isLowerPinBar && amplitude > 0.65 && prevLowerPinRate > 0.25 && prevPriceRate > 0.001 {
		rsiChange := lastRsi - prevRsi

		strategyPosition.Side = string(model.SideTypeBuy)
		strategyPosition.Score = 100 - lastRsi

		if prevRsi >= 20 && prevRsi < 26 && rsiChange > 8.5 && rsiChange < 12.5 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.8 && amplitude*prevLowerPinRate > 0.42 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.75 && amplitude*prevLowerPinRate > 0.68 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.7 && amplitude*prevLowerPinRate > 3 {
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.65 && amplitude*prevLowerPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 14 && prevRsi < 20 && rsiChange > 7 && rsiChange < 11 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.7 && amplitude*prevLowerPinRate > 0.42 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.65 && amplitude*prevLowerPinRate > 0.68 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.6 && amplitude*prevLowerPinRate > 3 {
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.55 && amplitude*prevLowerPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 8 && prevRsi < 14 && rsiChange > 5.5 && rsiChange < 9.5 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.6 && amplitude*prevLowerPinRate > 0.42 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.55 && amplitude*prevLowerPinRate > 0.68 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.5 && amplitude*prevLowerPinRate > 3 {
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.45 && amplitude*prevLowerPinRate > 2 {
					strategyPosition.Useable = 1
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi < 8 && rsiChange > 4 && rsiChange < 8 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.72 && amplitude*prevLowerPinRate > 0.91 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if lowerShadowChangeRate > 0.66 && amplitude*prevLowerPinRate > 1.8 {
					strategyPosition.Useable = 1
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.60 && amplitude*prevLowerPinRate > 2.0 {
					strategyPosition.Useable = 1
				}
			} else {
				if lowerShadowChangeRate > 0.3 && amplitude*prevLowerPinRate > 2 {
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
		stopLossDistance := calc.StopLossDistance(0.04, strategyPosition.OpenPrice, 20)
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		utils.Log.Tracef("[PARAMS] PositionSide:%s, %s", strategyPosition.Side, strategyPosition.OpenParams)
	}

	return strategyPosition
}
