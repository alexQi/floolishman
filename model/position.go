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
	IsLock    bool
	Close     float64
	Decrease  float64
	Floor     float64
	MaxProfit float64
	LossCount int
}

type PositionStrategy struct {
	ID           int64     `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	Pair         string    `db:"pair" json:"pair"`
	OrderFlag    string    `db:"order_flag" json:"order_flag"`
	Type         string    `db:"type" json:"type"`
	Useable      int       `db:"useable" json:"useable"`
	ChaseMode    int       `db:"chase_mode" json:"chase_mode"`
	Side         string    `db:"side" json:"side"`
	StrategyName string    `db:"strategy_name" json:"strategy_name"`
	Score        float64   `db:"score" json:"score"`
	Tendency     string    `db:"tendency" json:"tendency"`
	LastAtr      float64   `db:"last_atr" json:"last_atr"`
	OpenPrice    float64   `db:"open_price" json:"open_price"`
	OpenParams   string    `db:"open_params" json:"open_params"`
	IsFinal      int       `db:"is_final" json:"is_final"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
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
	return fmt.Sprintf("Pair: %s | PositionSide: %s | Main OrderFlag: %s, Quantity: %v,TotalQuantity: %v, Price: %v, StopLossPrice: %v, Time: %s",
		p.Pair,
		p.PositionSide,
		p.OrderFlag,
		p.Quantity,
		p.TotalQuantity,
		p.AvgPrice,
		p.StopLossPrice,
		p.CreatedAt.In(TimeLoc).Format("2006-01-02 15:04:05"),
	)
}
