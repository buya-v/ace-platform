// Package store provides in-memory storage for trades and candles.
package store

import (
	"sync"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

// TradeStore provides append-only in-memory storage for trades.
type TradeStore struct {
	mu     sync.RWMutex
	trades map[string][]types.Trade // instrumentID -> trades (ordered by sequence)
}

// NewTradeStore creates a new in-memory trade store.
func NewTradeStore() *TradeStore {
	return &TradeStore{
		trades: make(map[string][]types.Trade),
	}
}

// Append adds a trade to the store.
func (s *TradeStore) Append(trade types.Trade) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trades[trade.InstrumentID] = append(s.trades[trade.InstrumentID], trade)
}

// LastN returns the last N trades for an instrument, newest first.
func (s *TradeStore) LastN(instrumentID string, n int) []types.Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := s.trades[instrumentID]
	if len(all) == 0 {
		return nil
	}
	if n <= 0 || n > len(all) {
		n = len(all)
	}

	// Return newest first
	result := make([]types.Trade, n)
	for i := 0; i < n; i++ {
		result[i] = all[len(all)-1-i]
	}
	return result
}

// SinceSequence returns trades with sequence number > sinceSequence.
func (s *TradeStore) SinceSequence(instrumentID string, sinceSequence uint64) []types.Trade {
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

// InTimeRange returns trades in [start, end) for an instrument.
func (s *TradeStore) InTimeRange(instrumentID string, start, end time.Time, limit int) []types.Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []types.Trade
	for _, t := range s.trades[instrumentID] {
		if !t.ExecutedAt.Before(start) && t.ExecutedAt.Before(end) {
			result = append(result, t)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

// LastTrade returns the most recent trade for an instrument.
func (s *TradeStore) LastTrade(instrumentID string) (types.Trade, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trades := s.trades[instrumentID]
	if len(trades) == 0 {
		return types.Trade{}, false
	}
	return trades[len(trades)-1], true
}

// AllInstruments returns all instrument IDs that have trades.
func (s *TradeStore) AllInstruments() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.trades))
	for id := range s.trades {
		ids = append(ids, id)
	}
	return ids
}

// Len returns the number of trades for an instrument.
func (s *TradeStore) Len(instrumentID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.trades[instrumentID])
}
