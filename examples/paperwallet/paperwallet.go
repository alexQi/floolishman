package main

import (
	"context"
	"floolisher"
	"floolisher/model"
	"floolisher/plot"
	"floolisher/plot/indicator"
	"floolisher/types"
	"github.com/spf13/viper"

	"floolisher/exchange"
	"floolisher/storage"
	"floolisher/strategies"
	"floolisher/tools/log"
)

// This example shows how to use NinjaBot with a simulation with a fake exchange
// A peperwallet is a wallet that is not connected to any exchange, it is a simulation with live data (realtime)
func main() {
	var (
		ctx           = context.Background()
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
	)

	settings := model.Settings{
		Pairs: []string{
			"BTCUSDT",
			"ETHUSDT",
			"BNBUSDT",
			"LTCUSDT",
		},
		Telegram: model.TelegramSettings{
			Enabled: telegramToken != "" && telegramUser != 0,
			Token:   telegramToken,
			Users:   []int{telegramUser},
		},
	}

	// Use binance for realtime data feed
	binance, err := exchange.NewBinance(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// creating a storage to save trades
	store, err := storage.FromMemory()
	if err != nil {
		log.Fatal(err)
	}

	// creating a paper wallet to simulate an exchange waller for fake operataions
	// paper wallet is simulation of a real exchange wallet
	paperWallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperFee(0.001, 0.001),
		exchange.WithPaperAsset("USDT", 10000),
		exchange.WithDataFeed(binance),
	)

	// initializing my strategy
	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			&strategies.RSI15Min{},
		},
	}

	chart, err := plot.NewChart(
		plot.WithCustomIndicators(
			indicator.EMA(8, "red"),
			indicator.SMA(21, "blue"),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	// initializer ninjabot
	bot, err := floolisher.NewBot(
		ctx,
		settings,
		paperWallet,
		compositesStrategy,
		floolisher.WithStorage(store),
		floolisher.WithPaperWallet(paperWallet),
		floolisher.WithCandleSubscription(chart),
		floolisher.WithOrderSubscription(chart),
	)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := chart.Start()
		if err != nil {
			log.Fatal(err)
		}
	}()

	err = bot.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}
