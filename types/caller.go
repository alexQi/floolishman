package types

type CallerSetting struct {
	GuiderHost           string
	CheckMode            string
	FollowSymbol         bool
	Backtest             bool
	LossTimeDuration     int
	MaxAddPostion        int64   // 最大加仓次数
	MaxPositionHedge     bool    // 最大仓位后是否开启对冲
	MaxPositionLossRatio float64 // 最大仓位后最大止损
	WindowPeriod         float64 // 空窗期
	FullSpaceRatio       float64 // 仓位最大比例
	StopSpaceRatio       float64 // 停止加仓比例
	BaseLossRatio        float64 // 基础止损比例
	ProfitableScale      float64 // 利润回撤比例
	InitProfitRatioLimit float64 // 初始利润触发比例
}
