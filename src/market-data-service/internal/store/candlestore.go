package store

import (
	"sort"
	"sync"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

// candleKey uniquely identifies a stored candle.
type candleKey struct {
	InstrumentID string
	Interval     types.CandleInterval
	Bucket       time.Time
}

// CandleStore provides in-memory storage for historical (closed) candles.
// In production this would be backed by TimescaleDB continuous aggregates.
type CandleStore struct {
	mu      sync.RWMutex
	candles map[candleKey]types.Candle
}

// NewCandleStore creates a new candle store.
func NewCandleStore() *CandleStore {
	return &CandleStore{
		candles: make(map[candleKey]types.Candle),
	}
}

// Store persists a candle (upsert by key).
func (s *CandleStore) Store(c types.Candle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := candleKey{
		InstrumentID: c.InstrumentID,
		Interval:     c.Interval,
		Bucket:       c.Bucket,
	}
	s.candles[key] = c
}

// Query returns candles for an instrument and interval within [start, end), ordered by bucket ascending.
func (s *CandleStore) Query(instrumentID string, interval types.CandleInterval, start, end time.Time, limit int) []types.Candle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 500
	}

	var result []types.Candle
	for key, c := range s.candles {
		if key.InstrumentID != instrumentID || key.Interval != interval {
			continue
		}
		if !key.Bucket.Before(start) && key.Bucket.Before(end) {
			result = append(result, c)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Bucket.Before(result[j].Bucket)
	})

	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// DeleteBefore removes candles older than the given time for a specific interval.
// Used for data retention policy enforcement.
func (s *CandleStore) DeleteBefore(interval types.CandleInterval, before time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for key := range s.candles {
		if key.Interval == interval && key.Bucket.Before(before) {
			delete(s.candles, key)
			count++
		}
	}
	return count
}
