// Package engine — circuit breaker price validation for the matching engine.
package engine

import (
	"fmt"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// CircuitBreakerEngine validates order prices against configured circuit
// breaker thresholds and updates the last traded price after each trade.
type CircuitBreakerEngine struct {
	store store.CircuitBreakerStore
}

// NewCircuitBreakerEngine creates a new CircuitBreakerEngine backed by the
// given store. The store must not be nil.
func NewCircuitBreakerEngine(s store.CircuitBreakerStore) *CircuitBreakerEngine {
	return &CircuitBreakerEngine{store: s}
}

// ValidatePrice checks whether the given price for an instrument breaches any
// configured circuit breaker threshold. It returns:
//   - (true, nil, nil)  if the price is allowed (no config or no breach)
//   - (false, event, nil) if the price triggers a circuit breaker
//   - (false, nil, err) on store errors
func (cb *CircuitBreakerEngine) ValidatePrice(instrumentID string, price float64) (bool, *types.CircuitBreakerEvent, error) {
	config, err := cb.store.Get(instrumentID)
	if err != nil {
		return false, nil, fmt.Errorf("circuit breaker store get: %w", err)
	}
	// No circuit breaker configured — allow.
	if config == nil {
		return true, nil, nil
	}

	// Already triggered — reject.
	if config.Status == types.CBTriggered {
		return false, &types.CircuitBreakerEvent{
			InstrumentID:   instrumentID,
			Type:           types.CBTriggered,
			TriggerPrice:   price,
			ReferencePrice: config.ReferencePrice,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	ref := config.ReferencePrice

	// Static upper breach.
	if config.StaticUpperPct > 0 && price > ref*(1+config.StaticUpperPct/100) {
		return false, &types.CircuitBreakerEvent{
			InstrumentID:   instrumentID,
			Type:           types.CBStaticUpper,
			TriggerPrice:   price,
			ReferencePrice: ref,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	// Static lower breach.
	if config.StaticLowerPct > 0 && price < ref*(1-config.StaticLowerPct/100) {
		return false, &types.CircuitBreakerEvent{
			InstrumentID:   instrumentID,
			Type:           types.CBStaticLower,
			TriggerPrice:   price,
			ReferencePrice: ref,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	last := config.LastTradedPrice

	// Dynamic upper breach.
	if last > 0 && config.DynamicUpperPct > 0 && price > last*(1+config.DynamicUpperPct/100) {
		return false, &types.CircuitBreakerEvent{
			InstrumentID:   instrumentID,
			Type:           types.CBDynamicUpper,
			TriggerPrice:   price,
			ReferencePrice: last,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	// Dynamic lower breach.
	if last > 0 && config.DynamicLowerPct > 0 && price < last*(1-config.DynamicLowerPct/100) {
		return false, &types.CircuitBreakerEvent{
			InstrumentID:   instrumentID,
			Type:           types.CBDynamicLower,
			TriggerPrice:   price,
			ReferencePrice: last,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		}, nil
	}

	return true, nil, nil
}

// OnTrade updates the last traded price in the circuit breaker store.
func (cb *CircuitBreakerEngine) OnTrade(instrumentID string, price float64) error {
	return cb.store.UpdateLastPrice(instrumentID, price)
}
