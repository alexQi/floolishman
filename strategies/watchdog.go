package strategies

import (
	"floolishman/model"
	"floolishman/types"
	"reflect"
)

type Watchdog struct {
	BaseStrategy
}

func (s Watchdog) SortScore() int {
	return 1000
}

func (s Watchdog) Timeframe() string {
	return "15m"
}

func (s Watchdog) WarmupPeriod() int {
	return 50 // 预热期设定为50个数据点
}

func (s Watchdog) Indicators(_ *model.Dataframe) {

}

func (s *Watchdog) OnCandle(df *model.Dataframe) types.StrategyPosition {
	strategyPosition := types.StrategyPosition{
		Tendency:     "watchdog",
		StrategyName: reflect.TypeOf(s).Elem().Name(),
		Pair:         df.Pair,
		Score:        s.SortScore(),
	}

	// todo 获取跟单服务最新仓位数据
	flollowResult := true
	// 求稳的多单进场逻辑
	if flollowResult == true {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeBuy
	}

	// 求稳的空单进场逻辑
	if flollowResult == false {
		strategyPosition.Useable = true
		strategyPosition.Side = model.SideTypeSell
	}

	return strategyPosition
}
