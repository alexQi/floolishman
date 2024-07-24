package main

import (
	"floolishman/model"
	"floolishman/serv"
	"floolishman/storage"
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
		proxyStatus   = viper.GetBool("proxy.status")
		proxyUrl      = viper.GetString("proxy.url")
		pairsSetting  = viper.GetStringMap("pairs")
		guiderSetting = viper.Get("guiders")
	)
	// 转换配置
	guiderConfigMap := guiderSetting.(map[string]interface{})
	guiderConfigs := strutil.ConvertToNestedStringMap(guiderConfigMap)
	// 获取交易对配置
	pairOptions := []model.PairOption{}
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
		pairOptions = append(pairOptions, model.PairOption{
			Pair:       strings.ToUpper(pair),
			Leverage:   leverage,
			MarginType: futures.MarginType(strings.ToUpper(marginType)), // 假设 futures.MarginType 是一个类型别名
		})
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
	st, err := storage.FromSQL(sqlite.Open(storagePath))
	if err != nil {
		utils.Log.Fatal(err)
	}
	proxyOption := types.ProxyOption{
		Status: proxyStatus,
		Url:    proxyUrl,
	}
	serv.StartGrpcServer(guiderConfigs, pairOptions, proxyOption, st)
}
