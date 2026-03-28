package store

import (
	"testing"
	"time"

	"github.com/ace-platform/market-data-service/internal/types"
)

func mustDecimal(s string) types.Decimal {
	d, _ := types.ParseDecimal(s)
	return d
}

func makeTrade(id, instrumentID, price string, seq uint64, execAt time.Time) types.Trade {
	return types.Trade{
		TradeID:        id,
		InstrumentID:   instrumentID,
		Price:          mustDecimal(price),
		Quantity:       1,
		SequenceNumber: seq,
		ExecutedAt:     execAt,
	}
}

func TestTradeStore_LastN(t *testing.T) {
	s := NewTradeStore()
	now := time.Now()

	s.Append(makeTrade("t1", "inst-1", "100", 1, now))
	s.Append(makeTrade("t2", "inst-1", "101", 2, now.Add(time.Second)))
	s.Append(makeTrade("t3", "inst-1", "102", 3, now.Add(2*time.Second)))

	trades := s.LastN("inst-1", 2)
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	// Newest first
	if trades[0].TradeID != "t3" {
		t.Errorf("first trade = %s, want t3", trades[0].TradeID)
	}
	if trades[1].TradeID != "t2" {
		t.Errorf("second trade = %s, want t2", trades[1].TradeID)
	}
}

func TestTradeStore_LastN_Empty(t *testing.T) {
	s := NewTradeStore()
	trades := s.LastN("inst-1", 5)
	if trades != nil {
		t.Errorf("expected nil, got %d trades", len(trades))
	}
}

func TestTradeStore_SinceSequence(t *testing.T) {
	s := NewTradeStore()
	now := time.Now()

	s.Append(makeTrade("t1", "inst-1", "100", 1, now))
	s.Append(makeTrade("t2", "inst-1", "101", 2, now))
	s.Append(makeTrade("t3", "inst-1", "102", 3, now))

	trades := s.SinceSequence("inst-1", 1)
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].SequenceNumber != 2 {
		t.Errorf("first seq = %d, want 2", trades[0].SequenceNumber)
	}
}

func TestTradeStore_InTimeRange(t *testing.T) {
	s := NewTradeStore()
	base := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)

	s.Append(makeTrade("t1", "inst-1", "100", 1, base))
	s.Append(makeTrade("t2", "inst-1", "101", 2, base.Add(30*time.Second)))
	s.Append(makeTrade("t3", "inst-1", "102", 3, base.Add(90*time.Second)))

	trades := s.InTimeRange("inst-1", base, base.Add(60*time.Second), 0)
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades in range, got %d", len(trades))
	}
}

func TestTradeStore_LastTrade(t *testing.T) {
	s := NewTradeStore()
	now := time.Now()

	_, ok := s.LastTrade("inst-1")
	if ok {
		t.Error("expected not found for empty store")
	}

	s.Append(makeTrade("t1", "inst-1", "100", 1, now))
	s.Append(makeTrade("t2", "inst-1", "101", 2, now))

	trade, ok := s.LastTrade("inst-1")
	if !ok {
		t.Fatal("expected trade")
	}
	if trade.TradeID != "t2" {
		t.Errorf("last trade = %s, want t2", trade.TradeID)
	}
}

func TestTradeStore_AllInstruments(t *testing.T) {
	s := NewTradeStore()
	now := time.Now()

	s.Append(makeTrade("t1", "inst-1", "100", 1, now))
	s.Append(makeTrade("t2", "inst-2", "200", 1, now))

	ids := s.AllInstruments()
	if len(ids) != 2 {
		t.Errorf("expected 2 instruments, got %d", len(ids))
	}
}
