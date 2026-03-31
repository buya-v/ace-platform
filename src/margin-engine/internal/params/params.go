package params

import (
	"fmt"
	"sync"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// PriceScenario defines a single SPAN price scan point.
// SPAN evaluates the portfolio under multiple hypothetical price/volatility moves.
type PriceScenario struct {
	PriceMove      types.Decimal // Absolute price change (can be negative)
	VolatilityMove types.Decimal // Volatility shift (basis points, can be negative)
	Weight         types.Decimal // Scenario weight (typically 1.0, extreme scenarios may be fractional)
}

// InstrumentParams holds SPAN risk parameters for a single commodity instrument.
type InstrumentParams struct {
	InstrumentID    string
	PriceScanRange  types.Decimal    // Maximum expected price move (e.g., $3.00 for corn)
	VolScanRange    types.Decimal    // Maximum expected volatility shift
	SpotPrice       types.Decimal    // Current settlement/mark price
	ContractSize    int64            // Contract multiplier (e.g., 5000 bushels for corn)
	DeliveryCharge  types.Decimal    // Additional charge rate for delivery-month contracts
	IsDeliveryMonth bool             // Whether this instrument is in delivery month
	Scenarios       []PriceScenario  // Scan scenarios (typically 16 for SPAN)
}

// DefaultScenarios returns the standard 16 SPAN scanning scenarios.
// These scan 1/3, 2/3, and 3/3 of the price scan range at up/down volatility,
// plus two extreme moves at reduced weight.
func DefaultScenarios(priceScanRange, volScanRange types.Decimal) []PriceScenario {
	one := types.DecimalFromInt(1)
	third := types.NewDecimal(0, 3333) // 0.3333
	twoThirds := types.NewDecimal(0, 6667) // 0.6667
	extreme := types.NewDecimal(0, 3000)   // 0.30 weight for extreme scenarios

	// Price fractions: 0, 1/3, 2/3, 1 of scan range (up and down)
	// Volatility: up and down
	// Total: 4 price levels × 2 directions × 2 vol states = 16 scenarios
	// Plus 2 extreme moves (3x scan range at reduced weight) -- we'll use 16 standard here

	pZero := types.DecimalZero()
	pThird := priceScanRange.MulDecimal(third)
	pTwoThird := priceScanRange.MulDecimal(twoThirds)
	pFull := priceScanRange

	volUp := volScanRange
	volDown := volScanRange.Negate()

	scenarios := []PriceScenario{
		// No price move, vol up/down
		{PriceMove: pZero, VolatilityMove: volUp, Weight: one},
		{PriceMove: pZero, VolatilityMove: volDown, Weight: one},
		// 1/3 price up/down, vol up/down
		{PriceMove: pThird, VolatilityMove: volUp, Weight: one},
		{PriceMove: pThird, VolatilityMove: volDown, Weight: one},
		{PriceMove: pThird.Negate(), VolatilityMove: volUp, Weight: one},
		{PriceMove: pThird.Negate(), VolatilityMove: volDown, Weight: one},
		// 2/3 price up/down, vol up/down
		{PriceMove: pTwoThird, VolatilityMove: volUp, Weight: one},
		{PriceMove: pTwoThird, VolatilityMove: volDown, Weight: one},
		{PriceMove: pTwoThird.Negate(), VolatilityMove: volUp, Weight: one},
		{PriceMove: pTwoThird.Negate(), VolatilityMove: volDown, Weight: one},
		// Full price up/down, vol up/down
		{PriceMove: pFull, VolatilityMove: volUp, Weight: one},
		{PriceMove: pFull, VolatilityMove: volDown, Weight: one},
		{PriceMove: pFull.Negate(), VolatilityMove: volUp, Weight: one},
		{PriceMove: pFull.Negate(), VolatilityMove: volDown, Weight: one},
		// Extreme moves (3x scan range up/down) at reduced weight
		{PriceMove: priceScanRange.MulInt64(3), VolatilityMove: pZero, Weight: extreme},
		{PriceMove: priceScanRange.MulInt64(3).Negate(), VolatilityMove: pZero, Weight: extreme},
	}

	return scenarios
}

// Store holds risk parameters for all instruments.
type Store struct {
	mu     sync.RWMutex
	params map[string]*InstrumentParams
}

func NewStore() *Store {
	return &Store{
		params: make(map[string]*InstrumentParams),
	}
}

// Set upserts risk parameters for an instrument. If scenarios are nil,
// default SPAN scenarios are generated from the scan ranges.
func (s *Store) Set(p InstrumentParams) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(p.Scenarios) == 0 {
		p.Scenarios = DefaultScenarios(p.PriceScanRange, p.VolScanRange)
	}
	stored := p
	s.params[p.InstrumentID] = &stored
}

// Get returns parameters for an instrument.
func (s *Store) Get(instrumentID string) (InstrumentParams, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.params[instrumentID]
	if !ok {
		return InstrumentParams{}, fmt.Errorf("params: no risk parameters for instrument %s", instrumentID)
	}
	return *p, nil
}

// UpdateSpotPrice updates the mark/settlement price for an instrument.
func (s *Store) UpdateSpotPrice(instrumentID string, price types.Decimal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.params[instrumentID]
	if !ok {
		return fmt.Errorf("params: no risk parameters for instrument %s", instrumentID)
	}
	p.SpotPrice = price
	return nil
}

// GetSpotPrice returns the current spot/settlement price for an instrument or commodity.
// Returns (price, true) if found, or (zero, false) if the instrument is not configured.
func (s *Store) GetSpotPrice(instrumentID string) (types.Decimal, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.params[instrumentID]
	if !ok {
		return types.DecimalZero(), false
	}
	return p.SpotPrice, true
}

// All returns all instrument parameters.
func (s *Store) All() []InstrumentParams {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]InstrumentParams, 0, len(s.params))
	for _, p := range s.params {
		result = append(result, *p)
	}
	return result
}
