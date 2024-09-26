package types

import (
	"floolishman/model"
	"github.com/adshao/go-binance/v2/futures"
)

type CallerSetting struct {
	GuiderHost                string
	CheckMode                 string
	FollowSymbol              bool
	Backtest                  bool
	LossTimeDuration          int
	AllowPairs                []string
	IgnorePairs               []string
	IgnoreHours               []int
	Leverage                  int
	MarginType                futures.MarginType
	MarginMode                model.MarginMode
	MarginSize                float64
	ProfitableScale           float64
	ProfitableScaleDecrStep   float64
	ProfitableTrigger         float64
	ProfitableTriggerIncrStep float64
	PullMarginLossRatio       float64
	MaxMarginRatio            float64
	MaxMarginLossRatio        float64
	PauseCaller               int64
}

type PairStatus struct {
	Pair   string
	Status bool
}

type PairGridBuilderParam struct {
	Pair      string
	Timeframe string
	IsForce   bool
}

var PairStatusChan = make(chan PairStatus, 10)

var PairGridBuilderParamChan = make(chan PairGridBuilderParam, 100)

var PairPauserChan = make(chan string, 1000)
