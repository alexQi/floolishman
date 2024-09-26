package strategies

import (
	"encoding/json"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils"
	"fmt"
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
	upperPinRates, lowerPinRates, upperShadows, lowerShadows := indicator.PinBars(df.Open, df.Close, df.High, df.Low)
	df.Metadata["upperPinRates"] = upperPinRates
	df.Metadata["lowerPinRates"] = lowerPinRates
	df.Metadata["upperShadows"] = upperShadows
	df.Metadata["lowerShadows"] = lowerShadows
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

func (s *Scoop) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
	strategyPosition := model.PositionStrategy{
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
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowerPinRates.Last(1)

	var upperShadowChangeRate, lowerShadowChangeRate float64
	prevUpperShadow := upperShadows.Last(1)
	prevLowerShadow := lowerShadows.Last(1)
	if prevUpperShadow == 0 {
		upperShadowChangeRate = 0
	} else {
		upperShadowChangeRate = upperShadows.Last(0) / prevUpperShadow
	}
	if prevLowerShadow == 0 {
		lowerShadowChangeRate = 0
	} else {
		lowerShadowChangeRate = lowerShadows.Last(0) / prevLowerShadow
	}

	amplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))
	isUpperPinBar, isLowerPinBar := s.batchCheckPinBar(df, 2, 1, false)

	openParams := map[string]interface{}{
		"prevPrice":             df.Close.Last(1),
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

	// 将 map 转换为 JSON 字符串
	openParamsBytes, err := json.Marshal(openParams)
	if err != nil {
		fmt.Println("错误：", err)
	}
	strategyPosition.OpenParams = string(openParamsBytes)

	if isUpperPinBar && !isLowerPinBar && (prevRsi-lastRsi) > 8.5 && amplitude > 0.65 && prevUpperPinRate > 0.25 {
		if prevRsi >= 80 && prevRsi < 86 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.55 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.2 {
				if upperShadowChangeRate > 0.5 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.2 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.45 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 86 && prevRsi < 92 {
			if amplitude < 1.2 {
				if upperShadowChangeRate > 0.55 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.1 {
				if upperShadowChangeRate > 0.45 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else if amplitude > 2.1 && amplitude < 4.0 {
				if upperShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
				}
			} else {
				if upperShadowChangeRate > 0.35 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeSell)
					strategyPosition.Score = lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 92 && upperShadowChangeRate > 0.3 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = lastRsi
			strategyPosition.OpenPrice = lastPrice
		}
	}

	if isLowerPinBar && !isUpperPinBar && (lastRsi-prevRsi) > 8.5 && amplitude > 0.65 && prevLowerPinRate > 0.25 {
		if prevRsi >= 18 && prevRsi < 24 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.55 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.4 {
				if lowerShadowChangeRate > 0.5 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 2.4 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.45 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi >= 10 && prevRsi < 18 {
			if amplitude < 1.2 {
				if lowerShadowChangeRate > 0.50 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 1.2 && amplitude < 2.4 {
				if lowerShadowChangeRate > 0.45 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else if amplitude > 2.4 && amplitude < 4.0 {
				if lowerShadowChangeRate > 0.4 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
				}
			} else {
				if lowerShadowChangeRate > 0.35 {
					strategyPosition.Useable = 1
					strategyPosition.Side = string(model.SideTypeBuy)
					strategyPosition.Score = 100 - lastRsi
					strategyPosition.OpenPrice = lastPrice
				}
			}
		}
		if prevRsi < 10 && lowerShadowChangeRate > 0.3 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = 100 - lastRsi
			strategyPosition.OpenPrice = lastPrice
		}
	}

	if strategyPosition.Useable > 0 {
		utils.Log.Infof("[PARAMS] PositionSide:%s, %s", strategyPosition.Side, strategyPosition.OpenParams)
	}

	return strategyPosition
}
