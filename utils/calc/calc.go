package calc

import (
	"floolishman/model"
	"math"
)

func Max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func Min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func Abs(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

// CalculateAngle 计算数字序列的方向角度（根据整体趋势）
func CalculateAngle(sequence []float64) float64 {
	n := len(sequence)
	if n < 2 {
		return 0.0 // 如果序列长度不足，返回默认角度
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range sequence {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// 计算斜率 m = (n * Σ(xy) - Σx * Σy) / (n * Σ(x^2) - (Σx)^2)
	m := (float64(n)*sumXY - sumX*sumY) / (float64(n)*sumX2 - sumX*sumX)

	// 计算角度 angle = atan(m)
	angle := math.Atan(m)

	// 将弧度转换为角度
	angle = angle * 180.0 / math.Pi

	return angle
}

func PositionSize(balance, leverage, currentPrice float64) float64 {
	return (balance * leverage) / currentPrice
}

func OpenPositionSize(balance, leverage, currentPrice float64, scoreRadio float64, fullSpaceRadio float64) float64 {
	var amount float64
	fullPositionSize := (balance * leverage) / currentPrice
	if scoreRadio >= 0.5 {
		amount = fullPositionSize * fullSpaceRadio
	} else {
		if scoreRadio < 0.2 {
			amount = fullPositionSize * fullSpaceRadio * 0.4
		} else {
			amount = fullPositionSize * fullSpaceRadio * scoreRadio * 2
		}
	}
	return amount
}

func ProfitRatio(side model.SideType, entryPrice float64, currentPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据当前价格计算利润
	var profit float64
	if side == model.SideTypeSell {
		profit = (entryPrice - currentPrice) * quantity
	} else {
		profit = (currentPrice - entryPrice) * quantity
	}

	// 计算利润比
	return profit / margin
}

func StopLossDistance(profitRatio float64, entryPrice float64, leverage float64, quantity float64) float64 {
	// 计算保证金
	margin := (entryPrice * quantity) / leverage
	// 根据保证金，利润比计算利润
	profit := profitRatio * margin
	// 根据利润 计算价差
	if profit == 0 {
		return 0
	}
	return Abs(profit / quantity)
}

func IsRetracement(openPrice float64, currentPrice float64, side model.SideType, volatilityThreshold float64) bool {
	// 判断是否盈利中
	isWithoutVolatility := false
	// 获取环比
	priceChange := (currentPrice - openPrice) / openPrice
	volatility := Abs(priceChange) > volatilityThreshold
	if side == model.SideTypeBuy {
		if volatility && priceChange < 0 {
			isWithoutVolatility = true
		}
	}
	if side == model.SideTypeSell {
		if volatility && priceChange > 0 {
			isWithoutVolatility = true
		}
	}
	// 只有在盈利中且波动在合理范围内时，返回 true
	return isWithoutVolatility
}
