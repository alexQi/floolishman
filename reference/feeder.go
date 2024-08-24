package reference

import (
	"context"
	"floolishman/model"
	"time"
)

type Feeder interface {
	AssetsInfo(pair string) model.AssetInfo
	AssetsInfos() map[string]model.AssetInfo
	LastQuote(ctx context.Context, pair string) (float64, error)
	SetPairOption(ctx context.Context, option model.PairOption) error
	CandlesByPeriod(ctx context.Context, pair, period string, start, end time.Time) ([]model.Candle, error)
	CandlesByLimit(ctx context.Context, pair, period string, limit int) ([]model.Candle, error)
	CandlesSubscription(ctx context.Context, pair, timeframe string) (chan model.Candle, chan error)
	CandlesBatchSubscription(ctx context.Context, combineConfig map[string]string) (map[string]chan model.Candle, chan error)
}
