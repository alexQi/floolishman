package types

import (
	"floolisher/model"
	"floolisher/service"
	"fmt"
)

type StrategyPosition struct {
	Useable      bool
	Side         model.SideType
	Pair         string
	StrategyName string
	Score        int
}

func (sp StrategyPosition) String() string {
	return fmt.Sprintf("%s %s | Strategy: %s, Score: %d", sp.Side, sp.Pair, sp.StrategyName, sp.Score)
}

type OnDecisionFunc func(matchers []StrategyPosition, dataframe *model.Dataframe, broker service.Broker)

type Strategy interface {
	// 策略排序得分
	SortScore() int
	// GetPosition is decision result
	GetPosition() StrategyPosition
	// Timeframe is the time interval in which the strategy will be executed. eg: 1h, 1d, 1w
	Timeframe() string
	// WarmupPeriod is the necessary time to wait before executing the strategy, to load data for indicators.
	// This time is measured in the period specified in the `Timeframe` function.
	WarmupPeriod() int
	// Indicators will be executed for each new candle, in order to fill indicators before `OnCandle` function is called.
	Indicators(df *model.Dataframe) []ChartIndicator
	// OnCandle will be executed for each new candle, after indicators are filled, here you can do your trading logic.
	// OnCandle is executed after the candle close.
	OnCandle(df *model.Dataframe)
}

type CompositesStrategy struct {
	Strategies   []Strategy
	PositionSize float64 // 每次交易的仓位大小
}

func (cs *CompositesStrategy) OnDecision(dataframe *model.Dataframe, broker service.Broker, callback OnDecisionFunc) {
	matchers := []StrategyPosition{}
	for _, strategy := range cs.Strategies {
		strategyPosition := strategy.GetPosition()
		if strategyPosition.Useable == false {
			continue
		}
		matchers = append(matchers, strategyPosition)
	}
	callback(matchers, dataframe, broker)
}

type HighFrequencyStrategy interface {
	Strategy

	// OnPartialCandle will be executed for each new partial candle, after indicators are filled.
	OnPartialCandle(df *model.Dataframe, broker service.Broker)
}

type CompositesHighFrequencyStrategy struct {
	HighFrequencyStrategies []HighFrequencyStrategy
}
