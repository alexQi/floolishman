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
	isUpperPinBar, isLowerPinBar := s.bactchCheckPinBar(df, 2, 0.85, false)

	openParams := map[string]interface{}{
		"prevPriceRate":         prevPriceRate,
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

	paramWeightMap := map[model.SideType]map[string]float64{
		model.SideTypeBuy: {
			"prevPriceRate":         0.18,
			"amplitude":             0.15,
			"isUpperPinBar":         -0.5,
			"isLowerPinBar":         0.10,
			"lastRsi":               0.30,
			"prevRsi":               0.25,
			"prevUpperPinRate":      -0.08,
			"prevLowerPinRate":      0.05,
			"upperShadowChangeRate": -0.10,
			"lowerShadowChangeRate": 0.20,
		},
		model.SideTypeSell: {
			"prevPriceRate":         0.18,
			"amplitude":             0.12,
			"isUpperPinBar":         0.18,
			"isLowerPinBar":         -0.05,
			"lastRsi":               0.28,
			"prevRsi":               0.22,
			"prevUpperPinRate":      0.15,
			"prevLowerPinRate":      -0.10,
			"upperShadowChangeRate": 0.10,
			"lowerShadowChangeRate": -0.12,
		},
	}

	// 设置评分机制
	var longScore, shortScore float64
	if isUpperPinBar && !isLowerPinBar && (prevRsi-lastRsi) > 8.5 && prevRsi > 55 && amplitude > 0.65 && upperShadowChangeRate > 0.5 && calc.Abs(prevPriceRate) > 0.001 {
		shortScore += (prevPriceRate / 0.01) * 100 * paramWeightMap[model.SideTypeSell]["prevPriceRate"]
		shortScore += ((lastRsi - 50) / 50) * 100 * paramWeightMap[model.SideTypeSell]["lastRsi"]
		shortScore += ((prevRsi - 50) / 50) * 100 * paramWeightMap[model.SideTypeSell]["prevRsi"]
		shortScore += (amplitude / 5) * 100 * paramWeightMap[model.SideTypeSell]["amplitude"]
		shortScore += prevUpperPinRate * 100 * paramWeightMap[model.SideTypeSell]["prevUpperPinRate"]
		shortScore += prevLowerPinRate * 100 * paramWeightMap[model.SideTypeSell]["prevLowerPinRate"]
		shortScore += upperShadowChangeRate * 100 * paramWeightMap[model.SideTypeSell]["upperShadowChangeRate"]
		shortScore += lowerShadowChangeRate * 100 * paramWeightMap[model.SideTypeSell]["lowerShadowChangeRate"]

		if isUpperPinBar {
			shortScore += 100 * paramWeightMap[model.SideTypeSell]["isUpperPinBar"]
		}
		if isLowerPinBar {
			shortScore += 100 * paramWeightMap[model.SideTypeSell]["isLowerPinBar"]
		}

		openParams["openScore"] = shortScore

		if shortScore > 0 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeSell)
			strategyPosition.Score = lastRsi

			if shortScore > 150 {
				strategyPosition.OpenPrice = lastPrice
			}
		}
	}

	if isLowerPinBar && !isUpperPinBar && (lastRsi-prevRsi) > 8.5 && prevRsi < 45 && amplitude > 0.65 && lowerShadowChangeRate > 0.5 && prevPriceRate > 0.001 {
		shortScore += (prevPriceRate / 0.01) * 100 * paramWeightMap[model.SideTypeSell]["prevPriceRate"]
		longScore += ((50 - lastRsi) / 50) * 100 * paramWeightMap[model.SideTypeBuy]["lastRsi"]
		longScore += ((50 - prevRsi) / 50) * 100 * paramWeightMap[model.SideTypeBuy]["prevRsi"]
		longScore += (amplitude / 5) * 100 * paramWeightMap[model.SideTypeBuy]["amplitude"]
		longScore += prevUpperPinRate * 100 * paramWeightMap[model.SideTypeBuy]["prevUpperPinRate"]
		longScore += prevLowerPinRate * 100 * paramWeightMap[model.SideTypeBuy]["prevLowerPinRate"]
		longScore += upperShadowChangeRate * 100 * paramWeightMap[model.SideTypeBuy]["upperShadowChangeRate"]
		longScore += lowerShadowChangeRate * 100 * paramWeightMap[model.SideTypeBuy]["lowerShadowChangeRate"]

		if isUpperPinBar {
			shortScore += 100 * paramWeightMap[model.SideTypeSell]["isUpperPinBar"]
		}
		if isLowerPinBar {
			shortScore += 100 * paramWeightMap[model.SideTypeSell]["isLowerPinBar"]
		}
		openParams["openScore"] = longScore

		if longScore > 0 {
			strategyPosition.Useable = 1
			strategyPosition.Side = string(model.SideTypeBuy)
			strategyPosition.Score = 100 - lastRsi

			if longScore > 150 {
				strategyPosition.OpenPrice = lastPrice
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
		utils.Log.Tracef("[PARAMS] PositionSide:%s, %s", strategyPosition.Side, strategyPosition.OpenParams)
	}

	return strategyPosition
}
