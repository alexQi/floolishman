package floolisher

import (
	"context"
	"floolisher/strategies"
	"floolisher/types"
	"testing"

	"github.com/markcheno/go-talib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"floolisher/exchange"
	"floolisher/service"
	"floolisher/storage"
)

type fakeStrategy struct{}

func (e fakeStrategy) Timeframe() string {
	return "1d"
}

func (e fakeStrategy) WarmupPeriod() int {
	return 10
}

func (e fakeStrategy) Indicators(df *Dataframe) []types.ChartIndicator {
	df.Metadata["ema9"] = talib.Ema(df.Close, 9)
	return nil
}

func (e *fakeStrategy) OnCandle(df *Dataframe, broker service.Broker) {
	closePrice := df.Close.Last(0)
	assetPosition, quotePosition, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
	}

	if quotePosition > 0 && df.Close.Crossover(df.Metadata["ema9"]) {
		_, err := broker.CreateOrderMarket(SideTypeBuy, df.Pair, quotePosition/closePrice*0.5)
		if err != nil {
			log.Fatal(err)
		}
	}

	if assetPosition > 0 &&
		df.Close.Crossunder(df.Metadata["ema9"]) {
		_, err := broker.CreateOrderMarket(SideTypeSell, df.Pair, assetPosition)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func TestMarketOrder(t *testing.T) {
	ctx := context.Background()

	storage, err := storage.FromMemory()
	require.NoError(t, err)

	// initialize your strategy
	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			&strategies.RSI15Min{},
		},
	}
	dataFeeds := []exchange.PaperWalletOption{
		exchange.WithPaperAsset("USDT", 10000),
	}
	for _, strategy := range compositesStrategy.Strategies {
		// load historical data from CSV files
		csvFeed, err := exchange.NewCSVFeed(
			strategy.Timeframe(),
			exchange.PairFeed{
				Pair:      "BTCUSDT",
				File:      "testdata/btc-1h.csv",
				Timeframe: "1h",
			},
			exchange.PairFeed{
				Pair:      "ETHUSDT",
				File:      "testdata/eth-1h.csv",
				Timeframe: "1h",
			},
		)
		if err != nil {
			log.Fatal(err)
		}
		dataFeeds = append(dataFeeds, exchange.WithDataFeed(csvFeed))
	}
	require.NoError(t, err)

	paperWallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		dataFeeds...,
	)

	bot, err := NewBot(ctx, Settings{
		Pairs: []string{
			"BTCUSDT",
			"ETHUSDT",
		},
	},
		paperWallet,
		compositesStrategy,
		WithStorage(storage),
		WithBacktest(paperWallet),
		WithLogLevel(log.ErrorLevel),
	)
	require.NoError(t, err)
	require.NoError(t, bot.Run(ctx))

	assets, quote, err := bot.paperWallet.Position("BTCUSDT")
	require.NoError(t, err)
	require.Equal(t, assets, 0.0)
	require.InDelta(t, quote, 22930.9622, 0.001)

	results := bot.orderController.Results["BTCUSDT"]
	require.InDelta(t, 5340.224, results.Profit(), 0.001)
	require.Len(t, results.Win(), 5)
	require.Len(t, results.Lose(), 3)

	results = bot.orderController.Results["ETHUSDT"]
	require.InDelta(t, 7590.7381, results.Profit(), 0.001)
	require.Len(t, results.Win(), 7)
	require.Len(t, results.Lose(), 9)

	bot.Summary()
}
