package model

import "github.com/adshao/go-binance/v2/futures"

type PairOption struct {
	Pair       string             `json:"pair"`
	Leverage   int                `json:"leverage"`
	MarginType futures.MarginType `json:"marginType"`
}
