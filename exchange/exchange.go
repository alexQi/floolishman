package exchange

import (
	"context"
	"errors"
	"floolishman/model"
	"floolishman/reference"
	"floolishman/utils"
	"fmt"
	"strings"
	"sync"

	"github.com/StudioSol/set"
)

var (
	ErrInvalidQuantity   = errors.New("invalid quantity")
	ErrInsufficientFunds = errors.New("insufficient funds or locked")
	ErrInvalidAsset      = errors.New("invalid asset")
)

type DataFeed struct {
	Data chan model.Candle
	Err  chan error
}

type DataFeedSubscription struct {
	mu                      sync.Mutex
	exchange                reference.Exchange
	Feeds                   *set.LinkedHashSetString
	DataFeeds               map[string]*DataFeed
	SubscriptionsByDataFeed map[string][]Subscription
}

type Subscription struct {
	timeframe     string
	onCandleClose bool
	consumer      DataFeedConsumer
}

type OrderError struct {
	Err      error
	Pair     string
	Quantity float64
}

func (o *OrderError) Error() string {
	return fmt.Sprintf("order error: %v", o.Err)
}

type DataFeedConsumer func(string, model.Candle)

func NewDataFeed(exchange reference.Exchange) *DataFeedSubscription {
	return &DataFeedSubscription{
		exchange:                exchange,
		Feeds:                   set.NewLinkedHashSetString(),
		DataFeeds:               make(map[string]*DataFeed),
		SubscriptionsByDataFeed: make(map[string][]Subscription),
	}
}

func (d *DataFeedSubscription) feedKey(pair, timeframe string) string {
	return fmt.Sprintf("%s--%s", pair, timeframe)
}

func (d *DataFeedSubscription) pairTimeframeFromKey(key string) (pair, timeframe string) {
	parts := strings.Split(key, "--")
	return parts[0], parts[1]
}

func (d *DataFeedSubscription) Subscribe(pair, timeframe string, consumer DataFeedConsumer, onCandleClose bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := d.feedKey(pair, timeframe)
	d.Feeds.Add(key)
	d.SubscriptionsByDataFeed[key] = append(d.SubscriptionsByDataFeed[key], Subscription{
		onCandleClose: onCandleClose,
		consumer:      consumer,
		timeframe:     timeframe,
	})
}

func (d *DataFeedSubscription) Preload(pair, timeframe string, candles []model.Candle) {
	utils.Log.Infof("Preloading %d candles for %s-%s", len(candles), pair, timeframe)
	key := d.feedKey(pair, timeframe)
	for _, candle := range candles {
		if !candle.Complete {
			continue
		}

		for _, subscription := range d.SubscriptionsByDataFeed[key] {
			subscription.consumer(subscription.timeframe, candle)
		}
	}
}

func (d *DataFeedSubscription) BatchConnect() {
	utils.Log.Infof("Batch connecting to the exchange.")

	combineConfig := map[string]string{}
	for feed := range d.Feeds.Iter() {
		pair, timeframe := d.pairTimeframeFromKey(feed)
		combineConfig[pair] = timeframe
	}
	pairCcandle, cerr := d.exchange.CandlesBatchSubscription(context.Background(), combineConfig)
	for feed, ccandle := range pairCcandle {
		d.DataFeeds[feed] = &DataFeed{
			Data: ccandle,
			Err:  cerr,
		}
	}
}

func (d *DataFeedSubscription) Connect() {
	utils.Log.Infof("Connecting to the exchange.")
	var wg sync.WaitGroup // 用于等待所有并发任务完成

	for feed := range d.Feeds.Iter() {
		wg.Add(1) // 增加WaitGroup计数器

		go func(feed string) {
			d.mu.Lock()
			defer func() {
				d.mu.Unlock()
				wg.Done() // 当goroutine完成时，减少WaitGroup计数器
			}()

			pair, timeframe := d.pairTimeframeFromKey(feed)
			ccandle, cerr := d.exchange.CandlesSubscription(context.Background(), pair, timeframe)
			d.DataFeeds[feed] = &DataFeed{
				Data: ccandle,
				Err:  cerr,
			}
		}(feed)
	}
	wg.Wait()
}

func (d *DataFeedSubscription) Start(loadSync bool, isBatch bool) {
	if isBatch {
		d.BatchConnect()
	} else {
		d.Connect()
	}
	wg := new(sync.WaitGroup)
	for key, feed := range d.DataFeeds {
		wg.Add(1)
		go func(key string, feed *DataFeed) {
			for {
				select {
				case candle, ok := <-feed.Data:
					if !ok {
						wg.Done()
						return
					}
					for _, subscription := range d.SubscriptionsByDataFeed[key] {
						if subscription.onCandleClose && !candle.Complete {
							continue
						}
						subscription.consumer(subscription.timeframe, candle)
					}
				case err := <-feed.Err:
					if err != nil {
						utils.Log.Error("dataFeedSubscription/start: ", err)
					}
				}
			}
		}(key, feed)
		utils.Log.Infof("Load feed : %s", key)
	}

	if loadSync {
		wg.Wait()
	}
	utils.Log.Infof("Data feed connected.")
}
