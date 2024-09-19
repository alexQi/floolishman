package reference

import (
	"context"
	"floolishman/model"
	"floolishman/types"
	"time"
)

type Caller interface {
	Start()
	OpenTube(pair string)
	Init(context.Context, model.CompositesStrategy, Broker, Exchange, types.CallerSetting)
	SetPair(option model.PairOption)
	SetSample(pair string, timeframe string, strategyName string, dataframe *model.Dataframe)
	UpdatePairInfo(pair string, price float64, volume float64, updatedAt time.Time)
	CloseOrder(checkTimeout bool)
	EventCallOpen(pair string)
	EventCallClose(pair string)
}
