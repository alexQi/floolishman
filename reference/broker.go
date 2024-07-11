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
	CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string, size float64, limit float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error)
	CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, size float64, longShortRatio float64, matchStrategy map[string]int) (model.Order, error)
	CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, limit float64, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error)
	CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, stopPrice float64, orderFlag string, longShortRatio float64, matchStrategy map[string]int) (model.Order, error)
	Cancel(model.Order) error
}
