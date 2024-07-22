package calc

import (
	"fmt"
	"math/big"
	"testing"
)

func Test_formatFloat(t *testing.T) {
	ac := 0.012
	ab := 0.006
	processQuantityA := AccurateSub(ac, ab)
	fmt.Print(processQuantityA)

	processQuantityB, _ := new(big.Float).Sub(
		new(big.Float).SetFloat64(ac),
		new(big.Float).SetFloat64(ab),
	).Float64()

	fmt.Print(processQuantityB)
}
