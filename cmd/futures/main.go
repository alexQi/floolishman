package main

import (
	"context"
	"floolishman/bot"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/service"
	"floolishman/strategies"
	"floolishman/types"
	"floolishman/utils"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/spf13/viper"
	"log"
	"os"
	"strings"
)

func main() {
	// 获取基础配置
	var (
		ctx            = context.Background()
		mode           = viper.GetString("mode")
		apiKeyType     = viper.GetString("api.encrypt")
		apiKey         = viper.GetString("api.key")
		secretKey      = viper.GetString("api.secret")
		secretPem      = viper.GetString("api.pem")
		telegramToken  = viper.GetString("telegram.token")
		telegramUser   = viper.GetInt("telegram.user")
		proxyStatus    = viper.GetBool("proxy.status")
		proxyUrl       = viper.GetString("proxy.url")
		tradingSetting = service.StrategyServiceSetting{
			VolatilityThreshold:  viper.GetFloat64("trading.volatilityThreshold"),
			FullSpaceRadio:       viper.GetFloat64("trading.fullSpaceRadio"),
			InitLossRatio:        viper.GetFloat64("trading.initLossRatio"),
			ProfitableScale:      viper.GetFloat64("trading.profitableScale"),
			InitProfitRatioLimit: viper.GetFloat64("trading.initProfitRatioLimit"),
		}
		pairsSetting = viper.GetStringMap("pairs")
	)

	settings := model.Settings{
		PairOptions: []model.PairOption{},
		Telegram: model.TelegramSettings{
			Enabled: false,
			Token:   telegramToken,
			Users:   []int{telegramUser},
		},
	}
	for pair, val := range pairsSetting {
		valMap := val.(map[string]interface{})

		// 检查并处理 leverage
		leverageFloat, ok := valMap["leverage"].(float64)
		if !ok {
			log.Fatalf("Invalid leverage format for pair %s: %v", pair, valMap["leverage"])
		}

		marginType, ok := valMap["margintype"].(string)
		if !ok {
			log.Fatalf("Invalid marginType format for pair %s", pair)
		}

		// 将 leverage 从 float64 转换为 int
		leverage := int(leverageFloat)

		settings.PairOptions = append(settings.PairOptions, model.PairOption{
			Pair:       strings.ToUpper(pair),
			Leverage:   leverage,
			MarginType: futures.MarginType(strings.ToUpper(marginType)), // 假设 futures.MarginType 是一个类型别名
		})
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
	if mode == "test" {
		exhangeOptions = append(
			exhangeOptions,
			exchange.WithBinanceFutureTestnet(),
		)
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
			&strategies.Range15m{},
			&strategies.Momentum15m{},
			//&strategies.Momentum1h{},
			&strategies.Rsi1h{},
			&strategies.Emacross15m{},
			&strategies.Emacross1h{},

			//&strategies.Rsi1m{},

			//&strategies.Emacross1m{},
			//&strategies.Rsi15m{},
			//&strategies.Emacross4h{},
		},
	}
	b, err := bot.NewBot(ctx, settings, binance, tradingSetting, compositesStrategy)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
