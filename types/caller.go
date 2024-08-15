package types

type CallerSetting struct {
	GuiderHost       string
	CheckMode        string
	FollowSymbol     bool
	Backtest         bool
	LossTimeDuration int
}
