package main

import (
	"context"
	"floolisher"
	"floolisher/exchange"
	"floolisher/model"
	"floolisher/strategies"
	"floolisher/types"
	"github.com/spf13/viper"
	"log"
	"os"
)

// This example shows how to use futures market with NinjaBot.
func main() {
	var (
		ctx           = context.Background()
		apiKeyType    = viper.GetString("api.encrypt")
		apiKey        = viper.GetString("api.key")
		secretKey     = viper.GetString("api.secret")
		secretPem     = viper.GetString("api.pem")
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
	)

	settings := model.Settings{
		Pairs: []string{
			//"BTCUSDT",
			"ETHUSDT",
		},
		Telegram: model.TelegramSettings{
			Enabled: false,
			Token:   telegramToken,
			Users:   []int{telegramUser},
		},
	}

	if apiKeyType != "HMAC" {
		tempSecretKey, err := os.ReadFile(secretPem)
		if err != nil {
			log.Fatalf("error with load pem file:%s", err.Error())
		}
		secretKey = string(tempSecretKey)
	}

	// Initialize your exchange with futures
	binance, err := exchange.NewBinanceFuture(ctx,
		exchange.WithBinanceFutureCredentials(apiKey, secretKey, apiKeyType),
		exchange.WithBinanceFutureLeverage("ETHUSDT", 100, exchange.MarginTypeIsolated),
	)
	if err != nil {
		log.Fatal(err)
	}

	// initialize your strategy
	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			&strategies.RSI15Min{},
		},
	}
	bot, err := floolisher.NewBot(ctx, settings, binance, compositesStrategy)
	if err != nil {
		log.Fatalln(err)
	}

	err = bot.Run(ctx)
	if err != nil {
		log.Fatalln(err)
	}
}
