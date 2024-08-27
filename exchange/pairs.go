package exchange

import (
	"context"
	_ "embed"
	"encoding/json"
	"floolishman/utils"
	"fmt"
	"github.com/spf13/viper"
	"os"
)

type AssetQuote struct {
	Quote string
	Asset string
}

var (
	//go:embed pairs.json
	pairs             []byte
	pairAssetQuoteMap = make(map[string]AssetQuote)
)

func init() {
	err := json.Unmarshal(pairs, &pairAssetQuoteMap)
	if err != nil {
		panic(err)
	}
}

func SplitAssetQuote(pair string) (asset string, quote string) {
	data := pairAssetQuoteMap[pair]
	return data.Asset, data.Quote
}

func UpdateParisFile(isFuture bool) error {
	var (
		ctx         = context.Background()
		apiKeyType  = viper.GetString("api.encrypt")
		apiKey      = viper.GetString("api.key")
		secretKey   = viper.GetString("api.secret")
		secretPem   = viper.GetString("api.pem")
		proxyStatus = viper.GetBool("proxy.status")
		proxyUrl    = viper.GetString("proxy.url")
	)

	if apiKeyType != "HMAC" {
		tempSecretKey, err := os.ReadFile(secretPem)
		if err != nil {
			utils.Log.Fatalf("error with load pem file:%s", err.Error())
		}
		secretKey = string(tempSecretKey)
	}

	if isFuture {
		exhangeOptions := []BinanceFutureOption{
			WithBinanceFutureCredentials(apiKey, secretKey, apiKeyType),
		}
		if proxyStatus {
			exhangeOptions = append(
				exhangeOptions,
				WithBinanceFutureProxy(proxyUrl),
			)
		}

		// Initialize your exchange with futures
		binance, err := NewBinanceFuture(ctx, exhangeOptions...)
		if err != nil {
			utils.Log.Fatal(err)
		}
		for pair, assetInfo := range binance.AssetsInfos() {
			if assetInfo.QuoteAsset != "USDT" {
				continue
			}
			pairAssetQuoteMap[pair] = AssetQuote{
				Quote: assetInfo.QuoteAsset,
				Asset: assetInfo.BaseAsset,
			}
		}
	}

	fmt.Printf("Total pairs: %d\n", len(pairAssetQuoteMap))

	content, err := json.Marshal(pairAssetQuoteMap)
	if err != nil {
		return fmt.Errorf("failed to marshal pairs: %v", err)
	}

	err = os.WriteFile("pairs.json", content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to file: %v", err)
	}

	return nil
}
