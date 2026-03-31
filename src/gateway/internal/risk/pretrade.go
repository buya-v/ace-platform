package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// OrderRequest represents the fields from an incoming order needed for risk checks.
type OrderRequest struct {
	InstrumentID  string  `json:"instrument_id"`
	Side          string  `json:"side"`
	Price         float64 `json:"-"` // parsed from string
	PriceStr      string  `json:"price"`
	Quantity      float64 `json:"-"` // parsed from numeric or string
	QuantityRaw   json.RawMessage `json:"quantity"`
	ParticipantID string  `json:"participant_id,omitempty"`
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
		p, err := strconv.ParseFloat(raw.Price, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid price %q: %w", raw.Price, err)
		}
		or.Price = p
	}

	// Parse quantity (can be number or string)
	if len(raw.Quantity) > 0 {
		// Try as number first
		var qf float64
		if err := json.Unmarshal(raw.Quantity, &qf); err != nil {
			// Try as string
			var qs string
			if err2 := json.Unmarshal(raw.Quantity, &qs); err2 != nil {
				return nil, fmt.Errorf("invalid quantity: %w", err)
			}
			qf2, err3 := strconv.ParseFloat(qs, 64)
			if err3 != nil {
				return nil, fmt.Errorf("invalid quantity string %q: %w", qs, err3)
			}
			qf = qf2
		}
		or.Quantity = qf
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
func (c *PreTradeChecker) CheckOrder(ctx context.Context, order *OrderRequest, lastPrice float64) *RiskError {
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
	if order.Quantity > limits.MaxOrderQty {
		return &RiskError{
			Code:    "ORDER_QTY_EXCEEDED",
			Message: fmt.Sprintf("Order quantity %.4f exceeds maximum %.4f for %s", order.Quantity, limits.MaxOrderQty, order.InstrumentID),
			Field:   "quantity",
		}
	}

	// Check notional value (price * quantity)
	if order.Price > 0 {
		notional := order.Price * order.Quantity
		if notional > limits.MaxOrderValue {
			return &RiskError{
				Code:    "ORDER_VALUE_EXCEEDED",
				Message: fmt.Sprintf("Order value %.2f exceeds maximum %.2f for %s", notional, limits.MaxOrderValue, order.InstrumentID),
				Field:   "price",
			}
		}
	}

	return nil
}

// CheckPriceBand validates that the order price is within the allowed price band
// relative to the last traded price.
func CheckPriceBand(order *OrderRequest, lastPrice float64, limits *OrderLimits) *RiskError {
	// Skip price band check if no last price available (e.g., first trade of the day)
	if lastPrice <= 0 {
		return nil
	}

	// Skip for market orders (price == 0)
	if order.Price <= 0 {
		return nil
	}

	deviation := math.Abs(order.Price-lastPrice) / lastPrice * 100.0
	if deviation > limits.PriceBandPct {
		return &RiskError{
			Code:    "PRICE_BAND_EXCEEDED",
			Message: fmt.Sprintf("Order price %.4f deviates %.2f%% from last price %.4f, exceeding %.2f%% band", order.Price, deviation, lastPrice, limits.PriceBandPct),
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
