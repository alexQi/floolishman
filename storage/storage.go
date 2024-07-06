package storage

import (
	"time"

	"floolishman/model"
)

type OrderFilter func(model.Order) bool

type Storage interface {
	CreateOrder(order *model.Order) error
	UpdateOrder(order *model.Order) error
	Orders(filters ...OrderFilter) ([]*model.Order, error)
}

func WithStatusIn(status ...model.OrderStatusType) OrderFilter {
	return func(order model.Order) bool {
		for _, s := range status {
			if s == order.Status {
				return true
			}
		}
		return false
	}
}

func WithOrderTypeIn(orderType ...model.OrderType) OrderFilter {
	return func(order model.Order) bool {
		for _, t := range orderType {
			if t == order.Type {
				return true
			}
		}
		return false
	}
}

func WithStatus(status model.OrderStatusType) OrderFilter {
	return func(order model.Order) bool {
		return order.Status == status
	}
}

func WithTradingStatus(tradingStatus int64) OrderFilter {
	return func(order model.Order) bool {
		return order.TradingStatus == tradingStatus
	}
}

func WithOrderFlag(orderFlag string) OrderFilter {
	return func(order model.Order) bool {
		return order.OrderFlag == orderFlag
	}
}

func WithPair(pair string) OrderFilter {
	return func(order model.Order) bool {
		return order.Pair == pair
	}
}

func WithUpdateAtBeforeOrEqual(time time.Time) OrderFilter {
	return func(order model.Order) bool {
		return !order.UpdatedAt.After(time)
	}
}
