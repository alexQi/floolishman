package types

import (
	"floolishman/model"
	"floolishman/reference"
	"floolishman/utils"
	"fmt"
	"reflect"
)

type StrategyPosition struct {
	Useable      bool
	Side         model.SideType
	Pair         string
	StrategyName string
	Score        int
	Price        float64
	Tendency     string
}

func (sp StrategyPosition) String() string {
	return fmt.Sprintf("[%s] %s %s | Strategy: %s, Score: %d", sp.Tendency, sp.Side, sp.Pair, sp.StrategyName, sp.Score)
}

type OpenPositionFunc func(option model.PairOption, broker reference.Broker)

type ClosePositionFunc func(option model.PairOption, broker reference.Broker)

type Strategy interface {
	// 策略排序得分
	SortScore() int
	// Timeframe is the time interval in which the strategy will be executed. eg: 1h, 1d, 1w
	Timeframe() string
	// WarmupPeriod is the necessary time to wait before executing the strategy, to load data for indicators.
	// This time is measured in the period specified in the `Timeframe` function.
	WarmupPeriod() int
	// Indicators will be executed for each new candle, in order to fill indicators before `OnCandle` function is called.
	Indicators(df *model.Dataframe)
	// OnCandle will be executed for each new candle, after indicators are filled, here you can do your trading logic.
	// OnCandle is executed after the candle close.
	OnCandle(realCandle *model.Candle, df *model.Dataframe) StrategyPosition
}

type CompositesStrategy struct {
	Strategies   []Strategy
	PositionSize float64 // 每次交易的仓位大小
}

func (cs *CompositesStrategy) GetDetail() {
	for _, strategy := range cs.Strategies {
		utils.Log.Infof("Loaded Strategy: %s, Timeframe: %s", reflect.TypeOf(strategy).Elem().Name(), strategy.Timeframe())
	}
}

// TimeFrameMap 获取当前策略时间周期对应的热启动区间数
func (cs *CompositesStrategy) TimeWarmupMap() map[string]int {
	timeFrames := make(map[string]int)
	for _, strategy := range cs.Strategies {
		originPeriod, ok := timeFrames[strategy.Timeframe()]
		if ok {
			if strategy.WarmupPeriod() <= originPeriod {
				continue
			}
		}
		timeFrames[strategy.Timeframe()] = strategy.WarmupPeriod()
	}
	return timeFrames
}

func (cs *CompositesStrategy) CallMatchers(
	realCandles map[string]*model.Candle,
	dataframes map[string]map[string]*model.Dataframe,
) []StrategyPosition {
	var strategyName string
	matchers := []StrategyPosition{}
	for _, strategy := range cs.Strategies {
		strategyName = reflect.TypeOf(strategy).Elem().Name()
		strategyPosition := strategy.OnCandle(
			realCandles[strategy.Timeframe()],
			dataframes[strategy.Timeframe()][strategyName],
		)
		matchers = append(matchers, strategyPosition)
	}
	return matchers
}
