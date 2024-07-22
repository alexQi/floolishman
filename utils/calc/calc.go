package calc

import (
	"floolishman/model"
	"math"
	"math/big"
	"strconv"
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

func StringToFloat64(input string) (float64, error) {
	value, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func FloatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) <= epsilon
}

func FormatFloatRate(input float64) float64 {
	return math.Round(input*100) / 100
}

func RoundToDecimalPlaces(value float64, decimalPlaces int) float64 {
	factor := math.Pow(10, float64(decimalPlaces))
	return math.Round(value*factor) / factor
}

func AccurateAdd(a, b float64) float64 {
	bigFloatA := new(big.Float).SetFloat64(a)
	bigFloatB := new(big.Float).SetFloat64(b)
	result, _ := new(big.Float).Add(bigFloatA, bigFloatB).Float64()
	return result
}

func AccurateSub(a, b float64) float64 {
	bigFloatA := new(big.Float).SetFloat64(a)
	bigFloatB := new(big.Float).SetFloat64(b)
	result, _ := new(big.Float).Sub(bigFloatA, bigFloatB).Float64()
	return result
}

func MulFloat64(a, b float64) float64 {
	// 将 float64 转换为 *big.Float
	priceBig := new(big.Float).SetFloat64(a)
	quantityBig := new(big.Float).SetFloat64(b)
	// 进行乘法运算
	totalBig := new(big.Float).Mul(priceBig, quantityBig)

	// 将 *big.Float 转换回 float64
	total, _ := totalBig.Float64()
	return total
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

	denominator := (float64(n)*sumX2 - sumX*sumX)
	if denominator == 0 {
		return 0.0 // 避免除零
	}
	// 计算斜率 m = (n * Σ(xy) - Σx * Σy) / (n * Σ(x^2) - (Σx)^2)
	m := (float64(n)*sumXY - sumX*sumY) / denominator
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
	fullPositionSize := PositionSize(balance, leverage, currentPrice)
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
