package model

import (
	"fmt"
	"time"
)

type Position struct {
	ID                 int64          `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	Pair               string         `db:"pair" json:"pair"`
	OrderFlag          string         `db:"order_flag" json:"order_flag"`
	Side               string         `db:"side" json:"side"`
	PositionSide       string         `db:"position_side" json:"position_side"`
	AvgPrice           float64        `db:"avg_price" json:"avg_price"`
	Quantity           float64        `db:"quantity" json:"quantity"`
	Leverage           int            `db:"leverage" json:"leverage"`
	LongShortRatio     float64        `db:"long_short_ratio" json:"long_short_ratio"`
	GuiderPositionRate float64        `db:"guider_position_rate" json:"guider_position_rate"`
	GuiderOrigin       string         `db:"guider_origin" json:"guider_origin"`
	Status             int            `db:"status" json:"status"`
	CreatedAt          time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time      `db:"updated_at" json:"updated_at"`
	MatchStrategy      map[string]int `json:"-" gorm:"-"`
}

func (p Position) String() string {
	return fmt.Sprintf("[%s] %s | ID: %d, AvgPrice: %.2f, Quantity: %.2f",
		p.OrderFlag, p.Side, p.ID, p.AvgPrice, p.Quantity)
}
