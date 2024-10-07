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
	"floolishman/utils/strutil"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"log"
	"os"
	"path/filepath"
	"strings"
)

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
	// 判断是否是选币模式
	if callerSetting.CheckMode == "scoop" {
		coinAssetInfos := binance.AssetsInfos()
		for pair, assetInfo := range coinAssetInfos {
			if strutil.ContainsString(callerSetting.IgnorePairs, pair) {
				continue
			}
			if strutil.ContainsString(callerSetting.AllowPairs, pair) == false {
				continue
			}
			if assetInfo.QuoteAsset != "USDT" {
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
				Leverage:                  callerSetting.Leverage,
				IgnoreHours:               callerSetting.IgnoreHours,
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
	if callerSetting.CheckMode == "grid" {
		compositesStrategy.Strategies = append(compositesStrategy.Strategies, &strategies.Grid1h{})
	} else {
		for _, strategyName := range strategiesSetting {
			compositesStrategy.Strategies = append(compositesStrategy.Strategies, constants.ConstStraties[strategyName])
		}
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
