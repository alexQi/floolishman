package source

import (
	"floolishman/model"
	"floolishman/types"
)

type BaseSourceInterface interface {
	InitHttpClient(proxyOption types.ProxyOption)
	GetUserPortfolioList(authHeader map[string]string) ([]model.GuiderItem, error)
	CheckUserOrder(portfolioId string, authHeader map[string]string) ([]model.GuiderOrder, error)
	CheckGuiderSymbolConfig(portfolioId string, authHeader map[string]string) ([]model.GuiderSymbolConfig, error)
	CheckUserPosition(portfolioId string, authHeader map[string]string) ([]*model.GuiderPosition, error)
}
