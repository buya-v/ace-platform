package valuation

import (
	"fmt"
	"sync"
	"time"

	"github.com/ace-platform/settlement-engine/internal/types"
)

// PriceSource provides settlement prices for instruments.
type PriceSource interface {
	GetSettlementPrice(instrumentID string) (types.Decimal, error)
}

// Store manages settlement prices across settlement dates.
type Store struct {
	mu     sync.RWMutex
	prices map[priceKey]types.SettlementPrice
}

type priceKey struct {
	InstrumentID string
	Date         string // YYYY-MM-DD
}

func NewStore() *Store {
	return &Store{
		prices: make(map[priceKey]types.SettlementPrice),
	}
}

// SetSettlementPrice records the settlement price for an instrument on a given date.
func (s *Store) SetSettlementPrice(instrumentID string, date time.Time, price types.Decimal) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dateStr := date.Format("2006-01-02")
	key := priceKey{InstrumentID: instrumentID, Date: dateStr}

	sp := types.SettlementPrice{
		InstrumentID:    instrumentID,
		SettleDate:      date,
		SettlementPrice: price,
	}

	// Look up previous day's price
	prevDate := date.AddDate(0, 0, -1)
	prevKey := priceKey{InstrumentID: instrumentID, Date: prevDate.Format("2006-01-02")}
	if prev, ok := s.prices[prevKey]; ok {
		sp.PreviousPrice = prev.SettlementPrice
	}

	s.prices[key] = sp
}

// GetSettlementPrice returns the settlement price for an instrument on a given date.
func (s *Store) GetSettlementPrice(instrumentID string, date time.Time) (types.SettlementPrice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dateStr := date.Format("2006-01-02")
	key := priceKey{InstrumentID: instrumentID, Date: dateStr}
	sp, ok := s.prices[key]
	if !ok {
		return types.SettlementPrice{}, fmt.Errorf("no settlement price for %s on %s", instrumentID, dateStr)
	}
	return sp, nil
}

// HasPreviousPrice returns true if there is a settlement price for the prior day.
func (s *Store) HasPreviousPrice(instrumentID string, date time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prevDate := date.AddDate(0, 0, -1)
	prevKey := priceKey{InstrumentID: instrumentID, Date: prevDate.Format("2006-01-02")}
	_, ok := s.prices[prevKey]
	return ok
}

// ValuePosition calculates the mark-to-market value of a position.
// Returns (markPrice * abs(netQuantity)), the total notional exposure.
func ValuePosition(pos types.Position, markPrice types.Decimal) types.Decimal {
	qty := pos.NetQuantity
	if qty < 0 {
		qty = -qty
	}
	return markPrice.MulInt64(qty)
}
