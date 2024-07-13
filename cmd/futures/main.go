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

var ConstStraties = map[string]types.Strategy{
	"Range15m":    &strategies.Range15m{},
	"Momentum15m": &strategies.Momentum15m{},
	"Rsi1h":       &strategies.Rsi1h{},
	"Emacross15m": &strategies.Emacross15m{},
	"Emacross1h":  &strategies.Emacross1h{},
	"Momentum1h":  &strategies.Momentum1h{},
	"Rsi15m":      &strategies.Rsi15m{},
}

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
		tradingSetting = service.StrategySetting{
			CheckMode:            viper.GetString("trading.checkMode"),
			FullSpaceRadio:       viper.GetFloat64("trading.fullSpaceRadio"),
			LossTimeDuration:     viper.GetInt("trading.lossTimeDuration"),
			BaseLossRatio:        viper.GetFloat64("trading.baseLossRatio"),
			ProfitableScale:      viper.GetFloat64("trading.profitableScale"),
			InitProfitRatioLimit: viper.GetFloat64("trading.initProfitRatioLimit"),
		}
		pairsSetting      = viper.GetStringMap("pairs")
		strategiesSetting = viper.GetStringSlice("strategies")
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

	compositesStrategy := types.CompositesStrategy{}
	for _, strategyName := range strategiesSetting {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, ConstStraties[strategyName])
	}
	b, err := bot.NewBot(ctx, settings, binance, tradingSetting, compositesStrategy)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
