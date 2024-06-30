package strategies

import (
	"floolisher/types"
)

var StrategyScoresConst = map[string]int{
	"12h": 100,
	"8h":  80,
	"4h":  60,
	"1h":  40,
	"15m": 20,
	"5m":  10,
	"1m":  5,
}

type BaseStrategy struct {
	StrategyPosition types.StrategyPosition
}
