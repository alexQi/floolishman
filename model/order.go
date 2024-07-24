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
	PositionSideTypeBoth     PositionSideType = "BOTH"
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

type OrderExtra struct {
	OrderFlag          string
	LongShortRatio     float64
	Leverage           int
	GuiderPositionRate float64
	GuiderOrigin       string
	PositionAmount     float64
	MatchStrategy      map[string]int
}

type Order struct {
	ID                 int64            `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	OpenType           string           `db:"open_type" json:"open_type"`
	ExchangeID         int64            `db:"exchange_id" json:"exchange_id"`
	ClientOrderId      string           `db:"client_order_id" json:"client_order_id"`
	OrderFlag          string           `db:"order_flag" json:"order_flag"`
	Pair               string           `db:"pair" json:"pair"`
	Side               SideType         `db:"side" json:"side"`
	Type               OrderType        `db:"type" json:"type"`
	Status             OrderStatusType  `db:"status" json:"status"`
	Price              float64          `db:"price" json:"price"`
	Quantity           float64          `db:"quantity" json:"quantity"`
	Amount             float64          `db:"amount" json:"amount"`
	PositionSide       PositionSideType `db:"position_side" json:"position_side"`
	Leverage           int              `db:"leverage" json:"leverage"`
	LongShortRatio     float64          `db:"long_short_ratio" json:"long_short_ratio"`
	GuiderPositionRate float64          `db:"guider_position_rate" json:"guider_position_rate"`
	GuiderOrigin       string           `db:"guider_origin" json:"guider_origin"`
	CreatedAt          time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time        `db:"updated_at" json:"updated_at"`
	Profit             float64          `json:"profit" gorm:"profit"`
	ProfitValue        float64          `json:"profit_value" gorm:"profit_value"`

	Candle        Candle         `json:"-" gorm:"-"`
	MatchStrategy map[string]int `json:"-" gorm:"-"`
}

func (o Order) String() string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("[%s] %s %s %s %s | OrderFlag: %s, ID: %d,ClientOrderId: %s, Type: %s, %f x $%f (~$%.f)",
		o.Status, o.UpdatedAt.In(loc).Format("2006-01-02 15:04:05"), o.Side, o.PositionSide, o.Pair, o.OrderFlag, o.ExchangeID, o.ClientOrderId, o.Type, o.Quantity, o.Price, o.Quantity*o.Price)
}
