package main

import (
	"context"
	"floolishman/bot"
	"floolishman/constants"
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
)

var ConstStraties = map[string]types.Strategy{
	"Test15m":           &strategies.Test15m{},
	"Range15m":          &strategies.Range15m{},
	"Momentum15m":       &strategies.Momentum15m{},
	"MomentumVolume15m": &strategies.MomentumVolume15m{},
	"Rsi1h":             &strategies.Rsi1h{},
	"Emacross15m":       &strategies.Emacross15m{},
	"Resonance15m":      &strategies.Resonance15m{},
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
			CheckMode:                 viper.GetString("caller.checkMode"),
			LossTimeDuration:          viper.GetInt("caller.lossTimeDuration"),
			IgnorePairs:               viper.GetStringSlice("caller.ignorePairs"),
			Leverage:                  viper.GetInt("caller.leverage"),
			MarginType:                futures.MarginType(viper.GetString("caller.marginType")),
			MarginMode:                constants.MarginMode(viper.GetString("caller.marginMode")),
			MarginSize:                viper.GetFloat64("caller.marginSize"),
			ProfitableScale:           viper.GetFloat64("caller.profitableScale"),
			ProfitableScaleDecrStep:   viper.GetFloat64("caller.profitableScaleDecrStep"),
			ProfitableTrigger:         viper.GetFloat64("caller.profitableTrigger"),
			ProfitableTriggerIncrStep: viper.GetFloat64("caller.profitableTriggerIncrStep"),
			PullMarginLossRatio:       viper.GetFloat64("caller.pullMarginLossRatio"),
			MaxMarginRatio:            viper.GetFloat64("caller.maxMarginRatio"),
			MaxMarginLossRatio:        viper.GetFloat64("caller.maxMarginLossRatio"),
			PauseCaller:               viper.GetInt64("caller.pauseCaller"),
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
		pairOption := model.BuildPairOption(model.PairOption{
			Pair:                      pair,
			Leverage:                  callerSetting.Leverage,
			MarginType:                callerSetting.MarginType,
			MarginMode:                callerSetting.MarginMode,
			MarginSize:                callerSetting.MarginSize,
			ProfitableScale:           callerSetting.ProfitableScale,
			ProfitableScaleDecrStep:   callerSetting.ProfitableScaleDecrStep,
			ProfitableTrigger:         callerSetting.ProfitableTrigger,
			ProfitableTriggerIncrStep: callerSetting.ProfitableTriggerIncrStep,
			PullMarginLossRatio:       callerSetting.PullMarginLossRatio,
			MaxMarginRatio:            callerSetting.MaxMarginRatio,
			MaxMarginLossRatio:        callerSetting.MaxMarginLossRatio,
			PauseCaller:               callerSetting.PauseCaller,
		}, val.(map[string]interface{}))
		settings.PairOptions = append(settings.PairOptions, pairOption)
	}

	compositesStrategy := types.CompositesStrategy{}
	for _, strategyName := range strategiesSetting {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, ConstStraties[strategyName])
	}

	pairFeeds := []exchange.PairFeed{}
	for _, option := range settings.PairOptions {
		if option.Status == false {
			continue
		}
		pairFeeds = append(pairFeeds, exchange.PairFeed{
			Pair:      option.Pair,
			File:      "testdata/eth-15m.csv",
			Timeframe: "15m",
		})
	}

	csvFeed, err := exchange.NewCSVFeed(pairFeeds...)

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
