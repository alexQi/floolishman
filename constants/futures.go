package constants

import "github.com/adshao/go-binance/v2/futures"

type MarginType = futures.MarginType

var (
	MarginTypeIsolated MarginType = "ISOLATED"
	MarginTypeCrossed  MarginType = "CROSSED"
)
