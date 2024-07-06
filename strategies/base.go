package strategies

import (
	"floolishman/indicator"
	"floolishman/model"
	"github.com/markcheno/go-talib"
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
	// 移动平均线交叉
	shortMA := talib.Sma(df.Close, 8) // 短期移动平均线
	longMA := talib.Sma(df.Close, 21) // 长期移动平均线

	// 布林带
	upper, _, lower := talib.BBands(df.Close, 21, 2, 2, talib.SMA)

	// RSI
	rsi := talib.Rsi(df.Close, 14)

	// ATR
	atr := talib.Atr(df.High, df.Low, df.Close, 14)

	// 定义阈值
	threshold := 0.4 // 允许超过阈值的比例
	count := 0
	total := len(df.Close) // 计算区间的长度

	for i := 0; i < len(df.Close); i++ {
		if shortMA[i] > longMA[i] || df.Close[i] > upper[i] || df.Close[i] < lower[i] || rsi[i] > 70 || rsi[i] < 30 || atr[i] > 1.5 {
			count++
		}
	}

	if float64(count)/float64(total) > threshold {
		// 判断是单边上升还是单边下降
		if df.Close[len(df.Close)-1] > longMA[len(longMA)-1] {
			return "rise"
		} else {
			return "down"
		}
	} else {
		return "range"
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
