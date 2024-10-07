//go:generate go run github.com/vektra/mockery/v2 --all --with-expecter --output=../testdata/mocks

package reference

import (
	"floolishman/model"
	"time"
)

type Broker interface {
	Account() (model.Account, error)
	PairAsset(pair string) (asset, quote float64, err error)
	PairPosition() (map[string]map[string]*model.Position, error)
	FormatPrice(pair string, value float64) string
	FormatQuantity(pair string, value float64, toLot bool) string
	GetPositionsForPair(pair string) ([]*model.Position, error)
	GetPositionsForClosed(startTime time.Time) ([]*model.Position, error)
	GetPositionsForOpened() ([]*model.Position, error)
	Order(pair string, id int64) (model.Order, error)
	GetOrdersForUnfilled() (map[string]map[string][]*model.Order, error)
	GetOrdersForPairUnfilled(pair string) (map[string]map[string][]*model.Order, error)
	GetPositionOrdersForPairUnfilled(pair string) (map[string]map[model.PositionSideType]*model.Order, error)
	GetOrdersForPostionLossUnfilled(orderFlag string) ([]*model.Order, error)
	BatchCreateOrderLimit([]*model.OrderParam) ([]model.Order, error)
	BatchCreateOrderMarket([]*model.OrderParam) ([]model.Order, error)
	CreateOrderLimit(side model.SideType, positionSide model.PositionSideType, pair string, size float64, limit float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderMarket(side model.SideType, positionSide model.PositionSideType, pair string, size float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderStopLimit(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, limit float64, stopPrice float64, extra model.OrderExtra) (model.Order, error)
	CreateOrderStopMarket(side model.SideType, positionSide model.PositionSideType, pair string, quantity float64, stopPrice float64, extra model.OrderExtra) (model.Order, error)
	Cancel(model.Order) error
	ListenOrders()
}
