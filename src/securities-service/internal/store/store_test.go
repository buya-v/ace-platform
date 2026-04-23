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
