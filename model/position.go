package model

import (
	"fmt"
	"time"
)

type StopProfitLevel struct {
	TriggerRatio     float64
	DrawdownRatio    float64
	NextTriggerRatio float64
}

type PairProfit struct {
	Close    float64
	Decrease float64
	Floor    float64
}

type Position struct {
	ID                   int64          `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	Pair                 string         `db:"pair" json:"pair"`
	OrderFlag            string         `db:"order_flag" json:"order_flag"`
	Side                 string         `db:"side" json:"side"`
	PositionSide         string         `db:"position_side" json:"position_side"`
	AvgPrice             float64        `db:"avg_price" json:"avg_price"`
	ClosePrice           float64        `db:"close_price" json:"close_price"`
	Quantity             float64        `db:"quantity" json:"quantity"`
	TotalQuantity        float64        `db:"total_quantity" json:"total_quantity"`
	UnitQuantity         float64        `db:"unit_quantity" json:"unit_quantity"`
	MoreCount            int64          `db:"more_count" json:"more_count"`
	Profit               float64        `json:"profit" gorm:"profit"`
	ProfitValue          float64        `json:"profit_value" gorm:"profit_value"`
	MarginType           string         `db:"margin_type" json:"margin_type"`
	Leverage             int            `db:"leverage" json:"leverage"`
	StopLossPrice        float64        `db:"stop_loss_price" json:"stop_loss_price"`
	ChaseMode            int            `db:"chase_mode" json:"chase_mode"`
	LongShortRatio       float64        `db:"long_short_ratio" json:"long_short_ratio"`
	GuiderPositionRate   float64        `db:"guider_position_rate" json:"guider_position_rate"`
	GuiderOrigin         string         `db:"guider_origin" json:"guider_origin"`
	Status               int            `db:"status" json:"status"`
	CreatedAt            time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at" json:"updated_at"`
	MatcherStrategyCount map[string]int `json:"-" gorm:"-"`
}

func (p Position) String() string {
	return fmt.Sprintf("[%s] %s | ID: %d, AvgPrice: %.2f, Quantity: %.2f",
		p.OrderFlag, p.Side, p.ID, p.AvgPrice, p.Quantity)
}
