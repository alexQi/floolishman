package controllers

import (
	"floolishman/exchange"
	"floolishman/utils"
	"github.com/kataras/iris/v12"
	"github.com/spf13/viper"
	"os"
	"strconv"
)

var (
	apiKeyType  = viper.GetString("api.encrypt")
	apiKey      = viper.GetString("api.key")
	secretKey   = viper.GetString("api.secret")
	secretPem   = viper.GetString("api.pem")
	proxyStatus = viper.GetBool("proxy.status")
	proxyUrl    = viper.GetString("proxy.url")
)

type ExchangeController struct {
	BaseController
}

// Check 目标用户检测接口
func (c *ExchangeController) GetOrder(ctx iris.Context) error {
	data := map[string]interface{}{
		"code":    "0",
		"message": "success",
	}
	pair := ctx.URLParamTrim("pair")
	orderIdString := ctx.URLParamTrim("orderId")

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
	if proxyStatus {
		exhangeOptions = append(
			exhangeOptions,
			exchange.WithBinanceFutureProxy(proxyUrl),
		)
	}

	// Initialize your exchange with futures
	binance, err := exchange.NewBinanceFuture(ctx, exhangeOptions...)
	if err != nil {
		data["code"] = "100"
		data["message"] = err.Error()
		return ctx.JSON(data)
	}
	orderId, err := strconv.Atoi(orderIdString)
	if err != nil {
		return err
	}
	order, err := binance.Order(pair, int64(orderId))
	if err != nil {
		data["code"] = "100"
		data["message"] = err.Error()
		return ctx.JSON(data)
	}
	data["data"] = order
	// 返回响应
	return ctx.JSON(data)
}
