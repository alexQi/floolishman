package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
	"time"
)

type BaseStrategy struct {
}

var Loc *time.Location

func init() {
	Loc, _ = time.LoadLocation("Asia/Shanghai")
}
func (bs *BaseStrategy) handleIndicatos(df *model.Dataframe) error {
	bbUpper, bbMiddle, bbLower := indicator.BB(df.Close, 21, 2.0, 0)
	// 计算布林带宽度
	bbWidth := make([]float64, len(bbUpper))
	for i := 0; i < len(bbUpper); i++ {
		bbWidth[i] = bbUpper[i] - bbLower[i]
	}
	changeRates := make([]float64, len(bbWidth)-1)
	for i := 1; i < len(bbWidth); i++ {
		changeRates[i-1] = (bbWidth[i] - bbWidth[i-1]) / bbWidth[i-1]
	}

	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["ema21"] = indicator.SMA(df.Close, 21)
	df.Metadata["momentum"] = indicator.Momentum(df.Close, 14)
	df.Metadata["rsi"] = indicator.RSI(df.Close, 6)
	df.Metadata["avgVolume"] = indicator.SMA(df.Volume, 14)
	df.Metadata["volume"] = df.Volume
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)

	df.Metadata["bbUpper"] = bbUpper
	df.Metadata["bbMiddle"] = bbMiddle
	df.Metadata["bbLower"] = bbLower

	df.Metadata["bbWidth"] = bbWidth
	df.Metadata["bb_change_rate"] = changeRates

	return nil
}

func (bs *BaseStrategy) checkMarketTendency(df *model.Dataframe) string {
	tendency := df.Metadata["tendency"].Last(0)
	if calc.Abs(tendency) > 8 {
		if tendency > 0 {
			return "rise"
		} else {
			return "down"
		}
	}
	return "range"
}

func (bs *BaseStrategy) bactchCheckVolume(volume, avgVolume []float64, weight float64) (bool, string) {
	isCross := false
	for i := 0; i < len(volume); i++ {
		if volume[i] > avgVolume[i]*weight {
			isCross = true
		}
	}
	isIncreasing := true
	isDecreasing := true
	for i := 1; i < len(volume); i++ {
		if volume[i] > volume[i-1] {
			isDecreasing = false
		} else if volume[i] < volume[i-1] {
			isIncreasing = false
		}
		if !isIncreasing && !isDecreasing {
			break
		}
	}
	direction := "range"
	if isIncreasing {
		direction = "rise"
	} else if isDecreasing {
		direction = "fall"
	}
	return isCross, direction
}

func (bs *BaseStrategy) batchCheckPinBar(df *model.Dataframe, count int, weight float64, includeLatest bool) (bool, bool) {
	var upperPinRates, lowerPinRates, upperShadows, lowerShadows []float64
	if includeLatest {
		upperPinRates = df.Metadata["upperPinRates"].LastValues(count)
		lowerPinRates = df.Metadata["lowerPinRates"].LastValues(count)
		upperShadows = df.Metadata["upperShadows"].LastValues(count)
		lowerShadows = df.Metadata["lowerShadows"].LastValues(count)
	} else {
		upperPinRates = df.Metadata["upperPinRates"].GetLastValues(count, 1)
		lowerPinRates = df.Metadata["lowerPinRates"].GetLastValues(count, 1)
		upperShadows = df.Metadata["upperShadows"].GetLastValues(count, 1)
		lowerShadows = df.Metadata["lowerShadows"].GetLastValues(count, 1)
	}

	var upperPin, lowerPin int
	for i := 0; i < count; i++ {
		if upperPinRates[i] > weight && upperShadows[i] > lowerShadows[i] {
			upperPin += 1
		}
		if lowerPinRates[i] > weight && upperShadows[i] < lowerShadows[i] {
			lowerPin += 1
		}
	}
	var hasUpperPin, hasLowerPin bool
	if upperPin >= count/2 {
		hasUpperPin = true
	}
	if lowerPin >= count/2 {
		hasLowerPin = true
	}
	return hasUpperPin, hasLowerPin
}

func (bs *BaseStrategy) getCandleColor(open, close float64) string {
	// 获取蜡烛颜色
	if close > open {
		return "bullish" // 阳线
	} else if close < open {
		return "bearish" // 阴线
	}
	return "neutral" // 十字线
}

func (bs *BaseStrategy) checkCandleTendency(historyOpens, historyCloses []float64, count int, part int) string {
	tendency := "neutral"
	// 检查数据长度是否足够
	if len(historyOpens) < count || len(historyCloses) < count {
		return tendency
	}
	historyCandleColors := []string{}
	for i := 0; i < count; i++ {
		historyCandleColors = append(historyCandleColors, bs.getCandleColor(historyOpens[i], historyCloses[i]))
	}
	historyColorCount := map[string]float64{}
	for _, color := range historyCandleColors {
		if _, ok := historyColorCount[color]; ok {
			historyColorCount[color] += 1.0
		} else {
			historyColorCount[color] = 1.0
		}
	}
	if historyColorCount["bullish"] >= float64(count)/float64(part) {
		return "bullish"
	}
	if historyColorCount["bearish"] >= float64(count)/float64(part) {
		return "bearish"
	}
	return tendency
}

func (bs *BaseStrategy) checkBollingNearCross(df *model.Dataframe, period int, endIndex int, position string) (bool, bool) {
	isNearBand := true      // 默认设为true，表示所有蜡烛都未突破布林带
	isCrossAndBack := false // 检测到突破后回归的信号
	var high, low, limits []float64
	closes := df.Close.GetLastValues(period, endIndex)
	if position == "up" {
		// 获取布林带上轨、高点和收盘价序列
		limits = df.Metadata["bbUpper"].GetLastValues(period, endIndex)
		high = df.High.GetLastValues(period, endIndex)

		for i, price := range high {
			// 如果当前价格突破了布林带上轨
			if price > limits[i] {
				isNearBand = false // 如果任何一根蜡烛突破，取消靠近布林带的信号
				// 检查收盘价是否回归到布林带内
				if closes[i] < limits[i] {
					isCrossAndBack = true
				}
			}
		}
	} else {
		// 获取布林带下轨、低点和收盘价序列
		limits = df.Metadata["bbLower"].GetLastValues(period, endIndex)
		low = df.Low.GetLastValues(period, endIndex)

		for i, price := range low {
			// 如果当前价格突破了布林带下轨
			if price < limits[i] {
				isNearBand = false // 如果任何一根蜡烛突破，取消靠近布林带的信号
				// 检查收盘价是否回归到布林带内
				if closes[i] > limits[i] {
					isCrossAndBack = true
				}
			}
		}
	}

	return isNearBand, isCrossAndBack
}

func (bs *BaseStrategy) extraFeatures(df *model.Dataframe) *model.StrategyFeature {
	// 获取必要的市场数据
	lastPrice := df.Close.Last(0)
	prevPrice := df.Close.Last(1)
	penuPrie := df.Close.Last(2)

	bbUpper := df.Metadata["bbUpper"]
	bbLower := df.Metadata["bbUpper"]
	bbMiddle := df.Metadata["bbMiddle"]

	befoRsi := df.Metadata["rsi"].Last(3)
	penuRsi := df.Metadata["rsi"].Last(2)
	prevRsi := df.Metadata["rsi"].Last(1)
	lastRsi := df.Metadata["rsi"].Last(0)

	prevAvgVolume := df.Metadata["avgVolume"].Last(1)
	penuAvgVolume := df.Metadata["avgVolume"].Last(2)

	// 实际成交量
	prevVolume := df.Volume.Last(1)
	penuVolume := df.Volume.Last(2)

	penuPriceRate := df.Metadata["priceRate"].Last(2)
	prevPriceRate := df.Metadata["priceRate"].Last(1)

	macd := df.Metadata["macd"]
	signal := df.Metadata["signal"]
	upperPinRates := df.Metadata["upperPinRates"]
	lowerPinRates := df.Metadata["lowerPinRates"]
	upperShadows := df.Metadata["upperShadows"]
	lowerShadows := df.Metadata["lowerShadows"]

	penuUpperPinRate := upperPinRates.Last(2)
	penuLowerPinRate := lowerPinRates.Last(2)
	prevUpperPinRate := upperPinRates.Last(1)
	prevLowerPinRate := lowerPinRates.Last(1)
	lastUpperPinRate := upperPinRates.Last(0)
	lastLowerPinRate := lowerPinRates.Last(0)

	// 初始化一些参数
	var lastUpperShadowChangeRate, prevUpperShadowChangeRate, penuUpperShadowChangeRate, lastLowerShadowChangeRate, prevLowerShadowChangeRate, penuLowerShadowChangeRate, lastShadowRate, prevShadowRate, penuShadowRate float64
	lastUpperShadow := upperShadows.Last(0)
	lastLowerShadow := lowerShadows.Last(0)
	prevUpperShadow := upperShadows.Last(1)
	prevLowerShadow := lowerShadows.Last(1)
	penuUpperShadow := upperShadows.Last(2)
	penuLowerShadow := lowerShadows.Last(2)
	befoUpperShadow := upperShadows.Last(3)
	befoLowerShadow := lowerShadows.Last(3)
	// 计算影线变化率 -- upper
	if prevUpperShadow == 0 {
		lastUpperShadowChangeRate = 0
	} else {
		lastUpperShadowChangeRate = lastUpperShadow / prevUpperShadow
	}
	if penuUpperShadow == 0 {
		prevUpperShadowChangeRate = 0
	} else {
		prevUpperShadowChangeRate = prevUpperShadow / penuUpperShadow
	}
	if befoUpperShadow == 0 {
		penuUpperShadowChangeRate = 0
	} else {
		penuUpperShadowChangeRate = penuUpperShadow / befoUpperShadow
	}

	// 计算影线变化率 -- lower
	if prevLowerShadow == 0 {
		lastLowerShadowChangeRate = 0
	} else {
		lastLowerShadowChangeRate = lastLowerShadow / prevLowerShadow
	}
	if penuLowerShadow == 0 {
		prevLowerShadowChangeRate = 0
	} else {
		prevLowerShadowChangeRate = prevLowerShadow / penuLowerShadow
	}
	if befoLowerShadow == 0 {
		penuLowerShadowChangeRate = 0
	} else {
		penuLowerShadowChangeRate = penuLowerShadow / befoLowerShadow
	}

	// 计算影线比例
	if lastLowerShadow == 0 {
		lastShadowRate = 1.0
	} else {
		lastShadowRate = lastUpperShadow / lastLowerShadow
	}
	if prevLowerShadow == 0 {
		prevShadowRate = 1.0
	} else {
		prevShadowRate = prevUpperShadow / prevLowerShadow
	}
	if penuLowerShadow == 0 {
		penuShadowRate = 1.0
	} else {
		penuShadowRate = penuUpperShadow / penuLowerShadow
	}
	// RSI 极限值计算
	lastRsiExtreme := (lastRsi - 50.0) / 50.0
	prevRsiExtreme := (prevRsi - 50.0) / 50.0
	penuRsiExtreme := (penuRsi - 50.0) / 50.0

	// 布林带突破情况
	var lastBollingCrossRate, prevBollingCrossRate, penuBollingCrossRate, lastCloseCrossRate, prevCloseCrossRate, penuCloseCrossRate float64
	if lastPrice > bbMiddle.Last(0) {
		lastBollingCrossRate = df.High.Last(0) / bbUpper.Last(0)
		lastCloseCrossRate = lastPrice / bbUpper.Last(0)
	} else {
		lastBollingCrossRate = bbLower.Last(0) / df.Low.Last(0)
		lastCloseCrossRate = bbLower.Last(0) / lastPrice
	}

	if prevPrice > bbMiddle.Last(1) {
		prevBollingCrossRate = df.High.Last(1) / bbUpper.Last(1)
		prevCloseCrossRate = prevPrice / bbUpper.Last(1)
	} else {
		prevBollingCrossRate = bbLower.Last(0) / df.Low.Last(0)
		prevCloseCrossRate = bbLower.Last(0) / prevPrice
	}
	if lastPrice > bbMiddle.Last(2) {
		penuBollingCrossRate = df.High.Last(2) / bbUpper.Last(2)
		penuCloseCrossRate = penuPrie / bbUpper.Last(0)
	} else {
		penuBollingCrossRate = bbLower.Last(2) / df.Low.Last(2)
		penuCloseCrossRate = bbLower.Last(2) / penuPrie
	}

	// 创建 StrategyFeature 实例并填充数据
	feature := &model.StrategyFeature{
		LastPrice:                 lastPrice,
		LastRsi:                   lastRsi,
		PrevRsi:                   prevRsi,
		PenuRsi:                   penuRsi,
		LastRsiExtreme:            lastRsiExtreme,
		PrevRsiExtreme:            prevRsiExtreme,
		PenuRsiExtreme:            penuRsiExtreme,
		LastRsiDiff:               lastRsi - prevRsi,
		PrevRsiDiff:               prevRsi - penuRsi,
		PenuRsiDiff:               penuRsi - befoRsi,
		PrevAvgVolumeRate:         prevVolume / prevAvgVolume,
		PenuAvgVolumeRate:         penuVolume / penuAvgVolume,
		PrevPriceRate:             prevPriceRate,
		PenuPriceRate:             penuPriceRate,
		LastShadowRate:            lastShadowRate,
		PrevShadowRate:            prevShadowRate,
		PenuShadowRate:            penuShadowRate,
		LastUpperShadowChangeRate: lastUpperShadowChangeRate,
		PrevUpperShadowChangeRate: prevUpperShadowChangeRate,
		PenuUpperShadowChangeRate: penuUpperShadowChangeRate,
		LastLowerShadowChangeRate: lastLowerShadowChangeRate,
		PrevLowerShadowChangeRate: prevLowerShadowChangeRate,
		PenuLowerShadowChangeRate: penuLowerShadowChangeRate,
		LastUpperPinRate:          lastUpperPinRate,
		PrevUpperPinRate:          prevUpperPinRate,
		PenuUpperPinRate:          penuUpperPinRate,
		LastLowerPinRate:          lastLowerPinRate,
		PrevLowerPinRate:          prevLowerPinRate,
		PenuLowerPinRate:          penuLowerPinRate,
		LastMacdDiffRate:          (macd.Last(0) - signal.Last(0)) / signal.Last(0),
		PrevMacdDiffRate:          (macd.Last(1) - signal.Last(1)) / signal.Last(1),
		PenuMacdDiffRate:          (macd.Last(2) - signal.Last(2)) / signal.Last(2),
		LastAmplitude:             indicator.AMP(df.Open.Last(0), df.High.Last(0), df.Low.Last(0)),
		PrevAmplitude:             indicator.AMP(df.Open.Last(1), df.High.Last(1), df.Low.Last(1)),
		PenuAmplitude:             indicator.AMP(df.Open.Last(2), df.High.Last(2), df.Low.Last(2)),
		LastBollingCrossRate:      lastBollingCrossRate,
		PrevBollingCrossRate:      prevBollingCrossRate,
		PenuBollingCrossRate:      penuBollingCrossRate,
		LastCloseCrossRate:        lastCloseCrossRate,
		PrevCloseCrossRate:        prevCloseCrossRate,
		PenuCloseCrossRate:        penuCloseCrossRate,
		OpenAt:                    df.LastUpdate.In(Loc).Format("2006-01-02 15:04:05"),
	}

	return feature
}
