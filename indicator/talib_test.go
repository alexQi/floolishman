package indicator

import (
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

func TestA(t *testing.T) {
	amount := Bac(850, 100, 2940, 0.6)
	fmt.Print(amount)
}
