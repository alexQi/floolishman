package calc

import (
	"fmt"
	"testing"
)

func Test_formatFloat(t *testing.T) {
	a := FloatEquals(2.1994, 2.1996, 0.01)
	fmt.Print(a)
}
