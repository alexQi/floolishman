package storage

import (
	"floolishman/model"
	"time"
)

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type OrderFilterParams struct {
	Pair       string
	OrderFlag  string
	Statuses   []model.OrderStatusType
	OrderTypes []model.OrderType
}

type PositionFilterParams struct {
	Pair         string
	OrderFlag    string
	Status       []int
	Side         string
	PositionSide string
	TimeRange    TimeRange
}

type StrategyFilterParams struct {
	Pair         string
	OrderFlag    string
	Type         string
	Side         string
	PositionSide string
}

type ItemFilterParams struct {
	Account string
}

type Storage interface {
	ResetTables() error
	CreateOrder(order *model.Order) error
	UpdateOrder(order *model.Order) error
	Orders(filterParams OrderFilterParams) ([]*model.Order, error)
	CreatePosition(position *model.Position) error
	UpdatePosition(position *model.Position) error
	GetPosition(filterParams PositionFilterParams) (*model.Position, error)
	Positions(filterParams PositionFilterParams) ([]*model.Position, error)
	CreateStrategy(strategies []model.PositionStrategy) error
	Strategies(filterParams StrategyFilterParams) ([]*model.PositionStrategy, error)
	CreateGuiderItems(guiderItems []model.GuiderItem) error
	CreateSymbolConfigs(guiderSymbolConfigs []model.GuiderSymbolConfig) error
	CreateGuiderPositions(portfolioIds []string, guiderPositions []*model.GuiderPosition) error
	GuiderPositions(portfolioIds []string) ([]*model.GuiderPosition, error)
	CreateGuiderOrders(copyPortfolioId string, guiderOrders []model.GuiderOrder) error
	GetGuiderItems() ([]*model.GuiderItem, error)
	GetGuiderItemsByFilter(filterParams ItemFilterParams) ([]*model.GuiderItem, error)
	GetGuiderItemByPortfolioId(portfolioId string) (*model.GuiderItem, error)
	GetSymbolConfigByPortfolioId(portfolioId string, pair string) (*model.GuiderSymbolConfig, error)
}
