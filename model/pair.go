package model

import (
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
)

type PairOption struct {
	Pair       string             `json:"pair"`
	Leverage   int                `json:"leverage"`
	MarginType futures.MarginType `json:"marginType"`
	GridStep   float64            `json:"grid_step"`
}

func (o PairOption) String() string {
	return fmt.Sprintf("[STRATEGY] Loading Pair: %s, Leverage: %d, MarginType: %s", o.Pair, o.Leverage, o.MarginType)
}
