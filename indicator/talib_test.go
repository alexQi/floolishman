package indicator

import (
	"floolishman/utils/calc"
	"fmt"
	"math"
	"testing"
)

func TestEma8(t *testing.T) {
	value := 0.21345
	formatted := value * 100.0 // 将小数转换为百分比
	// 使用格式化字符串 %.2f%% 将浮点数格式化为百分比形式，并保留两位小数
	result := fmt.Sprintf("%.2f%%", formatted)

	fmt.Println(result) // 输出结果为 "21.34%"
}

func Bac(balance, leverage, currentPrice float64, scoreRadio float64) float64 {
	fullSpaceRadio := 0.1
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
	// 示例数据：布林带中轨价格序列
	// 定义一个特定的时间
	a := []float64{3123.3075757575753, 3122.2842857142855, 3121.799567099567, 3121.5439826839824, 3122.3583116883115, 3123.0663203463196, 3123.530216416, 3124.471774891774, 3125.329826839826, 3126.086709956709}
	b := calc.CalculateAngle(a)
	fmt.Print(b)
}
