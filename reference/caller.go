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
	Init(context.Context, types.CompositesStrategy, Broker, Exchange, types.CallerSetting)
	SetPair(option model.PairOption)
	SetSample(pair string, timeframe string, strategyName string, dataframe *model.Dataframe)
	BuildGird(pair string, timeframe string, isForce bool)
	UpdatePairInfo(pair string, price float64, volume float64, updatedAt time.Time)
	CloseOrder(checkTimeout bool)
	EventCallOpen(pair string)
	EventCallClose(pair string)
}
