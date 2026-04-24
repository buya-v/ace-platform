package store_test

import (
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
