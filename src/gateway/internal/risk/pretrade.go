package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/garudax-platform/decimal"
)

// OrderRequest represents the fields from an incoming order needed for risk checks.
//
// Price and Quantity are money/quantity values parsed from untrusted client
// input and use the shared fixed-point Decimal type rather than float64 (R020):
// order-value and price-band checks are money paths and must not drift.
type OrderRequest struct {
	InstrumentID  string          `json:"instrument_id"`
	Side          string          `json:"side"`
	Price         decimal.Decimal `json:"-"` // parsed from string
	PriceStr      string          `json:"price"`
	Quantity      decimal.Decimal `json:"-"` // parsed from numeric or string
	QuantityRaw   json.RawMessage `json:"quantity"`
	ParticipantID string          `json:"participant_id,omitempty"`
}

// ParseOrderRequest extracts and validates order fields from raw JSON.
func ParseOrderRequest(body json.RawMessage, participantID string) (*OrderRequest, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty order body")
	}

	var raw struct {
		InstrumentID string          `json:"instrument_id"`
		Side         string          `json:"side"`
		Price        string          `json:"price"`
		Quantity     json.RawMessage `json:"quantity"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid order JSON: %w", err)
	}

	or := &OrderRequest{
		InstrumentID:  raw.InstrumentID,
		Side:          raw.Side,
		PriceStr:      raw.Price,
		ParticipantID: participantID,
	}

	// Parse price (string in financial APIs)
	if raw.Price != "" {
		p, err := decimal.ParseDecimal(raw.Price)
		if err != nil {
			return nil, fmt.Errorf("invalid price %q: %w", raw.Price, err)
		}
		or.Price = p
	}

	// Parse quantity (can be a JSON number or a JSON string). Strip optional
	// surrounding quotes and parse through the precision-preserving decimal path.
	if len(raw.Quantity) > 0 {
		qstr := strings.Trim(strings.TrimSpace(string(raw.Quantity)), `"`)
		q, err := decimal.ParseDecimal(qstr)
		if err != nil {
			return nil, fmt.Errorf("invalid quantity %q: %w", qstr, err)
		}
		or.Quantity = q
	}

	return or, nil
}

// RiskError represents a pre-trade risk check failure.
type RiskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *RiskError) Error() string {
	return e.Message
}

// PreTradeChecker performs pre-trade risk validation.
type PreTradeChecker struct {
	store Store
}

// NewPreTradeChecker creates a new pre-trade risk checker.
// If store is nil, all checks are skipped (fail-open).
func NewPreTradeChecker(store Store) *PreTradeChecker {
	return &PreTradeChecker{store: store}
}

// Store returns the underlying risk store (may be nil).
func (c *PreTradeChecker) Store() Store {
	return c.store
}

// CheckOrder runs all pre-trade checks on an order.
// Returns nil if all checks pass. Returns a RiskError if any check fails.
// If the risk store is unavailable, checks are skipped (fail-open).
func (c *PreTradeChecker) CheckOrder(ctx context.Context, order *OrderRequest, lastPrice decimal.Decimal) *RiskError {
	if c.store == nil {
		return nil // fail-open: no store configured
	}

	limits, err := c.store.GetOrderLimits(ctx, order.InstrumentID)
	if err != nil {
		// DB error: fail-open
		return nil
	}

	if limits == nil {
		// No limits configured for this instrument: allow
		return nil
	}

	if rErr := CheckOrderSize(order, limits); rErr != nil {
		return rErr
	}

	if rErr := CheckPriceBand(order, lastPrice, limits); rErr != nil {
		return rErr
	}

	return nil
}

// CheckOrderSize validates order quantity and notional value against limits.
func CheckOrderSize(order *OrderRequest, limits *OrderLimits) *RiskError {
	if order.Quantity.GreaterThan(limits.MaxOrderQty) {
		return &RiskError{
			Code:    "ORDER_QTY_EXCEEDED",
			Message: fmt.Sprintf("Order quantity %s exceeds maximum %s for %s", order.Quantity.String(), limits.MaxOrderQty.String(), order.InstrumentID),
			Field:   "quantity",
		}
	}

	// Check notional value (price * quantity). A notional too large to even
	// represent in the fixed-point type necessarily exceeds any configured
	// limit, so treat an overflow as a value-exceeded rejection (fail-closed).
	if order.Price.IsPos() {
		notional, err := order.Price.TryMulDecimal(order.Quantity)
		if err != nil || notional.GreaterThan(limits.MaxOrderValue) {
			msg := fmt.Sprintf("Order value exceeds maximum %s for %s", limits.MaxOrderValue.String(), order.InstrumentID)
			if err == nil {
				msg = fmt.Sprintf("Order value %s exceeds maximum %s for %s", notional.String(), limits.MaxOrderValue.String(), order.InstrumentID)
			}
			return &RiskError{
				Code:    "ORDER_VALUE_EXCEEDED",
				Message: msg,
				Field:   "price",
			}
		}
	}

	return nil
}

// CheckPriceBand validates that the order price is within the allowed price band
// relative to the last traded price.
//
// The band decision avoids decimal/decimal division: since lastPrice > 0,
//
//	|price - lastPrice| / lastPrice * 100 > bandPct
//
// is equivalent to the exact decimal comparison
//
//	|price - lastPrice| * 100 > bandPct * lastPrice
//
// which is computed entirely in fixed point. The percentage shown in the error
// message is for display only and is the one place a float appears.
func CheckPriceBand(order *OrderRequest, lastPrice decimal.Decimal, limits *OrderLimits) *RiskError {
	// Skip price band check if no last price available (e.g., first trade of the day)
	if !lastPrice.IsPos() {
		return nil
	}

	// Skip for market orders (price == 0)
	if !order.Price.IsPos() {
		return nil
	}

	diff := order.Price.Sub(lastPrice).Abs()
	bandPct := decFromFloat(limits.PriceBandPct)

	lhs, err1 := diff.TryMulInt64(100)
	rhs, err2 := lastPrice.TryMulDecimal(bandPct)
	// Overflow here means astronomically large prices; reject conservatively.
	if err1 != nil || err2 != nil || lhs.GreaterThan(rhs) {
		deviation := diff.Float64() / lastPrice.Float64() * 100.0
		return &RiskError{
			Code:    "PRICE_BAND_EXCEEDED",
			Message: fmt.Sprintf("Order price %s deviates %.2f%% from last price %s, exceeding %.2f%% band", order.Price.String(), deviation, lastPrice.String(), limits.PriceBandPct),
			Field:   "price",
		}
	}

	return nil
}

// CheckParticipantStatus validates that the participant is not suspended.
// This is a simple lookup — in production, this would check against a participant status store.
func CheckParticipantStatus(participantID string, suspendedParticipants map[string]bool) *RiskError {
	if suspendedParticipants != nil && suspendedParticipants[participantID] {
		return &RiskError{
			Code:    "PARTICIPANT_SUSPENDED",
			Message: fmt.Sprintf("Participant %s is suspended from trading", participantID),
		}
	}
	return nil
}
