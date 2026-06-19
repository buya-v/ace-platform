package engine

import (
	"errors"
	"net/http"
	"testing"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

func shortSellOrder(locateID string) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:           "O-1",
		InstrumentID: "1",
		FirmID:       "10",
		Side:         types.OrderSideShortSell,
		OrderType:    types.OrderTypeLimit,
		Quantity:     5,
		Price:        10,
		LocateID:     locateID,
	}
}

func TestShortSellEngine_NonShortSellIsNoOp(t *testing.T) {
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), store.NewInMemoryLocateStore())
	order := &types.SecurityOrder{Side: types.OrderSideBuy}
	if err := e.EvaluateOrder(order, &types.Instrument{}); err != nil {
		t.Fatalf("BUY order should be a no-op, got %v", err)
	}
	// Nil order is also a no-op.
	if err := e.EvaluateOrder(nil, nil); err != nil {
		t.Fatalf("nil order should be a no-op, got %v", err)
	}
}

func TestShortSellEngine_Restricted(t *testing.T) {
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), store.NewInMemoryLocateStore())
	inst := &types.Instrument{ID: "1", ShortSellRestricted: true}
	err := e.EvaluateOrder(shortSellOrder("1"), inst)
	assertShortSellCode(t, err, CodeShortSellRestricted)
}

func TestShortSellEngine_LocateRequired(t *testing.T) {
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), store.NewInMemoryLocateStore())
	inst := &types.Instrument{ID: "1"}
	err := e.EvaluateOrder(shortSellOrder(""), inst)
	assertShortSellCode(t, err, CodeLocateRequired)
}

func TestShortSellEngine_NilLocateStore_PresenceOnly(t *testing.T) {
	// With no locate store, presence of locate_id is sufficient and accepted.
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), nil)
	inst := &types.Instrument{ID: "1"}
	if err := e.EvaluateOrder(shortSellOrder("anything"), inst); err != nil {
		t.Fatalf("expected acceptance with nil locate store, got %v", err)
	}
	// But missing locate_id still fails.
	if err := e.EvaluateOrder(shortSellOrder(""), inst); err == nil {
		t.Fatal("expected LOCATE_REQUIRED even with nil store")
	}
}

func TestShortSellEngine_InvalidLocate(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), ls)
	inst := &types.Instrument{ID: "1"}

	// Unknown locate id.
	err := e.EvaluateOrder(shortSellOrder("999"), inst)
	assertShortSellCode(t, err, CodeInvalidLocate)

	// Create a PENDING (not approved) locate -> INVALID_LOCATE.
	ls.Create(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 100})
	err = e.EvaluateOrder(shortSellOrder("1"), inst)
	assertShortSellCode(t, err, CodeInvalidLocate)
}

func TestShortSellEngine_HappyPathConsumesLocate(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), ls)
	inst := &types.Instrument{ID: "1"}

	ls.Create(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 100})
	if err := ls.Approve("1", "LENDER"); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	if err := e.EvaluateOrder(shortSellOrder("1"), inst); err != nil {
		t.Fatalf("expected acceptance, got %v", err)
	}

	// Locate should now be USED — a second short sell with the same locate fails.
	if err := e.EvaluateOrder(shortSellOrder("1"), inst); err == nil {
		t.Fatal("expected reuse of consumed locate to fail")
	}
	loc, _ := ls.Get("1")
	if loc.Status != LocateStatusUsed {
		t.Errorf("expected locate USED, got %q", loc.Status)
	}
}

func TestShortSellEngine_BorrowerMismatchRejected(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), ls)
	inst := &types.Instrument{ID: "1"}

	// Locate belongs to firm 99, order is from firm 10.
	ls.Create(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 99, Quantity: 100})
	ls.Approve("1", "LENDER")

	err := e.EvaluateOrder(shortSellOrder("1"), inst)
	assertShortSellCode(t, err, CodeInvalidLocate)
}

func TestShortSellEngine_InsufficientQuantityRejected(t *testing.T) {
	ls := store.NewInMemoryLocateStore()
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), ls)
	inst := &types.Instrument{ID: "1"}

	order := shortSellOrder("1")
	order.Quantity = 1000 // exceeds locate coverage of 100
	ls.Create(&types.LocateRequest{InstrumentID: 1, BorrowerFirmID: 10, Quantity: 100})
	ls.Approve("1", "LENDER")

	assertShortSellCode(t, e.EvaluateOrder(order, inst), CodeInvalidLocate)
}

func TestShortSellEngine_LocateEngineAccessor(t *testing.T) {
	e := NewShortSellEngine(store.NewInMemoryInstrumentStore(), store.NewInMemoryLocateStore())
	if e.LocateEngine() == nil {
		t.Fatal("expected non-nil LocateEngine")
	}
}

func TestShortSellError_Surface(t *testing.T) {
	err := newShortSellError(CodeLocateRequired, "need a locate")
	if err.Error() != "LOCATE_REQUIRED: need a locate" {
		t.Errorf("unexpected Error(): %q", err.Error())
	}
	if err.HTTPStatus() != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", err.HTTPStatus())
	}
}

func TestAtoiSafe(t *testing.T) {
	cases := map[string]int{"": 0, "abc": 0, "10": 10, "-3": -3, "007": 7}
	for in, want := range cases {
		if got := atoiSafe(in); got != want {
			t.Errorf("atoiSafe(%q) = %d, want %d", in, got, want)
		}
	}
}

func assertShortSellCode(t *testing.T, err error, code string) {
	t.Helper()
	var ssErr *ShortSellError
	if !errors.As(err, &ssErr) {
		t.Fatalf("expected *ShortSellError with code %q, got %v", code, err)
	}
	if ssErr.Code != code {
		t.Fatalf("expected code %q, got %q (%s)", code, ssErr.Code, ssErr.Message)
	}
}
