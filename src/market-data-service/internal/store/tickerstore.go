package store

import (
	"sync"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// TickerStore provides in-memory storage for ticker summaries.
type TickerStore struct {
	mu      sync.RWMutex
	tickers map[string]types.Ticker
}

// NewTickerStore creates a new in-memory ticker store.
func NewTickerStore() *TickerStore {
	return &TickerStore{
		tickers: make(map[string]types.Ticker),
	}
}

// Upsert inserts or updates a ticker for an instrument.
func (s *TickerStore) Upsert(t types.Ticker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickers[t.InstrumentID] = t
}

// Get returns the ticker for an instrument.
func (s *TickerStore) Get(instrumentID string) (types.Ticker, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tickers[instrumentID]
	return t, ok
}

// GetAll returns tickers for the specified instruments. If instrumentIDs is empty,
// returns all tickers.
func (s *TickerStore) GetAll(instrumentIDs []string) []types.Ticker {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(instrumentIDs) == 0 {
		result := make([]types.Ticker, 0, len(s.tickers))
		for _, t := range s.tickers {
			result = append(result, t)
		}
		return result
	}

	var result []types.Ticker
	for _, id := range instrumentIDs {
		if t, ok := s.tickers[id]; ok {
			result = append(result, t)
		}
	}
	return result
}
