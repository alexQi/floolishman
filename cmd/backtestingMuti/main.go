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
			PositionTimeOut:           viper.GetInt("caller.positionTimeout"),
			LossTrigger:               viper.GetInt("caller.lossTrigger"),
			LossPauseMin:              viper.GetFloat64("caller.lossPauseMin"),
			LossPauseMax:              viper.GetFloat64("caller.lossPauseMax"),
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
	compositesStrategy := model.CompositesStrategy{}
	for _, strategyName := range strategiesSetting {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, constants.ConstStraties[strategyName])
	}

	// ********************
	pairOptions := []model.PairOption{}
	for _, pair := range callerSetting.AllowPairs {
		//if strutil.ContainsString(callerSetting.IgnorePairs, pair) {
		//	continue
		//}
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
		pairOptions = append(pairOptions, pairOption)
	}

	storagePath := viper.GetString("storage.path")
	dir := filepath.Dir(storagePath)
	// 判断文件目录是否存在
	_, err := os.Stat(dir)
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

	// 临时使用
	settings.PairOptions = pairOptions
	pairFeeds := []exchange.PairFeed{}

	for _, val := range pairOptions {
		pairFeeds = append(pairFeeds, exchange.PairFeed{
			Pair:      val.Pair,
			File:      fmt.Sprintf("testdata/%s-%s.csv", val.Pair, "30m"),
			Timeframe: "30m",
		})
	}

	csvFeed, err := exchange.NewCSVFeed("30m", pairFeeds...)
	if err != nil {
		log.Fatal(err)
	}
	// create a paper wallet for simulation, initializing with 10.000 USDT
	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", 1000),
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
