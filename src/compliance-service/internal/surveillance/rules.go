// Package surveillance implements market surveillance capabilities for the
// GarudaX compliance service, including trade pattern detection (wash trading,
// spoofing, concentration, unusual volume) and alert generation.
package surveillance

import (
	"encoding/json"
	"time"
)

// RuleType identifies the kind of surveillance rule.
type RuleType string

const (
	RuleWashTrading       RuleType = "wash_trading"
	RuleSpoofing          RuleType = "spoofing"
	RulePriceManipulation RuleType = "price_manipulation"
	RuleConcentration     RuleType = "concentration"
	RuleUnusualVolume     RuleType = "unusual_volume"
)

// ValidRuleType returns true if rt is a recognised rule type.
func ValidRuleType(rt RuleType) bool {
	switch rt {
	case RuleWashTrading, RuleSpoofing, RulePriceManipulation,
		RuleConcentration, RuleUnusualVolume:
		return true
	}
	return false
}

// Severity classifies how serious an alert is.
type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// AlertStatus tracks the lifecycle of a surveillance alert.
type AlertStatus string

const (
	AlertStatusOpen     AlertStatus = "OPEN"
	AlertStatusResolved AlertStatus = "RESOLVED"
	AlertStatusEscalated AlertStatus = "ESCALATED"
)

// Rule is a configurable surveillance rule with typed parameters.
type Rule struct {
	ID        string          `json:"id"`
	RuleType  RuleType        `json:"rule_type"`
	Params    json.RawMessage `json:"parameters"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
}

// WashTradingParams holds thresholds for wash trading detection.
type WashTradingParams struct {
	TimeWindowSeconds int `json:"time_window_seconds"`
	MinTrades         int `json:"min_trades"`
}

// SpoofingParams holds thresholds for spoofing detection.
type SpoofingParams struct {
	CancelWindowSeconds int     `json:"cancel_window_seconds"`
	MinOrderSizePct     float64 `json:"min_order_size_pct"`
}

// ConcentrationParams holds thresholds for position concentration detection.
type ConcentrationParams struct {
	MaxPositionPct float64 `json:"max_position_pct"`
}

// UnusualVolumeParams holds thresholds for unusual volume detection.
type UnusualVolumeParams struct {
	StdDevThreshold float64 `json:"std_dev_threshold"`
	LookbackDays    int     `json:"lookback_days"`
}

// Alert is a surveillance alert generated when a rule fires.
type Alert struct {
	ID            string      `json:"id"`
	RuleID        string      `json:"rule_id"`
	ParticipantID string      `json:"participant_id"`
	InstrumentID  string      `json:"instrument_id,omitempty"`
	Severity      Severity    `json:"severity"`
	Details       string      `json:"details"`
	Status        AlertStatus `json:"status"`
	DetectedAt    time.Time   `json:"detected_at"`
	ResolvedAt    time.Time   `json:"resolved_at,omitempty"`
	ResolverID    string      `json:"resolver_id,omitempty"`
}

// Trade represents a trade event consumed from the matching engine.
// This is the input to the surveillance detector.
type Trade struct {
	TradeID      string    `json:"trade_id"`
	InstrumentID string    `json:"instrument_id"`
	BuyerID      string    `json:"buyer_id"`
	SellerID     string    `json:"seller_id"`
	Price        float64   `json:"price"`
	Quantity     float64   `json:"quantity"`
	Timestamp    time.Time `json:"timestamp"`
}

// OrderCancel represents an order cancellation event.
// Used for spoofing detection.
type OrderCancel struct {
	OrderID      string    `json:"order_id"`
	InstrumentID string    `json:"instrument_id"`
	ParticipantID string  `json:"participant_id"`
	Side         string    `json:"side"` // BUY or SELL
	Price        float64   `json:"price"`
	Quantity     float64   `json:"quantity"`
	SubmittedAt  time.Time `json:"submitted_at"`
	CancelledAt  time.Time `json:"cancelled_at"`
}

// VolumeSnapshot holds aggregated volume data for a given instrument and period.
// Used by the unusual volume detector.
type VolumeSnapshot struct {
	InstrumentID string  `json:"instrument_id"`
	DailyVolume  float64 `json:"daily_volume"`
	RollingMean  float64 `json:"rolling_mean"`
	RollingStdDev float64 `json:"rolling_std_dev"`
}

// PositionSnapshot holds current position data for concentration checks.
type PositionSnapshot struct {
	ParticipantID string  `json:"participant_id"`
	InstrumentID  string  `json:"instrument_id"`
	PositionSize  float64 `json:"position_size"`
	TotalMarketSize float64 `json:"total_market_size"`
}
