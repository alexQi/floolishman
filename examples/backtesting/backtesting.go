package main

import (
	"context"
	"floolisher"
	"floolisher/model"
	"floolisher/types"

	"floolisher/exchange"
	"floolisher/plot"
	"floolisher/plot/indicator"
	"floolisher/storage"
	"floolisher/strategies"
	"floolisher/tools/log"
)

// This example shows how to use backtesting with NinjaBot
// Backtesting is a simulation of the strategy in historical data (from CSV)
func main() {
	ctx := context.Background()

	// bot settings (eg: pairs, telegram, etc)
	settings := model.Settings{
		Pairs: []string{
			"BTCUSDT",
			//"ETHUSDT",
		},
	}
	// initialize your strategy
	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			&strategies.RSI15Min{},
			&strategies.CrossEMA{},
			&strategies.Turtle{},
		},
	}
	dataFeeds := []exchange.PaperWalletOption{
		exchange.WithPaperAsset("USDT", 50),
	}
	for _, strategy := range compositesStrategy.Strategies {
		// load historical data from CSV files
		csvFeed, err := exchange.NewCSVFeed(
			strategy.Timeframe(),
			exchange.PairFeed{
				Pair:      "BTCUSDT",
				File:      "testdata/btc-15m.csv",
				Timeframe: "15m",
			},
			exchange.PairFeed{
				Pair:      "BTCUSDT",
				File:      "testdata/btc-4h.csv",
				Timeframe: "4h",
			},
		)
		if err != nil {
			log.Fatal(err)
		}
		dataFeeds = append(dataFeeds, exchange.WithDataFeed(csvFeed))
	}

	// initialize a database in memory
	store, err := storage.FromMemory()
	if err != nil {
		log.Fatal(err)
	}

	// create a paper wallet for simulation, initializing with 10.000 USDT
	wallet := exchange.NewPaperWallet(ctx, "USDT", dataFeeds...)

	// create a chart  with indicators from the strategy and a custom additional RSI indicator
	chart, err := plot.NewChart(
		plot.WithStrategyIndicators(compositesStrategy),
		plot.WithCustomIndicators(
			indicator.RSI(2, "purple"),
			indicator.RSI(3, "red"),
		),
		plot.WithPaperWallet(wallet),
	)
	if err != nil {
		log.Fatal(err)
	}

	// initializer Ninjabot with the objects created before
	bot, err := floolisher.NewBot(
		ctx,
		settings,
		wallet,
		compositesStrategy,
		floolisher.WithBacktest(wallet), // Required for Backtest mode
		floolisher.WithStorage(store),

		// connect bot feed (candle and orders) to the chart
		floolisher.WithCandleSubscription(chart),
		floolisher.WithOrderSubscription(chart),
		floolisher.WithLogLevel(log.WarnLevel),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Initializer simulation
	err = bot.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Print bot results
	bot.Summary()

	// Display candlesticks chart in local browser
	err = chart.Start()
	if err != nil {
		log.Fatal(err)
	}
}
