package indicator

import (
	"fmt"
	"testing"
)

func TestEma8(t *testing.T) {
	data := []float64{3314.03, 3312.14, 3309.01, 3302, 3302.38, 3300.71, 3305.66, 3306.52, 3306.71}
	a := EMA(data, 8)
	fmt.Print(a)
}
