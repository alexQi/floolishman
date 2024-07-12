package indicator

import (
	"floolishman/utils/calc"
	"fmt"
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

func TestA(t *testing.T) {
	// 示例数据：布林带中轨价格序列
	sequence := []float64{100, 105, 110, 108, 106, 104, 102, 103, 105, 107}
	a := -12.3
	b := -22.1
	fmt.Println(a < b)
	// 计算布林带中轨的角度
	angle := calc.CalculateAngle(sequence)

	// 打印角度作为判断震荡行情的依据
	fmt.Printf("Angle of the Bollinger Band's midline: %.2f degrees\n", angle)
}
