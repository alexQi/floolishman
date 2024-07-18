package main

import (
	"context"
	"floolishman/bot"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/service"
	"floolishman/storage"
	"floolishman/strategies"
	"floolishman/types"
	"floolishman/utils"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/spf13/viper"
	"log"
	"strings"
)

var ConstStraties = map[string]types.Strategy{
	"Test15m":           &strategies.Test15m{},
	"Range15m":          &strategies.Range15m{},
	"Momentum15m":       &strategies.Momentum15m{},
	"MomentumVolume15m": &strategies.MomentumVolume15m{},
	"Rsi1h":             &strategies.Rsi1h{},
	"Emacross15m":       &strategies.Emacross15m{},
	"Emacross1h":        &strategies.Emacross1h{},
	"Rsi15m":            &strategies.Rsi15m{},
	"Vibrate15m":        &strategies.Vibrate15m{},
	"Kc15m":             &strategies.Kc15m{},
}

func main() {
	// 获取基础配置
	var (
		ctx            = context.Background()
		telegramToken  = viper.GetString("telegram.token")
		telegramUser   = viper.GetInt("telegram.user")
		tradingSetting = service.StrategySetting{
			CheckMode:            viper.GetString("trading.checkMode"),
			FullSpaceRadio:       viper.GetFloat64("trading.fullSpaceRadio"),
			BaseLossRatio:        viper.GetFloat64("trading.baseLossRatio"),
			LossTimeDuration:     viper.GetInt("trading.lossTimeDuration"),
			ProfitableScale:      viper.GetFloat64("trading.profitableScale"),
			InitProfitRatioLimit: viper.GetFloat64("trading.initProfitRatioLimit"),
		}
		pairsSetting      = viper.GetStringMap("pairs")
		strategiesSetting = viper.GetStringSlice("strategies")
	)

	utils.Log.SetLevel(6)
	tradingSetting.CheckMode = "candle"

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

	compositesStrategy := types.CompositesStrategy{}
	for _, strategyName := range strategiesSetting {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, ConstStraties[strategyName])
	}

	csvFeed, err := exchange.NewCSVFeed(
		exchange.PairFeed{
			Pair:      "ETHUSDT",
			File:      "testdata/eth-15m.csv",
			Timeframe: "15m",
		},
		//exchange.PairFeed{
		//	Pair:      "ETHUSDT",
		//	File:      "testdata/eth-1h.csv",
		//	Timeframe: "1h",
		//},
	)

	// initialize a database in memory
	//memory, err := storage.FromFile("runtime/data/backtest.db")
	memory, err := storage.FromMemory()
	if err != nil {
		log.Fatal(err)
	}
	// create a paper wallet for simulation, initializing with 10.000 USDT
	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", 10000),
		exchange.WithDataFeed(csvFeed),
	)

	b, err := bot.NewBot(
		ctx,
		settings,
		wallet,
		tradingSetting,
		compositesStrategy,
		bot.WithBacktest(wallet),
		bot.WithStorage(memory),
	)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
