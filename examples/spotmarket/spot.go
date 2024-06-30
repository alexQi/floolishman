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
)

// This example shows how to use spot market with NinjaBot in Binance
func main() {
	var (
		ctx           = context.Background()
		apiKey        = viper.GetString("api.key")
		secretKey     = viper.GetString("api.secret")
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
	)

	settings := model.Settings{
		Pairs: []string{
			"BTCUSDT",
			"ETHUSDT",
		},
		Telegram: model.TelegramSettings{
			Enabled: true,
			Token:   telegramToken,
			Users:   []int{telegramUser},
		},
	}

	// Initialize your exchange
	binance, err := exchange.NewBinance(ctx, exchange.WithBinanceCredentials(apiKey, secretKey))
	if err != nil {
		log.Fatalln(err)
	}

	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			&strategies.RSI15Min{},
		},
	}

	// Initialize your strategy and bot
	bot, err := floolisher.NewBot(ctx, settings, binance, compositesStrategy)
	if err != nil {
		log.Fatalln(err)
	}

	err = bot.Run(ctx)
	if err != nil {
		log.Fatalln(err)
	}
}
