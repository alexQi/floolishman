package main

import (
	"context"
	"floolishman/bot"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/storage"
	"floolishman/strategies"
	"floolishman/types"
	"floolishman/utils"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var ConstStraties = map[string]types.Strategy{
	"Range15m":          &strategies.Range15m{},
	"Momentum15m":       &strategies.Momentum15m{},
	"MomentumVolume15m": &strategies.MomentumVolume15m{},
	"Rsi1h":             &strategies.Rsi1h{},
	"Emacross15m":       &strategies.Emacross15m{},
	"Emacross1h":        &strategies.Emacross1h{},
	"Rsi15m":            &strategies.Rsi15m{},
	"Vibrate15m":        &strategies.Vibrate15m{},
	"Kc15m":             &strategies.Kc15m{},
	"Grid1h":            &strategies.Grid1h{},
}

func main() {
	// 获取基础配置
	var (
		ctx           = context.Background()
		mode          = viper.GetString("mode")
		apiKeyType    = viper.GetString("api.encrypt")
		apiKey        = viper.GetString("api.key")
		secretKey     = viper.GetString("api.secret")
		secretPem     = viper.GetString("api.pem")
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
		proxyStatus   = viper.GetBool("proxy.status")
		proxyUrl      = viper.GetString("proxy.url")
		callerSetting = types.CallerSetting{
			CheckMode:            viper.GetString("caller.checkMode"),
			LossTimeDuration:     viper.GetInt("caller.lossTimeDuration"),
			MaxAddPostion:        viper.GetInt64("caller.maxAddPostion"),          // 最大加仓次数
			MaxPositionHedge:     viper.GetBool("caller.maxPositionHedge"),        // 最大仓位后是否开启对冲
			MaxPositionLossRatio: viper.GetFloat64("caller.maxPositionLossRatio"), // 加仓后最大亏损比例
			WindowPeriod:         viper.GetFloat64("caller.windowPeriod"),         // 空窗期点数
			FullSpaceRatio:       viper.GetFloat64("caller.fullSpaceRatio"),
			StopSpaceRatio:       viper.GetFloat64("caller.stopSpaceRatio"),
			BaseLossRatio:        viper.GetFloat64("caller.baseLossRatio"),
			ProfitableScale:      viper.GetFloat64("caller.profitableScale"),
			InitProfitRatioLimit: viper.GetFloat64("caller.initProfitRatioLimit"),
		}
		pairsSetting      = viper.GetStringMap("pairs")
		strategiesSetting = viper.GetStringSlice("strategies")
	)

	settings := model.Settings{
		GuiderGrpcHost: viper.GetString("watchdog.host"),
		PairOptions:    []model.PairOption{},
		Telegram: model.TelegramSettings{
			Enabled: false,
			Token:   telegramToken,
			Users:   []int{telegramUser},
		},
	}
	callerSetting.GuiderHost = settings.GuiderGrpcHost

	for pair, val := range pairsSetting {
		valMap := val.(map[string]interface{})

		// 检查并处理 leverage
		leverageFloat, ok := valMap["leverage"].(float64)
		if !ok {
			log.Fatalf("Invalid leverage format for pair %s: %v", pair, valMap["leverage"])
		}
		// 检查并处理 leverage
		gridStep, ok := valMap["gridstep"].(float64)
		if !ok {
			log.Fatalf("Invalid gridStep format for pair %s: %v", pair, valMap["gridStep"])
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
			GridStep:   gridStep,
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
		//exchange.WithBinanceFuturesDebugMode(),
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
	storagePath := viper.GetString("storage.path")
	dir := filepath.Dir(storagePath)
	// 判断文件目录是否存在
	_, err = os.Stat(dir)
	if err != nil {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			utils.Log.Panicf("mkdir error : %s", err.Error())
		}
	}
	st, err := storage.FromSQL(sqlite.Open(storagePath))
	if err != nil {
		log.Fatal(err)
	}
	b, err := bot.NewBot(
		ctx,
		settings,
		binance,
		callerSetting,
		compositesStrategy,
		bot.WithStorage(st),
		bot.WithProxy(types.ProxyOption{
			Status: proxyStatus,
			Url:    proxyUrl,
		}),
	)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
