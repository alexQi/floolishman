package types

type CallerSetting struct {
	GuiderHost           string
	CheckMode            string
	FollowSymbol         bool
	Backtest             bool
	LossTimeDuration     int
	FullSpaceRadio       float64
	BaseLossRatio        float64
	ProfitableScale      float64
	InitProfitRatioLimit float64
}
