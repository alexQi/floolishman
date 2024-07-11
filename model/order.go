package model

import (
	"fmt"
	"time"
)

type SideType string
type PositionSideType string
type OrderType string
type OrderStatusType string

var (
	SideTypeBuy              SideType         = "BUY"
	SideTypeSell             SideType         = "SELL"
	PositionSideTypeLong     PositionSideType = "LONG"
	PositionSideTypeShort    PositionSideType = "SHORT"
	OrderTypeLimit           OrderType        = "LIMIT"
	OrderTypeMarket          OrderType        = "MARKET"
	OrderTypeLimitMaker      OrderType        = "LIMIT_MAKER"
	OrderTypeStop            OrderType        = "STOP"
	OrderTypeStopMarket      OrderType        = "STOP_MARKET"
	OrderTypeStopLoss        OrderType        = "STOP_LOSS"
	OrderTypeStopLossLimit   OrderType        = "STOP_LOSS_LIMIT"
	OrderTypeTakeProfit      OrderType        = "TAKE_PROFIT"
	OrderTypeTakeProfitLimit OrderType        = "TAKE_PROFIT_LIMIT"

	OrderStatusTypeNew             OrderStatusType = "NEW"
	OrderStatusTypePartiallyFilled OrderStatusType = "PARTIALLY_FILLED"
	OrderStatusTypeFilled          OrderStatusType = "FILLED"
	OrderStatusTypeCanceled        OrderStatusType = "CANCELED"
	OrderStatusTypePendingCancel   OrderStatusType = "PENDING_CANCEL"
	OrderStatusTypeRejected        OrderStatusType = "REJECTED"
	OrderStatusTypeExpired         OrderStatusType = "EXPIRED"
)

type Order struct {
	ID            int64            `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	ExchangeID    int64            `db:"exchange_id" json:"exchange_id"`
	ClientOrderId string           `db:"client_order_id" json:"client_order_id"`
	OrderFlag     string           `db:"order_flag" json:"order_flag"`
	Pair          string           `db:"pair" json:"pair"`
	Side          SideType         `db:"side" json:"side"`
	Type          OrderType        `db:"type" json:"type"`
	Status        OrderStatusType  `db:"status" json:"status"`
	Price         float64          `db:"price" json:"price"`
	Quantity      float64          `db:"quantity" json:"quantity"`
	PositionSide  PositionSideType `db:"position_side" json:"position_side"`
	TradingStatus int64            `db:"trading_status" json:"trading_status"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// strategy score
	LongShortRatio float64        `db:"long_short_ratio" json:"long_short_ratio"`
	MatchStrategy  map[string]int `db:"match_strategy" json:"match_strategy"`

	Profit      float64 `json:"profit" gorm:"-"`
	ProfitValue float64 `json:"profit_value" gorm:"-"`
	Candle      Candle  `json:"-" gorm:"-"`
}

func (o Order) String() string {
	return fmt.Sprintf("[%s] %s %s %s | OrderFlag: %s, ID: %d,ClientOrderId: %s, Type: %s, %f x $%f (~$%.f)",
		o.Status, o.Side, o.PositionSide, o.Pair, o.OrderFlag, o.ExchangeID, o.ClientOrderId, o.Type, o.Quantity, o.Price, o.Quantity*o.Price)
}
