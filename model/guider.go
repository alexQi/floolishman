package model

import "time"

type GuiderItem struct {
	ID                     int64     `json:"id" gorm:"primary_key;autoIncrement" db:"id"`
	Account                string    `json:"account" gorm:"account" db:"account"`
	AvatarUrl              string    `json:"avatarUrl" gorm:"avatar_url" db:"avatar_url"`
	Nickname               string    `json:"nickname" gorm:"nickname" db:"nickname"`
	CopyPortfolioId        string    `json:"copyPortfolioId" gorm:"copy_portfolio_id" db:"copy_portfolio_id"`
	LeadPortfolioId        string    `json:"leadPortfolioId" gorm:"lead_portfolio_id" db:"lead_portfolio_id"`
	StartDate              int64     `json:"startDate" gorm:"start_date" db:"start_date"`
	EndDate                int64     `json:"endDate" gorm:"end_date" db:"end_date"`
	ClosedReason           string    `json:"closedReason" gorm:"closed_reason" db:"closed_reason"`
	NetCopyAmount          float64   `json:"netCopyAmount" gorm:"net_copy_amount" db:"net_copy_amount"`
	NetCopyAsset           string    `json:"netCopyAsset" gorm:"net_copy_asset" db:"net_copy_asset"`
	UnrealizedPnl          float64   `json:"unrealizedPnl" gorm:"unrealized_pnl" db:"unrealized_pnl"`
	UnrealizedPnlAsset     string    `json:"unrealizedPnlAsset" gorm:"unrealized_pnl_asset" db:"unrealized_pnl_asset"`
	RealizedPnl            float64   `json:"realizedPnl" gorm:"realized_pnl" db:"realized_pnl"`
	RealizedPnlAsset       string    `json:"realizedPnlAsset" gorm:"realized_pnl_asset" db:"realized_pnl_asset"`
	NetProfitAmount        float64   `json:"netProfitAmount" gorm:"net_profit_amount" db:"net_profit_amount"`
	NetProfitAsset         string    `json:"netProfitAsset" gorm:"net_profit_asset" db:"net_profit_asset"`
	ProfitSharedAmount     float64   `json:"profitSharedAmount" gorm:"profit_shared_amount" db:"profit_shared_amount"`
	ProfitSharedAsset      string    `json:"profitSharedAsset" gorm:"profit_shared_asset" db:"profit_shared_asset"`
	UnProfitSharedAmount   float64   `json:"unProfitSharedAmount" gorm:"un_profit_shared_amount" db:"un_profit_shared_amount"`
	MarginBalance          float64   `json:"marginBalance" gorm:"margin_balance" db:"margin_balance"`
	MarginBalanceAsset     string    `json:"marginBalanceAsset" gorm:"margin_balance_asset" db:"margin_balance_asset"`
	ProfitSharingRate      string    `json:"profitSharingRate" gorm:"profit_sharing_rate" db:"profit_sharing_rate"`
	CopierUnlockPeriodDays int64     `json:"copierUnlockPeriodDays" gorm:"copier_unlock_period_days" db:"copier_unlock_period_days"`
	TotalSlRate            float64   `json:"totalSlRate" gorm:"total_sl_rate" db:"total_sl_rate"`
	CreatedAt              time.Time `gorm:"created_at" db:"created_at"`
	UpdatedAt              time.Time `gorm:"updated_at" db:"updated_at"`
}

type GuiderSymbolConfig struct {
	ID               int64     `json:"id" gorm:"primaryKey;autoIncrement" db:"id"`
	Account          string    `json:"account" gorm:"account" db:"account"`
	PortfolioId      string    `json:"portfolioId" gorm:"portfolio_id" db:"portfolio_id"`
	Symbol           string    `json:"symbol" gorm:"symbol" db:"symbol"`
	MarginType       string    `json:"marginType" gorm:"margin_type" db:"margin_type"`
	Leverage         int       `json:"leverage" gorm:"leverage" db:"leverage"`
	MaxNotionalValue string    `json:"maxNotionalValue" gorm:"max_notional_value" db:"max_notional_value"`
	CreatedAt        time.Time `gorm:"created_at" db:"created_at"`
	UpdatedAt        time.Time `gorm:"updated_at" db:"updated_at"`
}

type GuiderPosition struct {
	ID                     int64     `json:"id" gorm:"primaryKey;autoIncrement" db:"id"`
	PortfolioId            string    `json:"portfolioId" gorm:"portfolio_id" db:"portfolio_id"`
	Symbol                 string    `json:"symbol" db:"symbol"`
	PositionSide           string    `json:"positionSide" db:"position_side"`
	PositionAmount         float64   `json:"positionAmount" db:"position_amount"`
	EntryPrice             float64   `json:"entryPrice" db:"entry_price"`
	BreakEvenPrice         float64   `json:"breakEvenPrice" db:"break_even_price"`
	MarkPrice              float64   `json:"markPrice" db:"mark_price"`
	UnrealizedProfit       float64   `json:"unrealizedProfit" db:"unrealized_profit"`
	LiquidationPrice       float64   `json:"liquidationPrice" db:"liquidation_price"`
	IsolatedMargin         float64   `json:"isolatedMargin" db:"isolated_margin"`
	NotionalValue          float64   `json:"notionalValue" db:"notional_value"`
	Collateral             string    `json:"collateral" db:"collateral"`
	IsolatedWallet         float64   `json:"isolatedWallet" db:"isolated_wallet"`
	CumRealized            float64   `json:"cumRealized" db:"cum_realized"`
	InitialMargin          float64   `json:"initialMargin" db:"initial_margin"`
	MaintMargin            float64   `json:"maintMargin" db:"maint_margin"`
	AvailQuote             float64   `json:"AvailQuote" db:"avail_quote"`
	PositionInitialMargin  float64   `json:"positionInitialMargin" db:"position_initial_margin"`
	OpenOrderInitialMargin float64   `json:"openOrderInitialMargin" db:"open_order_initial_margin"`
	Adl                    int       `json:"adl" db:"adl"`
	AskNotional            float64   `json:"askNotional" db:"ask_notional"`
	BidNotional            float64   `json:"bidNotional" db:"bid_notional"`
	UpdateTime             int       `json:"updateTime" db:"update_time"`
	CreatedAt              time.Time `gorm:"created_at" db:"created_at"`
	UpdatedAt              time.Time `gorm:"updated_at" db:"updated_at"`
}

type GuiderOrder struct {
	ID                 int64     `gorm:"primaryKey;autoIncrement" db:"id"`
	OriginId           string    `json:"id" gorm:"id" db:"id"`
	OrderID            int64     `json:"orderId" gorm:"order_id" db:"order_id"`
	Symbol             string    `json:"symbol" gorm:"symbol" db:"symbol"`
	ClientOrderID      string    `json:"clientOrderId" gorm:"client_order_id" db:"client_order_id"`
	OrigClientOrderID  string    `json:"origClientOrderId" gorm:"orig_client_order_id" db:"orig_client_order_id"`
	Price              string    `json:"price" gorm:"price" db:"price"`
	OrigQty            string    `json:"origQty" gorm:"orig_qty" db:"orig_qty"`
	ExecutedQty        string    `json:"executedQty" gorm:"executed_qty" db:"executed_qty"`
	ExecutedQuoteQty   string    `json:"executedQuoteQty" gorm:"executed_quote_qty" db:"executed_quote_qty"`
	Status             string    `json:"status" gorm:"status" db:"status"`
	TimeInForce        string    `json:"timeInForce" gorm:"time_in_force" db:"time_in_force"`
	Type               string    `json:"type" gorm:"type" db:"type"`
	Side               string    `json:"side" gorm:"side" db:"side"`
	StopPrice          string    `json:"stopPrice" gorm:"stop_price" db:"stop_price"`
	InsertTime         time.Time `json:"insertTime" gorm:"insert_time" db:"insert_time"`
	UpdateTime         time.Time `json:"updateTime" gorm:"update_time" db:"update_time"`
	DelegateMoney      string    `json:"delegateMoney" gorm:"delegate_money" db:"delegate_money"`
	AvgPrice           string    `json:"avgPrice" gorm:"avg_price" db:"avg_price"`
	HasDetail          bool      `json:"hasDetail" gorm:"has_detail" db:"has_detail"`
	TargetStrategy     int       `json:"targetStrategy" gorm:"target_strategy" db:"target_strategy"`
	PriceProtect       int       `json:"priceProtect" gorm:"price_protect" db:"price_protect"`
	ReduceOnly         bool      `json:"reduceOnly" gorm:"reduce_only" db:"reduce_only"`
	WorkingType        string    `json:"workingType" gorm:"working_type" db:"working_type"`
	OrigType           string    `json:"origType" gorm:"orig_type" db:"orig_type"`
	PositionSide       string    `json:"positionSide" gorm:"position_side" db:"position_side"`
	ActivatePrice      string    `json:"activatePrice" gorm:"activate_price" db:"activate_price"`
	PriceRate          string    `json:"priceRate" gorm:"price_rate" db:"price_rate"`
	ClosePosition      bool      `json:"closePosition" gorm:"close_position" db:"close_position"`
	StrategyID         string    `json:"strategyId" gorm:"strategy_id" db:"strategy_id"`
	StrategySubID      string    `json:"strategySubId" gorm:"strategy_sub_id" db:"strategy_sub_id"`
	StrategyType       string    `json:"strategyType" gorm:"strategy_type" db:"strategy_type"`
	MarkPrice          string    `json:"markPrice" gorm:"mark_price" db:"mark_price"`
	BaseAsset          string    `json:"baseAsset" gorm:"base_asset" db:"base_asset"`
	QuoteAsset         string    `json:"quoteAsset" gorm:"quote_asset" db:"quote_asset"`
	MarginAsset        string    `json:"marginAsset" gorm:"margin_asset" db:"margin_asset"`
	GoodTillDate       int       `json:"goodTillDate" gorm:"good_till_date" db:"good_till_date"`
	PriceMatch         string    `json:"priceMatch" gorm:"price_match" db:"price_match"`
	SelfProtectionMode string    `json:"selfProtectionMode" gorm:"self_protection_mode" db:"self_protection_mode"`
	PortfolioId        string    `json:"portfolioId" gorm:"portfolio_id" db:"portfolio_id"`
	CreatedAt          time.Time `gorm:"created_at" db:"created_at"`
	UpdatedAt          time.Time `gorm:"updated_at" db:"updated_at"`
}
