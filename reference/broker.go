//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

package reference

import (
	"floolishman/model"
)

type Broker interface {
	Account() (model.Account, error)
	Position(pair string) (asset, quote float64, err error)
	Order(pair string, id int64) (model.Order, error)
	GetCurrentPositionOrders(pair string) ([]*model.Order, error)
	CreateOrderOCO(side model.SideType, pair string, size, price, stop, stopLimit float64) ([]model.Order, error)
	CreateOrderLimit(side model.SideType, pair string, size float64, limit float64) (model.Order, error)
	CreateOrderMarket(side model.SideType, pair string, size float64) (model.Order, error)
	CreateOrderMarketQuote(side model.SideType, pair string, quote float64) (model.Order, error)
	CreateOrderStop(pair string, quantity float64, limit float64) (model.Order, error)
	CreateOrderStopLimit(side model.SideType, pair string, quantity float64, limit float64) (model.Order, error)
	Cancel(model.Order) error
}
