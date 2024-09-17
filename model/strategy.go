package model

import (
	"time"
)

type Strategy struct {
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
	IsFinal      int       `db:"is_final" json:"is_final"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
}
