package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"floolishman/utils/calc"
)

type BaseStrategy struct {
}

func (bs *BaseStrategy) handleIndicatos(df *model.Dataframe) error {
	df.Metadata["ema8"] = indicator.EMA(df.Close, 8)
	df.Metadata["sma21"] = indicator.SMA(df.Close, 21)
	df.Metadata["obv"] = indicator.OBV(df.Close, df.Volume)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 14)

	return nil
}

func (bs *BaseStrategy) checkMarketTendency(df *model.Dataframe) string {
	bbMiddles := df.Metadata["bb_middle"]
	bbMiddlesNotZero := []float64{}
	for _, val := range bbMiddles.LastValues(30) {
		if val > 0 {
			bbMiddlesNotZero = append(bbMiddlesNotZero, val)
		}
	}
	if len(bbMiddlesNotZero) < 10 {
		return "ambiguity"
	}
	tendencyAngle := calc.CalculateAngle(bbMiddlesNotZero)

	if calc.Abs(tendencyAngle) > 20 {
		if tendencyAngle > 0 {
			return "rise"
		} else {
			return "down"
		}
	}
	return "range"
}

// checkPinBar 是否上方插针，是否上方插针，最终方向 true-方向向下，false-方向上香
func (bs *BaseStrategy) checkPinBar(open, close, hight, low float64) (bool, bool, bool) {
	upperShadow := hight - calc.Max(open, close)
	lowerShadow := calc.Min(open, close) - low
	bodyLength := calc.Abs(open - close)

	// 上插针条件
	isUpperPinBar := upperShadow >= 2*bodyLength && lowerShadow <= bodyLength/2
	// 下插针条件
	isLowerPinBar := lowerShadow >= 2*bodyLength && upperShadow <= bodyLength/2

	return isUpperPinBar, isLowerPinBar, upperShadow < lowerShadow
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

func (bs *BaseStrategy) checkCandleTendency(df *model.Dataframe, count int) string {
	historyOpens := df.Open.LastValues(count)
	historyCloses := df.Close.LastValues(count)

	tendency := "neutral"
	// 检查数据长度是否足够
	if len(historyOpens) < count || len(historyCloses) < count {
		return tendency
	}
	historyCandleColors := []string{}
	for i := 0; i < count; i++ {
		historyCandleColors = append(historyCandleColors, bs.getCandleColor(historyOpens[i], historyCloses[i]))
	}
	historyColorCount := map[string]int{}
	for _, color := range historyCandleColors {
		if _, ok := historyColorCount[color]; ok {
			historyColorCount[color] += 1
		} else {
			historyColorCount[color] = 1
		}
	}
	if historyColorCount["bullish"] > count/2 {
		return "bullish"
	}
	if historyColorCount["bearish"] > count/2 {
		return "bearish"
	}
	return tendency
}
