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
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
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
	"Macd4h":            &strategies.Macd4h{},
	"Grid1h":            &strategies.Grid1h{},
}

func main() {
	// 获取基础配置
	var (
		ctx           = context.Background()
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
		callerSetting = types.CallerSetting{
			CheckMode:            viper.GetString("caller.checkMode"),
			LossTimeDuration:     viper.GetInt("caller.lossTimeDuration"),
			MaxAddPostion:        viper.GetInt64("caller.maxAddPostion"),          // 最大加仓次数
			MinAddPostion:        viper.GetInt64("caller.minAddPostion"),          // 最小加仓次数
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

	utils.Log.SetLevel(6)
	callerSetting.CheckMode = "candle"

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
		//	File:      "testdata/eth-4h.csv",
		//	Timeframe: "4h",
		//},
		//exchange.PairFeed{
		//	Pair:      "BTCUSDT",
		//	File:      "testdata/btc-15m.csv",
		//	Timeframe: "15m",
		//},
	)

	// initialize a database in memory
	//memory, err := storage.FromFile("runtime/data/backtest.db")
	//memory, err := storage.FromMemory()
	//if err != nil {
	//	log.Fatal(err)
	//}

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
	st, err := storage.FromSQL(sqlite.Open(storagePath), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	// 重置表数据
	err = st.ResetTables()
	if err != nil {
		return
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
		callerSetting,
		compositesStrategy,
		bot.WithBacktest(wallet),
		bot.WithStorage(st),
	)
	if err != nil {
		utils.Log.Fatalln(err)
	}

	b.Run(ctx)
}
