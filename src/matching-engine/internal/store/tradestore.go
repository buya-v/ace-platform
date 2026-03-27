package store

import (
	"sync"

	"github.com/ace-platform/matching-engine/internal/types"
)

// TradeStore provides append-only storage for trades.
// In production, this would write to PostgreSQL or Kafka.
type TradeStore interface {
	// Append persists a trade. Must be append-only — no updates or deletes.
	Append(trade types.Trade) error
	// Trades returns all trades for an instrument (for recovery/replay).
	Trades(instrumentID string) []types.Trade
	// TradesBySequence returns trades since a given sequence number.
	TradesBySequence(instrumentID string, sinceSequence uint64) []types.Trade
	// LastTrade returns the most recent trade for an instrument.
	LastTrade(instrumentID string) (types.Trade, bool)
}

// InMemoryTradeStore is an append-only in-memory trade store for development/testing.
type InMemoryTradeStore struct {
	mu     sync.RWMutex
	trades map[string][]types.Trade // instrumentID -> trades
}

// NewInMemoryTradeStore creates a new in-memory trade store.
func NewInMemoryTradeStore() *InMemoryTradeStore {
	return &InMemoryTradeStore{
		trades: make(map[string][]types.Trade),
	}
}

func (s *InMemoryTradeStore) Append(trade types.Trade) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trades[trade.InstrumentID] = append(s.trades[trade.InstrumentID], trade)
	return nil
}

func (s *InMemoryTradeStore) Trades(instrumentID string) []types.Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.trades[instrumentID]
	out := make([]types.Trade, len(src))
	copy(out, src)
	return out
}

func (s *InMemoryTradeStore) TradesBySequence(instrumentID string, sinceSequence uint64) []types.Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []types.Trade
	for _, t := range s.trades[instrumentID] {
		if t.SequenceNumber > sinceSequence {
			result = append(result, t)
		}
	}
	return result
}

func (s *InMemoryTradeStore) LastTrade(instrumentID string) (types.Trade, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	trades := s.trades[instrumentID]
	if len(trades) == 0 {
		return types.Trade{}, false
	}
	return trades[len(trades)-1], true
}

func (s *InMemoryTradeStore) Len(instrumentID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.trades[instrumentID])
}
