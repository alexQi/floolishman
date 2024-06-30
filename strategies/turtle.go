package strategies

import (
	"floolisher/indicator"
	"floolisher/model"
	"floolisher/types"
	"reflect"
)

// https://www.investopedia.com/articles/trading/08/turtle-trading.asp
type Turtle struct {
	BaseStrategy
}

func (s Turtle) SortScore() int {
	return StrategyScoresConst[s.Timeframe()]
}

func (s Turtle) GetPosition() types.StrategyPosition {
	return s.StrategyPosition
}

func (s Turtle) Timeframe() string {
	return "4h"
}

func (s Turtle) WarmupPeriod() int {
	return 42
}

func (s Turtle) Indicators(df *model.Dataframe) []types.ChartIndicator {
	df.Metadata["max40"] = indicator.Max(df.Close, 40)
	df.Metadata["low20"] = indicator.Min(df.Close, 20)
	df.Metadata["atr"] = indicator.ATR(df.High, df.Low, df.Close, 7)

	return nil
}

func (s *Turtle) OnCandle(df *model.Dataframe) {
	closePrice := df.Close.Last(0)
	highest := df.Metadata["max40"].Last(0)
	lowest := df.Metadata["low20"].Last(0)

	if closePrice >= highest {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeBuy,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
	// RSI 小于30，买入信号
	if closePrice <= lowest {
		s.StrategyPosition = types.StrategyPosition{
			Useable:      true,
			Side:         model.SideTypeSell,
			Pair:         df.Pair,
			StrategyName: reflect.TypeOf(s).Elem().Name(),
			Score:        s.SortScore(),
		}
	}
}
