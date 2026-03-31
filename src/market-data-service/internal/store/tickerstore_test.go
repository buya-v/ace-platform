package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

func TestTickerStore_UpsertAndGet(t *testing.T) {
	s := NewTickerStore()

	// Get from empty store
	_, ok := s.Get("inst-1")
	if ok {
		t.Error("expected not found for empty store")
	}

	// Upsert a ticker
	tick := types.Ticker{
		InstrumentID: "inst-1",
		Symbol:       "WHT-HRW",
		LastPrice:    mustDecimal("105.50"),
		Volume24h:    1000,
		High24h:      mustDecimal("110"),
		Low24h:       mustDecimal("100"),
		Timestamp:    time.Now().UTC(),
	}
	s.Upsert(tick)

	got, ok := s.Get("inst-1")
	if !ok {
		t.Fatal("expected ticker to be found")
	}
	if got.Symbol != "WHT-HRW" {
		t.Errorf("symbol = %s, want WHT-HRW", got.Symbol)
	}
	if !got.LastPrice.Equal(mustDecimal("105.50")) {
		t.Errorf("last_price = %s, want 105.50", got.LastPrice)
	}

	// Upsert overwrites
	tick.LastPrice = mustDecimal("108")
	s.Upsert(tick)
	got, _ = s.Get("inst-1")
	if !got.LastPrice.Equal(mustDecimal("108")) {
		t.Errorf("after upsert, last_price = %s, want 108", got.LastPrice)
	}
}

func TestTickerStore_GetAll(t *testing.T) {
	s := NewTickerStore()
	now := time.Now().UTC()

	s.Upsert(types.Ticker{InstrumentID: "inst-1", LastPrice: mustDecimal("100"), Timestamp: now})
	s.Upsert(types.Ticker{InstrumentID: "inst-2", LastPrice: mustDecimal("200"), Timestamp: now})
	s.Upsert(types.Ticker{InstrumentID: "inst-3", LastPrice: mustDecimal("300"), Timestamp: now})

	// Get all
	all := s.GetAll(nil)
	if len(all) != 3 {
		t.Errorf("expected 3 tickers, got %d", len(all))
	}

	// Get specific
	subset := s.GetAll([]string{"inst-1", "inst-3"})
	if len(subset) != 2 {
		t.Errorf("expected 2 tickers, got %d", len(subset))
	}

	// Get with unknown ID
	unknown := s.GetAll([]string{"inst-99"})
	if len(unknown) != 0 {
		t.Errorf("expected 0 tickers for unknown, got %d", len(unknown))
	}
}

func TestTickerStore_GetAll_Empty(t *testing.T) {
	s := NewTickerStore()
	all := s.GetAll(nil)
	if len(all) != 0 {
		t.Errorf("expected 0 tickers, got %d", len(all))
	}
}
