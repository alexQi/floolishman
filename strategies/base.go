package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
)

type BaseStrategy struct {
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

func (bs *BaseStrategy) bactchCheckPinBar(df *model.Dataframe, count int, weight float64, includeLatest bool) (bool, bool) {
	var opens, closes, highs, lows []float64
	if includeLatest {
		opens = df.Open.LastValues(count)
		closes = df.Close.LastValues(count)
		highs = df.High.LastValues(count)
		lows = df.Low.LastValues(count)
	} else {
		opens = df.Open.GetLastValues(count, 1)
		closes = df.Close.GetLastValues(count, 1)
		highs = df.High.GetLastValues(count, 1)
		lows = df.Low.GetLastValues(count, 1)
	}

	upperPinBars := []float64{}
	lowerPinBars := []float64{}
	var prevBodyLength float64
	for i := 0; i < count; i++ {
		if i > 0 {
			prevBodyLength = calc.Abs(opens[i] - closes[i])
		}
		isUpperPinBar, isLowerPinBar, upperShadow, lowerShadow := calc.CheckPinBar(weight, 4, prevBodyLength, opens[i], closes[i], highs[i], lows[i])
		if isUpperPinBar && isLowerPinBar {
			continue
		}
		if isUpperPinBar {
			upperPinBars = append(upperPinBars, upperShadow)
		}
		if isLowerPinBar {
			lowerPinBars = append(lowerPinBars, lowerShadow)
		}
	}
	var upperLength float64
	for _, bar := range upperPinBars {
		upperLength += bar
	}
	var lowerLength float64
	for _, bar := range lowerPinBars {
		lowerLength += bar
	}
	if upperLength > lowerLength {
		return true, false
	} else if upperLength < lowerLength {
		return false, true
	} else {
		return false, false
	}
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

func (bs *BaseStrategy) checkBollingCross(df *model.Dataframe, period int, endIndex int, position string) (bool, bool) {
	hasCross := false
	hasBack := false
	var high, low, limits []float64
	if position == "up" {
		limits = df.Metadata["bbUpper"].GetLastValues(period, endIndex)
		high = df.High.GetLastValues(period, endIndex)
		for i, price := range high {
			if hasCross == false && price >= limits[i] {
				hasCross = true
				continue
			}
			if hasCross == true {
				hasBack = price < limits[i]
			}
		}
	} else {
		limits = df.Metadata["bbLower"].GetLastValues(period, endIndex)
		low = df.Low.GetLastValues(period, endIndex)
		for i, price := range low {
			if hasCross == false && price <= limits[i] {
				hasCross = true
				continue
			}
			if hasCross == true {
				hasBack = price > limits[i]
			}
		}
	}
	return hasCross, hasBack
}
