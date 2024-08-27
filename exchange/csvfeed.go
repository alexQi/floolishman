package exchange

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"floolishman/model"
	"github.com/samber/lo"
)

var ErrInsufficientData = errors.New("insufficient data")

type PairFeed struct {
	Pair       string
	File       string
	Timeframe  string
	HeikinAshi bool
}

type CSVFeed struct {
	Feeds               map[string]PairFeed
	CandlePairTimeFrame map[string][]model.Candle
}

func (c CSVFeed) AssetsInfo(pair string) model.AssetInfo {
	asset, quote := SplitAssetQuote(pair)
	return model.AssetInfo{
		BaseAsset:          asset,
		QuoteAsset:         quote,
		MaxPrice:           math.MaxFloat64,
		MaxQuantity:        math.MaxFloat64,
		StepSize:           0.00000001,
		TickSize:           0.00000001,
		QuotePrecision:     8,
		BaseAssetPrecision: 8,
	}
}

func (c CSVFeed) AssetsInfos() map[string]model.AssetInfo {
	return make(map[string]model.AssetInfo)
}

func parseHeaders(headers []string) (index map[string]int, additional []string, ok bool) {
	headerMap := map[string]int{
		"time": 0, "open": 1, "close": 2, "low": 3, "high": 4, "volume": 5,
	}

	_, err := strconv.Atoi(headers[0])
	if err == nil {
		return headerMap, additional, false
	}

	for index, h := range headers {
		if _, ok := headerMap[h]; !ok {
			additional = append(additional, h)
		}
		headerMap[h] = index
	}

	return headerMap, additional, true
}

// NewCSVFeed creates a new data feed from CSV files and resample
func NewCSVFeed(feeds ...PairFeed) (*CSVFeed, error) {
	csvFeed := &CSVFeed{
		Feeds:               make(map[string]PairFeed),
		CandlePairTimeFrame: make(map[string][]model.Candle),
	}

	for _, feed := range feeds {
		csvFeed.Feeds[feed.Pair] = feed

		csvFile, err := os.Open(feed.File)
		if err != nil {
			return nil, err
		}

		csvLines, err := csv.NewReader(csvFile).ReadAll()
		if err != nil {
			return nil, err
		}

		var candles []model.Candle
		ha := model.NewHeikinAshi()

		// map each header label with its index
		headerMap, additionalHeaders, hasCustomHeaders := parseHeaders(csvLines[0])
		if hasCustomHeaders {
			csvLines = csvLines[1:]
		}

		for _, line := range csvLines {
			timestamp, err := strconv.Atoi(line[headerMap["time"]])
			if err != nil {
				return nil, err
			}

			candle := model.Candle{
				Time:      time.Unix(int64(timestamp), 0).UTC(),
				UpdatedAt: time.Unix(int64(timestamp), 0).UTC(),
				Pair:      feed.Pair,
				Complete:  true,
			}

			candle.Open, err = strconv.ParseFloat(line[headerMap["open"]], 64)
			if err != nil {
				return nil, err
			}

			candle.Close, err = strconv.ParseFloat(line[headerMap["close"]], 64)
			if err != nil {
				return nil, err
			}

			candle.Low, err = strconv.ParseFloat(line[headerMap["low"]], 64)
			if err != nil {
				return nil, err
			}

			candle.High, err = strconv.ParseFloat(line[headerMap["high"]], 64)
			if err != nil {
				return nil, err
			}

			candle.Volume, err = strconv.ParseFloat(line[headerMap["volume"]], 64)
			if err != nil {
				return nil, err
			}

			if hasCustomHeaders {
				candle.Metadata = make(map[string]float64)
				for _, header := range additionalHeaders {
					candle.Metadata[header], err = strconv.ParseFloat(line[headerMap[header]], 64)
					if err != nil {
						return nil, err
					}
				}
			}

			if feed.HeikinAshi {
				candle = candle.ToHeikinAshi(ha)
			}

			candles = append(candles, candle)
		}

		csvFeed.CandlePairTimeFrame[csvFeed.feedTimeframeKey(feed.Pair, feed.Timeframe)] = candles
	}

	return csvFeed, nil
}

func (b CSVFeed) SetPairOption(_ context.Context, _ model.PairOption) error {
	return nil
}

func (c CSVFeed) feedTimeframeKey(pair, timeframe string) string {
	return fmt.Sprintf("%s--%s", pair, timeframe)
}

func (c CSVFeed) LastQuote(_ context.Context, _ string) (float64, error) {
	return 0, errors.New("invalid operation")
}

func (c *CSVFeed) Limit(duration time.Duration) *CSVFeed {
	for pair, candles := range c.CandlePairTimeFrame {
		start := candles[len(candles)-1].Time.Add(-duration)
		c.CandlePairTimeFrame[pair] = lo.Filter(candles, func(candle model.Candle, _ int) bool {
			return candle.Time.After(start)
		})
	}
	return c
}

func (c CSVFeed) CandlesByPeriod(_ context.Context, pair, timeframe string,
	start, end time.Time) ([]model.Candle, error) {

	key := c.feedTimeframeKey(pair, timeframe)
	candles := make([]model.Candle, 0)
	for _, candle := range c.CandlePairTimeFrame[key] {
		if candle.Time.Before(start) || candle.Time.After(end) {
			continue
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

func (c *CSVFeed) CandlesByLimit(_ context.Context, pair, timeframe string, limit int) ([]model.Candle, error) {
	var result []model.Candle
	key := c.feedTimeframeKey(pair, timeframe)
	if len(c.CandlePairTimeFrame[key]) < limit {
		return nil, fmt.Errorf("%w: %s", ErrInsufficientData, pair)
	}
	result, c.CandlePairTimeFrame[key] = c.CandlePairTimeFrame[key][:limit], c.CandlePairTimeFrame[key][limit:]
	return result, nil
}

func (c CSVFeed) CandlesSubscription(_ context.Context, pair, timeframe string) (chan model.Candle, chan error) {
	ccandle := make(chan model.Candle)
	cerr := make(chan error)
	key := c.feedTimeframeKey(pair, timeframe)
	go func() {
		for _, candle := range c.CandlePairTimeFrame[key] {
			ccandle <- candle
		}
		close(ccandle)
		close(cerr)
	}()
	return ccandle, cerr
}

func (c CSVFeed) CandlesBatchSubscription(ctx context.Context, combineConfig map[string]string) (map[string]chan model.Candle, chan error) {
	pairCcandle := make(map[string]chan model.Candle, 2000)
	cerr := make(chan error)
	for pair, timeframe := range combineConfig {
		pairCcandle[c.feedTimeframeKey(pair, timeframe)] = make(chan model.Candle)
	}

	for feedKey, candles := range c.CandlePairTimeFrame {
		go func(feedKey string, candles []model.Candle) {
			for _, candle := range candles {
				pairCcandle[feedKey] <- candle
			}
			close(pairCcandle[feedKey])
		}(feedKey, candles)
	}
	close(cerr)
	return pairCcandle, cerr
}
