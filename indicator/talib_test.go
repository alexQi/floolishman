package indicator

import (
	"fmt"
	"testing"
	"time"
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
	// 定义一个特定的时间
	time1 := time.Now().Add(-time.Duration(15) * time.Minute)

	// 获取当前时间
	now := time.Now()
	// 比较时间
	if time1.Before(now) {
		fmt.Println("time1 is before the current time")
	} else if time1.After(now) {
		fmt.Println("time1 is after the current time")
	} else {
		fmt.Println("time1 is equal to the current time")
	}
}
