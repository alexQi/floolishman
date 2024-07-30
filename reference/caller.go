package reference

import (
	"context"
	"floolishman/model"
	"floolishman/types"
	"time"
)

type Caller interface {
	Start()
	Init(context.Context, types.CompositesStrategy, Broker, Exchange, types.CallerSetting)
	SetPair(option model.PairOption)
	SetSample(pair string, timeframe string, strategyName string, dataframe *model.Dataframe)
	UpdatePairInfo(pair string, price float64, updatedAt time.Time)
	CheckOrderTimeout()
	EventCallOpen(pair string)
	EventCallClose(pair string)
}
