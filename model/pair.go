package model

import (
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
)

type PairOption struct {
	Pair               string             `json:"pair"`
	Leverage           int                `json:"leverage"`
	MarginType         futures.MarginType `json:"marginType"`
	MaxGridStep        float64            `json:"max_grid_step"`
	MinGridStep        float64            `json:"min_grid_step"`
	UndulatePriceLimit float64            `json:"undulate_price_limit"`
}

func (o PairOption) String() string {
	return fmt.Sprintf("[STRATEGY] Loading Pair: %s, Leverage: %d, MarginType: %s", o.Pair, o.Leverage, o.MarginType)
}
