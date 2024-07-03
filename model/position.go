package model

import (
	"fmt"
	"time"
)

type Position struct {
	ID               int64     `db:"id" json:"id" gorm:"primaryKey,autoIncrement"`
	EntryPrice       float64   `db:"entry_price"`
	BreakEvenPrice   float64   `db:"break_even_price"`
	MarginType       string    `db:"margin_type"`
	IsAutoAddMargin  bool      `db:"isAuto_add_margin"`
	IsolatedMargin   int       `db:"isolated_margin"`
	Leverage         int       `db:"leverage"`
	LiquidationPrice float64   `db:"liquidation_price"`
	MarkPrice        float64   `db:"mark_price"`
	MaxNotionalValue float64   `db:"max_notional_value"`
	PositionAmt      float64   `db:"position_amt"`
	Symbol           string    `db:"symbol"`
	UnRealizedProfit float64   `db:"un_realized_profit"`
	Side             SideType  `db:"side"`
	PositionSide     string    `db:"position_side"`
	Notional         float64   `db:"notional"`
	IsolatedWallet   float64   `db:"isolated_wallet"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time `db:"updated_at" json:"updated_at"`
}

func (o Position) String() string {
	return fmt.Sprintf("[%s] %s | ID: %d, Leverage: %s, %f x $%f",
		o.Symbol, o.Side, o.ID, o.Leverage, o.PositionAmt, o.EntryPrice)
}
