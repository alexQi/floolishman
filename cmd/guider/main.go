package main

import (
	"floolishman/model"
	"floolishman/serv"
	"floolishman/storage"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/strutil"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
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
		pairOption := model.BuildPairOption(pair, val.(map[string]interface{}))
		if pairOption.Status == false {
			continue
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
