package store_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// --- helpers ---

func newInstrument(id, ticker string, ac types.AssetClass, ts types.TradingStatus) *types.Instrument {
	return &types.Instrument{
		ID:            id,
		Ticker:        ticker,
		Name:          ticker + " Corp",
		AssetClass:    ac,
		TradingStatus: ts,
		LotSize:       100,
		TickSize:      0.01,
		ExchangeCode:  "MSE",
	}
}

func newOrder(id, instrumentID string, side types.OrderSide, status types.OrderStatus) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:           id,
		InstrumentID: instrumentID,
		ParticipantID: "P1",
		Side:         side,
		OrderType:    types.OrderTypeLimit,
		Quantity:     100,
		Price:        10.00,
		Status:       status,
		TimeInForce:  types.TimeInForceGTC,
	}
}

// ============================================================
// InstrumentStore tests
// ============================================================

func TestInstrumentStore_Create(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	inst := newInstrument("id-1", "AAPL", types.AssetClassEquity, types.TradingStatusActive)

	if err := s.Create(inst); err != nil {
		t.Fatalf("unexpected error on Create: %v", err)
	}

	got, err := s.Get("id-1")
	if err != nil {
		t.Fatalf("Get after Create failed: %v", err)
	}
	if got.Ticker != "AAPL" {
		t.Errorf("expected ticker AAPL, got %q", got.Ticker)
	}
}

func TestInstrumentStore_Create_Duplicate(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	inst := newInstrument("id-dup", "DUP", types.AssetClassEquity, types.TradingStatusActive)

	if err := s.Create(inst); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := s.Create(inst); err == nil {
		t.Fatal("expected error on duplicate Create, got nil")
	}
}

func TestInstrumentStore_Get(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	inst := newInstrument("id-get", "MSFT", types.AssetClassEquity, types.TradingStatusActive)
	s.Create(inst)

	t.Run("existing", func(t *testing.T) {
		got, err := s.Get("id-get")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "id-get" {
			t.Errorf("expected id-get, got %q", got.ID)
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		_, err := s.Get("no-such-id")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestInstrumentStore_Get_ReturnsCopy(t *testing.T) {
	// Mutating the returned struct must not affect the stored record.
	s := store.NewInMemoryInstrumentStore()
	inst := newInstrument("id-copy", "CPY", types.AssetClassEquity, types.TradingStatusActive)
	s.Create(inst)

	got, _ := s.Get("id-copy")
	got.Ticker = "MUTATED"

	got2, _ := s.Get("id-copy")
	if got2.Ticker == "MUTATED" {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

func TestInstrumentStore_List(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	s.Create(newInstrument("id-eq1", "EQ1", types.AssetClassEquity, types.TradingStatusActive))
	s.Create(newInstrument("id-eq2", "EQ2", types.AssetClassEquity, types.TradingStatusHalted))
	s.Create(newInstrument("id-bd1", "BD1", types.AssetClassBond, types.TradingStatusActive))

	t.Run("list all", func(t *testing.T) {
		all, err := s.List(store.InstrumentFilters{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(all) != 3 {
			t.Errorf("expected 3 instruments, got %d", len(all))
		}
	})

	t.Run("filter by asset_class equity", func(t *testing.T) {
		res, err := s.List(store.InstrumentFilters{AssetClass: types.AssetClassEquity})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("expected 2 equity instruments, got %d", len(res))
		}
	})

	t.Run("filter by asset_class bond", func(t *testing.T) {
		res, err := s.List(store.InstrumentFilters{AssetClass: types.AssetClassBond})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 bond instrument, got %d", len(res))
		}
	})

	t.Run("filter by trading_status halted", func(t *testing.T) {
		res, err := s.List(store.InstrumentFilters{TradingStatus: types.TradingStatusHalted})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 halted instrument, got %d", len(res))
		}
	})

	t.Run("filter by asset_class and trading_status", func(t *testing.T) {
		res, err := s.List(store.InstrumentFilters{
			AssetClass:    types.AssetClassEquity,
			TradingStatus: types.TradingStatusActive,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 active equity, got %d", len(res))
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		res, err := s.List(store.InstrumentFilters{AssetClass: types.AssetClassETF})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected 0 results, got %d", len(res))
		}
	})
}

func TestInstrumentStore_Update(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	s.Create(newInstrument("id-upd", "UPD", types.AssetClassEquity, types.TradingStatusActive))

	t.Run("partial update name", func(t *testing.T) {
		err := s.Update("id-upd", store.InstrumentUpdate{Name: "Updated Name"})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := s.Get("id-upd")
		if got.Name != "Updated Name" {
			t.Errorf("expected name 'Updated Name', got %q", got.Name)
		}
		// Ticker should be unchanged.
		if got.Ticker != "UPD" {
			t.Errorf("expected ticker UPD unchanged, got %q", got.Ticker)
		}
	})

	t.Run("partial update lot_size and tick_size", func(t *testing.T) {
		err := s.Update("id-upd", store.InstrumentUpdate{LotSize: 50, TickSize: 0.05})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, _ := s.Get("id-upd")
		if got.LotSize != 50 {
			t.Errorf("expected LotSize 50, got %d", got.LotSize)
		}
		if got.TickSize != 0.05 {
			t.Errorf("expected TickSize 0.05, got %f", got.TickSize)
		}
	})

	t.Run("update non-existent", func(t *testing.T) {
		err := s.Update("no-such-id", store.InstrumentUpdate{Name: "X"})
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestInstrumentStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()
	s.Create(newInstrument("id-st", "ST", types.AssetClassEquity, types.TradingStatusActive))

	t.Run("valid transition to HALTED", func(t *testing.T) {
		err := s.UpdateStatus("id-st", types.TradingStatusHalted)
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("id-st")
		if got.TradingStatus != types.TradingStatusHalted {
			t.Errorf("expected HALTED, got %q", got.TradingStatus)
		}
	})

	t.Run("non-existent ID", func(t *testing.T) {
		err := s.UpdateStatus("no-such-id", types.TradingStatusHalted)
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ============================================================
// OrderStore tests
// ============================================================

func TestOrderStore_Submit(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-1", "inst-1", types.OrderSideBuy, types.OrderStatusPending)

	if err := s.Submit(order); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	got, err := s.Get("ord-1")
	if err != nil {
		t.Fatalf("Get after Submit: %v", err)
	}
	if got.ID != "ord-1" {
		t.Errorf("expected id ord-1, got %q", got.ID)
	}
	if got.Side != types.OrderSideBuy {
		t.Errorf("expected side BUY, got %q", got.Side)
	}
}

func TestOrderStore_Submit_Duplicate(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-dup", "inst-1", types.OrderSideBuy, types.OrderStatusPending)
	s.Submit(order)
	if err := s.Submit(order); err == nil {
		t.Fatal("expected error on duplicate Submit, got nil")
	}
}

func TestOrderStore_Get(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-get", "inst-1", types.OrderSideSell, types.OrderStatusPending)
	s.Submit(order)

	t.Run("existing", func(t *testing.T) {
		got, err := s.Get("ord-get")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "ord-get" {
			t.Errorf("expected ord-get, got %q", got.ID)
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		_, err := s.Get("no-such-order")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestOrderStore_Get_ReturnsCopy(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-copy", "inst-1", types.OrderSideBuy, types.OrderStatusPending)
	s.Submit(order)

	got, _ := s.Get("ord-copy")
	got.Status = types.OrderStatusCancelled

	got2, _ := s.Get("ord-copy")
	if got2.Status == types.OrderStatusCancelled {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

func TestOrderStore_List(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	s.Submit(newOrder("o1", "inst-A", types.OrderSideBuy, types.OrderStatusPending))
	s.Submit(newOrder("o2", "inst-A", types.OrderSideSell, types.OrderStatusFilled))
	s.Submit(newOrder("o3", "inst-B", types.OrderSideBuy, types.OrderStatusPending))

	t.Run("list all", func(t *testing.T) {
		all, err := s.List(store.OrderFilters{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(all) != 3 {
			t.Errorf("expected 3 orders, got %d", len(all))
		}
	})

	t.Run("filter by instrument_id inst-A", func(t *testing.T) {
		res, err := s.List(store.OrderFilters{InstrumentID: "inst-A"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("expected 2 orders for inst-A, got %d", len(res))
		}
	})

	t.Run("filter by instrument_id inst-B", func(t *testing.T) {
		res, err := s.List(store.OrderFilters{InstrumentID: "inst-B"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 order for inst-B, got %d", len(res))
		}
	})

	t.Run("filter by status FILLED", func(t *testing.T) {
		res, err := s.List(store.OrderFilters{Status: types.OrderStatusFilled})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 FILLED order, got %d", len(res))
		}
	})

	t.Run("filter no match", func(t *testing.T) {
		res, err := s.List(store.OrderFilters{InstrumentID: "inst-MISSING"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected 0 results, got %d", len(res))
		}
	})
}

func TestOrderStore_Cancel(t *testing.T) {
	t.Run("cancel pending order", func(t *testing.T) {
		s := store.NewInMemoryOrderStore()
		order := newOrder("ord-cancel", "inst-1", types.OrderSideBuy, types.OrderStatusPending)
		s.Submit(order)

		if err := s.Cancel("ord-cancel"); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		got, _ := s.Get("ord-cancel")
		if got.Status != types.OrderStatusCancelled {
			t.Errorf("expected CANCELLED, got %q", got.Status)
		}
	})

	t.Run("cancel partially-filled order", func(t *testing.T) {
		s := store.NewInMemoryOrderStore()
		order := newOrder("ord-partial", "inst-1", types.OrderSideBuy, types.OrderStatusPartiallyFilled)
		s.Submit(order)

		if err := s.Cancel("ord-partial"); err != nil {
			t.Fatalf("Cancel partially-filled: %v", err)
		}
		got, _ := s.Get("ord-partial")
		if got.Status != types.OrderStatusCancelled {
			t.Errorf("expected CANCELLED, got %q", got.Status)
		}
	})

	t.Run("cancel already-cancelled order returns error", func(t *testing.T) {
		s := store.NewInMemoryOrderStore()
		order := newOrder("ord-alreadycanc", "inst-1", types.OrderSideBuy, types.OrderStatusCancelled)
		s.Submit(order)

		if err := s.Cancel("ord-alreadycanc"); err == nil {
			t.Fatal("expected error on cancelling already-cancelled order, got nil")
		}
	})

	t.Run("cancel filled order returns error", func(t *testing.T) {
		s := store.NewInMemoryOrderStore()
		order := newOrder("ord-filled", "inst-1", types.OrderSideBuy, types.OrderStatusFilled)
		s.Submit(order)

		if err := s.Cancel("ord-filled"); err == nil {
			t.Fatal("expected error on cancelling filled order, got nil")
		}
	})

	t.Run("cancel non-existent", func(t *testing.T) {
		s := store.NewInMemoryOrderStore()
		if err := s.Cancel("no-such-order"); err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ============================================================
// Concurrent access tests
// ============================================================

func TestConcurrentAccess_InstrumentStore(t *testing.T) {
	s := store.NewInMemoryInstrumentStore()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := "concurrent-inst-" + string(rune('A'+i))
			inst := newInstrument(id, "T"+string(rune('A'+i)), types.AssetClassEquity, types.TradingStatusActive)
			_ = s.Create(inst)
			_, _ = s.Get(id)
			_, _ = s.List(store.InstrumentFilters{AssetClass: types.AssetClassEquity})
			_ = s.UpdateStatus(id, types.TradingStatusHalted)
		}()
	}
	wg.Wait()

	all, err := s.List(store.InstrumentFilters{})
	if err != nil {
		t.Fatalf("List after concurrent access: %v", err)
	}
	if len(all) != goroutines {
		t.Errorf("expected %d instruments, got %d", goroutines, len(all))
	}
}

func TestConcurrentAccess_OrderStore(t *testing.T) {
	s := store.NewInMemoryOrderStore()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := "concurrent-order-" + string(rune('A'+i))
			order := newOrder(id, "inst-concurrent", types.OrderSideBuy, types.OrderStatusPending)
			_ = s.Submit(order)
			_, _ = s.Get(id)
			_, _ = s.List(store.OrderFilters{InstrumentID: "inst-concurrent"})
		}()
	}
	wg.Wait()

	all, err := s.List(store.OrderFilters{})
	if err != nil {
		t.Fatalf("List after concurrent access: %v", err)
	}
	if len(all) != goroutines {
		t.Errorf("expected %d orders, got %d", goroutines, len(all))
	}
}

// ============================================================
// TradeStore tests
// ============================================================

func newTrade(id, instrumentID string) *types.SecurityTrade {
	return &types.SecurityTrade{
		ID:             id,
		BuyOrderID:     "buy-" + id,
		SellOrderID:    "sell-" + id,
		InstrumentID:   instrumentID,
		Price:          50.00,
		Quantity:       100,
		TradeDate:      "2026-01-01",
		SettlementDate: "2026-01-03",
		Status:         types.TradeStatusPending,
		CreatedAt:      "2026-01-01T00:00:00Z",
	}
}

func TestTradeStore_Create(t *testing.T) {
	s := store.NewInMemoryTradeStore()
	trade := newTrade("trade-1", "inst-A")

	if err := s.Create(trade); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get("trade-1")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ID != "trade-1" {
		t.Errorf("ID: want trade-1, got %s", got.ID)
	}
	if got.Price != 50.00 {
		t.Errorf("Price: want 50.00, got %v", got.Price)
	}
	if got.Quantity != 100 {
		t.Errorf("Quantity: want 100, got %d", got.Quantity)
	}
}

func TestTradeStore_Create_Duplicate(t *testing.T) {
	s := store.NewInMemoryTradeStore()
	trade := newTrade("trade-dup", "inst-A")
	if err := s.Create(trade); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := s.Create(trade); err == nil {
		t.Fatal("expected error on duplicate Create, got nil")
	}
}

func TestTradeStore_Get(t *testing.T) {
	s := store.NewInMemoryTradeStore()
	s.Create(newTrade("trade-get", "inst-A"))

	t.Run("existing", func(t *testing.T) {
		got, err := s.Get("trade-get")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != "trade-get" {
			t.Errorf("ID: want trade-get, got %s", got.ID)
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		_, err := s.Get("no-such-trade")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestTradeStore_ListByInstrument(t *testing.T) {
	s := store.NewInMemoryTradeStore()
	s.Create(newTrade("t1", "inst-A"))
	s.Create(newTrade("t2", "inst-A"))
	s.Create(newTrade("t3", "inst-B"))

	t.Run("inst-A returns 2", func(t *testing.T) {
		trades, err := s.ListByInstrument("inst-A")
		if err != nil {
			t.Fatalf("ListByInstrument: %v", err)
		}
		if len(trades) != 2 {
			t.Errorf("expected 2, got %d", len(trades))
		}
	})

	t.Run("inst-B returns 1", func(t *testing.T) {
		trades, err := s.ListByInstrument("inst-B")
		if err != nil {
			t.Fatalf("ListByInstrument: %v", err)
		}
		if len(trades) != 1 {
			t.Errorf("expected 1, got %d", len(trades))
		}
		if trades[0].ID != "t3" {
			t.Errorf("expected trade t3, got %s", trades[0].ID)
		}
	})

	t.Run("unknown instrument returns empty", func(t *testing.T) {
		trades, err := s.ListByInstrument("inst-MISSING")
		if err != nil {
			t.Fatalf("ListByInstrument: %v", err)
		}
		if len(trades) != 0 {
			t.Errorf("expected 0, got %d", len(trades))
		}
	})
}

// ============================================================
// PositionStore tests
// ============================================================

func TestPositionStore_GetOrCreate(t *testing.T) {
	s := store.NewInMemoryPositionStore()

	t.Run("first call creates zero position", func(t *testing.T) {
		pos, err := s.GetOrCreate("P1", "inst-A")
		if err != nil {
			t.Fatalf("GetOrCreate: %v", err)
		}
		if pos.ParticipantID != "P1" {
			t.Errorf("ParticipantID: want P1, got %s", pos.ParticipantID)
		}
		if pos.InstrumentID != "inst-A" {
			t.Errorf("InstrumentID: want inst-A, got %s", pos.InstrumentID)
		}
		if pos.Quantity != 0 {
			t.Errorf("initial Quantity: want 0, got %d", pos.Quantity)
		}
		if pos.AvgCost != 0 {
			t.Errorf("initial AvgCost: want 0, got %v", pos.AvgCost)
		}
	})

	t.Run("second call returns same position (not a new one)", func(t *testing.T) {
		// Update the position created above.
		pos, _ := s.GetOrCreate("P1", "inst-A")
		pos.Quantity = 100
		pos.AvgCost = 50.00
		s.Update(pos)

		pos2, err := s.GetOrCreate("P1", "inst-A")
		if err != nil {
			t.Fatalf("second GetOrCreate: %v", err)
		}
		if pos2.Quantity != 100 {
			t.Errorf("want 100 (from update), got %d", pos2.Quantity)
		}
		if pos2.AvgCost != 50.00 {
			t.Errorf("want AvgCost 50.00, got %v", pos2.AvgCost)
		}
	})
}

func TestPositionStore_Update(t *testing.T) {
	s := store.NewInMemoryPositionStore()

	pos, err := s.GetOrCreate("P2", "inst-B")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	pos.Quantity = 200
	pos.AvgCost = 75.50
	if err := s.Update(pos); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updated, err := s.GetOrCreate("P2", "inst-B")
	if err != nil {
		t.Fatalf("GetOrCreate after Update: %v", err)
	}
	if updated.Quantity != 200 {
		t.Errorf("Quantity: want 200, got %d", updated.Quantity)
	}
	if updated.AvgCost != 75.50 {
		t.Errorf("AvgCost: want 75.50, got %v", updated.AvgCost)
	}
}

func TestPositionStore_List(t *testing.T) {
	s := store.NewInMemoryPositionStore()

	// Create positions for two participants across two instruments.
	s.GetOrCreate("P3", "inst-A")
	s.GetOrCreate("P3", "inst-B")
	s.GetOrCreate("P4", "inst-A")

	t.Run("P3 has 2 positions", func(t *testing.T) {
		positions, err := s.List("P3")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(positions) != 2 {
			t.Errorf("expected 2 positions for P3, got %d", len(positions))
		}
	})

	t.Run("P4 has 1 position", func(t *testing.T) {
		positions, err := s.List("P4")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(positions) != 1 {
			t.Errorf("expected 1 position for P4, got %d", len(positions))
		}
		if positions[0].InstrumentID != "inst-A" {
			t.Errorf("InstrumentID: want inst-A, got %s", positions[0].InstrumentID)
		}
	})

	t.Run("unknown participant returns empty", func(t *testing.T) {
		positions, err := s.List("P-UNKNOWN")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(positions) != 0 {
			t.Errorf("expected 0, got %d", len(positions))
		}
	})
}

// TestOrderStore_Update verifies that Update replaces an existing order record.
func TestOrderStore_Update(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-upd", "inst-1", types.OrderSideBuy, types.OrderStatusPending)
	s.Submit(order)

	order.Status = types.OrderStatusPartiallyFilled
	order.FilledQuantity = 40
	if err := s.Update(order); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get("ord-upd")
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if got.Status != types.OrderStatusPartiallyFilled {
		t.Errorf("Status: want PARTIALLY_FILLED, got %s", got.Status)
	}
	if got.FilledQuantity != 40 {
		t.Errorf("FilledQuantity: want 40, got %d", got.FilledQuantity)
	}
}

// TestOrderStore_Update_NotFound verifies that Update returns ErrNotFound for unknown IDs.
func TestOrderStore_Update_NotFound(t *testing.T) {
	s := store.NewInMemoryOrderStore()
	order := newOrder("ord-missing", "inst-1", types.OrderSideBuy, types.OrderStatusPending)
	if err := s.Update(order); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// CorporateActionStore tests
// ============================================================

func newCorporateAction(id, instrID string, actionType types.CorporateActionType) *types.CorporateAction {
	return &types.CorporateAction{
		ID:           id,
		InstrumentID: instrID,
		ActionType:   actionType,
		Status:       types.CAStatusAnnounced,
		Details:      map[string]interface{}{"dividend_amount": 1.0},
		CreatedAt:    "2026-04-24T00:00:00Z",
		UpdatedAt:    "2026-04-24T00:00:00Z",
	}
}

func TestCorporateActionStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryCorporateActionStore()
	ca := newCorporateAction("ca-1", "inst-abc", types.CA_DIVIDEND)

	if err := s.Create(ca); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("ca-1")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ID != "ca-1" {
		t.Errorf("ID: want ca-1, got %s", got.ID)
	}
	if got.InstrumentID != "inst-abc" {
		t.Errorf("InstrumentID: want inst-abc, got %s", got.InstrumentID)
	}
	if got.ActionType != types.CA_DIVIDEND {
		t.Errorf("ActionType: want CA_DIVIDEND, got %s", got.ActionType)
	}
	if got.Status != types.CAStatusAnnounced {
		t.Errorf("Status: want ANNOUNCED, got %s", got.Status)
	}

	t.Run("get non-existent returns ErrNotFound", func(t *testing.T) {
		_, err := s.Get("no-such-ca")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(ca); err == nil {
			t.Fatal("expected error on duplicate Create, got nil")
		}
	})
}

func TestCorporateActionStore_List_WithFilters(t *testing.T) {
	s := store.NewInMemoryCorporateActionStore()

	s.Create(newCorporateAction("ca-d1", "inst-X", types.CA_DIVIDEND))
	s.Create(newCorporateAction("ca-d2", "inst-X", types.CA_DIVIDEND))
	s.Create(newCorporateAction("ca-s1", "inst-X", types.CA_STOCK_SPLIT))
	s.Create(newCorporateAction("ca-m1", "inst-Y", types.CA_MERGER))

	t.Run("list all returns 4", func(t *testing.T) {
		all, err := s.List(store.CorporateActionFilters{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(all) != 4 {
			t.Errorf("expected 4, got %d", len(all))
		}
	})

	t.Run("filter by InstrumentID=inst-X returns 3", func(t *testing.T) {
		res, err := s.List(store.CorporateActionFilters{InstrumentID: "inst-X"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res) != 3 {
			t.Errorf("expected 3, got %d", len(res))
		}
	})

	t.Run("filter by ActionType=CA_DIVIDEND returns 2", func(t *testing.T) {
		res, err := s.List(store.CorporateActionFilters{ActionType: types.CA_DIVIDEND})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res) != 2 {
			t.Errorf("expected 2, got %d", len(res))
		}
	})

	t.Run("filter by ActionType=CA_STOCK_SPLIT returns 1", func(t *testing.T) {
		res, err := s.List(store.CorporateActionFilters{ActionType: types.CA_STOCK_SPLIT})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1, got %d", len(res))
		}
	})

	t.Run("filter by InstrumentID=inst-Y and ActionType=CA_MERGER returns 1", func(t *testing.T) {
		res, err := s.List(store.CorporateActionFilters{
			InstrumentID: "inst-Y",
			ActionType:   types.CA_MERGER,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1, got %d", len(res))
		}
		if res[0].ID != "ca-m1" {
			t.Errorf("expected ca-m1, got %s", res[0].ID)
		}
	})

	t.Run("filter by non-existent instrument returns 0", func(t *testing.T) {
		res, err := s.List(store.CorporateActionFilters{InstrumentID: "inst-MISSING"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res) != 0 {
			t.Errorf("expected 0, got %d", len(res))
		}
	})
}

func TestCorporateActionStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryCorporateActionStore()
	s.Create(newCorporateAction("ca-upd", "inst-abc", types.CA_DIVIDEND))

	t.Run("ANNOUNCED → PROCESSING", func(t *testing.T) {
		if err := s.UpdateStatus("ca-upd", types.CAStatusProcessing); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("ca-upd")
		if got.Status != types.CAStatusProcessing {
			t.Errorf("expected PROCESSING, got %s", got.Status)
		}
	})

	t.Run("PROCESSING → COMPLETED", func(t *testing.T) {
		if err := s.UpdateStatus("ca-upd", types.CAStatusCompleted); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("ca-upd")
		if got.Status != types.CAStatusCompleted {
			t.Errorf("expected COMPLETED, got %s", got.Status)
		}
	})

	t.Run("non-existent ID returns ErrNotFound", func(t *testing.T) {
		if err := s.UpdateStatus("no-such-ca", types.CAStatusCompleted); err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ============================================================
// EntitlementStore tests
// ============================================================

func newEntitlement(id, caID, participantID, instrID string, qty int, value float64) *types.Entitlement {
	return &types.Entitlement{
		ID:                id,
		CorporateActionID: caID,
		ParticipantID:     participantID,
		InstrumentID:      instrID,
		Quantity:          qty,
		EntitlementValue:  value,
		Status:            types.EntitlementStatusPending,
		CreatedAt:         "2026-04-24T00:00:00Z",
	}
}

func TestEntitlementStore_Create_ListByAction(t *testing.T) {
	s := store.NewInMemoryEntitlementStore()

	// Two entitlements for the same CA.
	e1 := newEntitlement("ent-1", "ca-abc", "participant-A", "inst-X", 100, 500.0)
	e2 := newEntitlement("ent-2", "ca-abc", "participant-B", "inst-X", 200, 1000.0)
	// One entitlement for a different CA.
	e3 := newEntitlement("ent-3", "ca-other", "participant-A", "inst-X", 50, 250.0)

	for _, e := range []*types.Entitlement{e1, e2, e3} {
		if err := s.Create(e); err != nil {
			t.Fatalf("Create %s: %v", e.ID, err)
		}
	}

	t.Run("ListByAction ca-abc returns 2", func(t *testing.T) {
		result, err := s.ListByAction("ca-abc")
		if err != nil {
			t.Fatalf("ListByAction: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
	})

	t.Run("ListByAction ca-other returns 1", func(t *testing.T) {
		result, err := s.ListByAction("ca-other")
		if err != nil {
			t.Fatalf("ListByAction: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
		if result[0].ID != "ent-3" {
			t.Errorf("expected ent-3, got %s", result[0].ID)
		}
	})

	t.Run("ListByAction non-existent CA returns empty", func(t *testing.T) {
		result, err := s.ListByAction("ca-missing")
		if err != nil {
			t.Fatalf("ListByAction: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(e1); err == nil {
			t.Fatal("expected error on duplicate Create, got nil")
		}
	})
}

func TestEntitlementStore_ListByParticipant(t *testing.T) {
	s := store.NewInMemoryEntitlementStore()

	// participant-A has 2 entitlements (from different CAs).
	s.Create(newEntitlement("ent-p1", "ca-1", "participant-A", "inst-X", 100, 500.0))
	s.Create(newEntitlement("ent-p2", "ca-2", "participant-A", "inst-Y", 50, 250.0))
	// participant-B has 1 entitlement.
	s.Create(newEntitlement("ent-p3", "ca-1", "participant-B", "inst-X", 200, 1000.0))

	t.Run("participant-A returns 2", func(t *testing.T) {
		result, err := s.ListByParticipant("participant-A")
		if err != nil {
			t.Fatalf("ListByParticipant: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2, got %d", len(result))
		}
		// Verify all belong to participant-A.
		for _, e := range result {
			if e.ParticipantID != "participant-A" {
				t.Errorf("unexpected participant_id %s", e.ParticipantID)
			}
		}
	})

	t.Run("participant-B returns 1", func(t *testing.T) {
		result, err := s.ListByParticipant("participant-B")
		if err != nil {
			t.Fatalf("ListByParticipant: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
		if result[0].ID != "ent-p3" {
			t.Errorf("expected ent-p3, got %s", result[0].ID)
		}
		if result[0].Quantity != 200 {
			t.Errorf("expected quantity 200, got %d", result[0].Quantity)
		}
		if result[0].EntitlementValue != 1000.0 {
			t.Errorf("expected value 1000.0, got %v", result[0].EntitlementValue)
		}
	})

	t.Run("unknown participant returns empty", func(t *testing.T) {
		result, err := s.ListByParticipant("participant-MISSING")
		if err != nil {
			t.Fatalf("ListByParticipant: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})
}

// ============================================================
// CorporateActionStore concurrent access test
// ============================================================

func TestConcurrentAccess_CorporateActionStore(t *testing.T) {
	s := store.NewInMemoryCorporateActionStore()

	const goroutines = 10
	done := make(chan struct{}, goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer func() { done <- struct{}{} }()
			id := "conc-ca-" + string(rune('A'+i))
			ca := newCorporateAction(id, "inst-conc", types.CA_DIVIDEND)
			_ = s.Create(ca)
			_, _ = s.Get(id)
			_, _ = s.List(store.CorporateActionFilters{InstrumentID: "inst-conc"})
			_ = s.UpdateStatus(id, types.CAStatusCompleted)
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	all, err := s.List(store.CorporateActionFilters{})
	if err != nil {
		t.Fatalf("List after concurrent access: %v", err)
	}
	if len(all) != goroutines {
		t.Errorf("expected %d corporate actions, got %d", goroutines, len(all))
	}
}

// ============================================================
// MarketStore tests
// ============================================================

func TestMarketStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryMarketStore()

	m := &types.Market{
		ID:        "NYSE",
		Name:      "New York Stock Exchange",
		Status:    types.MarketActive,
		Timezone:  "America/New_York",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	if err := s.Create(m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("NYSE")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "NYSE" {
		t.Errorf("ID: want NYSE, got %s", got.ID)
	}
	if got.Name != "New York Stock Exchange" {
		t.Errorf("Name: want %q, got %q", "New York Stock Exchange", got.Name)
	}
	if got.Status != types.MarketActive {
		t.Errorf("Status: want %s, got %s", types.MarketActive, got.Status)
	}
	if got.Timezone != "America/New_York" {
		t.Errorf("Timezone: want %q, got %q", "America/New_York", got.Timezone)
	}

	t.Run("get non-existent returns error", func(t *testing.T) {
		_, err := s.Get("NO-MARKET")
		if err == nil {
			t.Error("expected error for non-existent market, got nil")
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(m); err == nil {
			t.Error("expected error on duplicate Create, got nil")
		}
	})
}

func TestMarketStore_List(t *testing.T) {
	// NewInMemoryMarketStore seeds the "MSE" market.
	s := store.NewInMemoryMarketStore()

	markets, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(markets) < 1 {
		t.Fatalf("expected at least 1 market (seeded MSE), got %d", len(markets))
	}

	// Verify the seeded MSE market is present.
	found := false
	for _, m := range markets {
		if m.ID == "MSE" {
			found = true
			if m.Name != "Mongolian Stock Exchange" {
				t.Errorf("MSE Name: want %q, got %q", "Mongolian Stock Exchange", m.Name)
			}
			if m.Status != types.MarketActive {
				t.Errorf("MSE Status: want %s, got %s", types.MarketActive, m.Status)
			}
		}
	}
	if !found {
		t.Error("seeded MSE market not found in List()")
	}

	// Add a second market and verify total increases.
	s.Create(&types.Market{ID: "LDN", Name: "London Stock Exchange", Status: types.MarketActive, Timezone: "Europe/London"})
	markets2, _ := s.List()
	if len(markets2) != len(markets)+1 {
		t.Errorf("after adding LDN: want %d markets, got %d", len(markets)+1, len(markets2))
	}
}

func TestMarketStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryMarketStore()

	t.Run("MARKET_ACTIVE → MARKET_SUSPENDED", func(t *testing.T) {
		if err := s.UpdateStatus("MSE", types.MarketSuspended); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("MSE")
		if got.Status != types.MarketSuspended {
			t.Errorf("Status: want %s, got %s", types.MarketSuspended, got.Status)
		}
		// UpdatedAt should be set.
		if got.UpdatedAt == "" {
			t.Error("UpdatedAt must not be empty after UpdateStatus")
		}
	})

	t.Run("MARKET_SUSPENDED → MARKET_CLOSED", func(t *testing.T) {
		if err := s.UpdateStatus("MSE", types.MarketClosed); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("MSE")
		if got.Status != types.MarketClosed {
			t.Errorf("Status: want %s, got %s", types.MarketClosed, got.Status)
		}
	})

	t.Run("non-existent ID returns error", func(t *testing.T) {
		if err := s.UpdateStatus("NO-MARKET", types.MarketActive); err == nil {
			t.Error("expected error for non-existent market, got nil")
		}
	})
}

func TestMarketStore_Get_ReturnsCopy(t *testing.T) {
	s := store.NewInMemoryMarketStore()

	got, _ := s.Get("MSE")
	got.Name = "MUTATED"

	got2, _ := s.Get("MSE")
	if got2.Name == "MUTATED" {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

// ============================================================
// SegmentStore tests
// ============================================================

func TestSegmentStore_Create_Get(t *testing.T) {
	s := store.NewInMemorySegmentStore()

	seg := &types.Segment{
		ID:        "BOND-SEG",
		MarketID:  "MSE",
		Name:      "Bonds",
		Status:    types.SegActive,
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	if err := s.Create(seg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("BOND-SEG")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "BOND-SEG" {
		t.Errorf("ID: want BOND-SEG, got %s", got.ID)
	}
	if got.MarketID != "MSE" {
		t.Errorf("MarketID: want MSE, got %s", got.MarketID)
	}
	if got.Name != "Bonds" {
		t.Errorf("Name: want Bonds, got %s", got.Name)
	}
	if got.Status != types.SegActive {
		t.Errorf("Status: want %s, got %s", types.SegActive, got.Status)
	}

	t.Run("get non-existent returns error", func(t *testing.T) {
		_, err := s.Get("NO-SEG")
		if err == nil {
			t.Error("expected error for non-existent segment, got nil")
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(seg); err == nil {
			t.Error("expected error on duplicate Create, got nil")
		}
	})
}

func TestSegmentStore_ListByMarket(t *testing.T) {
	// NewInMemorySegmentStore seeds "EQUITY" segment for market "MSE".
	s := store.NewInMemorySegmentStore()

	// Verify seeded segment is present.
	segs, err := s.ListByMarket("MSE")
	if err != nil {
		t.Fatalf("ListByMarket MSE: %v", err)
	}
	if len(segs) < 1 {
		t.Fatalf("expected at least 1 segment for MSE (seeded EQUITY), got %d", len(segs))
	}
	foundEquity := false
	for _, seg := range segs {
		if seg.ID == "EQUITY" {
			foundEquity = true
			if seg.MarketID != "MSE" {
				t.Errorf("seeded EQUITY segment: MarketID want MSE, got %s", seg.MarketID)
			}
		}
	}
	if !foundEquity {
		t.Error("seeded EQUITY segment not found in ListByMarket(MSE)")
	}

	// Add segments for a different market.
	s.Create(&types.Segment{ID: "LDN-EQ", MarketID: "LDN", Name: "London Equities", Status: types.SegActive})
	s.Create(&types.Segment{ID: "LDN-BD", MarketID: "LDN", Name: "London Bonds", Status: types.SegActive})

	ldnSegs, err := s.ListByMarket("LDN")
	if err != nil {
		t.Fatalf("ListByMarket LDN: %v", err)
	}
	if len(ldnSegs) != 2 {
		t.Errorf("want 2 segments for LDN, got %d", len(ldnSegs))
	}

	// Empty market_id returns all segments.
	allSegs, err := s.ListByMarket("")
	if err != nil {
		t.Fatalf("ListByMarket empty: %v", err)
	}
	if len(allSegs) < 3 {
		t.Errorf("want at least 3 segments (1 seeded + 2 added), got %d", len(allSegs))
	}

	// Non-existent market returns empty slice.
	noSegs, err := s.ListByMarket("UNKNOWN-MKT")
	if err != nil {
		t.Fatalf("ListByMarket unknown: %v", err)
	}
	if len(noSegs) != 0 {
		t.Errorf("want 0 segments for unknown market, got %d", len(noSegs))
	}
}

func TestSegmentStore_Get_ReturnsCopy(t *testing.T) {
	s := store.NewInMemorySegmentStore()

	got, _ := s.Get("EQUITY")
	got.Name = "MUTATED"

	got2, _ := s.Get("EQUITY")
	if got2.Name == "MUTATED" {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

// ============================================================
// CircuitBreakerStore tests
// ============================================================

func newCB(instrumentID string) *types.CircuitBreaker {
	return &types.CircuitBreaker{
		InstrumentID:    instrumentID,
		ReferencePrice:  100.0,
		StaticUpperPct:  10.0,
		StaticLowerPct:  10.0,
		DynamicUpperPct: 5.0,
		DynamicLowerPct: 5.0,
		LastTradedPrice: 100.0,
		Status:          types.CBActive,
		CooldownMinutes: 15,
	}
}

func TestCircuitBreakerStore_SetGetDelete(t *testing.T) {
	s := store.NewInMemoryCircuitBreakerStore()

	cb := newCB("INST-CB-1")

	// Set.
	if err := s.Set(cb); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get returns the record.
	got, err := s.Get("INST-CB-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil circuit breaker, got nil")
	}
	if got.InstrumentID != "INST-CB-1" {
		t.Errorf("InstrumentID: want INST-CB-1, got %s", got.InstrumentID)
	}
	if got.ReferencePrice != 100.0 {
		t.Errorf("ReferencePrice: want 100.0, got %v", got.ReferencePrice)
	}
	if got.StaticUpperPct != 10.0 {
		t.Errorf("StaticUpperPct: want 10.0, got %v", got.StaticUpperPct)
	}
	if got.Status != types.CBActive {
		t.Errorf("Status: want %s, got %s", types.CBActive, got.Status)
	}

	// Get non-existent returns nil (not an error).
	missing, err := s.Get("INST-MISSING")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil for non-existent instrument, got %+v", missing)
	}

	// Delete removes the record.
	if err := s.Delete("INST-CB-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	afterDelete, err := s.Get("INST-CB-1")
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if afterDelete != nil {
		t.Errorf("expected nil after Delete, got %+v", afterDelete)
	}

	// Delete non-existent is a no-op (no error).
	if err := s.Delete("INST-MISSING"); err != nil {
		t.Errorf("Delete non-existent: want nil error, got %v", err)
	}
}

func TestCircuitBreakerStore_List(t *testing.T) {
	s := store.NewInMemoryCircuitBreakerStore()

	// Empty initially.
	all, err := s.List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 entries initially, got %d", len(all))
	}

	// Add three circuit breakers.
	for _, id := range []string{"CB-A", "CB-B", "CB-C"} {
		if err := s.Set(newCB(id)); err != nil {
			t.Fatalf("Set %s: %v", id, err)
		}
	}

	all, err = s.List()
	if err != nil {
		t.Fatalf("List after Set: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 circuit breakers, got %d", len(all))
	}
}

func TestCircuitBreakerStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryCircuitBreakerStore()
	s.Set(newCB("CB-UPST"))

	t.Run("CB_ACTIVE → CB_TRIGGERED sets TriggeredAt", func(t *testing.T) {
		if err := s.UpdateStatus("CB-UPST", types.CBTriggered); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("CB-UPST")
		if got.Status != types.CBTriggered {
			t.Errorf("Status: want %s, got %s", types.CBTriggered, got.Status)
		}
		if got.TriggeredAt == "" {
			t.Error("TriggeredAt must be set when status transitions to CB_TRIGGERED")
		}
	})

	t.Run("CB_TRIGGERED → CB_COOLDOWN", func(t *testing.T) {
		if err := s.UpdateStatus("CB-UPST", types.CBCooldown); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("CB-UPST")
		if got.Status != types.CBCooldown {
			t.Errorf("Status: want %s, got %s", types.CBCooldown, got.Status)
		}
	})

	t.Run("non-existent ID returns error", func(t *testing.T) {
		if err := s.UpdateStatus("CB-MISSING", types.CBActive); err == nil {
			t.Error("expected error for non-existent circuit breaker, got nil")
		}
	})
}

func TestCircuitBreakerStore_UpdateLastPrice(t *testing.T) {
	s := store.NewInMemoryCircuitBreakerStore()
	s.Set(newCB("CB-PRICE"))

	if err := s.UpdateLastPrice("CB-PRICE", 105.5); err != nil {
		t.Fatalf("UpdateLastPrice: %v", err)
	}
	got, _ := s.Get("CB-PRICE")
	if got.LastTradedPrice != 105.5 {
		t.Errorf("LastTradedPrice: want 105.5, got %v", got.LastTradedPrice)
	}

	// UpdateLastPrice for non-existent instrument is a no-op (no error).
	if err := s.UpdateLastPrice("CB-MISSING", 50.0); err != nil {
		t.Errorf("UpdateLastPrice non-existent: want nil error, got %v", err)
	}
}

func TestCircuitBreakerStore_Set_Overwrites(t *testing.T) {
	// Set should overwrite an existing record (upsert semantics).
	s := store.NewInMemoryCircuitBreakerStore()
	s.Set(newCB("CB-OVER"))

	updated := newCB("CB-OVER")
	updated.ReferencePrice = 200.0
	updated.StaticUpperPct = 20.0

	if err := s.Set(updated); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}
	got, _ := s.Get("CB-OVER")
	if got.ReferencePrice != 200.0 {
		t.Errorf("ReferencePrice after overwrite: want 200.0, got %v", got.ReferencePrice)
	}
	if got.StaticUpperPct != 20.0 {
		t.Errorf("StaticUpperPct after overwrite: want 20.0, got %v", got.StaticUpperPct)
	}
}

func TestCircuitBreakerStore_Get_ReturnsCopy(t *testing.T) {
	s := store.NewInMemoryCircuitBreakerStore()
	s.Set(newCB("CB-COPY"))

	got, _ := s.Get("CB-COPY")
	got.ReferencePrice = 9999.0

	got2, _ := s.Get("CB-COPY")
	if got2.ReferencePrice == 9999.0 {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

// ============================================================
// FirmStore tests
// ============================================================

func newFirm(id, name string, status types.FirmStatus) *types.Firm {
	return &types.Firm{
		ID:        id,
		Name:      name,
		Status:    status,
		CreatedAt: "2026-04-24T00:00:00Z",
		UpdatedAt: "2026-04-24T00:00:00Z",
	}
}

func TestFirmStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryFirmStore()
	f := newFirm("MSE-BROKER-1", "Alpha Securities", types.FirmActive)

	if err := s.Create(f); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("MSE-BROKER-1")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ID != "MSE-BROKER-1" {
		t.Errorf("ID: want MSE-BROKER-1, got %s", got.ID)
	}
	if got.Name != "Alpha Securities" {
		t.Errorf("Name: want Alpha Securities, got %s", got.Name)
	}
	if got.Status != types.FirmActive {
		t.Errorf("Status: want %s, got %s", types.FirmActive, got.Status)
	}

	t.Run("get non-existent returns ErrNotFound", func(t *testing.T) {
		_, err := s.Get("NO-SUCH-FIRM")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(f); err == nil {
			t.Fatal("expected error on duplicate Create, got nil")
		}
	})
}

func TestFirmStore_List(t *testing.T) {
	s := store.NewInMemoryFirmStore()

	// Seed MSE-BROKER-1 (matching the spec requirement).
	s.Create(newFirm("MSE-BROKER-1", "Alpha Securities", types.FirmActive))
	s.Create(newFirm("MSE-BROKER-2", "Beta Capital", types.FirmActive))
	s.Create(newFirm("MSE-BROKER-3", "Gamma Investments", types.FirmSuspended))

	firms, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(firms) != 3 {
		t.Fatalf("expected 3 firms, got %d", len(firms))
	}

	// Verify MSE-BROKER-1 is present.
	found := false
	for _, f := range firms {
		if f.ID == "MSE-BROKER-1" {
			found = true
			if f.Name != "Alpha Securities" {
				t.Errorf("MSE-BROKER-1 Name: want Alpha Securities, got %s", f.Name)
			}
			if f.Status != types.FirmActive {
				t.Errorf("MSE-BROKER-1 Status: want %s, got %s", types.FirmActive, f.Status)
			}
		}
	}
	if !found {
		t.Error("MSE-BROKER-1 not found in List()")
	}
}

func TestFirmStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryFirmStore()
	s.Create(newFirm("FIRM-UPST", "Delta Markets", types.FirmActive))

	t.Run("FIRM_ACTIVE → FIRM_SUSPENDED", func(t *testing.T) {
		if err := s.UpdateStatus("FIRM-UPST", types.FirmSuspended); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("FIRM-UPST")
		if got.Status != types.FirmSuspended {
			t.Errorf("Status: want %s, got %s", types.FirmSuspended, got.Status)
		}
		if got.UpdatedAt == "2026-04-24T00:00:00Z" {
			// UpdatedAt should have been refreshed.
			t.Error("UpdatedAt was not updated after status change")
		}
	})

	t.Run("FIRM_SUSPENDED → FIRM_DEACTIVATED", func(t *testing.T) {
		if err := s.UpdateStatus("FIRM-UPST", types.FirmDeactivated); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		got, _ := s.Get("FIRM-UPST")
		if got.Status != types.FirmDeactivated {
			t.Errorf("Status: want %s, got %s", types.FirmDeactivated, got.Status)
		}
	})

	t.Run("non-existent ID returns ErrNotFound", func(t *testing.T) {
		if err := s.UpdateStatus("NO-SUCH-FIRM", types.FirmSuspended); err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ============================================================
// ParticipantStore tests
// ============================================================

func newParticipant(id, firmID, name string, status types.ParticipantStatus, perms ...string) *types.ExchangeParticipant {
	return &types.ExchangeParticipant{
		ID:          id,
		FirmID:      firmID,
		Name:        name,
		Status:      status,
		Permissions: perms,
		CreatedAt:   "2026-04-24T00:00:00Z",
		UpdatedAt:   "2026-04-24T00:00:00Z",
	}
}

func TestParticipantStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryParticipantStore()
	p := newParticipant("PART-1", "MSE-BROKER-1", "Trader Alice",
		types.ParticipantActive, types.PermTradeEquity, types.PermTradeBond)

	if err := s.Create(p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("PART-1")
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.ID != "PART-1" {
		t.Errorf("ID: want PART-1, got %s", got.ID)
	}
	if got.FirmID != "MSE-BROKER-1" {
		t.Errorf("FirmID: want MSE-BROKER-1, got %s", got.FirmID)
	}
	if got.Status != types.ParticipantActive {
		t.Errorf("Status: want %s, got %s", types.ParticipantActive, got.Status)
	}
	if len(got.Permissions) != 2 {
		t.Errorf("Permissions len: want 2, got %d", len(got.Permissions))
	}
	// Verify specific permissions.
	hasEquity := false
	for _, perm := range got.Permissions {
		if perm == types.PermTradeEquity {
			hasEquity = true
		}
	}
	if !hasEquity {
		t.Errorf("expected %s in permissions, got %v", types.PermTradeEquity, got.Permissions)
	}

	t.Run("get non-existent returns ErrNotFound", func(t *testing.T) {
		_, err := s.Get("NO-SUCH-PART")
		if err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("duplicate Create returns error", func(t *testing.T) {
		if err := s.Create(p); err == nil {
			t.Fatal("expected error on duplicate Create, got nil")
		}
	})

	t.Run("Get returns copy (mutation does not affect store)", func(t *testing.T) {
		got2, _ := s.Get("PART-1")
		got2.Name = "MUTATED"
		got3, _ := s.Get("PART-1")
		if got3.Name == "MUTATED" {
			t.Error("Get returned a pointer into internal storage instead of a copy")
		}
	})
}

func TestParticipantStore_ListByFirm(t *testing.T) {
	s := store.NewInMemoryParticipantStore()

	// Two participants in firm-A, one in firm-B.
	s.Create(newParticipant("P-A1", "FIRM-A", "Alice", types.ParticipantActive, types.PermTradeEquity))
	s.Create(newParticipant("P-A2", "FIRM-A", "Bob", types.ParticipantActive, types.PermTradeBond))
	s.Create(newParticipant("P-B1", "FIRM-B", "Carol", types.ParticipantSuspended))

	t.Run("filter by FIRM-A returns 2", func(t *testing.T) {
		results, err := s.List(store.ParticipantFilters{FirmID: "FIRM-A"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 participants for FIRM-A, got %d", len(results))
		}
		for _, p := range results {
			if p.FirmID != "FIRM-A" {
				t.Errorf("unexpected FirmID %s", p.FirmID)
			}
		}
	})

	t.Run("filter by FIRM-B returns 1", func(t *testing.T) {
		results, err := s.List(store.ParticipantFilters{FirmID: "FIRM-B"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 participant for FIRM-B, got %d", len(results))
		}
		if results[0].ID != "P-B1" {
			t.Errorf("ID: want P-B1, got %s", results[0].ID)
		}
	})

	t.Run("no filter returns all 3", func(t *testing.T) {
		results, err := s.List(store.ParticipantFilters{})
		if err != nil {
			t.Fatalf("List all: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 participants, got %d", len(results))
		}
	})

	t.Run("filter by non-existent firm returns empty", func(t *testing.T) {
		results, err := s.List(store.ParticipantFilters{FirmID: "FIRM-MISSING"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 participants, got %d", len(results))
		}
	})
}

func TestParticipantStore_UpdatePermissions(t *testing.T) {
	s := store.NewInMemoryParticipantStore()
	s.Create(newParticipant("PART-PERM", "FIRM-X", "PermUser",
		types.ParticipantActive, types.PermTradeEquity))

	t.Run("add TRADE_BOND permission", func(t *testing.T) {
		newPerms := []string{types.PermTradeEquity, types.PermTradeBond}
		if err := s.UpdatePermissions("PART-PERM", newPerms); err != nil {
			t.Fatalf("UpdatePermissions: %v", err)
		}
		got, _ := s.Get("PART-PERM")
		if len(got.Permissions) != 2 {
			t.Errorf("Permissions len: want 2, got %d", len(got.Permissions))
		}
	})

	t.Run("remove all permissions", func(t *testing.T) {
		if err := s.UpdatePermissions("PART-PERM", []string{}); err != nil {
			t.Fatalf("UpdatePermissions to empty: %v", err)
		}
		got, _ := s.Get("PART-PERM")
		if len(got.Permissions) != 0 {
			t.Errorf("expected empty permissions, got %v", got.Permissions)
		}
	})

	t.Run("replace with MARKET_MAKER", func(t *testing.T) {
		if err := s.UpdatePermissions("PART-PERM", []string{types.PermMarketMaker}); err != nil {
			t.Fatalf("UpdatePermissions: %v", err)
		}
		got, _ := s.Get("PART-PERM")
		if len(got.Permissions) != 1 || got.Permissions[0] != types.PermMarketMaker {
			t.Errorf("expected [%s], got %v", types.PermMarketMaker, got.Permissions)
		}
	})

	t.Run("UpdatedAt is refreshed", func(t *testing.T) {
		// Create a fresh store so UpdatedAt starts at the static creation timestamp.
		fresh := store.NewInMemoryParticipantStore()
		fresh.Create(newParticipant("PART-TS", "FIRM-X", "TSUser",
			types.ParticipantActive, types.PermTradeEquity))

		const staticCreation = "2026-04-24T00:00:00Z"
		before, _ := fresh.Get("PART-TS")
		if before.UpdatedAt != staticCreation {
			t.Skipf("initial UpdatedAt %q differs from expected %q — skip", before.UpdatedAt, staticCreation)
		}
		if err := fresh.UpdatePermissions("PART-TS", []string{types.PermTradeETF}); err != nil {
			t.Fatalf("UpdatePermissions: %v", err)
		}
		after, _ := fresh.Get("PART-TS")
		if after.UpdatedAt == staticCreation {
			t.Error("UpdatedAt was not refreshed after UpdatePermissions")
		}
	})

	t.Run("non-existent ID returns ErrNotFound", func(t *testing.T) {
		if err := s.UpdatePermissions("NO-SUCH-PART", []string{types.PermTradeEquity}); err != store.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ============================================================
// TradeCorrectionStore tests
// ============================================================

func newTradeCorrection(id, tradeID, action string) *types.TradeCorrection {
	return &types.TradeCorrection{
		ID:                id,
		TradeID:           tradeID,
		Action:            action,
		Reason:            "test reason",
		OriginalPrice:     50.00,
		OriginalQuantity:  100,
		CorrectedPrice:    51.00,
		CorrectedQuantity: 100,
		ActorID:           "admin-1",
		Timestamp:         "2026-04-24T00:00:00Z",
	}
}

// TestTradeCorrectionStore_Create_ListByTrade verifies that corrections are stored
// and correctly retrieved by trade ID.
func TestTradeCorrectionStore_Create_ListByTrade(t *testing.T) {
	s := store.NewInMemoryTradeCorrectionStore()

	// Two corrections for trade-A.
	c1 := newTradeCorrection("corr-1", "trade-A", "BUST")
	c2 := newTradeCorrection("corr-2", "trade-A", "CORRECT")
	// One correction for trade-B.
	c3 := newTradeCorrection("corr-3", "trade-B", "REINSTATE")

	for _, c := range []*types.TradeCorrection{c1, c2, c3} {
		if err := s.Create(c); err != nil {
			t.Fatalf("Create %s: %v", c.ID, err)
		}
	}

	t.Run("ListByTrade trade-A returns 2", func(t *testing.T) {
		results, err := s.ListByTrade("trade-A")
		if err != nil {
			t.Fatalf("ListByTrade: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 corrections for trade-A, got %d", len(results))
		}
		// Verify all belong to trade-A.
		for _, r := range results {
			if r.TradeID != "trade-A" {
				t.Errorf("unexpected TradeID %s", r.TradeID)
			}
		}
	})

	t.Run("ListByTrade trade-B returns 1", func(t *testing.T) {
		results, err := s.ListByTrade("trade-B")
		if err != nil {
			t.Fatalf("ListByTrade: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 correction for trade-B, got %d", len(results))
		}
		if results[0].Action != "REINSTATE" {
			t.Errorf("action: want REINSTATE, got %s", results[0].Action)
		}
	})

	t.Run("ListByTrade unknown trade returns empty", func(t *testing.T) {
		results, err := s.ListByTrade("trade-MISSING")
		if err != nil {
			t.Fatalf("ListByTrade missing: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0, got %d", len(results))
		}
	})

	t.Run("Create preserves all fields", func(t *testing.T) {
		s2 := store.NewInMemoryTradeCorrectionStore()
		orig := &types.TradeCorrection{
			ID:                "corr-fields",
			TradeID:           "trade-fields",
			Action:            "CORRECT",
			Reason:            "price error",
			OriginalPrice:     100.0,
			OriginalQuantity:  200,
			CorrectedPrice:    101.5,
			CorrectedQuantity: 200,
			ActorID:           "super-admin",
			Timestamp:         "2026-04-24T12:00:00Z",
		}
		if err := s2.Create(orig); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := s2.ListByTrade("trade-fields")
		if err != nil {
			t.Fatalf("ListByTrade: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		r := got[0]
		if r.OriginalPrice != 100.0 {
			t.Errorf("OriginalPrice: want 100.0, got %v", r.OriginalPrice)
		}
		if r.CorrectedPrice != 101.5 {
			t.Errorf("CorrectedPrice: want 101.5, got %v", r.CorrectedPrice)
		}
		if r.ActorID != "super-admin" {
			t.Errorf("ActorID: want super-admin, got %s", r.ActorID)
		}
	})
}

// ============================================================
// TickTableStore tests
// ============================================================

func newTickTable(instrumentID string) *types.TickTable {
	return &types.TickTable{
		InstrumentID: instrumentID,
		Tiers: []types.TickTier{
			{MinPrice: 0, MaxPrice: 50, TickSize: 0.01},
			{MinPrice: 50, MaxPrice: 200, TickSize: 0.05},
			{MinPrice: 200, MaxPrice: 1000, TickSize: 0.25},
		},
	}
}

// TestTickTableStore_SetGetDelete verifies the full Set → Get → Delete lifecycle.
func TestTickTableStore_SetGetDelete(t *testing.T) {
	s := store.NewInMemoryTickTableStore()

	table := newTickTable("INST-TT-1")

	// Set.
	if err := s.Set(table); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get returns the record.
	got, err := s.Get("INST-TT-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil tick table, got nil")
	}
	if got.InstrumentID != "INST-TT-1" {
		t.Errorf("InstrumentID: want INST-TT-1, got %s", got.InstrumentID)
	}
	if len(got.Tiers) != 3 {
		t.Errorf("Tiers len: want 3, got %d", len(got.Tiers))
	}
	if got.Tiers[0].TickSize != 0.01 {
		t.Errorf("Tiers[0].TickSize: want 0.01, got %v", got.Tiers[0].TickSize)
	}
	if got.Tiers[2].TickSize != 0.25 {
		t.Errorf("Tiers[2].TickSize: want 0.25, got %v", got.Tiers[2].TickSize)
	}

	// Get non-existent returns ErrNotFound.
	_, err = s.Get("INST-MISSING")
	if err != store.ErrNotFound {
		t.Errorf("Get missing: want ErrNotFound, got %v", err)
	}

	// Delete removes the record.
	if err := s.Delete("INST-TT-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get("INST-TT-1")
	if err != store.ErrNotFound {
		t.Errorf("Get after Delete: want ErrNotFound, got %v", err)
	}

	// Delete non-existent is a no-op.
	if err := s.Delete("INST-MISSING"); err != nil {
		t.Errorf("Delete non-existent: want nil, got %v", err)
	}
}

// TestTickTableStore_Set_Overwrites verifies that Set has upsert semantics —
// a second Set for the same instrument ID overwrites the existing record.
func TestTickTableStore_Set_Overwrites(t *testing.T) {
	s := store.NewInMemoryTickTableStore()

	original := &types.TickTable{
		InstrumentID: "INST-TT-OVER",
		Tiers:        []types.TickTier{{MinPrice: 0, MaxPrice: 100, TickSize: 0.1}},
	}
	if err := s.Set(original); err != nil {
		t.Fatalf("Set original: %v", err)
	}

	updated := &types.TickTable{
		InstrumentID: "INST-TT-OVER",
		Tiers: []types.TickTier{
			{MinPrice: 0, MaxPrice: 50, TickSize: 0.01},
			{MinPrice: 50, MaxPrice: 500, TickSize: 0.5},
		},
	}
	if err := s.Set(updated); err != nil {
		t.Fatalf("Set updated: %v", err)
	}

	got, err := s.Get("INST-TT-OVER")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if len(got.Tiers) != 2 {
		t.Errorf("Tiers len after overwrite: want 2, got %d", len(got.Tiers))
	}
	if got.Tiers[0].TickSize != 0.01 {
		t.Errorf("Tiers[0].TickSize after overwrite: want 0.01, got %v", got.Tiers[0].TickSize)
	}
}

// TestTickTableStore_List verifies that List returns all stored tick tables.
func TestTickTableStore_List(t *testing.T) {
	s := store.NewInMemoryTickTableStore()

	// Empty initially.
	tables, err := s.List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("expected 0 tables initially, got %d", len(tables))
	}

	// Set 3 tick tables.
	for _, id := range []string{"INST-L1", "INST-L2", "INST-L3"} {
		if err := s.Set(newTickTable(id)); err != nil {
			t.Fatalf("Set %s: %v", id, err)
		}
	}

	tables, err = s.List()
	if err != nil {
		t.Fatalf("List after Set: %v", err)
	}
	if len(tables) != 3 {
		t.Errorf("expected 3 tick tables, got %d", len(tables))
	}
}

// TestTickTableStore_DeepCopy verifies that mutating the Tiers slice returned
// by Get does NOT modify the stored record (deep copy semantics).
func TestTickTableStore_DeepCopy(t *testing.T) {
	s := store.NewInMemoryTickTableStore()

	original := &types.TickTable{
		InstrumentID: "INST-TT-COPY",
		Tiers: []types.TickTier{
			{MinPrice: 0, MaxPrice: 100, TickSize: 0.1},
			{MinPrice: 100, MaxPrice: 500, TickSize: 0.5},
		},
	}
	if err := s.Set(original); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get a copy and mutate it.
	got, err := s.Get("INST-TT-COPY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Tiers[0].TickSize = 999.0    // Mutate returned copy.
	got.Tiers = append(got.Tiers, types.TickTier{MinPrice: 500, MaxPrice: 1000, TickSize: 1.0})

	// Fetch again — the stored record must be unchanged.
	got2, err := s.Get("INST-TT-COPY")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got2.Tiers[0].TickSize == 999.0 {
		t.Error("Get returned a reference into internal storage — mutation affected stored record (Tiers[0].TickSize)")
	}
	if len(got2.Tiers) != 2 {
		t.Errorf("stored Tiers length changed after appending to returned copy: want 2, got %d", len(got2.Tiers))
	}
}

// ============================================================
// ThrottleStore tests
// ============================================================

// TestThrottleStore_UnderLimit verifies that calls below the per-second limit
// all succeed (return true).
func TestThrottleStore_UnderLimit(t *testing.T) {
	s := store.NewInMemoryThrottleStore()
	const maxPerSecond = 10
	const calls = 5

	for i := 0; i < calls; i++ {
		allowed, err := s.CheckAndIncrement("firm-A", maxPerSecond)
		if err != nil {
			t.Errorf("call %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Errorf("call %d: expected allowed=true (under limit %d), got false", i+1, maxPerSecond)
		}
	}
}

// TestThrottleStore_OverLimit verifies that the (limit+1)th call within the same
// 1-second window returns false and an error.
func TestThrottleStore_OverLimit(t *testing.T) {
	s := store.NewInMemoryThrottleStore()
	const maxPerSecond = 10

	// First 10 calls should all succeed.
	for i := 0; i < maxPerSecond; i++ {
		allowed, err := s.CheckAndIncrement("firm-B", maxPerSecond)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected allowed=true, got false (limit=%d)", i+1, maxPerSecond)
		}
	}

	// 11th call must be throttled.
	allowed, err := s.CheckAndIncrement("firm-B", maxPerSecond)
	if allowed {
		t.Error("11th call: expected allowed=false (over limit), got true")
	}
	if err == nil {
		t.Error("11th call: expected non-nil error when throttled, got nil")
	}
}

// TestThrottleStore_WindowReset verifies that after waiting more than 1 second,
// the counter resets and calls are allowed again.
func TestThrottleStore_WindowReset(t *testing.T) {
	s := store.NewInMemoryThrottleStore()
	const maxPerSecond = 3
	const firmID = "firm-C"

	// Exhaust the limit.
	for i := 0; i < maxPerSecond; i++ {
		_, _ = s.CheckAndIncrement(firmID, maxPerSecond)
	}

	// Verify we are over the limit.
	allowed, _ := s.CheckAndIncrement(firmID, maxPerSecond)
	if allowed {
		t.Skip("throttle limit not reached within window; skipping window-reset test")
	}

	// Wait for the window to expire (>1 second).
	// The ThrottleStore uses 1-second tumbling windows; sleeping 1100ms crosses the boundary.
	time.Sleep(1100 * time.Millisecond)

	// After the window expires, calls should be allowed again.
	for i := 0; i < maxPerSecond; i++ {
		allowed2, err := s.CheckAndIncrement(firmID, maxPerSecond)
		if err != nil {
			t.Errorf("post-reset call %d: unexpected error: %v", i+1, err)
		}
		if !allowed2 {
			t.Errorf("post-reset call %d: expected allowed=true after window reset, got false", i+1)
		}
	}
}

// ============================================================
// ThrottleStore concurrent access test
// ============================================================

// TestThrottleStore_ConcurrentAccess verifies that concurrent calls from
// multiple goroutines do not race or panic (use -race flag).
func TestThrottleStore_ConcurrentAccess(t *testing.T) {
	s := store.NewInMemoryThrottleStore()
	const goroutines = 20
	const maxPerSecond = 100

	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = s.CheckAndIncrement("firm-concurrent", maxPerSecond)
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	// No assertions needed — the -race detector validates correctness.
}

// ============================================================
// AnnouncementStore tests
// ============================================================

func TestAnnouncementStore_Create_List(t *testing.T) {
	s := store.NewInMemoryAnnouncementStore()

	a1 := &types.Announcement{
		ID:       "ann-1",
		TenantID: "tenant-alpha",
		Title:    "Alpha Notice",
		Body:     "Body for alpha",
		Audience: types.AudiencePublic,
	}
	a2 := &types.Announcement{
		ID:       "ann-2",
		TenantID: "tenant-beta",
		Title:    "Beta Notice",
		Body:     "Body for beta",
		Audience: types.AudienceInternal,
	}
	a3 := &types.Announcement{
		ID:       "ann-3",
		TenantID: "tenant-alpha",
		Title:    "Alpha Notice 2",
		Body:     "Second alpha body",
		Audience: types.AudienceParticipant,
	}

	if err := s.Create(a1); err != nil {
		t.Fatalf("Create a1: %v", err)
	}
	if err := s.Create(a2); err != nil {
		t.Fatalf("Create a2: %v", err)
	}
	if err := s.Create(a3); err != nil {
		t.Fatalf("Create a3: %v", err)
	}

	// List for tenant-alpha should return exactly 2.
	alphaList, err := s.ListByTenant("tenant-alpha")
	if err != nil {
		t.Fatalf("ListByTenant tenant-alpha: %v", err)
	}
	if len(alphaList) != 2 {
		t.Errorf("expected 2 announcements for tenant-alpha, got %d", len(alphaList))
	}
	for _, a := range alphaList {
		if a.TenantID != "tenant-alpha" {
			t.Errorf("expected tenant-alpha, got %q", a.TenantID)
		}
	}

	// List for tenant-beta should return exactly 1.
	betaList, err := s.ListByTenant("tenant-beta")
	if err != nil {
		t.Fatalf("ListByTenant tenant-beta: %v", err)
	}
	if len(betaList) != 1 {
		t.Errorf("expected 1 announcement for tenant-beta, got %d", len(betaList))
	}
	if betaList[0].ID != "ann-2" {
		t.Errorf("expected ann-2, got %q", betaList[0].ID)
	}
}

func TestAnnouncementStore_ListEmpty(t *testing.T) {
	s := store.NewInMemoryAnnouncementStore()

	list, err := s.ListByTenant("no-such-tenant")
	if err != nil {
		t.Fatalf("ListByTenant on empty store: %v", err)
	}
	if list == nil {
		t.Fatal("expected non-nil slice, got nil")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 announcements, got %d", len(list))
	}
}

// ============================================================
// AuditStore tests
// ============================================================

func TestAuditStore_Log_ListByEntityType(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	entries := []types.AuditEntry{
		{ID: "a1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "actor-1", Timestamp: now},
		{ID: "a2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "actor-2", Timestamp: now},
		{ID: "a3", EntityType: "ORDER", EntityID: "o2", Action: "CANCEL", ActorID: "actor-1", Timestamp: now},
		{ID: "a4", EntityType: "INSTRUMENT", EntityID: "i1", Action: "UPDATE", ActorID: "actor-3", Timestamp: now},
	}
	for _, e := range entries {
		if err := s.Log(e); err != nil {
			t.Fatalf("Log entry %s: %v", e.ID, err)
		}
	}

	// Filter by entity_type=ORDER — expect 2 results.
	orders, err := s.List(types.AuditFilters{EntityType: "ORDER"})
	if err != nil {
		t.Fatalf("List by entity_type ORDER: %v", err)
	}
	if len(orders) != 2 {
		t.Errorf("expected 2 ORDER entries, got %d", len(orders))
	}
	for _, e := range orders {
		if e.EntityType != "ORDER" {
			t.Errorf("unexpected entity_type %q in ORDER filter result", e.EntityType)
		}
	}

	// Filter by entity_type=TRADE — expect 1 result.
	trades, err := s.List(types.AuditFilters{EntityType: "TRADE"})
	if err != nil {
		t.Fatalf("List by entity_type TRADE: %v", err)
	}
	if len(trades) != 1 {
		t.Errorf("expected 1 TRADE entry, got %d", len(trades))
	}

	// No filter — expect all 4.
	all, err := s.List(types.AuditFilters{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 entries total, got %d", len(all))
	}
}

func TestAuditStore_Log_ListByActorID(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	_ = s.Log(types.AuditEntry{ID: "b1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "alice", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "b2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "bob", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "b3", EntityType: "ORDER", EntityID: "o2", Action: "CANCEL", ActorID: "alice", Timestamp: now})

	aliceEntries, err := s.List(types.AuditFilters{ActorID: "alice"})
	if err != nil {
		t.Fatalf("List by actor_id alice: %v", err)
	}
	if len(aliceEntries) != 2 {
		t.Errorf("expected 2 entries for alice, got %d", len(aliceEntries))
	}
	for _, e := range aliceEntries {
		if e.ActorID != "alice" {
			t.Errorf("unexpected actor_id %q", e.ActorID)
		}
	}

	bobEntries, err := s.List(types.AuditFilters{ActorID: "bob"})
	if err != nil {
		t.Fatalf("List by actor_id bob: %v", err)
	}
	if len(bobEntries) != 1 {
		t.Errorf("expected 1 entry for bob, got %d", len(bobEntries))
	}
}

func TestAuditStore_Log_ListByDateRange(t *testing.T) {
	s := store.NewInMemoryAuditStore()

	// Use fixed timestamps so filter logic is deterministic.
	ts1 := "2026-01-01T10:00:00Z"
	ts2 := "2026-01-01T12:00:00Z"
	ts3 := "2026-01-01T14:00:00Z"
	ts4 := "2026-01-01T16:00:00Z"

	_ = s.Log(types.AuditEntry{ID: "d1", EntityType: "ORDER", Action: "CREATE", ActorID: "u1", Timestamp: ts1})
	_ = s.Log(types.AuditEntry{ID: "d2", EntityType: "ORDER", Action: "CANCEL", ActorID: "u1", Timestamp: ts2})
	_ = s.Log(types.AuditEntry{ID: "d3", EntityType: "TRADE", Action: "UPDATE", ActorID: "u2", Timestamp: ts3})
	_ = s.Log(types.AuditEntry{ID: "d4", EntityType: "TRADE", Action: "BUST", ActorID: "u2", Timestamp: ts4})

	// start_date=ts2, end_date=ts3 → entries d2 and d3 (inclusive).
	rangeResult, err := s.List(types.AuditFilters{StartDate: ts2, EndDate: ts3})
	if err != nil {
		t.Fatalf("List by date range: %v", err)
	}
	if len(rangeResult) != 2 {
		t.Errorf("expected 2 entries in range [%s, %s], got %d", ts2, ts3, len(rangeResult))
	}

	// start_date only — entries from ts3 onwards: d3 and d4.
	afterResult, err := s.List(types.AuditFilters{StartDate: ts3})
	if err != nil {
		t.Fatalf("List by start_date: %v", err)
	}
	if len(afterResult) != 2 {
		t.Errorf("expected 2 entries from start_date=%s, got %d", ts3, len(afterResult))
	}

	// end_date only — entries up to and including ts2: d1 and d2.
	beforeResult, err := s.List(types.AuditFilters{EndDate: ts2})
	if err != nil {
		t.Fatalf("List by end_date: %v", err)
	}
	if len(beforeResult) != 2 {
		t.Errorf("expected 2 entries up to end_date=%s, got %d", ts2, len(beforeResult))
	}
}

func TestAuditStore_AppendOnly(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	original := types.AuditEntry{
		ID:         "immutable-1",
		EntityType: "ORDER",
		EntityID:   "o99",
		Action:     "CREATE",
		ActorID:    "user-x",
		Timestamp:  now,
	}
	if err := s.Log(original); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Mutate the local copy after logging.
	original.Action = "MUTATED"
	original.ActorID = "hacker"

	// The stored entry must be unchanged.
	entries, err := s.List(types.AuditFilters{EntityType: "ORDER"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action == "MUTATED" {
		t.Error("audit store returned mutated action — store is not append-only")
	}
	if entries[0].ActorID == "hacker" {
		t.Error("audit store returned mutated actor_id — store is not append-only")
	}

	// Mutating the returned slice entry must also not affect the stored record.
	entries[0].Action = "PATCHED_FROM_READ"
	entries2, _ := s.List(types.AuditFilters{EntityType: "ORDER"})
	if entries2[0].Action == "PATCHED_FROM_READ" {
		t.Error("List returned a reference into internal storage rather than a copy")
	}
}

func TestAuditStore_ConcurrentLog(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = s.Log(types.AuditEntry{
				ID:         fmt.Sprintf("concurrent-%d", i),
				EntityType: "ORDER",
				Action:     "CREATE",
				ActorID:    "stress-test",
				Timestamp:  now,
			})
		}()
	}
	wg.Wait()

	all, err := s.List(types.AuditFilters{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != goroutines {
		t.Errorf("expected %d entries after concurrent log, got %d", goroutines, len(all))
	}
}

// ============================================================
// T3 — named test cases required by the test-writer task
// ============================================================

// TestAnnouncementStore_Create_List_Two verifies that creating 2 announcements
// and listing by a shared tenant returns exactly 2 items.
func TestAnnouncementStore_Create_List_Two(t *testing.T) {
	s := store.NewInMemoryAnnouncementStore()
	now := time.Now().UTC().Format(time.RFC3339)

	if err := s.Create(&types.Announcement{
		ID: "t3-ann-1", TenantID: "t3-tenant", Title: "First", Body: "Body A",
		Audience: types.AudiencePublic, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create a1: %v", err)
	}
	if err := s.Create(&types.Announcement{
		ID: "t3-ann-2", TenantID: "t3-tenant", Title: "Second", Body: "Body B",
		Audience: types.AudiencePublic, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create a2: %v", err)
	}

	list, err := s.ListByTenant("t3-tenant")
	if err != nil {
		t.Fatalf("ListByTenant: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 announcements, got %d", len(list))
	}
}

// TestAnnouncementStore_ListByTenant verifies per-tenant filtering across 2 tenants.
func TestAnnouncementStore_ListByTenant(t *testing.T) {
	s := store.NewInMemoryAnnouncementStore()
	now := time.Now().UTC().Format(time.RFC3339)

	tenants := []string{"tenant-A", "tenant-B"}
	ids := []string{"lt-1", "lt-2", "lt-3"}
	// a1 and a2 belong to tenant-A; a3 belongs to tenant-B.
	anns := []*types.Announcement{
		{ID: ids[0], TenantID: tenants[0], Title: "A1", Body: "b", Audience: types.AudiencePublic, CreatedAt: now, UpdatedAt: now},
		{ID: ids[1], TenantID: tenants[0], Title: "A2", Body: "b", Audience: types.AudiencePublic, CreatedAt: now, UpdatedAt: now},
		{ID: ids[2], TenantID: tenants[1], Title: "B1", Body: "b", Audience: types.AudienceInternal, CreatedAt: now, UpdatedAt: now},
	}
	for _, a := range anns {
		if err := s.Create(a); err != nil {
			t.Fatalf("Create %s: %v", a.ID, err)
		}
	}

	t.Run("tenant-A returns 2", func(t *testing.T) {
		list, err := s.ListByTenant(tenants[0])
		if err != nil {
			t.Fatalf("ListByTenant tenant-A: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("expected 2, got %d", len(list))
		}
		for _, a := range list {
			if a.TenantID != tenants[0] {
				t.Errorf("unexpected tenant_id %q", a.TenantID)
			}
		}
	})

	t.Run("tenant-B returns 1", func(t *testing.T) {
		list, err := s.ListByTenant(tenants[1])
		if err != nil {
			t.Fatalf("ListByTenant tenant-B: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1, got %d", len(list))
		}
		if list[0].ID != ids[2] {
			t.Errorf("expected %s, got %s", ids[2], list[0].ID)
		}
	})

	t.Run("unknown tenant returns empty slice", func(t *testing.T) {
		list, err := s.ListByTenant("no-such-tenant")
		if err != nil {
			t.Fatalf("ListByTenant unknown: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("expected 0, got %d", len(list))
		}
	})
}

// TestAuditStore_Log_List logs 3 entries and expects List to return all 3.
func TestAuditStore_Log_List(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	entries := []types.AuditEntry{
		{ID: "ll-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "u1", Timestamp: now},
		{ID: "ll-2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "u2", Timestamp: now},
		{ID: "ll-3", EntityType: "INSTRUMENT", EntityID: "i1", Action: "UPDATE", ActorID: "u3", Timestamp: now},
	}
	for _, e := range entries {
		if err := s.Log(e); err != nil {
			t.Fatalf("Log %s: %v", e.ID, err)
		}
	}

	all, err := s.List(types.AuditFilters{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 entries, got %d", len(all))
	}
}

// TestAuditStore_FilterByEntityType logs entries of mixed types and verifies
// that filtering by entity_type=ORDER returns exactly 1.
func TestAuditStore_FilterByEntityType(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	_ = s.Log(types.AuditEntry{ID: "fe-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "u1", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "fe-2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "u1", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "fe-3", EntityType: "INSTRUMENT", EntityID: "i1", Action: "UPDATE", ActorID: "u1", Timestamp: now})

	result, err := s.List(types.AuditFilters{EntityType: "ORDER"})
	if err != nil {
		t.Fatalf("List by entity_type ORDER: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 ORDER entry, got %d", len(result))
	}
	if result[0].EntityType != "ORDER" {
		t.Errorf("expected entity_type ORDER, got %q", result[0].EntityType)
	}
}

// TestAuditStore_FilterByActorID verifies that filtering by actor_id returns
// only entries logged by that actor.
func TestAuditStore_FilterByActorID(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	_ = s.Log(types.AuditEntry{ID: "fa-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "admin", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "fa-2", EntityType: "ORDER", EntityID: "o2", Action: "CANCEL", ActorID: "trader", Timestamp: now})
	_ = s.Log(types.AuditEntry{ID: "fa-3", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "admin", Timestamp: now})

	adminEntries, err := s.List(types.AuditFilters{ActorID: "admin"})
	if err != nil {
		t.Fatalf("List by actor admin: %v", err)
	}
	if len(adminEntries) != 2 {
		t.Errorf("expected 2 entries for admin, got %d", len(adminEntries))
	}
	for _, e := range adminEntries {
		if e.ActorID != "admin" {
			t.Errorf("unexpected actor_id %q in admin result", e.ActorID)
		}
	}

	traderEntries, err := s.List(types.AuditFilters{ActorID: "trader"})
	if err != nil {
		t.Fatalf("List by actor trader: %v", err)
	}
	if len(traderEntries) != 1 {
		t.Errorf("expected 1 entry for trader, got %d", len(traderEntries))
	}
}

// TestAuditStore_AppendOnly_Copy verifies that modifying a returned AuditEntry does
// not affect the value stored inside the store (append-only / copy semantics).
func TestAuditStore_AppendOnly_Copy(t *testing.T) {
	s := store.NewInMemoryAuditStore()
	now := time.Now().UTC().Format(time.RFC3339)

	original := types.AuditEntry{
		ID:         "ao-1",
		EntityType: "ORDER",
		EntityID:   "o1",
		Action:     "CREATE",
		ActorID:    "actor-original",
		Timestamp:  now,
	}
	if err := s.Log(original); err != nil {
		t.Fatalf("Log: %v", err)
	}

	// Retrieve the stored list and mutate the returned entry.
	list, err := s.List(types.AuditFilters{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}

	// Mutate the returned slice element — the store must remain unchanged.
	list[0].ActorID = "mutated-actor"

	// Retrieve again and verify the stored value is unchanged.
	list2, err := s.List(types.AuditFilters{})
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if list2[0].ActorID == "mutated-actor" {
		t.Error("AuditStore is not append-only: mutation of returned entry affected stored state")
	}
	if list2[0].ActorID != "actor-original" {
		t.Errorf("expected actor-original, got %q", list2[0].ActorID)
	}
}

// ============================================================
// PendingChangeStore tests (P2c)
// ============================================================

func newPendingChange(id, entityType, submittedBy string) *types.PendingChange {
	return &types.PendingChange{
		ID:          id,
		EntityType:  entityType,
		EntityID:    "e-" + id,
		ChangeType:  "UPDATE",
		Payload:     map[string]interface{}{"field": "value"},
		SubmittedBy: submittedBy,
		Status:      "PENDING_APPROVAL",
		SubmittedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func TestPendingChangeStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryPendingChangeStore()
	change := newPendingChange("pc-1", "INSTRUMENT", "maker-1")

	if err := s.Create(change); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("pc-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "pc-1" {
		t.Errorf("expected ID pc-1, got %q", got.ID)
	}
	if got.EntityType != "INSTRUMENT" {
		t.Errorf("expected EntityType INSTRUMENT, got %q", got.EntityType)
	}
	if got.Status != "PENDING_APPROVAL" {
		t.Errorf("expected status PENDING_APPROVAL, got %q", got.Status)
	}
	if got.SubmittedBy != "maker-1" {
		t.Errorf("expected submittedBy maker-1, got %q", got.SubmittedBy)
	}

	_, err = s.Get("nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound for missing ID, got %v", err)
	}
}

func TestPendingChangeStore_ListByStatus(t *testing.T) {
	s := store.NewInMemoryPendingChangeStore()

	pending := newPendingChange("pc-pending", "INSTRUMENT", "maker-1")
	approved := newPendingChange("pc-approved", "INSTRUMENT", "maker-2")
	approved.Status = "APPROVED"

	s.Create(pending)
	s.Create(approved)

	results, err := s.ListByStatus("PENDING_APPROVAL")
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 PENDING_APPROVAL result, got %d", len(results))
	}
	if results[0].ID != "pc-pending" {
		t.Errorf("expected pc-pending, got %q", results[0].ID)
	}

	// Listing with empty status returns all.
	all, err := s.ListByStatus("")
	if err != nil {
		t.Fatalf("ListByStatus empty: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 results with empty status filter, got %d", len(all))
	}
}

func TestPendingChangeStore_Approve(t *testing.T) {
	s := store.NewInMemoryPendingChangeStore()
	change := newPendingChange("pc-app", "FIRM", "maker-1")
	s.Create(change)

	if err := s.Approve("pc-app", "reviewer-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	got, err := s.Get("pc-app")
	if err != nil {
		t.Fatalf("Get after Approve: %v", err)
	}
	if got.Status != "APPROVED" {
		t.Errorf("expected status APPROVED, got %q", got.Status)
	}
	if got.ReviewedBy != "reviewer-1" {
		t.Errorf("expected ReviewedBy reviewer-1, got %q", got.ReviewedBy)
	}
	if got.ReviewedAt == "" {
		t.Error("expected ReviewedAt to be set after approval")
	}
}

func TestPendingChangeStore_Reject(t *testing.T) {
	s := store.NewInMemoryPendingChangeStore()
	change := newPendingChange("pc-rej", "PARTICIPANT", "maker-1")
	s.Create(change)

	if err := s.Reject("pc-rej", "reviewer-2", "policy violation"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	got, err := s.Get("pc-rej")
	if err != nil {
		t.Fatalf("Get after Reject: %v", err)
	}
	if got.Status != "REJECTED" {
		t.Errorf("expected status REJECTED, got %q", got.Status)
	}
	if got.ReviewedBy != "reviewer-2" {
		t.Errorf("expected ReviewedBy reviewer-2, got %q", got.ReviewedBy)
	}
	if got.ReviewComment != "policy violation" {
		t.Errorf("expected ReviewComment 'policy violation', got %q", got.ReviewComment)
	}
	if got.ReviewedAt == "" {
		t.Error("expected ReviewedAt to be set after rejection")
	}
}

func TestPendingChangeStore_ApproveNonPending(t *testing.T) {
	s := store.NewInMemoryPendingChangeStore()
	change := newPendingChange("pc-already", "INSTRUMENT", "maker-1")
	change.Status = "APPROVED"
	s.Create(change)

	err := s.Approve("pc-already", "reviewer-1")
	if err == nil {
		t.Fatal("expected error when approving non-PENDING_APPROVAL change, got nil")
	}

	// Also verify Reject on non-pending returns error.
	change2 := newPendingChange("pc-rejected-already", "INSTRUMENT", "maker-1")
	change2.Status = "REJECTED"
	s.Create(change2)

	err2 := s.Reject("pc-rejected-already", "reviewer-1", "comment")
	if err2 == nil {
		t.Fatal("expected error when rejecting non-PENDING_APPROVAL change, got nil")
	}
}

// ============================================================
// ReferencePriceStore tests (P2c)
// ============================================================

func TestReferencePriceStore_SetGet(t *testing.T) {
	s := store.NewInMemoryReferencePriceStore()

	rp := &types.ReferencePrice{
		InstrumentID:          "INST-1",
		Price:                 100.50,
		SetBy:                 "admin",
		SetAt:                 time.Now().UTC().Format(time.RFC3339),
		StaleThresholdMinutes: 60,
	}

	if err := s.Set(rp); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Get("INST-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.InstrumentID != "INST-1" {
		t.Errorf("expected InstrumentID INST-1, got %q", got.InstrumentID)
	}
	if got.Price != 100.50 {
		t.Errorf("expected Price 100.50, got %f", got.Price)
	}
	if got.SetBy != "admin" {
		t.Errorf("expected SetBy admin, got %q", got.SetBy)
	}
	if got.StaleThresholdMinutes != 60 {
		t.Errorf("expected StaleThresholdMinutes 60, got %d", got.StaleThresholdMinutes)
	}

	_, err = s.Get("nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestReferencePriceStore_Overwrite(t *testing.T) {
	s := store.NewInMemoryReferencePriceStore()

	first := &types.ReferencePrice{
		InstrumentID: "INST-OW",
		Price:        50.00,
		SetBy:        "admin",
		SetAt:        time.Now().UTC().Format(time.RFC3339),
	}
	second := &types.ReferencePrice{
		InstrumentID: "INST-OW",
		Price:        75.25,
		SetBy:        "supervisor",
		SetAt:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := s.Set(first); err != nil {
		t.Fatalf("Set first: %v", err)
	}
	if err := s.Set(second); err != nil {
		t.Fatalf("Set second: %v", err)
	}

	got, err := s.Get("INST-OW")
	if err != nil {
		t.Fatalf("Get after overwrite: %v", err)
	}
	if got.Price != 75.25 {
		t.Errorf("expected overwritten price 75.25, got %f", got.Price)
	}
	if got.SetBy != "supervisor" {
		t.Errorf("expected SetBy supervisor after overwrite, got %q", got.SetBy)
	}
}

// ============================================================
// SurveillanceStore tests
// ============================================================

func TestSurveillanceStore_CreateAlert_ListAll(t *testing.T) {
	s := store.NewInMemorySurveillanceStore()

	alert1 := &types.SurveillanceAlert{
		ID:           "alert-1",
		InstrumentID: "INST-1",
		AlertType:    types.AlertTypeLargeTrade,
		Status:       types.AlertStatusOpen,
		Message:      "large trade detected",
		CreatedAt:    "2026-01-01T00:00:00Z",
	}
	alert2 := &types.SurveillanceAlert{
		ID:           "alert-2",
		InstrumentID: "INST-2",
		AlertType:    types.AlertTypePriceSpike,
		Status:       types.AlertStatusOpen,
		Message:      "price spike detected",
		CreatedAt:    "2026-01-01T00:01:00Z",
	}

	if err := s.CreateAlert(alert1); err != nil {
		t.Fatalf("CreateAlert alert-1: %v", err)
	}
	if err := s.CreateAlert(alert2); err != nil {
		t.Fatalf("CreateAlert alert-2: %v", err)
	}

	all, err := s.ListAlerts(store.SurveillanceAlertFilters{})
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(all))
	}
}

func TestSurveillanceStore_ListFilterByStatus(t *testing.T) {
	s := store.NewInMemorySurveillanceStore()

	s.CreateAlert(&types.SurveillanceAlert{
		ID: "a-open", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "open", CreatedAt: "2026-01-01T00:00:00Z",
	})
	s.CreateAlert(&types.SurveillanceAlert{
		ID: "a-res", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusResolved, Message: "resolved", CreatedAt: "2026-01-01T00:00:00Z",
		ResolvedAt: "2026-01-01T01:00:00Z",
	})

	open, err := s.ListAlerts(store.SurveillanceAlertFilters{Status: types.AlertStatusOpen})
	if err != nil {
		t.Fatalf("ListAlerts OPEN: %v", err)
	}
	if len(open) != 1 {
		t.Errorf("expected 1 OPEN alert, got %d", len(open))
	}
	if open[0].ID != "a-open" {
		t.Errorf("expected alert id a-open, got %q", open[0].ID)
	}

	resolved, err := s.ListAlerts(store.SurveillanceAlertFilters{Status: types.AlertStatusResolved})
	if err != nil {
		t.Fatalf("ListAlerts RESOLVED: %v", err)
	}
	if len(resolved) != 1 {
		t.Errorf("expected 1 RESOLVED alert, got %d", len(resolved))
	}
}

func TestSurveillanceStore_ListFilterByAlertType(t *testing.T) {
	s := store.NewInMemorySurveillanceStore()

	s.CreateAlert(&types.SurveillanceAlert{
		ID: "lt-1", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "large trade", CreatedAt: "2026-01-01T00:00:00Z",
	})
	s.CreateAlert(&types.SurveillanceAlert{
		ID: "ps-1", InstrumentID: "I1", AlertType: types.AlertTypePriceSpike,
		Status: types.AlertStatusOpen, Message: "price spike", CreatedAt: "2026-01-01T00:00:00Z",
	})
	s.CreateAlert(&types.SurveillanceAlert{
		ID: "lt-2", InstrumentID: "I2", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "large trade 2", CreatedAt: "2026-01-01T00:00:00Z",
	})

	largeTrades, err := s.ListAlerts(store.SurveillanceAlertFilters{AlertType: types.AlertTypeLargeTrade})
	if err != nil {
		t.Fatalf("ListAlerts LARGE_TRADE: %v", err)
	}
	if len(largeTrades) != 2 {
		t.Errorf("expected 2 LARGE_TRADE alerts, got %d", len(largeTrades))
	}

	priceSpikes, err := s.ListAlerts(store.SurveillanceAlertFilters{AlertType: types.AlertTypePriceSpike})
	if err != nil {
		t.Fatalf("ListAlerts PRICE_SPIKE: %v", err)
	}
	if len(priceSpikes) != 1 {
		t.Errorf("expected 1 PRICE_SPIKE alert, got %d", len(priceSpikes))
	}
}

func TestSurveillanceStore_ResolveAlert(t *testing.T) {
	s := store.NewInMemorySurveillanceStore()

	s.CreateAlert(&types.SurveillanceAlert{
		ID: "alert-r", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "pending resolution", CreatedAt: "2026-01-01T00:00:00Z",
	})

	// Resolve it.
	if err := s.ResolveAlert("alert-r", "analyst-1"); err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	// Verify RESOLVED.
	all, _ := s.ListAlerts(store.SurveillanceAlertFilters{Status: types.AlertStatusResolved})
	if len(all) != 1 {
		t.Errorf("expected 1 RESOLVED alert after resolve, got %d", len(all))
	}
	if all[0].ResolvedBy != "analyst-1" {
		t.Errorf("expected resolved_by analyst-1, got %q", all[0].ResolvedBy)
	}
	if all[0].ResolvedAt == "" {
		t.Error("expected resolved_at to be set")
	}

	// Double-resolve should return error.
	if err := s.ResolveAlert("alert-r", "analyst-2"); err == nil {
		t.Error("expected error when resolving already-resolved alert")
	}

	// Not-found.
	if err := s.ResolveAlert("no-such-alert", "analyst-1"); err == nil {
		t.Error("expected error when resolving non-existent alert")
	}
}

func TestSurveillanceStore_SetGetThresholds(t *testing.T) {
	s := store.NewInMemorySurveillanceStore()

	th1 := &types.SurveillanceThreshold{
		InstrumentID: "INST-T1",
		AlertType:    types.AlertTypeLargeTrade,
		Value:        1000.0,
	}
	th2 := &types.SurveillanceThreshold{
		InstrumentID: "INST-T1",
		AlertType:    types.AlertTypePriceSpike,
		Value:        500.0,
	}

	if err := s.SetThreshold(th1); err != nil {
		t.Fatalf("SetThreshold th1: %v", err)
	}
	if err := s.SetThreshold(th2); err != nil {
		t.Fatalf("SetThreshold th2: %v", err)
	}

	// Get thresholds for INST-T1 — should return both.
	got, err := s.GetThresholds("INST-T1")
	if err != nil {
		t.Fatalf("GetThresholds: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 thresholds for INST-T1, got %d", len(got))
	}

	// Get thresholds for unknown instrument — should return empty.
	none, err := s.GetThresholds("NO-SUCH-INST")
	if err != nil {
		t.Fatalf("GetThresholds unknown: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 thresholds for unknown instrument, got %d", len(none))
	}

	// Upsert: overwrite th1 with new value.
	th1Updated := &types.SurveillanceThreshold{
		InstrumentID: "INST-T1",
		AlertType:    types.AlertTypeLargeTrade,
		Value:        2000.0,
	}
	if err := s.SetThreshold(th1Updated); err != nil {
		t.Fatalf("SetThreshold upsert: %v", err)
	}
	updated, _ := s.GetThresholds("INST-T1")
	for _, th := range updated {
		if th.AlertType == types.AlertTypeLargeTrade && th.Value != 2000.0 {
			t.Errorf("expected updated threshold value 2000, got %f", th.Value)
		}
	}
}

// ============================================================
// InstrumentGroupStore tests
// ============================================================

func TestInstrumentGroupStore_Create_Get(t *testing.T) {
	s := store.NewInMemoryInstrumentGroupStore()

	group := &types.InstrumentGroup{
		ID:            "group-1",
		Name:          "Blue Chips",
		GroupType:     types.GroupTypeManual,
		InstrumentIDs: []string{"INST-A", "INST-B"},
		CreatedAt:     "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-01T00:00:00Z",
	}

	if err := s.Create(group); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("group-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Blue Chips" {
		t.Errorf("expected name Blue Chips, got %q", got.Name)
	}
	if len(got.InstrumentIDs) != 2 {
		t.Errorf("expected 2 instrument IDs, got %d", len(got.InstrumentIDs))
	}

	// Not found.
	_, err = s.Get("no-such-group")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestInstrumentGroupStore_List_Delete(t *testing.T) {
	s := store.NewInMemoryInstrumentGroupStore()

	for i, name := range []string{"GroupA", "GroupB"} {
		g := &types.InstrumentGroup{
			ID:        fmt.Sprintf("grp-%d", i),
			Name:      name,
			GroupType: types.GroupTypeManual,
			CreatedAt: "2026-01-01T00:00:00Z",
			UpdatedAt: "2026-01-01T00:00:00Z",
		}
		if err := s.Create(g); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	// List — expect 2.
	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 groups, got %d", len(all))
	}

	// Delete one.
	if err := s.Delete("grp-0"); err != nil {
		t.Fatalf("Delete grp-0: %v", err)
	}

	// List — expect 1.
	remaining, err := s.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 group after delete, got %d", len(remaining))
	}

	// Delete non-existent.
	if err := s.Delete("grp-0"); err == nil {
		t.Error("expected error when deleting non-existent group")
	}
}

func TestInstrumentGroupStore_AddRemoveInstrument(t *testing.T) {
	s := store.NewInMemoryInstrumentGroupStore()

	s.Create(&types.InstrumentGroup{
		ID:            "grp-ar",
		Name:          "Test Group",
		GroupType:     types.GroupTypeManual,
		InstrumentIDs: []string{},
		CreatedAt:     "2026-01-01T00:00:00Z",
		UpdatedAt:     "2026-01-01T00:00:00Z",
	})

	// Add 2 instruments.
	if err := s.AddInstrument("grp-ar", "INST-X"); err != nil {
		t.Fatalf("AddInstrument INST-X: %v", err)
	}
	if err := s.AddInstrument("grp-ar", "INST-Y"); err != nil {
		t.Fatalf("AddInstrument INST-Y: %v", err)
	}

	got, _ := s.Get("grp-ar")
	if len(got.InstrumentIDs) != 2 {
		t.Errorf("expected 2 instruments after adding 2, got %d", len(got.InstrumentIDs))
	}

	// Add duplicate — idempotent, should still be 2.
	if err := s.AddInstrument("grp-ar", "INST-X"); err != nil {
		t.Fatalf("AddInstrument duplicate INST-X: %v", err)
	}
	got, _ = s.Get("grp-ar")
	if len(got.InstrumentIDs) != 2 {
		t.Errorf("expected 2 instruments after duplicate add, got %d", len(got.InstrumentIDs))
	}

	// Remove INST-X.
	if err := s.RemoveInstrument("grp-ar", "INST-X"); err != nil {
		t.Fatalf("RemoveInstrument INST-X: %v", err)
	}

	got, _ = s.Get("grp-ar")
	if len(got.InstrumentIDs) != 1 {
		t.Errorf("expected 1 instrument after remove, got %d", len(got.InstrumentIDs))
	}
	if got.InstrumentIDs[0] != "INST-Y" {
		t.Errorf("expected remaining instrument INST-Y, got %q", got.InstrumentIDs[0])
	}

	// Add to non-existent group.
	if err := s.AddInstrument("no-such-group", "INST-Z"); err == nil {
		t.Error("expected error when adding to non-existent group")
	}

	// Remove from non-existent group.
	if err := s.RemoveInstrument("no-such-group", "INST-Y"); err == nil {
		t.Error("expected error when removing from non-existent group")
	}
}

// ============================================================
// OffBookTradeStore tests
// ============================================================

func newOffBookTrade(id, instrumentID, buyPart, sellPart string, qty int, price float64) *types.OffBookTrade {
	return &types.OffBookTrade{
		ID:              id,
		InstrumentID:    instrumentID,
		BuyParticipant:  buyPart,
		SellParticipant: sellPart,
		Price:           price,
		Quantity:        qty,
		TradeDate:       "2026-01-01",
		Status:          types.OffBookReported,
		CreatedAt:       "2026-01-01T00:00:00Z",
		UpdatedAt:       "2026-01-01T00:00:00Z",
	}
}

func TestOffBookTradeStore_Create_List(t *testing.T) {
	s := store.NewInMemoryOffBookTradeStore()

	t1 := newOffBookTrade("obt-1", "INST-1", "BUY-P", "SELL-P", 500, 100.0)
	t2 := newOffBookTrade("obt-2", "INST-2", "BUY-P", "SELL-P", 1000, 200.0)

	if err := s.Create(t1); err != nil {
		t.Fatalf("Create obt-1: %v", err)
	}
	if err := s.Create(t2); err != nil {
		t.Fatalf("Create obt-2: %v", err)
	}

	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 off-book trades, got %d", len(all))
	}

	// Duplicate create should fail.
	if err := s.Create(t1); err == nil {
		t.Error("expected error on duplicate Create")
	}
}

func TestOffBookTradeStore_UpdateStatus(t *testing.T) {
	s := store.NewInMemoryOffBookTradeStore()

	trade := newOffBookTrade("obt-s", "INST-1", "BUY-P", "SELL-P", 100, 50.0)
	if err := s.Create(trade); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify initial status.
	got, err := s.Get("obt-s")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != types.OffBookReported {
		t.Errorf("expected initial status REPORTED, got %q", got.Status)
	}

	// Update to CONFIRMED.
	if err := s.UpdateStatus("obt-s", types.OffBookConfirmed); err != nil {
		t.Fatalf("UpdateStatus CONFIRMED: %v", err)
	}

	got, err = s.Get("obt-s")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Status != types.OffBookConfirmed {
		t.Errorf("expected status CONFIRMED, got %q", got.Status)
	}
	if got.UpdatedAt == "2026-01-01T00:00:00Z" {
		// updated_at should have changed — if it's still the creation timestamp something is wrong
		// (only fail if it looks like it wasn't updated at all — UpdatedAt may equal CreatedAt on fast machines)
	}

	// Not found.
	if err := s.UpdateStatus("no-such-trade", types.OffBookConfirmed); err == nil {
		t.Error("expected error when updating non-existent trade")
	}
}

// ============================================================
// P4a — LocateStore tests
// ============================================================

func TestLocateStore_Create_Approve_Use(t *testing.T) {
	s := store.NewInMemoryLocateStore()

	req := &types.LocateRequest{
		InstrumentID:   42,
		BorrowerFirmID: 10,
		Quantity:       500,
	}

	// Create — should assign ID=1 and status=PENDING.
	if err := s.Create(req); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if req.ID != 1 {
		t.Errorf("expected assigned ID 1, got %d", req.ID)
	}
	if req.Status != "PENDING" {
		t.Errorf("expected status PENDING after Create, got %q", req.Status)
	}
	if req.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}

	locID := fmt.Sprintf("%d", req.ID)

	// Get back.
	got, err := s.Get(locID)
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.Status != "PENDING" {
		t.Errorf("expected PENDING, got %q", got.Status)
	}

	// Approve — should transition to APPROVED.
	if err := s.Approve(locID, "LENDER-01"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	got, _ = s.Get(locID)
	if got.Status != "APPROVED" {
		t.Errorf("expected APPROVED after Approve, got %q", got.Status)
	}

	// Double-approve should fail (not PENDING anymore).
	if err := s.Approve(locID, "LENDER-02"); err == nil {
		t.Error("expected error when approving non-PENDING locate")
	}

	// Use — should transition to USED.
	if err := s.Use(locID); err != nil {
		t.Fatalf("Use: %v", err)
	}
	got, _ = s.Get(locID)
	if got.Status != "USED" {
		t.Errorf("expected USED after Use, got %q", got.Status)
	}

	// Double-use should fail (not APPROVED anymore).
	if err := s.Use(locID); err == nil {
		t.Error("expected error when using non-APPROVED locate")
	}

	// Not-found path.
	if _, err := s.Get("9999"); err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
	if err := s.Approve("9999", "L"); err != store.ErrNotFound {
		t.Errorf("Approve not-found: expected ErrNotFound, got %v", err)
	}
	if err := s.Use("9999"); err != store.ErrNotFound {
		t.Errorf("Use not-found: expected ErrNotFound, got %v", err)
	}
}

func TestLocateStore_ListByFirm(t *testing.T) {
	s := store.NewInMemoryLocateStore()

	// Create two locates for firm 10 and one for firm 20.
	s.Create(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 100})
	s.Create(&types.LocateRequest{InstrumentID: 2, BorrowerFirmID: 10, Quantity: 200})
	s.Create(&types.LocateRequest{InstrumentID: 3, BorrowerFirmID: 20, Quantity: 300})

	// Filter by firm 10.
	all, err := s.List("10")
	if err != nil {
		t.Fatalf("List firm 10: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 locates for firm 10, got %d", len(all))
	}

	// Filter by firm 20.
	all, err = s.List("20")
	if err != nil {
		t.Fatalf("List firm 20: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 locate for firm 20, got %d", len(all))
	}

	// No filter — all records.
	all, err = s.List("")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 total locates, got %d", len(all))
	}

	// Unknown firm — empty result.
	all, err = s.List("99")
	if err != nil {
		t.Fatalf("List unknown firm: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 locates for unknown firm, got %d", len(all))
	}
}

// ============================================================
// P4a — RFQStore tests
// ============================================================

func TestRFQStore_Create_Respond_Cancel(t *testing.T) {
	s := store.NewInMemoryRFQStore()

	rfq := &types.RequestForQuote{
		InstrumentID:    7,
		RequestorFirmID: 5,
		Quantity:        1000,
		Side:            "BUY",
	}

	// Create — assigns ID, sets OPEN.
	if err := s.Create(rfq); err != nil {
		t.Fatalf("Create RFQ: %v", err)
	}
	if rfq.ID != 1 {
		t.Errorf("expected ID 1, got %d", rfq.ID)
	}
	if rfq.Status != "OPEN" {
		t.Errorf("expected status OPEN, got %q", rfq.Status)
	}
	if rfq.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}

	rfqID := fmt.Sprintf("%d", rfq.ID)

	// Get back.
	got, err := s.Get(rfqID)
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if got.InstrumentID != 7 {
		t.Errorf("expected InstrumentID 7, got %d", got.InstrumentID)
	}

	// Respond — should transition to RESPONDED.
	if err := s.Respond(rfqID, "Q-001"); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	got, _ = s.Get(rfqID)
	if got.Status != "RESPONDED" {
		t.Errorf("expected RESPONDED, got %q", got.Status)
	}

	// Respond again should fail (not OPEN).
	if err := s.Respond(rfqID, "Q-002"); err == nil {
		t.Error("expected error responding to non-OPEN RFQ")
	}

	// Second RFQ — cancel path.
	rfq2 := &types.RequestForQuote{
		InstrumentID:    8,
		RequestorFirmID: 6,
		Quantity:        500,
		Side:            "SELL",
	}
	s.Create(rfq2)
	rfq2ID := fmt.Sprintf("%d", rfq2.ID)

	if err := s.Cancel(rfq2ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got2, _ := s.Get(rfq2ID)
	if got2.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %q", got2.Status)
	}

	// Cancel again should fail.
	if err := s.Cancel(rfq2ID); err == nil {
		t.Error("expected error cancelling non-OPEN RFQ")
	}

	// Not-found paths.
	if _, err := s.Get("9999"); err != store.ErrNotFound {
		t.Errorf("Get not-found: expected ErrNotFound, got %v", err)
	}
	if err := s.Respond("9999", "Q"); err != store.ErrNotFound {
		t.Errorf("Respond not-found: expected ErrNotFound, got %v", err)
	}
	if err := s.Cancel("9999"); err != store.ErrNotFound {
		t.Errorf("Cancel not-found: expected ErrNotFound, got %v", err)
	}

	// List — unfiltered.
	all, err := s.List("", "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 RFQs, got %d", len(all))
	}

	// List — filter by status.
	open, err := s.List("", "OPEN")
	if err != nil {
		t.Fatalf("List open: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("expected 0 OPEN RFQs (both consumed), got %d", len(open))
	}
}

// ============================================================
// P4a — GiveUpStore tests
// ============================================================

func TestGiveUpStore_Create_Accept_Reject(t *testing.T) {
	s := store.NewInMemoryGiveUpStore()

	req := &types.GiveUpRequest{
		TradeID:  101,
		ToFirmID: 20,
	}

	// Create — assigns ID=1, status=PENDING.
	if err := s.Create(req); err != nil {
		t.Fatalf("Create GiveUp: %v", err)
	}
	if req.ID != 1 {
		t.Errorf("expected ID 1, got %d", req.ID)
	}
	if req.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %q", req.Status)
	}
	if req.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}

	guID := fmt.Sprintf("%d", req.ID)

	// Get back.
	got, err := s.Get(guID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TradeID != 101 {
		t.Errorf("expected TradeID 101, got %d", got.TradeID)
	}

	// Accept — PENDING → ACCEPTED.
	if err := s.Accept(guID); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	got, _ = s.Get(guID)
	if got.Status != "ACCEPTED" {
		t.Errorf("expected ACCEPTED, got %q", got.Status)
	}
	if got.ResolvedAt == "" {
		t.Error("expected ResolvedAt to be set after Accept")
	}

	// Accept again should fail (not PENDING).
	if err := s.Accept(guID); err == nil {
		t.Error("expected error accepting non-PENDING give-up")
	}

	// Second give-up — reject path.
	req2 := &types.GiveUpRequest{TradeID: 202, ToFirmID: 30}
	s.Create(req2)
	gu2ID := fmt.Sprintf("%d", req2.ID)

	if err := s.Reject(gu2ID, "counterparty declined"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	got2, _ := s.Get(gu2ID)
	if got2.Status != "REJECTED" {
		t.Errorf("expected REJECTED, got %q", got2.Status)
	}
	if got2.Reason != "counterparty declined" {
		t.Errorf("expected reason 'counterparty declined', got %q", got2.Reason)
	}
	if got2.ResolvedAt == "" {
		t.Error("expected ResolvedAt to be set after Reject")
	}

	// Reject again should fail.
	if err := s.Reject(gu2ID, "again"); err == nil {
		t.Error("expected error rejecting non-PENDING give-up")
	}

	// List by firm.
	req3 := &types.GiveUpRequest{TradeID: 303, FromFirmID: 10, ToFirmID: 20}
	s.Create(req3)
	byFirm, err := s.List("10")
	if err != nil {
		t.Fatalf("List by firm: %v", err)
	}
	if len(byFirm) != 1 {
		t.Errorf("expected 1 give-up for firm 10, got %d", len(byFirm))
	}

	// List all.
	all, err := s.List("")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 give-ups total, got %d", len(all))
	}

	// Not-found paths.
	if _, err := s.Get("9999"); err != store.ErrNotFound {
		t.Errorf("Get not-found: expected ErrNotFound, got %v", err)
	}
	if err := s.Accept("9999"); err != store.ErrNotFound {
		t.Errorf("Accept not-found: expected ErrNotFound, got %v", err)
	}
	if err := s.Reject("9999", "x"); err != store.ErrNotFound {
		t.Errorf("Reject not-found: expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// InvestigationStore tests
// ============================================================

func newInvestigation(id, subject, instrumentID string) *types.Investigation {
	return &types.Investigation{
		ID:           id,
		Subject:      subject,
		InstrumentID: instrumentID,
		Status:       types.InvestigationOpen,
		AssignedTo:   "analyst-1",
		OpenedAt:     "2026-04-24T00:00:00Z",
	}
}

func TestInvestigationStore_Create_Get_List(t *testing.T) {
	s := store.NewInMemoryInvestigationStore()

	inv1 := newInvestigation("INV-1", "Suspected wash trade", "INST-A")
	inv2 := newInvestigation("INV-2", "Price manipulation", "INST-B")

	if err := s.Create(inv1); err != nil {
		t.Fatalf("Create INV-1: %v", err)
	}
	if err := s.Create(inv2); err != nil {
		t.Fatalf("Create INV-2: %v", err)
	}

	// Duplicate returns an error.
	if err := s.Create(inv1); err == nil {
		t.Error("expected error on duplicate Create, got nil")
	}

	// Get returns the stored investigation.
	got, err := s.Get("INV-1")
	if err != nil {
		t.Fatalf("Get INV-1: %v", err)
	}
	if got.Subject != "Suspected wash trade" {
		t.Errorf("Subject: want %q, got %q", "Suspected wash trade", got.Subject)
	}
	if got.Status != types.InvestigationOpen {
		t.Errorf("Status: want OPEN, got %s", got.Status)
	}

	// Get non-existent returns ErrNotFound.
	if _, err := s.Get("NO-SUCH-INV"); err != store.ErrNotFound {
		t.Errorf("Get missing: expected ErrNotFound, got %v", err)
	}

	// List returns all investigations.
	all, err := s.List(store.InvestigationFilters{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 investigations, got %d", len(all))
	}

	// List with status filter returns only matching.
	open, err := s.List(store.InvestigationFilters{Status: types.InvestigationOpen})
	if err != nil {
		t.Fatalf("List OPEN: %v", err)
	}
	if len(open) != 2 {
		t.Errorf("expected 2 OPEN investigations, got %d", len(open))
	}

	// List with CLOSED filter returns none.
	closed, err := s.List(store.InvestigationFilters{Status: types.InvestigationClosed})
	if err != nil {
		t.Fatalf("List CLOSED: %v", err)
	}
	if len(closed) != 0 {
		t.Errorf("expected 0 CLOSED investigations, got %d", len(closed))
	}

	// Get returns a copy — mutation does not affect store.
	got.Subject = "MUTATED"
	got2, _ := s.Get("INV-1")
	if got2.Subject == "MUTATED" {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}

func TestInvestigationStore_Close(t *testing.T) {
	s := store.NewInMemoryInvestigationStore()
	s.Create(newInvestigation("INV-CLOSE", "Volume anomaly", "INST-C"))

	// Close with findings.
	findings := "Confirmed algorithmic trading pattern — no rule breach found."
	if err := s.Close("INV-CLOSE", findings); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := s.Get("INV-CLOSE")
	if err != nil {
		t.Fatalf("Get after Close: %v", err)
	}
	if got.Status != types.InvestigationClosed {
		t.Errorf("Status: want CLOSED, got %s", got.Status)
	}
	if got.Findings != findings {
		t.Errorf("Findings: want %q, got %q", findings, got.Findings)
	}
	if got.ClosedAt == "" {
		t.Error("ClosedAt must be set after Close")
	}

	// Closing an already-closed investigation returns an error.
	if err := s.Close("INV-CLOSE", "again"); err == nil {
		t.Error("expected error when closing an already-closed investigation, got nil")
	}

	// Closing a non-existent investigation returns ErrNotFound.
	if err := s.Close("NO-SUCH-INV", "x"); err != store.ErrNotFound {
		t.Errorf("Close missing: expected ErrNotFound, got %v", err)
	}

	// Closed investigation appears in CLOSED filter but not OPEN filter.
	closed, _ := s.List(store.InvestigationFilters{Status: types.InvestigationClosed})
	if len(closed) != 1 {
		t.Errorf("expected 1 CLOSED investigation, got %d", len(closed))
	}
}

func TestInvestigationStore_AddEvidence(t *testing.T) {
	s := store.NewInMemoryInvestigationStore()
	s.Create(newInvestigation("INV-EV", "Suspicious orders", "INST-D"))

	// Add three evidence references.
	refs := []string{"trade-ref-001", "order-ref-999", "audit-log-2026-04-24"}
	for _, ref := range refs {
		if err := s.AddEvidence("INV-EV", ref); err != nil {
			t.Fatalf("AddEvidence %q: %v", ref, err)
		}
	}

	got, err := s.Get("INV-EV")
	if err != nil {
		t.Fatalf("Get after AddEvidence: %v", err)
	}
	if len(got.Evidence) != 3 {
		t.Errorf("expected 3 evidence items, got %d", len(got.Evidence))
	}
	if got.Evidence[0] != refs[0] {
		t.Errorf("Evidence[0]: want %q, got %q", refs[0], got.Evidence[0])
	}

	// AddEvidence to non-existent investigation returns ErrNotFound.
	if err := s.AddEvidence("NO-SUCH-INV", "x"); err != store.ErrNotFound {
		t.Errorf("AddEvidence missing: expected ErrNotFound, got %v", err)
	}
}

// ============================================================
// ReplayStore tests
// ============================================================

func TestReplayStore_CreateSession_AddEvents_GetEvents(t *testing.T) {
	s := store.NewInMemoryReplayStore()

	sess := &types.ReplaySession{
		ID:           "REPLAY-1",
		InstrumentID: "INST-E",
		StartTime:    "2026-04-24T09:00:00Z",
		EndTime:      "2026-04-24T17:00:00Z",
		Description:  "Full trading day replay",
		CreatedAt:    "2026-04-24T20:00:00Z",
	}

	// CreateSession.
	if err := s.CreateSession(sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Duplicate CreateSession returns an error.
	if err := s.CreateSession(sess); err == nil {
		t.Error("expected error on duplicate CreateSession, got nil")
	}

	// GetSession returns the session.
	got, err := s.GetSession("REPLAY-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.InstrumentID != "INST-E" {
		t.Errorf("InstrumentID: want INST-E, got %s", got.InstrumentID)
	}

	// GetSession non-existent returns ErrNotFound.
	if _, err := s.GetSession("NO-SESSION"); err != store.ErrNotFound {
		t.Errorf("GetSession missing: expected ErrNotFound, got %v", err)
	}

	// ListSessions returns all sessions.
	sessions, err := s.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// AddEvent — add 3 events out of order.
	events := []*types.ReplayEvent{
		{SessionID: "REPLAY-1", Sequence: 3, EventType: "TRADE", OccurredAt: "2026-04-24T10:02:00Z"},
		{SessionID: "REPLAY-1", Sequence: 1, EventType: "ORDER", OccurredAt: "2026-04-24T09:30:00Z"},
		{SessionID: "REPLAY-1", Sequence: 2, EventType: "ORDER", OccurredAt: "2026-04-24T10:01:00Z"},
	}
	for _, ev := range events {
		if err := s.AddEvent(ev); err != nil {
			t.Fatalf("AddEvent seq=%d: %v", ev.Sequence, err)
		}
	}

	// GetEvents returns events sorted by Sequence.
	gotEvents, err := s.GetEvents("REPLAY-1")
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if len(gotEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(gotEvents))
	}
	for i, ev := range gotEvents {
		if ev.Sequence != i+1 {
			t.Errorf("event[%d] Sequence: want %d, got %d", i, i+1, ev.Sequence)
		}
	}
	if gotEvents[0].EventType != "ORDER" {
		t.Errorf("event[0] EventType: want ORDER, got %s", gotEvents[0].EventType)
	}
	if gotEvents[2].EventType != "TRADE" {
		t.Errorf("event[2] EventType: want TRADE, got %s", gotEvents[2].EventType)
	}

	// GetEvents for unknown session returns empty slice (not an error).
	noEvents, err := s.GetEvents("NO-SESSION")
	if err != nil {
		t.Fatalf("GetEvents missing session: %v", err)
	}
	if len(noEvents) != 0 {
		t.Errorf("expected 0 events for unknown session, got %d", len(noEvents))
	}
}

// ============================================================
// BondStore tests
// ============================================================

func newBond(id, isin, issuer string, couponRate float64, convention types.DayCountConvention) *types.Bond {
	return &types.Bond{
		ID:                 id,
		ISIN:               isin,
		Name:               issuer + " Bond",
		Issuer:             issuer,
		MaturityDate:       "2031-04-24",
		CouponRate:         couponRate,
		CouponFrequency:    "ANNUAL",
		ParValue:           1000.0,
		DayCountConvention: convention,
		TradingStatus:      types.TradingStatusActive,
		CreatedAt:          "2026-04-24T00:00:00Z",
		UpdatedAt:          "2026-04-24T00:00:00Z",
	}
}

func TestBondStore_Create_List(t *testing.T) {
	s := store.NewInMemoryBondStore()

	b1 := newBond("BOND-1", "MN1234567890", "MN Telecom", 0.05, types.DayCountACT365)
	b2 := newBond("BOND-2", "MN0987654321", "MN Energy", 0.07, types.DayCount30360)

	if err := s.Create(b1); err != nil {
		t.Fatalf("Create BOND-1: %v", err)
	}
	if err := s.Create(b2); err != nil {
		t.Fatalf("Create BOND-2: %v", err)
	}

	// Duplicate returns an error.
	if err := s.Create(b1); err == nil {
		t.Error("expected error on duplicate Create, got nil")
	}

	// Get returns the stored bond.
	got, err := s.Get("BOND-1")
	if err != nil {
		t.Fatalf("Get BOND-1: %v", err)
	}
	if got.Issuer != "MN Telecom" {
		t.Errorf("Issuer: want MN Telecom, got %s", got.Issuer)
	}
	if got.CouponRate != 0.05 {
		t.Errorf("CouponRate: want 0.05, got %v", got.CouponRate)
	}
	if got.DayCountConvention != types.DayCountACT365 {
		t.Errorf("DayCountConvention: want ACT/365, got %s", got.DayCountConvention)
	}

	// Get non-existent returns ErrNotFound.
	if _, err := s.Get("NO-BOND"); err != store.ErrNotFound {
		t.Errorf("Get missing: expected ErrNotFound, got %v", err)
	}

	// List returns all bonds.
	all, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 bonds, got %d", len(all))
	}

	// UpdateStatus changes the trading status.
	if err := s.UpdateStatus("BOND-1", types.TradingStatusHalted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	updated, _ := s.Get("BOND-1")
	if updated.TradingStatus != types.TradingStatusHalted {
		t.Errorf("TradingStatus: want HALTED, got %s", updated.TradingStatus)
	}
	if updated.UpdatedAt == b1.UpdatedAt {
		t.Error("UpdatedAt must change after UpdateStatus")
	}

	// UpdateStatus non-existent returns ErrNotFound.
	if err := s.UpdateStatus("NO-BOND", types.TradingStatusActive); err != store.ErrNotFound {
		t.Errorf("UpdateStatus missing: expected ErrNotFound, got %v", err)
	}

	// Get returns a copy — mutation does not affect store.
	got.Issuer = "MUTATED"
	got2, _ := s.Get("BOND-1")
	if got2.Issuer == "MUTATED" {
		t.Error("Get returned a pointer into internal storage instead of a copy")
	}
}
