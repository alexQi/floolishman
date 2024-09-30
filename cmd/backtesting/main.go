package main

import (
	"context"
	"floolishman/bot"
	"floolishman/constants"
	"floolishman/exchange"
	"floolishman/model"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/fileutil"
	"floolishman/utils/strutil"
	"fmt"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// 获取基础配置
	var (
		ctx           = context.Background()
		telegramToken = viper.GetString("telegram.token")
		telegramUser  = viper.GetInt("telegram.user")
		callerSetting = types.CallerSetting{
			CheckMode:                 viper.GetString("caller.checkMode"),
			LossTimeDuration:          viper.GetInt("caller.lossTimeDuration"),
			AllowPairs:                viper.GetStringSlice("caller.allowPairs"),
			IgnorePairs:               viper.GetStringSlice("caller.ignorePairs"),
			IgnoreHours:               viper.GetIntSlice("caller.ignoreHours"),
			Leverage:                  viper.GetInt("caller.leverage"),
			MarginType:                futures.MarginType(viper.GetString("caller.marginType")),
			MarginMode:                model.MarginMode(viper.GetString("caller.marginMode")),
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
	// 判断是否是选币模式
	var dataCsvPath string
	if callerSetting.CheckMode == "scoop" {
		for _, pair := range callerSetting.AllowPairs {
			if strutil.ContainsString(callerSetting.IgnorePairs, pair) {
				continue
			}
			dataCsvPath = fmt.Sprintf("testdata/%s-%s.csv", pair, "30m")
			exists, err := fileutil.PathExists(dataCsvPath)
			if err != nil {
				utils.Log.Error(err)
				return
			}
			if !exists {
				continue
			}
			pairOption := model.PairOption{
				Pair:                      strings.ToUpper(pair),
				Status:                    true,
				IgnoreHours:               callerSetting.IgnoreHours,
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
			}
			settings.PairOptions = append(settings.PairOptions, pairOption)
			if len(settings.PairOptions) == 200 {
				break
			}
		}
	} else {
		for pair, val := range pairsSetting {
			pairOption := model.BuildPairOption(model.PairOption{
				Pair:                      strings.ToUpper(pair),
				IgnoreHours:               callerSetting.IgnoreHours,
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
	}

	compositesStrategy := model.CompositesStrategy{}
	for _, strategyName := range strategiesSetting {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, constants.ConstStraties[strategyName])
	}
	pairFeeds := []exchange.PairFeed{}
	for _, option := range settings.PairOptions {
		if option.Status == false {
			continue
		}
		dataCsvPath = fmt.Sprintf("testdata/%s-%s.csv", option.Pair, "1m")
		pairFeeds = append(pairFeeds, exchange.PairFeed{
			Pair:      option.Pair,
			File:      dataCsvPath,
			Timeframe: "1m",
		})
	}

	csvFeed, err := exchange.NewCSVFeed("30m", pairFeeds...)
	if err != nil {
		log.Fatal(err)
	}
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
		exchange.WithPaperAsset("USDT", 850),
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
