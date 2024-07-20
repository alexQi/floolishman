//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

package reference

import (
	"floolishman/model"
)

type Broker interface {
	Account() (model.Account, error)
	Position(pair string) (asset, quote float64, err error)
	GetPositionsForPair(pair string) ([]*model.Position, error)
	GetPositionsForOpened() ([]*model.Position, error)
	Order(pair string, id int64) (model.Order, error)
	GetOrdersForUnfilled() ([]*model.Order, error)
	GetOrdersForPostionLossUnfilled(orderFlag string) ([]*model.Order, error)
	CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string, size float64, limit float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, size float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, limit float64, stopPrice float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, stopPrice float64, extra model.OrderExtra) (model.Order, error)
	Cancel(model.Order) error
	ListenOrders()
}
