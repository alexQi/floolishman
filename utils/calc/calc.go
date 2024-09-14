package calc

import (
	"floolishman/model"
	"fmt"
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

func FormatAmountToSize(amount, unit float64) float64 {
	if unit <= 0 {
		return amount // 如果单位为0或负数，则返回原始金额
	}

	// 计算精度（小数位数）
	precision := int(math.Round(-math.Log10(unit)))

	// 使用 big.Float 进行精确除法
	amountFormat := new(big.Float).SetFloat64(amount)
	unitFormat := new(big.Float).SetFloat64(unit)

	formattedAmountStr := new(big.Float).Quo(amountFormat, unitFormat).String()
	// 将字符串转换为 float64
	amountFloat, err := strconv.ParseFloat(formattedAmountStr, 64)
	if err != nil {
		fmt.Println("转换错误:", err)
		return 0
	}
	// 格式化金额
	formattedAmount := math.Trunc(amountFloat) * unit

	// 返回格式化后的金额，保留指定的精度
	return math.Round(formattedAmount*math.Pow10(precision)) / math.Pow10(precision)
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

func FormatFloatRate(input float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Floor(input*shift) / shift
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

func GetPinBarRate(open, close, hight, low float64) (float64, float64, float64, float64) {
	upperShadow := hight - Max(open, close)
	lowerShadow := Min(open, close) - low
	bodyLength := Abs(open - close)

	return upperShadow / bodyLength, lowerShadow / bodyLength, upperShadow, lowerShadow
}

func CheckPinBar(weight, n, prevBodyLength, open, close, hight, low float64) (bool, bool, float64, float64) {
	upperShadow := hight - Max(open, close)
	lowerShadow := Min(open, close) - low
	bodyLength := Abs(open - close)

	if prevBodyLength != 0 && bodyLength/prevBodyLength > n {
		weight = weight / n
	}
	// 上插针条件
	isUpperPinBar := upperShadow >= weight*bodyLength && lowerShadow <= upperShadow/weight
	// 下插针条件
	isLowerPinBar := lowerShadow >= weight*bodyLength && upperShadow <= lowerShadow/weight

	return isUpperPinBar, isLowerPinBar, upperShadow, lowerShadow
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

	var x, y, sumX, sumY, sumXY, sumX2 float64
	//baseValue := sequence[0]
	for i, value := range sequence {
		x = float64(i)
		if i == 0 || sequence[i-1] == 0 {
			y = 0
		} else {
			y = ((value - sequence[i-1]) / math.Abs(sequence[i-1])) * 100 // 计算百分比变化
		}
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denominator := (float64(n)*sumX2 - sumX*sumX)
	if denominator == 0 {
		return 0.0 // 避免除零
	}

	m := (float64(n)*sumXY - sumX*sumY) / denominator
	angle := math.Atan(m) * 180.0 / math.Pi

	return angle
}

func PositionSize(balance, leverage, currentPrice float64) float64 {
	return (balance * leverage) / currentPrice
}

func StopPositionSizeRatio(balance, leverage, price, positionQuantity float64) float64 {
	originPositionSize := PositionSize(balance, leverage, price)
	return positionQuantity / originPositionSize
}

func OpenPositionSize(balance, leverage, currentPrice float64, marginRatio float64) float64 {
	fullPositionSize := PositionSize(balance, leverage, currentPrice)
	return fullPositionSize * marginRatio
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

func CalculateDualProfitRatio(mainSide model.SideType, mainQuantity, mainPrice, subQuantity, subPrice, currentPrice, leverage float64) float64 {
	// 计算保证金
	margin := (mainQuantity*mainPrice - subQuantity*subPrice) / leverage
	// 根据当前价格计算利润
	var profit float64
	if mainSide == model.SideTypeSell {
		profit = (mainPrice-currentPrice)*mainQuantity + (currentPrice-subPrice)*subQuantity
	} else {
		profit = (currentPrice-mainPrice)*mainQuantity + (subPrice-currentPrice)*subQuantity
	}
	// 计算利润比
	return profit / margin
}

// 根据亏损比例计算加仓数量
func CalculateAddQuantity(mainSide model.SideType, mainQuantity, mainPrice, subQuantity, subPrice, currentPrice, leverage, limitProfitRatio float64) float64 {
	var addAmount float64
	if mainSide == model.SideTypeSell {
		addAmount = (leverage*(currentPrice*subQuantity-subPrice*subQuantity+(mainPrice-currentPrice)*mainQuantity)/(0-limitProfitRatio) + mainPrice*mainQuantity - subPrice*subQuantity) / currentPrice
	} else {
		addAmount = (leverage*(subPrice*subQuantity-currentPrice*subQuantity+(currentPrice-mainPrice)*mainQuantity)/(0-limitProfitRatio) + mainPrice*mainQuantity - subPrice*subQuantity) / currentPrice
	}
	return addAmount
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
