package store

import (
	"testing"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

func TestCandleStore_StoreAndQuery(t *testing.T) {
	s := NewCandleStore()
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		s.Store(types.Candle{
			InstrumentID: "inst-1",
			Interval:     types.Interval1m,
			Bucket:       base.Add(time.Duration(i) * time.Minute),
			Open:         mustDecimal("100"),
			IsClosed:     true,
		})
	}

	// Query all
	candles := s.Query("inst-1", types.Interval1m, base, base.Add(5*time.Minute), 0)
	if len(candles) != 5 {
		t.Errorf("expected 5 candles, got %d", len(candles))
	}

	// Query with limit
	candles = s.Query("inst-1", types.Interval1m, base, base.Add(5*time.Minute), 3)
	if len(candles) != 3 {
		t.Errorf("expected 3 candles, got %d", len(candles))
	}

	// Query subset
	candles = s.Query("inst-1", types.Interval1m, base.Add(2*time.Minute), base.Add(4*time.Minute), 0)
	if len(candles) != 2 {
		t.Errorf("expected 2 candles, got %d", len(candles))
	}
}

func TestCandleStore_QueryEmpty(t *testing.T) {
	s := NewCandleStore()
	candles := s.Query("inst-1", types.Interval1m, time.Now(), time.Now().Add(time.Hour), 0)
	if len(candles) != 0 {
		t.Errorf("expected 0 candles, got %d", len(candles))
	}
}

func TestCandleStore_DeleteBefore(t *testing.T) {
	s := NewCandleStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Store candles over 3 months
	for i := 0; i < 90; i++ {
		s.Store(types.Candle{
			InstrumentID: "inst-1",
			Interval:     types.Interval1d,
			Bucket:       base.Add(time.Duration(i) * 24 * time.Hour),
			Open:         mustDecimal("100"),
			IsClosed:     true,
		})
	}

	// Delete candles older than 30 days
	cutoff := base.Add(30 * 24 * time.Hour)
	deleted := s.DeleteBefore(types.Interval1d, cutoff)
	if deleted != 30 {
		t.Errorf("expected 30 deleted, got %d", deleted)
	}

	remaining := s.Query("inst-1", types.Interval1d, base, base.Add(90*24*time.Hour), 0)
	if len(remaining) != 60 {
		t.Errorf("expected 60 remaining, got %d", len(remaining))
	}
}

func TestCandleStore_Upsert(t *testing.T) {
	s := NewCandleStore()
	bucket := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	s.Store(types.Candle{
		InstrumentID: "inst-1",
		Interval:     types.Interval1m,
		Bucket:       bucket,
		Open:         mustDecimal("100"),
		Close:        mustDecimal("101"),
	})

	// Upsert with new close
	s.Store(types.Candle{
		InstrumentID: "inst-1",
		Interval:     types.Interval1m,
		Bucket:       bucket,
		Open:         mustDecimal("100"),
		Close:        mustDecimal("105"),
	})

	candles := s.Query("inst-1", types.Interval1m, bucket, bucket.Add(time.Minute), 0)
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles))
	}
	if !candles[0].Close.Equal(mustDecimal("105")) {
		t.Errorf("close = %s, want 105", candles[0].Close)
	}
}
