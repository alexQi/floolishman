package types

type CallerSetting struct {
	GuiderHost       string
	CheckMode        string
	FollowSymbol     bool
	Backtest         bool
	LossTimeDuration int
}

type PairStatus struct {
	Pair   string
	Status bool
}

var PairStatusChan = make(chan PairStatus, 10)
