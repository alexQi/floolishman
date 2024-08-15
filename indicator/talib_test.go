package indicator

import (
	"floolishman/utils/calc"
	"fmt"
	"math"
	"strconv"
	"testing"
)

func TestEma8(t *testing.T) {
	value := 0.21345
	formatted := value * 100.0 // 将小数转换为百分比
	// 使用格式化字符串 %.2f%% 将浮点数格式化为百分比形式，并保留两位小数
	result := fmt.Sprintf("%.2f%%", formatted)

	fmt.Println(result) // 输出结果为 "21.34%"
}

func calculateWidthChangeRate(bbWidth []float64) (float64, error) {
	if len(bbWidth) == 0 {
		return 0, fmt.Errorf("bbWidth array is empty")
	}

	// 计算布林带宽度的平均值
	var sumWidth float64
	for _, width := range bbWidth {
		sumWidth += width
	}
	averageWidth := sumWidth / float64(len(bbWidth))

	// 获取当前周期的布林带宽度
	currentWidth := bbWidth[len(bbWidth)-1]

	// 计算变化率
	widthChangeRate := (currentWidth - averageWidth) / averageWidth

	return widthChangeRate, nil
}

// CalculateAngle 计算由第一对和最后一对点构成的线相对于水平线的角度（以度为单位）
func CalculateAngle(sequence []float64) float64 {
	if len(sequence) < 2 {
		return 0.0 // 如果序列长度不足，返回默认角度
	}

	// 使用第一对和最后一对点来计算斜率
	firstX, firstY := 0.0, sequence[0]
	lastX, lastY := float64(len(sequence)-1), sequence[len(sequence)-1]

	// 计算斜率
	m := (lastY - firstY) / (lastX - firstX)

	// 计算角度
	angle := math.Atan(m)

	// 将弧度转换为角度
	angleDeg := angle * (180.0 / math.Pi)

	return angleDeg
}

func TestA(t *testing.T) {
	//b := AMP(2597.89, 2644.63, 2574.54)
	//fmt.Print(b)
	//return
	a := 0.007383041777774303
	stepSize := 0.001
	//
	//a := 0.53
	//stepSize := 0.001

	val := calc.FormatAmountToSize(a, stepSize)
	fmt.Print(strconv.FormatFloat(val, 'f', -1, 64))
}
