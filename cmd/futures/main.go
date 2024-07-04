package main

import (
	"context"
	"floolishman/bot"
	"floolishman/constants"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/strategies"
	"floolishman/types"
	"floolishman/utils"
	"github.com/spf13/viper"
	"os"
)

func main() {
	// 获取基础配置
	var (
		ctx           = context.Background()
		apiKeyType    = viper.GetString("api.encrypt")
		apiKey        = viper.GetString("api.key")
		secretKey     = viper.GetString("api.secret")
		secretPem     = viper.GetString("api.pem")
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
		proxyStatus   = viper.GetBool("proxy.status")
		proxyUrl      = viper.GetString("proxy.url")
	)

	// 设置需要处理的交易对
	settings := model.Settings{
		PairOptions: []model.PairOption{
			{
				Pair:       "ETHUSDT",
				Leverage:   100,
				MarginType: constants.MarginTypeCrossed,
			},
			//{
			//	Pair:       "BTCUSDT",
			//	Leverage:   100,
			//	MarginType: constants.MarginTypeCrossed,
			//},
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
			utils.Log.Fatalf("error with load pem file:%s", err.Error())
		}
		secretKey = string(tempSecretKey)
	}

	exhangeOptions := []exchange.BinanceFutureOption{
		exchange.WithBinanceFutureCredentials(apiKey, secretKey, apiKeyType),
	}
	if proxyStatus {
		exhangeOptions = append(
			exhangeOptions,
			exchange.WithBinanceFutureProxy(proxyUrl),
		)
	}

	// Initialize your exchange with futures
	binance, err := exchange.NewBinanceFuture(ctx, exhangeOptions...)
	if err != nil {
		utils.Log.Fatal(err)
	}

	// initialize your strategy
	compositesStrategy := types.CompositesStrategy{
		Strategies: []types.Strategy{
			//&strategies.Rsi1m{},
			//&strategies.Emacross1m{},
			&strategies.Momentum15m{},
			&strategies.Rsi15m{},
			&strategies.Rsi1h{},
			&strategies.Emacross15m{},
			&strategies.Emacross1h{},
			&strategies.Emacross4h{},
		},
	}
	b, err := bot.NewBot(ctx, settings, binance, compositesStrategy)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
