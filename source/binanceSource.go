package source

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"floolishman/model"
	"floolishman/types"
	"floolishman/utils"
	"floolishman/utils/httputil"
	"fmt"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type BinanceSource struct {
	Client *http.Client
}

var headers = map[string]string{
	"accept":          "*/*",
	"accept-language": "zh-CN,zh;q=0.9",
	"bnc-location":    "BINANCE",
	"clienttype":      "web",
	"content-type":    "application/json",
	"lang":            "zh-CN",
	"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
}

var baseUrl = "https://www.binance.com/bapi/futures"

func (s *BinanceSource) GetUserPortfolioList(authHeader map[string]string) ([]model.GuiderItem, error) {
	var response types.PortfolioDetailListResponse

	apiURL := fmt.Sprintf("%s/v1/private/future/copy-trade/copy-portfolio/detail-list?ongoing=true", baseUrl)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return response.Data, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}
	for aKey, aval := range authHeader {
		req.Header.Set(aKey, aval)
	}

	data, err := s.Send(req)
	if err != nil {
		return response.Data, err
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return response.Data, err
	}

	if response.Success != true {
		return response.Data, errors.New(response.Message)
	}
	return response.Data, nil
}

func (s *BinanceSource) CheckGuiderSymbolConfig(portfolioId string, authHeader map[string]string) ([]model.GuiderSymbolConfig, error) {
	var response types.UserLeverageResponse

	apiURL := fmt.Sprintf("%s/v1/private/future/user-data/symbol-config", baseUrl)
	payload := strings.NewReader(fmt.Sprintf(`{"copyTradeType":"COPY","portfolioId":"%s"}`, portfolioId))

	req, err := http.NewRequest("POST", apiURL, payload)
	if err != nil {
		return response.Data.SymbolConfigItemList, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}
	for aKey, aval := range authHeader {
		req.Header.Set(aKey, aval)
	}

	data, err := s.Send(req)
	if err != nil {
		return response.Data.SymbolConfigItemList, err
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return response.Data.SymbolConfigItemList, err
	}

	if response.Success != true {
		return response.Data.SymbolConfigItemList, errors.New(response.Message)
	}
	return response.Data.SymbolConfigItemList, nil
}

func (s *BinanceSource) CheckUserOrder(portfolioId string, authHeader map[string]string) ([]model.GuiderOrder, error) {
	var response types.UserOrderResponse

	apiURL := fmt.Sprintf("%s/v1/private/future/order/order-history", baseUrl)
	payload := strings.NewReader(fmt.Sprintf(`{"copyTradeType":"COPY","portfolioId":"%s"}`, portfolioId))

	req, err := http.NewRequest("POST", apiURL, payload)
	if err != nil {
		return response.Data, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}
	for aKey, aval := range authHeader {
		req.Header.Set(aKey, aval)
	}

	data, err := s.Send(req)
	if err != nil {
		return response.Data, err
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return response.Data, err
	}

	if response.Success != true {
		return response.Data, errors.New(response.Message)
	}
	for i := range response.Data {
		response.Data[i].PortfolioId = portfolioId
	}
	return response.Data, nil
}

func (s *BinanceSource) CheckUserPosition(portfolioId string, authHeader map[string]string) ([]*model.GuiderPosition, error) {
	guiderPositions := []*model.GuiderPosition{}
	var response types.UserPositionResponse
	apiURL := fmt.Sprintf("%s/v6/private/future/user-data/user-position", baseUrl)
	payload := strings.NewReader(fmt.Sprintf(`{"copyTradeType":"COPY","portfolioId":"%s"}`, portfolioId))

	req, err := http.NewRequest("POST", apiURL, payload)
	if err != nil {
		return guiderPositions, err
	}

	for key, val := range headers {
		req.Header.Set(key, val)
	}
	for aKey, aval := range authHeader {
		req.Header.Set(aKey, aval)
	}

	data, err := s.Send(req)
	if err != nil {
		return guiderPositions, err
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		return guiderPositions, err
	}

	if response.Success != true {
		return guiderPositions, errors.New(response.Message)
	}
	for i := range response.Data {
		position, err := s.newGuiderPosition(response.Data[i])
		if err != nil {
			return guiderPositions, err
		}
		position.PortfolioId = portfolioId
		guiderPositions = append(guiderPositions, &position)
	}
	return guiderPositions, nil
}

func (s *BinanceSource) InitHttpClient(proxyOption types.ProxyOption) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	if proxyOption.Status {
		proxy, err := url.Parse(proxyOption.Url)
		if err != nil {
			utils.Log.Fatal(err)
		}
		tr.Proxy = http.ProxyURL(proxy)
	}
	s.Client = &http.Client{
		Transport: tr,
		Timeout:   viper.GetDuration("http.Timeout") * time.Millisecond,
	}
}

func (s *BinanceSource) Send(req *http.Request) ([]byte, error) {
	resp, err := s.Client.Do(req)
	if resp != nil {
		defer httputil.BodyCloser(resp.Body)
	}
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(ioutil.Discard, resp.Body)
	return body, nil
}

func (s *BinanceSource) newGuiderPosition(up types.UserPosition) (model.GuiderPosition, error) {
	guiderPosition := model.GuiderPosition{}
	positionAmount, err := strconv.ParseFloat(up.PositionAmount, 64)
	if err != nil {
		return guiderPosition, err
	}
	entryPrice, err := strconv.ParseFloat(up.EntryPrice, 64)
	if err != nil {
		return guiderPosition, err
	}
	breakEvenPrice, err := strconv.ParseFloat(up.BreakEvenPrice, 64)
	if err != nil {
		return guiderPosition, err
	}
	markPrice, err := strconv.ParseFloat(up.MarkPrice, 64)
	if err != nil {
		return guiderPosition, err
	}
	unrealizedProfit, err := strconv.ParseFloat(up.UnrealizedProfit, 64)
	if err != nil {
		return guiderPosition, err
	}
	liquidationPrice, err := strconv.ParseFloat(up.LiquidationPrice, 64)
	if err != nil {
		return guiderPosition, err
	}
	isolatedMargin, err := strconv.ParseFloat(up.IsolatedMargin, 64)
	if err != nil {
		return guiderPosition, err
	}
	notionalValue, err := strconv.ParseFloat(up.NotionalValue, 64)
	if err != nil {
		return guiderPosition, err
	}
	isolatedWallet, err := strconv.ParseFloat(up.IsolatedWallet, 64)
	if err != nil {
		return guiderPosition, err
	}
	cumRealized, err := strconv.ParseFloat(up.CumRealized, 64)
	if err != nil {
		return guiderPosition, err
	}
	initialMargin, err := strconv.ParseFloat(up.InitialMargin, 64)
	if err != nil {
		return guiderPosition, err
	}
	maintMargin, err := strconv.ParseFloat(up.MaintMargin, 64)
	if err != nil {
		return guiderPosition, err
	}
	positionInitialMargin, err := strconv.ParseFloat(up.PositionInitialMargin, 64)
	if err != nil {
		return guiderPosition, err
	}
	openOrderInitialMargin, err := strconv.ParseFloat(up.OpenOrderInitialMargin, 64)
	if err != nil {
		return guiderPosition, err
	}
	askNotional, err := strconv.ParseFloat(up.AskNotional, 64)
	if err != nil {
		return guiderPosition, err
	}
	bidNotional, err := strconv.ParseFloat(up.BidNotional, 64)
	if err != nil {
		return guiderPosition, err
	}

	return model.GuiderPosition{
		Symbol:                 up.Symbol,
		PositionSide:           up.PositionSide,
		PositionAmount:         positionAmount,
		EntryPrice:             entryPrice,
		BreakEvenPrice:         breakEvenPrice,
		MarkPrice:              markPrice,
		UnrealizedProfit:       unrealizedProfit,
		LiquidationPrice:       liquidationPrice,
		IsolatedMargin:         isolatedMargin,
		NotionalValue:          notionalValue,
		Collateral:             up.Collateral,
		IsolatedWallet:         isolatedWallet,
		CumRealized:            cumRealized,
		InitialMargin:          initialMargin,
		MaintMargin:            maintMargin,
		PositionInitialMargin:  positionInitialMargin,
		OpenOrderInitialMargin: openOrderInitialMargin,
		Adl:                    up.Adl,
		AskNotional:            askNotional,
		BidNotional:            bidNotional,
		UpdateTime:             up.UpdateTime,
	}, nil
}
