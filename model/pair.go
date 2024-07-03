package model

import "github.com/adshao/go-binance/v2/futures"

type PairOption struct {
	Pair       string
	Leverage   int
	MarginType futures.MarginType
}
