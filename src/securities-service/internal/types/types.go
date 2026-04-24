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
	SegmentID         string        `json:"segment_id,omitempty"`
	STPMode           STPMode       `json:"stp_mode,omitempty"`
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
	FilledQuantity  int         `json:"filled_quantity"`
	AvgFillPrice    float64     `json:"avg_fill_price"`
	VisibleQuantity int         `json:"visible_quantity,omitempty"` // Iceberg: visible (displayed) quantity
	HiddenQuantity  int         `json:"hidden_quantity,omitempty"`  // Iceberg: hidden (reserve) quantity
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
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

// SettlementStatus represents the lifecycle state of a settlement obligation.
type SettlementStatus string

const (
	SettlePending    SettlementStatus = "SETTLE_PENDING"
	SettleAffirmed   SettlementStatus = "SETTLE_AFFIRMED"
	SettleNetted     SettlementStatus = "SETTLE_NETTED"
	SettleInstructed SettlementStatus = "SETTLE_INSTRUCTED"
	SettleSettling   SettlementStatus = "SETTLE_SETTLING"
	SettleSettled    SettlementStatus = "SETTLE_SETTLED"
	SettleFailed     SettlementStatus = "SETTLE_FAILED"
)

// SettlementObligation represents a T+2 settlement obligation derived from a trade.
type SettlementObligation struct {
	ID                  string           `json:"id"`
	TradeID             string           `json:"trade_id"`
	InstrumentID        string           `json:"instrument_id"`
	BuyerParticipantID  string           `json:"buyer_participant_id"`
	SellerParticipantID string           `json:"seller_participant_id"`
	Quantity            int              `json:"quantity"`
	Price               float64          `json:"price"`
	NetAmount           float64          `json:"net_amount"`
	SettlementDate      string           `json:"settlement_date"`
	Status              SettlementStatus `json:"status"`
	CreatedAt           string           `json:"created_at"`
	UpdatedAt           string           `json:"updated_at"`
}

// SettlementResult summarises the outcome of a settlement cycle run.
type SettlementResult struct {
	Date      string `json:"date"`
	Processed int    `json:"processed"`
	Affirmed  int    `json:"affirmed"`
	Netted    int    `json:"netted"`
	Settled   int    `json:"settled"`
	Failed    int    `json:"failed"`
}

// CorporateActionType represents the type of a corporate action event.
type CorporateActionType string

const (
	CA_DIVIDEND    CorporateActionType = "CA_DIVIDEND"
	CA_STOCK_SPLIT CorporateActionType = "CA_STOCK_SPLIT"
	CA_RIGHTS_ISSUE CorporateActionType = "CA_RIGHTS_ISSUE"
	CA_MERGER      CorporateActionType = "CA_MERGER"
)

// CorporateActionStatus represents the lifecycle state of a corporate action.
type CorporateActionStatus string

const (
	CAStatusAnnounced  CorporateActionStatus = "ANNOUNCED"
	CAStatusProcessing CorporateActionStatus = "PROCESSING"
	CAStatusCompleted  CorporateActionStatus = "COMPLETED"
	CAStatusCancelled  CorporateActionStatus = "CANCELLED"
)

// CorporateAction represents a declared corporate action for an instrument.
type CorporateAction struct {
	ID               string                 `json:"id"`
	InstrumentID     string                 `json:"instrument_id"`
	ActionType       CorporateActionType    `json:"action_type"`
	AnnouncementDate string                 `json:"announcement_date"`
	ExDate           string                 `json:"ex_date"`
	RecordDate       string                 `json:"record_date"`
	PaymentDate      string                 `json:"payment_date"`
	Details          map[string]interface{} `json:"details"`
	Status           CorporateActionStatus  `json:"status"`
	TenantID         string                 `json:"tenant_id"`
	CreatedAt        string                 `json:"created_at"`
	UpdatedAt        string                 `json:"updated_at"`
}

// EntitlementStatus represents the lifecycle state of an entitlement.
type EntitlementStatus string

const (
	EntitlementStatusPending EntitlementStatus = "PENDING"
	EntitlementStatusPaid    EntitlementStatus = "PAID"
)

// Entitlement represents a participant's entitlement from a corporate action.
type Entitlement struct {
	ID                string            `json:"id"`
	CorporateActionID string            `json:"corporate_action_id"`
	ParticipantID     string            `json:"participant_id"`
	InstrumentID      string            `json:"instrument_id"`
	Quantity          int               `json:"quantity"`
	EntitlementValue  float64           `json:"entitlement_value"`
	Status            EntitlementStatus `json:"status"`
	CreatedAt         string            `json:"created_at"`
}

// FRCReport represents a regulatory report submitted to the FRC.
type FRCReport struct {
	ID          string                 `json:"id"`
	TenantID    string                 `json:"tenant_id"`
	ReportType  string                 `json:"report_type"`
	ReportDate  string                 `json:"report_date"`
	Data        map[string]interface{} `json:"data"`
	GeneratedAt string                 `json:"generated_at"`
}

// MarketSession represents the current trading session phase of an instrument.
type MarketSession string

const (
	SessionPreOpen        MarketSession = "PRE_OPEN"
	SessionContinuous     MarketSession = "CONTINUOUS"
	SessionClosingAuction MarketSession = "CLOSING_AUCTION"
	SessionClosed         MarketSession = "CLOSED"
)

// AuctionResult summarises the outcome of a call auction.
type AuctionResult struct {
	InstrumentID      string  `json:"instrument_id"`
	ClearingPrice     float64 `json:"clearing_price"`
	MatchedVolume     int     `json:"matched_volume"`
	UnmatchedBuyVolume  int   `json:"unmatched_buy_volume"`
	UnmatchedSellVolume int   `json:"unmatched_sell_volume"`
	TradeCount        int     `json:"trade_count"`
}

// ── Exchange Participants ────────────────────────────────────────────────────

// ParticipantStatus represents the lifecycle state of an exchange participant.
type ParticipantStatus string

const (
	ParticipantActive    ParticipantStatus = "PARTICIPANT_ACTIVE"
	ParticipantSuspended ParticipantStatus = "PARTICIPANT_SUSPENDED"
)

// Permission constants for exchange participant capabilities.
const (
	PermTradeEquity    = "TRADE_EQUITY"
	PermTradeBond      = "TRADE_BOND"
	PermTradeETF       = "TRADE_ETF"
	PermMarketMaker    = "MARKET_MAKER"
	PermSponsoredAccess = "SPONSORED_ACCESS"
)

// ExchangeParticipant represents a registered trading participant on the exchange.
type ExchangeParticipant struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      ParticipantStatus `json:"status"`
	Permissions []string          `json:"permissions"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

// ── Day State ────────────────────────────────────────────────────────────────

// DayState represents the overall trading day lifecycle state.
type DayState string

const (
	DayClosed   DayState = "DAY_CLOSED"
	DayPreOpen  DayState = "DAY_PRE_OPEN"
	DayTrading  DayState = "DAY_TRADING"
	DayPostClose DayState = "DAY_POST_CLOSE"
)

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

// ── Market/Segment Hierarchy (MillenniumIT P1) ──────────────────────────────

const (
	MarketActive    = "MARKET_ACTIVE"
	MarketSuspended = "MARKET_SUSPENDED"
	MarketClosed    = "MARKET_CLOSED"
)

const (
	SegActive    = "SEG_ACTIVE"
	SegSuspended = "SEG_SUSPENDED"
)

type Market struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Timezone  string `json:"timezone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Segment struct {
	ID        string `json:"id"`
	MarketID  string `json:"market_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ── Circuit Breakers (MillenniumIT P1) ───────────────────────────────────────

const (
	CBActive    = "CB_ACTIVE"
	CBTriggered = "CB_TRIGGERED"
	CBCooldown  = "CB_COOLDOWN"
)

const (
	CBStaticUpper  = "CB_STATIC_UPPER"
	CBStaticLower  = "CB_STATIC_LOWER"
	CBDynamicUpper = "CB_DYNAMIC_UPPER"
	CBDynamicLower = "CB_DYNAMIC_LOWER"
)

type CircuitBreaker struct {
	InstrumentID    string  `json:"instrument_id"`
	StaticUpperPct  float64 `json:"static_upper_pct"`
	StaticLowerPct  float64 `json:"static_lower_pct"`
	DynamicUpperPct float64 `json:"dynamic_upper_pct"`
	DynamicLowerPct float64 `json:"dynamic_lower_pct"`
	ReferencePrice  float64 `json:"reference_price"`
	LastTradedPrice float64 `json:"last_traded_price"`
	Status          string  `json:"status"`
	CooldownMinutes int     `json:"cooldown_minutes"`
	TriggeredAt     string  `json:"triggered_at,omitempty"`
}

type CircuitBreakerEvent struct {
	InstrumentID   string  `json:"instrument_id"`
	Type           string  `json:"type"`
	TriggerPrice   float64 `json:"trigger_price"`
	ReferencePrice float64 `json:"reference_price"`
	Timestamp      string  `json:"timestamp"`
}

// ── Self-Trade Prevention (MillenniumIT P1) ──────────────────────────────────

type STPMode = string

const (
	STPCancelNewest STPMode = "STP_CANCEL_NEWEST"
	STPCancelOldest STPMode = "STP_CANCEL_OLDEST"
	STPCancelBoth   STPMode = "STP_CANCEL_BOTH"
)
