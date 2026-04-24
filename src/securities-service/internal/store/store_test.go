package store_test

import (
	"sync"
	"testing"

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
