package strategies

import (
	"encoding/json"
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils"
	"floolishman/utils/calc"
	"math"
	"reflect"
)

type Radicalization struct {
	BaseStrategy
}

// SortScore
func (s Radicalization) SortScore() float64 {
	return 90
}

// Timeframe
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
	// 其他指标
	df.Metadata["priceRate"] = indicator.PriceRate(df.Open, df.Close)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 7)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)
	df.Metadata["adx"] = indicator.ADX(df.High, df.Low, df.Close, 14)
	df.Metadata["tendency"] = indicator.TendencyAngles(bbMiddle, 5)
}

func (s *Radicalization) OnCandle(option *model.PairOption, df *model.Dataframe) model.PositionStrategy {
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

	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	upperPinRates := df.Metadata["upperPinRates"]
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

	prevSignal := signal.Last(1)
	prevMacd := macd.Last(1)
	penuUpperPinRate := upperPinRates.Last(2)
	penuLowerPinRate := lowerPinRates.Last(2)
	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowerPinRates.Last(1)
	lastUpperPinRate := upperPinRates.Last(0)
	lastLowerPinRate := lowerPinRates.Last(0)

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
	prevMacdDiffRate := (prevMacd - prevSignal) / prevSignal

	penuAmplitude := indicator.AMP(df.Open.Last(2), df.High.Last(2), df.Low.Last(2))
	prevAmplitude := indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1))

	openParams := map[string]interface{}{
		"prevPriceRate":    prevPriceRate / 0.02,
		"prevMacdDiffRate": prevMacdDiffRate,
		"prevPinLenRate":   calc.Abs(prevUpperPinRate - prevLowerPinRate),

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

		"prevPrice": prevPrice,
		"lastPrice": lastPrice,
	}

	var lastRsiChange, rsiSeedRate, decayFactorFloor, decayFactorAmplitude, decayFactorDistance, floor, upper, distanceRate, limitShadowChangeRate float64

	floorK := 1.077993
	distanceK := 1.445
	shadowK := 0.4405
	amplitudeK := 1.1601
	datum := 1.2
	deltaRsiRatio := 0.1
	baseFloor := 10.0
	baseDistanceRate := 0.275

	if prevRsi > 50 && prevRsi > lastRsi && prevPriceRate*prevUpperPinRate > 0.00125 {
		lastRsiChange = prevRsi - lastRsi
		rsiSeedRate = (prevRsi - 50.0) / 50.0

		openParams["positionSide"] = string(model.SideTypeSell)
		openParams["prevBollingCrossRate"] = prevHigh / prevBbUpper
		openParams["prevCloseCrossRate"] = prevPrice / prevBbUpper
		openParams["lastBollingCrossRate"] = lastHigh / lastBbUpper
		openParams["lastCloseCrossRate"] = lastPrice / lastBbUpper
		openParams["prevPricePinRate"] = prevPriceRate / 0.02 * prevUpperPinRate
		openParams["lastShadowChangeRate"] = upperShadowChangeRate
		if penuLowerPinRate > 0 {
			openParams["penuPinDiffRate"] = penuUpperPinRate / penuLowerPinRate
		} else {
			openParams["penuPinDiffRate"] = 0
		}
		if penuLowerPinRate > 0 {
			openParams["prevPinDiffRate"] = prevUpperPinRate / prevLowerPinRate
		} else {
			openParams["prevPinDiffRate"] = 0
		}
		if penuLowerPinRate > 0 {
			openParams["lastPinDiffRate"] = lastUpperPinRate / lastLowerPinRate
		} else {
			openParams["lastPinDiffRate"] = 0
		}
		openParams["lastRsiExtreme"] = (lastRsi - 50.0) / 50.0
		openParams["prevRsiExtreme"] = rsiSeedRate
		openParams["penuRsiExtreme"] = (penuRsi - 50.0) / 50.0
		openParams["prevAmplitudePinRate"] = prevAmplitude * prevUpperPinRate
		openParams["prevReserveAmplitudePinRate"] = prevAmplitude * prevLowerPinRate

		openParams["prevRsiChange"] = prevRsi - penuRsi
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeSell)

		decayFactorFloor = calc.CalculateFactor(rsiSeedRate, floorK)
		decayFactorAmplitude = calc.CalculateFactor(prevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(rsiSeedRate, distanceK-decayFactorAmplitude)

		floor = decayFactorFloor * baseFloor
		upper = math.Exp(floorK*deltaRsiRatio) * floor
		distanceRate = decayFactorDistance * baseDistanceRate
		limitShadowChangeRate = calc.CalculateRate(prevAmplitude*rsiSeedRate, datum, shadowK)

		if lastRsiChange > floor &&
			lastRsiChange < upper {
			if upperShadowChangeRate > limitShadowChangeRate {
				strategyPosition.Useable = 1
				strategyPosition.Score = 100 * rsiSeedRate
			}
		}
	}
	if prevRsi < 50 && lastRsi > prevRsi && prevPriceRate*prevUpperPinRate > 0.00125 {
		lastRsiChange = lastRsi - prevRsi
		rsiSeedRate = (50 - prevRsi) / 50

		openParams["positionSide"] = string(model.SideTypeBuy)
		openParams["prevBollingCrossRate"] = prevBbLower / prevLow
		openParams["prevCloseCrossRate"] = prevBbLower / prevPrice
		openParams["lastBollingCrossRate"] = lastBbLower / lastLow
		openParams["lastCloseCrossRate"] = lastBbLower / lastPrice
		openParams["prevPricePinRate"] = prevPriceRate / 0.02 * prevUpperPinRate
		openParams["lastShadowChangeRate"] = lowerShadowChangeRate
		if penuLowerPinRate > 0 {
			openParams["penuPinDiffRate"] = penuLowerPinRate / penuUpperPinRate
		} else {
			openParams["penuPinDiffRate"] = 0
		}
		if penuLowerPinRate > 0 {
			openParams["prevPinDiffRate"] = prevLowerPinRate / prevUpperPinRate
		} else {
			openParams["prevPinDiffRate"] = 0
		}
		if penuLowerPinRate > 0 {
			openParams["lastPinDiffRate"] = lastLowerPinRate / lastUpperPinRate
		} else {
			openParams["lastPinDiffRate"] = 0
		}
		openParams["lastRsiExtreme"] = (50.0 - lastRsi) / 50.0
		openParams["prevRsiExtreme"] = rsiSeedRate
		openParams["penuRsiExtreme"] = (50.0 / penuRsi) / 50.0
		openParams["prevAmplitudePinRate"] = prevAmplitude * prevLowerPinRate
		openParams["prevReserveAmplitudePinRate"] = prevAmplitude * prevUpperPinRate

		openParams["prevRsiChange"] = penuRsi - prevRsi
		openParams["lastRsiChange"] = lastRsiChange

		strategyPosition.Side = string(model.SideTypeBuy)

		decayFactorFloor = calc.CalculateFactor(rsiSeedRate, floorK)
		decayFactorAmplitude = calc.CalculateFactor(prevAmplitude, amplitudeK)
		decayFactorDistance = calc.CalculateFactor(rsiSeedRate, distanceK-decayFactorAmplitude)

		floor = decayFactorFloor * baseFloor
		upper = math.Exp(floorK*deltaRsiRatio) * floor
		distanceRate = decayFactorDistance * baseDistanceRate
		limitShadowChangeRate = calc.CalculateRate(prevAmplitude*rsiSeedRate, datum, shadowK)

		if lastRsiChange > floor &&
			lastRsiChange < upper {
			if lowerShadowChangeRate > limitShadowChangeRate {
				strategyPosition.Useable = 1
				strategyPosition.Score = 100 * rsiSeedRate
			}
		}
	}

	if strategyPosition.Useable > 0 {
		stopLossDistance := calc.StopLossDistance(distanceRate, strategyPosition.OpenPrice, float64(option.Leverage))
		if strategyPosition.Side == string(model.SideTypeBuy) {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice - stopLossDistance
		} else {
			strategyPosition.OpenPrice = strategyPosition.OpenPrice + stopLossDistance
		}
		openParams["floor"] = floor
		openParams["upper"] = upper
		openParams["limitShadowChangeRate"] = limitShadowChangeRate
		openParams["distanceRate"] = distanceRate
		openParams["openPrice"] = strategyPosition.OpenPrice
		// 将 map 转换为 JSON 字符串
		openParamsBytes, err := json.Marshal(openParams)
		if err != nil {
			utils.Log.Error("错误：", err)
		}
		strategyPosition.OpenParams = string(openParamsBytes)
		utils.Log.Tracef("[PARAMS] %s", strategyPosition.OpenParams)
	}

	return strategyPosition
}
