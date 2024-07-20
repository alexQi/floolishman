package types

import "floolishman/model"

type UserOrderResponse struct {
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Data    []model.GuiderOrder `json:"data"`
	Total   int                 `json:"total"`
	Success bool                `json:"success"`
}

type UserLeverageResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		SymbolConfigItemList []model.GuiderSymbolConfig `json:"symbolConfigItemList"`
	} `json:"data"`
	Success bool `json:"success"`
}

type PortfolioDetailListResponse struct {
	Code    string             `json:"code"`
	Message string             `json:"message"`
	Success bool               `json:"success"`
	Data    []model.GuiderItem `json:"data"`
}

type UserPositionResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Success bool           `json:"success"`
	Data    []UserPosition `json:"data"`
}

type UserPosition struct {
	Id                     string `json:"id"`
	Symbol                 string `json:"symbol"`
	PositionSide           string `json:"positionSide"`
	PositionAmount         string `json:"positionAmount"`
	EntryPrice             string `json:"entryPrice"`
	BreakEvenPrice         string `json:"breakEvenPrice"`
	MarkPrice              string `json:"markPrice"`
	UnrealizedProfit       string `json:"unrealizedProfit"`
	LiquidationPrice       string `json:"liquidationPrice"`
	IsolatedMargin         string `json:"isolatedMargin"`
	NotionalValue          string `json:"notionalValue"`
	Collateral             string `json:"collateral"`
	IsolatedWallet         string `json:"isolatedWallet"`
	CumRealized            string `json:"cumRealized"`
	InitialMargin          string `json:"initialMargin"`
	MaintMargin            string `json:"maintMargin"`
	AvailQuote             string `json:"AvailQuote"`
	PositionInitialMargin  string `json:"positionInitialMargin"`
	OpenOrderInitialMargin string `json:"openOrderInitialMargin"`
	Adl                    int    `json:"adl"`
	AskNotional            string `json:"askNotional"`
	BidNotional            string `json:"bidNotional"`
	UpdateTime             int    `json:"updateTime"`
}
