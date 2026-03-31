package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// TestTradeRepository_InMemory tests the in-memory TradeStore through the TradeRepository interface.
func TestTradeRepository_InMemory(t *testing.T) {
	var repo TradeRepository = NewTradeStore()
	now := time.Now().UTC()

	repo.Append(makeTrade("t1", "inst-1", "100", 1, now))
	repo.Append(makeTrade("t2", "inst-1", "101", 2, now.Add(time.Second)))
	repo.Append(makeTrade("t3", "inst-1", "102", 3, now.Add(2*time.Second)))

	// LastN
	trades := repo.LastN("inst-1", 2)
	if len(trades) != 2 {
		t.Fatalf("LastN: expected 2, got %d", len(trades))
	}
	if trades[0].TradeID != "t3" {
		t.Errorf("LastN: first = %s, want t3", trades[0].TradeID)
	}

	// SinceSequence
	trades = repo.SinceSequence("inst-1", 1)
	if len(trades) != 2 {
		t.Fatalf("SinceSequence: expected 2, got %d", len(trades))
	}

	// InTimeRange
	trades = repo.InTimeRange("inst-1", now, now.Add(2*time.Second), 0)
	if len(trades) != 2 {
		t.Fatalf("InTimeRange: expected 2, got %d", len(trades))
	}

	// LastTrade
	last, ok := repo.LastTrade("inst-1")
	if !ok || last.TradeID != "t3" {
		t.Errorf("LastTrade: expected t3, got %s (ok=%v)", last.TradeID, ok)
	}

	// AllInstruments
	ids := repo.AllInstruments()
	if len(ids) != 1 {
		t.Errorf("AllInstruments: expected 1, got %d", len(ids))
	}

	// Len
	if repo.Len("inst-1") != 3 {
		t.Errorf("Len: expected 3, got %d", repo.Len("inst-1"))
	}
}

// TestCandleRepository_InMemory tests the in-memory CandleStore through the CandleRepository interface.
func TestCandleRepository_InMemory(t *testing.T) {
	var repo CandleRepository = NewCandleStore()
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		repo.Store(types.Candle{
			InstrumentID: "inst-1",
			Interval:     types.Interval1m,
			Bucket:       base.Add(time.Duration(i) * time.Minute),
			Open:         mustDecimal("100"),
			IsClosed:     true,
		})
	}

	candles := repo.Query("inst-1", types.Interval1m, base, base.Add(5*time.Minute), 0)
	if len(candles) != 5 {
		t.Errorf("Query: expected 5, got %d", len(candles))
	}

	deleted := repo.DeleteBefore(types.Interval1m, base.Add(3*time.Minute))
	if deleted != 3 {
		t.Errorf("DeleteBefore: expected 3 deleted, got %d", deleted)
	}
}

// TestTickerRepository_InMemory tests the in-memory TickerStore through the TickerRepository interface.
func TestTickerRepository_InMemory(t *testing.T) {
	var repo TickerRepository = NewTickerStore()

	_, ok := repo.Get("inst-1")
	if ok {
		t.Error("Get: expected not found")
	}

	repo.Upsert(types.Ticker{
		InstrumentID: "inst-1",
		LastPrice:    mustDecimal("100"),
	})
	repo.Upsert(types.Ticker{
		InstrumentID: "inst-2",
		LastPrice:    mustDecimal("200"),
	})

	tick, ok := repo.Get("inst-1")
	if !ok {
		t.Fatal("Get: expected found")
	}
	if !tick.LastPrice.Equal(mustDecimal("100")) {
		t.Errorf("Get: price = %s, want 100", tick.LastPrice)
	}

	all := repo.GetAll(nil)
	if len(all) != 2 {
		t.Errorf("GetAll: expected 2, got %d", len(all))
	}
}
