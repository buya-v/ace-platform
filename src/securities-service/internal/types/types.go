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
	TradingCycleID    string        `json:"trading_cycle_id,omitempty"` // T1: links instrument to a TradingCycle
	STPMode           STPMode       `json:"stp_mode,omitempty"`
	DeletionStatus      string        `json:"deletion_status,omitempty"`
	DeletionDate        string        `json:"deletion_date,omitempty"`
	ShortSellRestricted bool          `json:"short_sell_restricted,omitempty"`
	FolderID            string        `json:"folder_id,omitempty"` // Sprint 8: instrument folder
	CreatedAt           string        `json:"created_at"`
	UpdatedAt           string        `json:"updated_at"`
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
	ID              string      `json:"id"`
	InstrumentID    string      `json:"instrument_id"`
	ParticipantID   string      `json:"participant_id"`
	FirmID          string      `json:"firm_id,omitempty"`          // T1: originating firm
	ClientOrderID   string      `json:"client_order_id,omitempty"` // T1: client-assigned order id
	Side            OrderSide   `json:"side"`
	OrderType       OrderType   `json:"order_type"`
	Quantity        int         `json:"quantity"`
	Price           float64     `json:"price"`
	StopPrice       float64     `json:"stop_price"`
	TimeInForce     TimeInForce `json:"time_in_force"`
	Status          OrderStatus `json:"status"`
	FilledQuantity  int         `json:"filled_quantity"`
	AvgFillPrice    float64     `json:"avg_fill_price"`
	VisibleQuantity int         `json:"visible_quantity,omitempty"` // Iceberg: visible (displayed) quantity
	HiddenQuantity  int         `json:"hidden_quantity,omitempty"`  // Iceberg: hidden (reserve) quantity
	LocateID        string      `json:"locate_id,omitempty"`        // P4a: required for SHORT_SELL orders
	ClientID        string      `json:"client_id,omitempty"`        // Part D: originating client entity
	CreatedAt       string      `json:"created_at"`
	UpdatedAt       string      `json:"updated_at"`
	ArchivedAt      string      `json:"archived_at,omitempty"`
}

// TradeStatus represents the lifecycle state of a trade.
type TradeStatus string

const (
	TradeStatusPending   TradeStatus = "TRADE_PENDING"
	TradeStatusConfirmed TradeStatus = "TRADE_CONFIRMED"
	TradeStatusSettled   TradeStatus = "TRADE_SETTLED"
	TradeStatusFailed    TradeStatus = "TRADE_FAILED"
	TradeStatusBusted    TradeStatus = "TRADE_BUSTED"
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
	ArchivedAt     string      `json:"archived_at,omitempty"`
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
	AccruedInterest     float64          `json:"accrued_interest,omitempty"`
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

// ── Firms ────────────────────────────────────────────────────────────────────

type FirmStatus string

const (
	FirmActive      FirmStatus = "FIRM_ACTIVE"
	FirmSuspended   FirmStatus = "FIRM_SUSPENDED"
	FirmDeactivated FirmStatus = "FIRM_DEACTIVATED"
)

type Firm struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Status         FirmStatus `json:"status"`
	ClearingFirmID string     `json:"clearing_firm_id,omitempty"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

// ── Exchange Participants ────────────────────────────────────────────────────

type ParticipantStatus string

const (
	ParticipantActive    ParticipantStatus = "PARTICIPANT_ACTIVE"
	ParticipantSuspended ParticipantStatus = "PARTICIPANT_SUSPENDED"
	ParticipantLocked    ParticipantStatus = "PARTICIPANT_LOCKED"
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
	FirmID      string            `json:"firm_id"`
	Name        string            `json:"name"`
	Role        string            `json:"role,omitempty"`
	Status      ParticipantStatus `json:"status"`
	Permissions []string          `json:"permissions"`
	LockedAt    string            `json:"locked_at,omitempty"`
	LockReason  string            `json:"lock_reason,omitempty"`
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
	MarketHalted    = "MARKET_HALTED"
	MarketSuspended = "MARKET_SUSPENDED"
	MarketClosed    = "MARKET_CLOSED"
)

const (
	SegActive    = "SEG_ACTIVE"
	SegHalted    = "SEG_HALTED"
	SegSuspended = "SEG_SUSPENDED"
)

type Market struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Timezone    string `json:"timezone"`
	OpenTime    string `json:"open_time,omitempty"`    // T1: HH:MM in market timezone
	CloseTime   string `json:"close_time,omitempty"`   // T1: HH:MM in market timezone
	TradingDate string `json:"trading_date,omitempty"` // T1: set to today ISO date on StartDay
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
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

// ── IOC / FOK aliases (Part C) ────────────────────────────────────────────────
// TimeInForceIOC and TimeInForceFOK are the canonical constants; these aliases
// provide the TIF_ prefix expected by matching-engine and order-validation code.
const (
	TIF_IOC = TimeInForceIOC // Immediate Or Cancel
	TIF_FOK = TimeInForceFOK // Fill Or Kill
)

// ── Trade Correction (Part A) ─────────────────────────────────────────────────

// TradeCorrection represents a post-trade correction action (bust, correct, or reinstate).
type TradeCorrection struct {
	ID                string  `json:"id"`
	TradeID           string  `json:"trade_id"`
	Action            string  `json:"action"`             // BUST | CORRECT | REINSTATE
	Reason            string  `json:"reason"`
	OriginalPrice     float64 `json:"original_price"`
	OriginalQuantity  int     `json:"original_quantity"`
	CorrectedPrice    float64 `json:"corrected_price"`
	CorrectedQuantity int     `json:"corrected_quantity"`
	ActorID           string  `json:"actor_id"`
	Timestamp         string  `json:"timestamp"`
}

// ── Tiered Tick Table (Part B) ────────────────────────────────────────────────

// TickTier defines the tick size applicable within a specific price band.
type TickTier struct {
	MinPrice float64 `json:"min_price"`
	MaxPrice float64 `json:"max_price"`
	TickSize float64 `json:"tick_size"`
}

// TickTable holds the ordered list of price tiers for a single instrument.
type TickTable struct {
	InstrumentID string     `json:"instrument_id"`
	Tiers        []TickTier `json:"tiers"`
}

// ── Announcements ─────────────────────────────────────────────────────────────

// AnnouncementAudience controls who can see an announcement.
type AnnouncementAudience string

const (
	AudiencePublic      AnnouncementAudience = "PUBLIC"
	AudienceParticipant AnnouncementAudience = "PARTICIPANT"
	AudienceInternal    AnnouncementAudience = "INTERNAL"
)

// Announcement is a market notice published by the exchange operator.
type Announcement struct {
	ID        string               `json:"id"`
	TenantID  string               `json:"tenant_id"`
	Title     string               `json:"title"`
	Body      string               `json:"body"`
	Audience  AnnouncementAudience `json:"audience"`
	CreatedAt string               `json:"created_at"`
	UpdatedAt string               `json:"updated_at"`
}

// ── Audit Trail ───────────────────────────────────────────────────────────────

// AuditEntry records a single auditable action on a domain entity.
type AuditEntry struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"` // ORDER | TRADE | INSTRUMENT | PARTICIPANT
	EntityID   string `json:"entity_id"`
	Action     string `json:"action"` // CREATE | UPDATE | BUST | SUSPEND | REINSTATE
	ActorID    string `json:"actor_id"`
	TenantID   string `json:"tenant_id"`
	Timestamp  string `json:"timestamp"`
	Detail     string `json:"detail,omitempty"`
}

// AuditFilters carries optional filter parameters for listing audit entries.
type AuditFilters struct {
	EntityType string
	EntityID   string
	ActorID    string
	StartDate  string
	EndDate    string
}

// ── Pending Changes (P2c) ──────────────────────────────────────────────────────

// PendingChange represents a maker/checker workflow record for a proposed entity change.
type PendingChange struct {
	ID             string                 `json:"id"`
	EntityType     string                 `json:"entity_type"`
	EntityID       string                 `json:"entity_id"`
	ChangeType     string                 `json:"change_type"` // CREATE | UPDATE | DELETE
	Payload        map[string]interface{} `json:"payload"`
	SubmittedBy    string                 `json:"submitted_by"`
	Status         string                 `json:"status"` // PENDING_APPROVAL | APPROVED | REJECTED
	ReviewedBy     string                 `json:"reviewed_by,omitempty"`
	ReviewComment  string                 `json:"review_comment,omitempty"`
	SubmittedAt    string                 `json:"submitted_at"`
	ReviewedAt     string                 `json:"reviewed_at,omitempty"`
}

// ReferencePrice represents the official reference price for an instrument,
// used as the anchor for circuit breaker percentage limits.
type ReferencePrice struct {
	InstrumentID          string  `json:"instrument_id"`
	Price                 float64 `json:"price"`
	SetBy                 string  `json:"set_by"`
	SetAt                 string  `json:"set_at"`
	StaleThresholdMinutes int     `json:"stale_threshold_minutes"`
}

// ── Surveillance ──────────────────────────────────────────────────────────────

// AlertStatus represents the lifecycle state of a surveillance alert.
type AlertStatus string

const (
	AlertStatusOpen         AlertStatus = "OPEN"
	AlertStatusInvestigating AlertStatus = "INVESTIGATING"
	AlertStatusResolved     AlertStatus = "RESOLVED"
)

// AlertType represents the category of a surveillance alert.
type AlertType string

const (
	AlertTypeLargeTrade         AlertType = "LARGE_TRADE"
	AlertTypePriceSpike         AlertType = "PRICE_SPIKE"
	AlertTypeWashTrade          AlertType = "WASH_TRADE"
	AlertTypeVolumeAnomaly      AlertType = "VOLUME_ANOMALY"
	AlertTypeFrontRunning       AlertType = "FRONT_RUNNING"
	AlertTypeSpoofing           AlertType = "SPOOFING"
	AlertTypeLayering           AlertType = "LAYERING"
	AlertTypeInsiderTrading     AlertType = "INSIDER_TRADING"
	AlertTypeMarketManipulation AlertType = "MARKET_MANIPULATION"
	AlertTypeConcentration      AlertType = "CONCENTRATION"
	AlertTypeUnusualActivity    AlertType = "UNUSUAL_ACTIVITY"
	AlertTypeCrossMarket        AlertType = "CROSS_MARKET"
)

// SurveillanceAlert represents a market surveillance alert raised by the engine.
type SurveillanceAlert struct {
	ID           string      `json:"id"`
	InstrumentID string      `json:"instrument_id"`
	AlertType    AlertType   `json:"alert_type"`
	Status       AlertStatus `json:"status"`
	Message      string      `json:"message"`
	TradeID      string      `json:"trade_id,omitempty"`
	CreatedAt    string      `json:"created_at"`
	ResolvedAt   string      `json:"resolved_at,omitempty"`
	ResolvedBy   string      `json:"resolved_by,omitempty"`
}

// SurveillanceThreshold defines a monitoring threshold for a specific alert type on an instrument.
type SurveillanceThreshold struct {
	InstrumentID string    `json:"instrument_id"`
	AlertType    AlertType `json:"alert_type"`
	Value        float64   `json:"value"`
	UpdatedAt    string    `json:"updated_at"`
}

// ── Instrument Groups ─────────────────────────────────────────────────────────

// GroupType classifies how an instrument group was formed.
type GroupType string

const (
	GroupTypeManual   GroupType = "MANUAL"
	GroupTypeSector   GroupType = "SECTOR"
	GroupTypeIndex    GroupType = "INDEX"
)

// InstrumentGroup represents a named collection of instruments (e.g., an index or sector basket).
type InstrumentGroup struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	GroupType     GroupType `json:"group_type"`
	InstrumentIDs []string  `json:"instrument_ids"`
	CreatedAt     string    `json:"created_at"`
	UpdatedAt     string    `json:"updated_at"`
}

// ── Off-Book Trades ───────────────────────────────────────────────────────────

// OffBookStatus represents the reporting state of an off-book trade.
type OffBookStatus string

const (
	OffBookReported  OffBookStatus = "REPORTED"
	OffBookConfirmed OffBookStatus = "CONFIRMED"
	OffBookRejected  OffBookStatus = "REJECTED"
)

// OffBookTrade represents an off-exchange negotiated trade reported to the exchange.
type OffBookTrade struct {
	ID              string        `json:"id"`
	InstrumentID    string        `json:"instrument_id"`
	BuyParticipant  string        `json:"buy_participant"`
	SellParticipant string        `json:"sell_participant"`
	Price           float64       `json:"price"`
	Quantity        int           `json:"quantity"`
	TradeDate       string        `json:"trade_date"`
	Status          OffBookStatus `json:"status"`
	Notes           string        `json:"notes,omitempty"`
	ConfirmedBy     string        `json:"confirmed_by,omitempty"`
	RejectedBy      string        `json:"rejected_by,omitempty"`
	RejectionReason string        `json:"rejection_reason,omitempty"`
	CreatedAt       string        `json:"created_at"`
	UpdatedAt       string        `json:"updated_at"`
}

// ── P3b — Market Data ─────────────────────────────────────────────────────────

// PriceLevel represents an aggregated price level in the order book.
type PriceLevel struct {
	Price      float64 `json:"price"`
	Quantity   int     `json:"quantity"`
	OrderCount int     `json:"order_count"`
}

// OrderBookSnapshot is a point-in-time view of the CLOB for one instrument.
type OrderBookSnapshot struct {
	InstrumentID string       `json:"instrument_id"`
	Bids         []PriceLevel `json:"bids"`
	Asks         []PriceLevel `json:"asks"`
	Timestamp    string       `json:"timestamp"`
}

// TickerData carries the latest market summary for one instrument.
type TickerData struct {
	InstrumentID string  `json:"instrument_id"`
	LastPrice    float64 `json:"last_price"`
	BidPrice     float64 `json:"bid_price"`
	AskPrice     float64 `json:"ask_price"`
	Volume       int     `json:"volume"`
	DayHigh      float64 `json:"day_high"`
	DayLow       float64 `json:"day_low"`
	Timestamp    string  `json:"timestamp"`
}

// BulkUploadResult summarises the outcome of a bulk instrument upload.
type BulkUploadResult struct {
	Total   int         `json:"total"`
	Created int         `json:"created"`
	Failed  int         `json:"failed"`
	Errors  []BulkError `json:"errors"`
}

// BulkError describes a single validation failure within a bulk upload.
type BulkError struct {
	Index  int    `json:"index"`
	Ticker string `json:"ticker"`
	Error  string `json:"error"`
}

// ── P4a — Short Sell, Locate, RFQ, Give-Up ───────────────────────────────────

// LocateRequest represents a request by a borrower firm to locate shares for short selling.
// Status lifecycle: PENDING → APPROVED or REJECTED → EXPIRED or USED.
type LocateRequest struct {
	ID             int    `json:"id"`
	InstrumentID   int    `json:"instrument_id"`
	BorrowerFirmID int    `json:"borrower_firm_id"`
	LenderFirmID   int    `json:"lender_firm_id"`
	Quantity       int    `json:"quantity"`
	Status         string `json:"status"` // PENDING | APPROVED | REJECTED | EXPIRED | USED
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
}

// RequestForQuote represents a request for quote (RFQ) submitted by a firm for a block trade.
// Status lifecycle: OPEN → RESPONDED or EXPIRED or CANCELLED.
type RequestForQuote struct {
	ID              int    `json:"id"`
	InstrumentID    int    `json:"instrument_id"`
	RequestorFirmID int    `json:"requestor_firm_id"`
	Quantity        int    `json:"quantity"`
	Side            string `json:"side"`
	Status          string `json:"status"` // OPEN | RESPONDED | EXPIRED | CANCELLED
	ResponseQuoteID int    `json:"response_quote_id,omitempty"`
	TenantID        string `json:"tenant_id"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
}

// GiveUpRequest represents a trade give-up instruction from one firm to another.
// Status lifecycle: PENDING → ACCEPTED or REJECTED.
type GiveUpRequest struct {
	ID         int    `json:"id"`
	TradeID    int    `json:"trade_id"`
	FromFirmID int    `json:"from_firm_id"`
	ToFirmID   int    `json:"to_firm_id"`
	Status     string `json:"status"` // PENDING | ACCEPTED | REJECTED
	Reason     string `json:"reason,omitempty"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// ── Surveillance Investigations ───────────────────────────────────────────────

// InvestigationStatus represents the lifecycle state of an investigation.
type InvestigationStatus string

const (
	InvestigationOpen   InvestigationStatus = "OPEN"
	InvestigationClosed InvestigationStatus = "CLOSED"
)

// Investigation represents a formal market surveillance investigation into potential rule breaches.
type Investigation struct {
	ID           string              `json:"id"`
	AlertID      string              `json:"alert_id,omitempty"` // surveillance alert that triggered this investigation
	Subject      string              `json:"subject"`             // brief description of what is being investigated
	InstrumentID string              `json:"instrument_id"`       // instrument under investigation (may be empty)
	Status       InvestigationStatus `json:"status"`
	AssignedTo   string              `json:"assigned_to,omitempty"`
	Findings     string              `json:"findings,omitempty"`
	Evidence     []string            `json:"evidence,omitempty"` // list of evidence references
	OpenedAt     string              `json:"opened_at"`
	ClosedAt     string              `json:"closed_at,omitempty"`
}

// ── Market Replay ─────────────────────────────────────────────────────────────

// ReplaySession represents a recorded market replay session.
type ReplaySession struct {
	ID          string `json:"id"`
	InstrumentID string `json:"instrument_id"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// ReplayEvent represents a single event within a replay session.
type ReplayEvent struct {
	SessionID  string      `json:"session_id"`
	Sequence   int         `json:"sequence"`
	EventType  string      `json:"event_type"` // ORDER | TRADE | STATUS_CHANGE
	Payload    interface{} `json:"payload"`
	OccurredAt string      `json:"occurred_at"`
}

// ── Fixed-Income Bonds ────────────────────────────────────────────────────────

// DayCountConvention specifies the accrued-interest day-count basis for a bond.
type DayCountConvention string

const (
	DayCountACT360 DayCountConvention = "ACT/360"
	DayCountACT365 DayCountConvention = "ACT/365"
	DayCount30360  DayCountConvention = "30/360"
)

// Bond represents a fixed-income bond instrument listed on the exchange.
type Bond struct {
	ID                 string             `json:"id"`
	ISIN               string             `json:"isin"`
	Name               string             `json:"name"`
	Issuer             string             `json:"issuer"`
	MaturityDate       string             `json:"maturity_date"`
	CouponRate         float64            `json:"coupon_rate"`   // annual rate, e.g. 0.05 for 5%
	CouponFrequency    string             `json:"coupon_frequency"` // ANNUAL | SEMI_ANNUAL | QUARTERLY
	ParValue           float64            `json:"par_value"`
	DayCountConvention DayCountConvention `json:"day_count_convention"`
	TradingStatus      TradingStatus      `json:"trading_status"`
	CreatedAt          string             `json:"created_at"`
	UpdatedAt          string             `json:"updated_at"`
}

// ── Trading Strategies ────────────────────────────────────────────────────────

// StrategyType classifies the pattern of a multi-leg trading strategy.
type StrategyType string

const (
	StrategyTypeSpread     StrategyType = "SPREAD"
	StrategyTypeStraddle   StrategyType = "STRADDLE"
	StrategyTypeStrangle   StrategyType = "STRANGLE"
	StrategyTypeButterfly  StrategyType = "BUTTERFLY"
	StrategyTypeCustom     StrategyType = "CUSTOM"
)

// StrategyStatus represents the lifecycle state of a trading strategy.
type StrategyStatus string

const (
	StrategyStatusActive   StrategyStatus = "STRATEGY_ACTIVE"
	StrategyStatusInactive StrategyStatus = "STRATEGY_INACTIVE"
	StrategyStatusDeleted  StrategyStatus = "STRATEGY_DELETED"
)

// StrategyLeg defines one leg of a multi-leg trading strategy.
type StrategyLeg struct {
	InstrumentID string    `json:"instrument_id"`
	Side         OrderSide `json:"side"`   // BUY or SELL
	RatioQty     int       `json:"ratio_qty"` // relative quantity ratio (e.g. 1, 2)
}

// TradingStrategy represents a named multi-leg strategy definition.
type TradingStrategy struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	StrategyType StrategyType   `json:"strategy_type"`
	Legs         []StrategyLeg  `json:"legs"`
	Status       StrategyStatus `json:"status"`
	TenantID     string         `json:"tenant_id"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

// ── Per-Firm Throttle Configuration ──────────────────────────────────────────

// ThrottleConfig holds the per-firm rate limit configuration for order submission.
type ThrottleConfig struct {
	FirmID             string `json:"firm_id"`
	MaxOrdersPerSecond int    `json:"max_orders_per_second"`
	Enabled            bool   `json:"enabled"`
	UpdatedAt          string `json:"updated_at"`
}

// ── CSD — Custody Accounts, Balances, Transfers ───────────────────────────────

// CustodyAccount represents a securities holding account at the central securities depository.
type CustodyAccount struct {
	ID        string `json:"id"`
	FirmID    string `json:"firm_id"`
	Name      string `json:"name"`
	Currency  string `json:"currency"`
	TenantID  string `json:"tenant_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CustodyBalance represents the holdings of a single instrument within a custody account.
type CustodyBalance struct {
	AccountID    string  `json:"account_id"`
	InstrumentID string  `json:"instrument_id"`
	Quantity     int     `json:"quantity"`
	AvgCost      float64 `json:"avg_cost"`
	UpdatedAt    string  `json:"updated_at"`
}

// CSDTransferType classifies the type of a CSD securities transfer.
type CSDTransferType string

const (
	CSDTransferDVP CSDTransferType = "DVP" // Delivery Versus Payment
	CSDTransferFOP CSDTransferType = "FOP" // Free Of Payment
)

// CSDTransferStatus represents the lifecycle state of a CSD transfer instruction.
type CSDTransferStatus string

const (
	CSDTransferPending   CSDTransferStatus = "CSD_PENDING"
	CSDTransferCompleted CSDTransferStatus = "CSD_COMPLETED"
	CSDTransferFailed    CSDTransferStatus = "CSD_FAILED"
)

// CSDTransfer represents a securities transfer instruction between custody accounts.
type CSDTransfer struct {
	ID              string            `json:"id"`
	FromAccountID   string            `json:"from_account_id"`
	ToAccountID     string            `json:"to_account_id"`
	InstrumentID    string            `json:"instrument_id"`
	Quantity        int               `json:"quantity"`
	TransferType    CSDTransferType   `json:"transfer_type"`
	SettlementAmount float64          `json:"settlement_amount,omitempty"` // only for DVP
	Status          CSDTransferStatus `json:"status"`
	FailReason      string            `json:"fail_reason,omitempty"`
	TenantID        string            `json:"tenant_id"`
	CreatedAt       string            `json:"created_at"`
	UpdatedAt       string            `json:"updated_at"`
}

// ── Node Hierarchy ────────────────────────────────────────────────────────────

// Node represents a hierarchical organisational node within a firm (e.g. desk, team, branch).
// Permissions are inherited from parent nodes and merged with local overrides.
type Node struct {
	ID           string   `json:"id"`
	FirmID       string   `json:"firm_id"`
	ParentNodeID string   `json:"parent_node_id,omitempty"`
	Name         string   `json:"name"`
	Permissions  []string `json:"permissions"`
	CreatedAt    string   `json:"created_at"`
}

// ── Watch Lists ───────────────────────────────────────────────────────────────

// WatchList is a named collection of instruments and/or clients/firms that a
// user monitors for surveillance or alerting purposes.
type WatchList struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	OwnerID       string   `json:"owner_id"`
	InstrumentIDs []string `json:"instrument_ids"`
	ClientIDs     []string `json:"client_ids"`
	FirmIDs       []string `json:"firm_ids"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// ── IP Restrictions ───────────────────────────────────────────────────────────

// IPRestriction defines which IP addresses a given participant is allowed to
// connect from. When Enabled is false the allow-list is not enforced.
type IPRestriction struct {
	ParticipantID string   `json:"participant_id"`
	AllowedIPs    []string `json:"allowed_ips"`
	Enabled       bool     `json:"enabled"`
}

// ── Password Policy ───────────────────────────────────────────────────────────

// PasswordPolicy defines the complexity and expiry rules for passwords within a
// tenant. Each tenant has exactly one policy record.
type PasswordPolicy struct {
	TenantID         string `json:"tenant_id"`
	MinLength        int    `json:"min_length"`
	RequireUppercase bool   `json:"require_uppercase"`
	RequireLowercase bool   `json:"require_lowercase"`
	RequireDigit     bool   `json:"require_digit"`
	RequireSpecial   bool   `json:"require_special"`
	MaxAgeDays       int    `json:"max_age_days"`
}

// ── RBAC — Roles and Fine-Grained Permissions ─────────────────────────────────

// Permission constants for fine-grained RBAC enforcement across the platform.
// These supplement the coarse-grained participant capabilities (PermTradeEquity, etc.)
// with operator / admin-level action gates.
const (
	// Instrument lifecycle
	PermInstrumentCreate = "INSTRUMENT_CREATE"
	PermInstrumentUpdate = "INSTRUMENT_UPDATE"
	PermInstrumentDelete = "INSTRUMENT_DELETE"
	PermInstrumentHalt   = "INSTRUMENT_HALT"

	// Order management
	PermOrderCreate     = "ORDER_CREATE"
	PermOrderCancel     = "ORDER_CANCEL"
	PermOrderMassCancel = "ORDER_MASS_CANCEL"
	PermOrderView       = "ORDER_VIEW"

	// Trade management
	PermTradeBust     = "TRADE_BUST"
	PermTradeCorrect  = "TRADE_CORRECT"
	PermTradeReinstate = "TRADE_REINSTATE"
	PermTradeView      = "TRADE_VIEW"

	// Market / session control
	PermMarketStartDay   = "MARKET_START_DAY"
	PermMarketEndDay     = "MARKET_END_DAY"
	PermMarketStartTrading = "MARKET_START_TRADING"
	PermMarketEndTrading   = "MARKET_END_TRADING"
	PermMarketHalt         = "MARKET_HALT"

	// Settlement
	PermSettlementTrigger = "SETTLEMENT_TRIGGER"
	PermSettlementView    = "SETTLEMENT_VIEW"

	// Corporate actions
	PermCorporateActionCreate  = "CORPORATE_ACTION_CREATE"
	PermCorporateActionProcess = "CORPORATE_ACTION_PROCESS"

	// Participants & firms
	PermParticipantCreate  = "PARTICIPANT_CREATE"
	PermParticipantSuspend = "PARTICIPANT_SUSPEND"
	PermParticipantView    = "PARTICIPANT_VIEW"
	PermFirmCreate         = "FIRM_CREATE"
	PermFirmView           = "FIRM_VIEW"

	// Surveillance
	PermSurveillanceView    = "SURVEILLANCE_VIEW"
	PermSurveillanceResolve = "SURVEILLANCE_RESOLVE"
	PermSurveillanceInvestigate = "SURVEILLANCE_INVESTIGATE"

	// Admin
	PermAdminAnnouncements = "ADMIN_ANNOUNCEMENTS"
	PermAdminForceLogout   = "ADMIN_FORCE_LOGOUT"
	PermAdminAuditView     = "ADMIN_AUDIT_VIEW"
	PermAdminRoleManage    = "ADMIN_ROLE_MANAGE"

	// Reference data
	PermRefDataView  = "REF_DATA_VIEW"
	PermRefDataWrite = "REF_DATA_WRITE"
)

// Role defines a named set of permissions that can be assigned to participants.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// ── Session Extension ─────────────────────────────────────────────────────────

// SessionExtension records an operator-initiated extend or shorten of a trading session.
type SessionExtension struct {
	InstrumentID    string `json:"instrument_id"`
	Action          string `json:"action"`           // EXTEND | SHORTEN
	DurationMinutes int    `json:"duration_minutes"`
	Reason          string `json:"reason"`
	CreatedAt       string `json:"created_at"`
}

// ── Trading Parameter Sets ────────────────────────────────────────────────────

// AuctionConfig holds auction-phase parameters for a trading parameter set.
type AuctionConfig struct {
	RandomEndSeconds          int    `json:"random_end_seconds"`
	SurplusHandling           string `json:"surplus_handling"`            // PRO_RATA | TIME_PRIORITY
	MinAuctionDurationSeconds int    `json:"min_auction_duration_seconds"`
}

// TradingParameterSet defines the unified set of trading controls applied to
// an instrument. All constraint fields are optional — a zero value means the
// check is skipped during order validation.
type TradingParameterSet struct {
	ID                  string        `json:"id"`
	InstrumentID        string        `json:"instrument_id"`
	Name                string        `json:"name"`
	TickTableID         string        `json:"tick_table_id,omitempty"`
	CircuitBreakerID    string        `json:"circuit_breaker_id,omitempty"`
	AllowedOrderTypes   []string      `json:"allowed_order_types,omitempty"`
	AllowedTimeInForce  []string      `json:"allowed_time_in_force,omitempty"`
	MinOrderSize        int           `json:"min_order_size,omitempty"`
	MaxOrderSize        int           `json:"max_order_size,omitempty"`
	MaxOrderValue       float64       `json:"max_order_value,omitempty"`
	AuctionParams       AuctionConfig `json:"auction_params"`
	STPMode             string        `json:"stp_mode,omitempty"`
	ShortSellingAllowed bool          `json:"short_selling_allowed"`
	CreatedAt           string        `json:"created_at"`
	UpdatedAt           string        `json:"updated_at"`
}

// ── Trading Cycles (T1) ───────────────────────────────────────────────────────

// TradingCycle defines a named sequence of trading sessions (phases) for a market.
// Instruments reference a TradingCycle via TradingCycleID.
// Example cycles: "STANDARD" (pre-open → continuous → closing auction → closed),
// "OFF_BOOK" (off-book only, no continuous auction).
type TradingCycle struct {
	ID              string   `json:"id"`
	MarketID        string   `json:"market_id"`
	Name            string   `json:"name"`
	SessionSequence []string `json:"session_sequence"` // ordered list of MarketSession values
	IsDefault       bool     `json:"is_default"`
	CreatedAt       string   `json:"created_at"`
}

// ── Part B — Post-Trade Parameters ───────────────────────────────────────────

// PostTradeParams holds the clearing and settlement configuration for an instrument.
type PostTradeParams struct {
	ID               string  `json:"id"`
	InstrumentID     string  `json:"instrument_id"`
	SettlementCycle  string  `json:"settlement_cycle"`   // e.g. "T+2"
	ClearingFirmID   string  `json:"clearing_firm_id"`
	FeeScheduleID    string  `json:"fee_schedule_id"`
	PenaltyRatePct   float64 `json:"penalty_rate_pct"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

// ── Part C — Config Tables ────────────────────────────────────────────────────

// ConfigTableType classifies the purpose of a configurable tabular structure.
type ConfigTableType string

const (
	ConfigTableTypeFeeSchedule   ConfigTableType = "FEE_SCHEDULE"
	ConfigTableTypeTaxRate       ConfigTableType = "TAX_RATE"
	ConfigTableTypeHoliday       ConfigTableType = "HOLIDAY"
	ConfigTableTypeMarginMatrix  ConfigTableType = "MARGIN_MATRIX"
	ConfigTableTypeThrottle      ConfigTableType = "THROTTLE"
	ConfigTableTypeCustom        ConfigTableType = "CUSTOM"
)

// ConfigTable stores an operator-managed tabular configuration structure.
type ConfigTable struct {
	ID        string                   `json:"id"`
	TableType ConfigTableType          `json:"table_type"`
	Name      string                   `json:"name"`
	Rows      []map[string]interface{} `json:"rows"`
	CreatedAt string                   `json:"created_at"`
	UpdatedAt string                   `json:"updated_at"`
}

// ── Part D — Client Entities ──────────────────────────────────────────────────

// ClientType classifies the category of a registered client.
type ClientType string

const (
	ClientTypeIndividual    ClientType = "INDIVIDUAL"
	ClientTypeInstitutional ClientType = "INSTITUTIONAL"
	ClientTypeProprietary   ClientType = "PROPRIETARY"
)

// Client represents an end-client registered under a member firm.
type Client struct {
	ID          string     `json:"id"`
	FirmID      string     `json:"firm_id"`
	Name        string     `json:"name"`
	Nationality string     `json:"nationality"`
	ClientType  ClientType `json:"client_type"`
	CreatedAt   string     `json:"created_at"`
}

// ── Sprint 8 — Part A: Index ─────────────────────────────────────────────────

// Index represents a market index calculated from a weighted basket of instruments.
type Index struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	InstrumentWeights  map[string]float64 `json:"instrument_weights"`
	BaseValue          float64            `json:"base_value"`
	CurrentValue       float64            `json:"current_value"`
	ChangePercent      float64            `json:"change_percent"`
	LastCalculatedAt   string             `json:"last_calculated_at"`
	CreatedAt          string             `json:"created_at"`
}

// ── Sprint 8 — Part B: Entity Permissions ────────────────────────────────────

// EntityPermission defines what a role is allowed to do on a given entity type.
type EntityPermission struct {
	ID           string `json:"id"`
	RoleID       string `json:"role_id"`
	EntityType   string `json:"entity_type"`
	AllowCreate  bool   `json:"allow_create"`
	AllowView    bool   `json:"allow_view"`
	AllowEdit    bool   `json:"allow_edit"`
	AllowDelete  bool   `json:"allow_delete"`
	AllowApprove bool   `json:"allow_approve"`
}

// ── Sprint 8 — Part C: Instrument Folders ────────────────────────────────────

// Folder represents a hierarchical grouping node for instruments.
type Folder struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id,omitempty"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ── Sprint 8 — Part D: Warnings ──────────────────────────────────────────────

// Warning severity and type constants.
const (
	WarnDeleteActive         = "WARN_DELETE_ACTIVE"
	WarnHaltDuringAuction    = "WARN_HALT_DURING_AUCTION"
	WarnLargeOrder           = "WARN_LARGE_ORDER"
	WarnCircuitBreakerChange = "WARN_CIRCUIT_BREAKER_CHANGE"
	WarnRoleDeletion         = "WARN_ROLE_DELETION"
)

// Warning represents an operator-visible advisory raised by the system.
type Warning struct {
	ID             string `json:"id"`
	WarningType    string `json:"warning_type"`
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	Message        string `json:"message"`
	Severity       string `json:"severity"`
	AcknowledgedBy string `json:"acknowledged_by,omitempty"`
	CreatedAt      string `json:"created_at"`
}
