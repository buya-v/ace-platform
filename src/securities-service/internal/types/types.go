// Package types defines all domain types for the securities-service.
package types

// AssetClass represents the broad category of a financial instrument.
type AssetClass string

const (
	AssetClassEquity AssetClass = "EQUITY"
	AssetClassBond   AssetClass = "BOND"
	AssetClassETF    AssetClass = "ETF"
)

// TradingStatus represents the current trading state of an instrument.
type TradingStatus string

const (
	TradingStatusActive    TradingStatus = "ACTIVE"
	TradingStatusHalted    TradingStatus = "HALTED"
	TradingStatusSuspended TradingStatus = "SUSPENDED"
	TradingStatusDelisted  TradingStatus = "DELISTED"
)

// OrderType represents the type of a securities order.
type OrderType string

const (
	OrderTypeLimit     OrderType = "LIMIT"
	OrderTypeMarket    OrderType = "MARKET"
	OrderTypeStop      OrderType = "STOP"
	OrderTypeStopLimit OrderType = "STOP_LIMIT"
)

// OrderSide represents the direction of an order.
type OrderSide string

const (
	OrderSideBuy       OrderSide = "BUY"
	OrderSideSell      OrderSide = "SELL"
	OrderSideShortSell OrderSide = "SHORT_SELL"
)

// OrderStatus represents the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusPending         OrderStatus = "PENDING"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusCancelled       OrderStatus = "CANCELLED"
	OrderStatusRejected        OrderStatus = "REJECTED"
	OrderStatusExpired         OrderStatus = "EXPIRED"
)

// TimeInForce specifies how long an order remains active.
type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "GTC" // Good Till Cancelled
	TimeInForceIOC TimeInForce = "IOC" // Immediate Or Cancel
	TimeInForceFOK TimeInForce = "FOK" // Fill Or Kill
	TimeInForceDAY TimeInForce = "DAY" // Day order
	TimeInForceGTD TimeInForce = "GTD" // Good Till Date
)

// Instrument holds reference data for a listed security.
type Instrument struct {
	ID                string        `json:"id"`
	ISIN              string        `json:"isin"`
	CUSIP             string        `json:"cusip"`
	SEDOL             string        `json:"sedol"`
	Ticker            string        `json:"ticker"`
	Name              string        `json:"name"`
	AssetClass        AssetClass    `json:"asset_class"`
	ExchangeCode      string        `json:"exchange_code"`
	LotSize           int           `json:"lot_size"`
	TickSize          float64       `json:"tick_size"`
	Currency          string        `json:"currency"`
	ListingDate       string        `json:"listing_date"`
	TradingStatus     TradingStatus `json:"trading_status"`
	OutstandingShares int64         `json:"outstanding_shares"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
}

// BondDetails holds bond-specific attributes for a fixed-income instrument.
type BondDetails struct {
	InstrumentID       string  `json:"instrument_id"`
	MaturityDate       string  `json:"maturity_date"`
	CouponRate         float64 `json:"coupon_rate"`
	CouponFrequency    string  `json:"coupon_frequency"`
	ParValue           float64 `json:"par_value"`
	DayCountConvention string  `json:"day_count_convention"`
}

// SecurityOrder represents an order submitted against a listed security.
type SecurityOrder struct {
	ID             string      `json:"id"`
	InstrumentID   string      `json:"instrument_id"`
	ParticipantID  string      `json:"participant_id"`
	Side           OrderSide   `json:"side"`
	OrderType      OrderType   `json:"order_type"`
	Quantity       int         `json:"quantity"`
	Price          float64     `json:"price"`
	StopPrice      float64     `json:"stop_price"`
	TimeInForce    TimeInForce `json:"time_in_force"`
	Status         OrderStatus `json:"status"`
	FilledQuantity int         `json:"filled_quantity"`
	AvgFillPrice   float64     `json:"avg_fill_price"`
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      string      `json:"updated_at"`
}

// TradeStatus represents the lifecycle state of a trade.
type TradeStatus string

const (
	TradeStatusPending   TradeStatus = "TRADE_PENDING"
	TradeStatusConfirmed TradeStatus = "TRADE_CONFIRMED"
	TradeStatusSettled   TradeStatus = "TRADE_SETTLED"
	TradeStatusFailed    TradeStatus = "TRADE_FAILED"
)

// SecurityTrade represents a matched trade between a buy and sell order.
type SecurityTrade struct {
	ID             string      `json:"id"`
	BuyOrderID     string      `json:"buy_order_id"`
	SellOrderID    string      `json:"sell_order_id"`
	InstrumentID   string      `json:"instrument_id"`
	Price          float64     `json:"price"`
	Quantity       int         `json:"quantity"`
	TradeDate      string      `json:"trade_date"`
	SettlementDate string      `json:"settlement_date"`
	Status         TradeStatus `json:"status"`
	CreatedAt      string      `json:"created_at"`
}

// Position represents a participant's holdings in a specific instrument.
type Position struct {
	ID            string  `json:"id"`
	ParticipantID string  `json:"participant_id"`
	InstrumentID  string  `json:"instrument_id"`
	Quantity      int     `json:"quantity"`
	AvgCost       float64 `json:"avg_cost"`
	MarketValue   float64 `json:"market_value"`
	UnrealizedPnl float64 `json:"unrealized_pnl"`
	UpdatedAt     string  `json:"updated_at"`
}

// ErrorDetail carries a machine-readable code and human-readable message.
type ErrorDetail struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// ErrorResponse is the standard error envelope returned by all endpoints.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}
