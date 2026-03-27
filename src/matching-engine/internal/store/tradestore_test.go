package store

import (
	"testing"

	"github.com/ace-platform/matching-engine/internal/types"
)

func TestInMemoryTradeStoreAppendOnly(t *testing.T) {
	store := NewInMemoryTradeStore()

	trade1 := types.Trade{
		TradeID:        "t1",
		InstrumentID:   "WHEAT",
		BuyOrderID:     "b1",
		SellOrderID:    "s1",
		Price:          types.DecimalFromInt(100),
		Quantity:       10,
		SequenceNumber: 1,
	}
	trade2 := types.Trade{
		TradeID:        "t2",
		InstrumentID:   "WHEAT",
		BuyOrderID:     "b2",
		SellOrderID:    "s2",
		Price:          types.DecimalFromInt(101),
		Quantity:       5,
		SequenceNumber: 2,
	}

	if err := store.Append(trade1); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(trade2); err != nil {
		t.Fatal(err)
	}

	trades := store.Trades("WHEAT")
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	if trades[0].TradeID != "t1" {
		t.Errorf("expected first trade t1, got %s", trades[0].TradeID)
	}
	if trades[1].TradeID != "t2" {
		t.Errorf("expected second trade t2, got %s", trades[1].TradeID)
	}
}

func TestInMemoryTradeStoreBySequence(t *testing.T) {
	store := NewInMemoryTradeStore()

	for i := uint64(1); i <= 5; i++ {
		store.Append(types.Trade{
			TradeID:        "t" + string(rune('0'+i)),
			InstrumentID:   "WHEAT",
			SequenceNumber: i,
		})
	}

	trades := store.TradesBySequence("WHEAT", 3)
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades since seq 3, got %d", len(trades))
	}
	if trades[0].SequenceNumber != 4 {
		t.Errorf("expected seq 4, got %d", trades[0].SequenceNumber)
	}
}

func TestInMemoryTradeStoreLastTrade(t *testing.T) {
	store := NewInMemoryTradeStore()

	_, ok := store.LastTrade("WHEAT")
	if ok {
		t.Error("expected no last trade for empty store")
	}

	store.Append(types.Trade{TradeID: "t1", InstrumentID: "WHEAT", SequenceNumber: 1})
	store.Append(types.Trade{TradeID: "t2", InstrumentID: "WHEAT", SequenceNumber: 2})

	last, ok := store.LastTrade("WHEAT")
	if !ok {
		t.Fatal("expected last trade")
	}
	if last.TradeID != "t2" {
		t.Errorf("expected last trade t2, got %s", last.TradeID)
	}
}

func TestInMemoryTradeStoreIsolation(t *testing.T) {
	store := NewInMemoryTradeStore()

	store.Append(types.Trade{TradeID: "t1", InstrumentID: "WHEAT", SequenceNumber: 1})
	store.Append(types.Trade{TradeID: "t2", InstrumentID: "CORN", SequenceNumber: 1})

	wheat := store.Trades("WHEAT")
	corn := store.Trades("CORN")

	if len(wheat) != 1 || wheat[0].TradeID != "t1" {
		t.Error("WHEAT should have only t1")
	}
	if len(corn) != 1 || corn[0].TradeID != "t2" {
		t.Error("CORN should have only t2")
	}
}
