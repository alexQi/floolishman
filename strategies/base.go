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
