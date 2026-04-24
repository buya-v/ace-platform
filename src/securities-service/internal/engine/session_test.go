// Package engine_test — session manager tests for SessionManager.
package engine_test

import (
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ---- session test helpers --------------------------------------------------

const (
	sessionInstID = "EQUITY-SESSION"
	sessionTenant = "mse-equities"
)

// sessionTestEnv bundles a SessionManager with the underlying in-memory stores
// and engines needed to drive it.
type sessionTestEnv struct {
	sm  *engine.SessionManager
	me  *engine.MatchingEngine
	ae  *engine.AuctionEngine
	ord *store.InMemoryOrderStore
	trd *store.InMemoryTradeStore
	pos *store.InMemoryPositionStore
}

// setupSessionTest creates fresh stores + engines + a SessionManager.
// It also registers an ACTIVE EQUITY instrument against the instrument store.
func setupSessionTest(t *testing.T) *sessionTestEnv {
	t.Helper()

	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()

	// Register the test instrument.
	createInstrument(t, &testStores{inst: inst, ord: ord, trd: trd, pos: pos}, sessionInstID, types.TradingStatusActive)

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)

	return &sessionTestEnv{sm: sm, me: me, ae: ae, ord: ord, trd: trd, pos: pos}
}

// sessionOrder builds a PENDING limit order for the session instrument.
func sessionOrder(id, participantID string, side types.OrderSide, qty int, price float64) *types.SecurityOrder {
	return &types.SecurityOrder{
		ID:            id,
		InstrumentID:  sessionInstID,
		ParticipantID: participantID,
		Side:          side,
		OrderType:     types.OrderTypeLimit,
		Quantity:      qty,
		Price:         price,
		Status:        types.OrderStatusPending,
		TimeInForce:   types.TimeInForceGTC,
		CreatedAt:     ts(0),
		UpdatedAt:     ts(0),
	}
}

// ---- session tests ---------------------------------------------------------

// TestSession_DefaultClosed: GetSession for unknown instrument → SESSION_CLOSED.
func TestSession_DefaultClosed(t *testing.T) {
	env := setupSessionTest(t)

	sess := env.sm.GetSession("INSTRUMENT-UNKNOWN")
	if sess != types.SessionClosed {
		t.Errorf("unknown instrument session: want SESSION_CLOSED, got %s", sess)
	}
}

// TestSession_ValidTransitions: full lifecycle CLOSED→PRE_OPEN→CONTINUOUS→CLOSING_AUCTION→CLOSED.
func TestSession_ValidTransitions(t *testing.T) {
	env := setupSessionTest(t)

	transitions := []struct {
		to   types.MarketSession
		name string
	}{
		{types.SessionPreOpen, "CLOSED→PRE_OPEN"},
		{types.SessionContinuous, "PRE_OPEN→CONTINUOUS"},
		{types.SessionClosingAuction, "CONTINUOUS→CLOSING_AUCTION"},
		{types.SessionClosed, "CLOSING_AUCTION→CLOSED"},
	}

	for _, step := range transitions {
		_, err := env.sm.TransitionTo(sessionInstID, sessionTenant, step.to)
		if err != nil {
			t.Errorf("TransitionTo %s failed: %v", step.name, err)
			break
		}
		got := env.sm.GetSession(sessionInstID)
		if got != step.to {
			t.Errorf("after %s: GetSession=%s, want %s", step.name, got, step.to)
		}
	}
}

// TestSession_InvalidTransition: CLOSED→CONTINUOUS is not a valid transition.
func TestSession_InvalidTransition(t *testing.T) {
	env := setupSessionTest(t)

	_, err := env.sm.TransitionTo(sessionInstID, sessionTenant, types.SessionContinuous)
	if err == nil {
		t.Error("expected error for CLOSED→CONTINUOUS transition, got nil")
	}

	// State should remain CLOSED.
	if got := env.sm.GetSession(sessionInstID); got != types.SessionClosed {
		t.Errorf("session after failed transition: want CLOSED, got %s", got)
	}
}

// TestSession_InvalidTransition_All: verify all disallowed transitions.
func TestSession_InvalidTransition_All(t *testing.T) {
	type step struct {
		from types.MarketSession
		to   types.MarketSession
	}
	invalid := []step{
		{types.SessionClosed, types.SessionContinuous},
		{types.SessionClosed, types.SessionClosingAuction},
		{types.SessionClosed, types.SessionClosed},
		{types.SessionPreOpen, types.SessionClosed},
		{types.SessionPreOpen, types.SessionPreOpen},
		{types.SessionPreOpen, types.SessionClosingAuction},
		{types.SessionContinuous, types.SessionPreOpen},
		{types.SessionContinuous, types.SessionClosed},
		{types.SessionContinuous, types.SessionContinuous},
		{types.SessionClosingAuction, types.SessionPreOpen},
		{types.SessionClosingAuction, types.SessionContinuous},
		{types.SessionClosingAuction, types.SessionClosingAuction},
	}

	for _, tc := range invalid {
		// Use a fresh manager per case to set starting state cleanly.
		env := setupSessionTest(t)
		instrID := "inst-" + string(tc.from) + "-" + string(tc.to)

		// Register a fresh instrument for this test case.
		inst2 := store.NewInMemoryInstrumentStore()
		ord2 := store.NewInMemoryOrderStore()
		trd2 := store.NewInMemoryTradeStore()
		pos2 := store.NewInMemoryPositionStore()
		createInstrument(t, &testStores{inst: inst2, ord: ord2, trd: trd2, pos: pos2}, instrID, types.TradingStatusActive)
		me2 := engine.NewMatchingEngine(inst2, ord2, trd2, pos2, nil, nil, nil)
		ae2 := engine.NewAuctionEngine(ord2, trd2, pos2, nil)
		sm2 := engine.NewSessionManager(ae2, me2)
		_ = env

		// Advance to `from` state through valid transitions.
		validPath := validPathTo(tc.from)
		for _, step := range validPath {
			if _, err := sm2.TransitionTo(instrID, sessionTenant, step); err != nil {
				t.Fatalf("setup for %s→%s: advance to %s failed: %v", tc.from, tc.to, step, err)
			}
		}

		_, err := sm2.TransitionTo(instrID, sessionTenant, tc.to)
		if err == nil {
			t.Errorf("expected error for %s→%s, got nil", tc.from, tc.to)
		}
	}
}

// validPathTo returns the sequence of transitions needed to reach target from CLOSED.
func validPathTo(target types.MarketSession) []types.MarketSession {
	switch target {
	case types.SessionClosed:
		return nil // Already there.
	case types.SessionPreOpen:
		return []types.MarketSession{types.SessionPreOpen}
	case types.SessionContinuous:
		return []types.MarketSession{types.SessionPreOpen, types.SessionContinuous}
	case types.SessionClosingAuction:
		return []types.MarketSession{types.SessionPreOpen, types.SessionContinuous, types.SessionClosingAuction}
	}
	return nil
}

// TestSession_PreOpenCollectsOrders: during PRE_OPEN, SubmitOrder → CollectOrder → 0 trades.
func TestSession_PreOpenCollectsOrders(t *testing.T) {
	env := setupSessionTest(t)

	// Move to PRE_OPEN.
	if _, err := env.sm.TransitionTo(sessionInstID, sessionTenant, types.SessionPreOpen); err != nil {
		t.Fatalf("transition to PRE_OPEN: %v", err)
	}

	buy := sessionOrder("pre-buy", "buyer", types.OrderSideBuy, 50, 100.0)
	trades, err := env.sm.SubmitOrder(buy, sessionTenant)
	if err != nil {
		t.Fatalf("SubmitOrder during PRE_OPEN: %v", err)
	}

	// In auction mode, no trades are returned immediately.
	if len(trades) != 0 {
		t.Errorf("expected 0 trades during PRE_OPEN, got %d", len(trades))
	}

	// Order must be stored as PENDING.
	stored, err := env.ord.Get(buy.ID)
	if err != nil {
		t.Fatalf("order not in store after CollectOrder: %v", err)
	}
	if stored.Status != types.OrderStatusPending {
		t.Errorf("order status: want PENDING, got %s", stored.Status)
	}
}

// TestSession_ClosingAuctionCollectsOrders: during CLOSING_AUCTION, SubmitOrder → CollectOrder → 0 trades.
func TestSession_ClosingAuctionCollectsOrders(t *testing.T) {
	env := setupSessionTest(t)

	// Advance to CLOSING_AUCTION.
	steps := []types.MarketSession{
		types.SessionPreOpen,
		types.SessionContinuous,
		types.SessionClosingAuction,
	}
	for _, step := range steps {
		if _, err := env.sm.TransitionTo(sessionInstID, sessionTenant, step); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}

	sell := sessionOrder("close-sell", "seller", types.OrderSideSell, 30, 90.0)
	trades, err := env.sm.SubmitOrder(sell, sessionTenant)
	if err != nil {
		t.Fatalf("SubmitOrder during CLOSING_AUCTION: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("expected 0 trades during CLOSING_AUCTION, got %d", len(trades))
	}

	stored, err := env.ord.Get(sell.ID)
	if err != nil {
		t.Fatalf("order not in store after CollectOrder: %v", err)
	}
	if stored.Status != types.OrderStatusPending {
		t.Errorf("order status: want PENDING, got %s", stored.Status)
	}
}

// TestSession_ContinuousMatches: during CONTINUOUS, SubmitOrder → MatchOrder → trades returned.
func TestSession_ContinuousMatches(t *testing.T) {
	// Build stores + engines from scratch so we can pre-populate a resting order.
	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()
	createInstrument(t, &testStores{inst: inst, ord: ord, trd: trd, pos: pos}, sessionInstID, types.TradingStatusActive)

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)

	// Advance to CONTINUOUS via PRE_OPEN.
	if _, err := sm.TransitionTo(sessionInstID, sessionTenant, types.SessionPreOpen); err != nil {
		t.Fatalf("transition PRE_OPEN: %v", err)
	}
	// Transition to CONTINUOUS runs the opening auction (empty book → no-op).
	if _, err := sm.TransitionTo(sessionInstID, sessionTenant, types.SessionContinuous); err != nil {
		t.Fatalf("transition CONTINUOUS: %v", err)
	}

	// Seed a resting sell order directly into the order store.
	resting := &types.SecurityOrder{
		ID: "resting-sell", InstrumentID: sessionInstID, ParticipantID: "seller",
		Side: types.OrderSideSell, OrderType: types.OrderTypeLimit,
		Quantity: 50, Price: 100.0, Status: types.OrderStatusPending,
		TimeInForce: types.TimeInForceGTC, CreatedAt: ts(0), UpdatedAt: ts(0),
	}
	if err := ord.Submit(resting); err != nil {
		t.Fatalf("seed resting order: %v", err)
	}

	// The incoming buy order must be submitted to the store before MatchOrder is called,
	// because MatchOrder calls orderStore.Update(order) to reflect the fill status.
	// SubmitOrder → SessionManager → MatchingEngine.MatchOrder does NOT auto-submit the
	// incoming order; the caller is responsible for persisting it first (same pattern as
	// the existing engine_test.go submitAndMatch helper). Submit it directly, then route
	// via SubmitOrder.
	buy := sessionOrder("cont-buy", "buyer", types.OrderSideBuy, 50, 100.0)
	if err := ord.Submit(buy); err != nil {
		t.Fatalf("submit buy order to store: %v", err)
	}

	trades, err := sm.SubmitOrder(buy, sessionTenant)
	if err != nil {
		t.Fatalf("SubmitOrder during CONTINUOUS: %v", err)
	}
	if len(trades) == 0 {
		t.Error("expected at least 1 trade during CONTINUOUS, got 0")
	}
	if trades[0].Price != 100.0 {
		t.Errorf("trade price: want 100.0, got %v", trades[0].Price)
	}
}

// TestSession_ClosedRejects: during CLOSED, SubmitOrder → error.
func TestSession_ClosedRejects(t *testing.T) {
	env := setupSessionTest(t)

	// Session starts as CLOSED.
	order := sessionOrder("closed-buy", "buyer", types.OrderSideBuy, 10, 50.0)
	_, err := env.sm.SubmitOrder(order, sessionTenant)
	if err == nil {
		t.Error("expected error when submitting to CLOSED session, got nil")
	}
}

// TestSession_GetAllSessions: GetAllSessions returns a snapshot of all sessions.
func TestSession_GetAllSessions(t *testing.T) {
	// Create two independent instruments and advance them to different states.
	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()

	for _, id := range []string{"INST-A", "INST-B"} {
		createInstrument(t, &testStores{inst: inst, ord: ord, trd: trd, pos: pos}, id, types.TradingStatusActive)
	}

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)

	// INST-A → PRE_OPEN, INST-B stays CLOSED.
	if _, err := sm.TransitionTo("INST-A", sessionTenant, types.SessionPreOpen); err != nil {
		t.Fatalf("transition INST-A to PRE_OPEN: %v", err)
	}

	all := sm.GetAllSessions()

	if all["INST-A"] != types.SessionPreOpen {
		t.Errorf("INST-A: want PRE_OPEN, got %s", all["INST-A"])
	}
	// INST-B was never transitioned, so it won't appear in the map (GetSession defaults to CLOSED).
	// GetAllSessions only includes instruments with an explicit session.
	if sess, ok := all["INST-B"]; ok && sess != types.SessionClosed {
		t.Errorf("INST-B: unexpected non-CLOSED session %s", sess)
	}
}

// TestSession_TransitionRunsOpeningAuction: PRE_OPEN→CONTINUOUS executes the opening auction.
func TestSession_TransitionRunsOpeningAuction(t *testing.T) {
	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()
	createInstrument(t, &testStores{inst: inst, ord: ord, trd: trd, pos: pos}, sessionInstID, types.TradingStatusActive)

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)

	// Go to PRE_OPEN.
	if _, err := sm.TransitionTo(sessionInstID, sessionTenant, types.SessionPreOpen); err != nil {
		t.Fatalf("PRE_OPEN: %v", err)
	}

	// Collect crossing orders during PRE_OPEN.
	// SubmitOrder during PRE_OPEN calls CollectOrder, which calls orderStore.Submit,
	// so the orders are persisted automatically.
	buyOrder := sessionOrder("open-buy", "buyer", types.OrderSideBuy, 20, 100.0)
	sellOrder := sessionOrder("open-sell", "seller", types.OrderSideSell, 20, 100.0)
	if _, err := sm.SubmitOrder(buyOrder, sessionTenant); err != nil {
		t.Fatalf("collect buy: %v", err)
	}
	if _, err := sm.SubmitOrder(sellOrder, sessionTenant); err != nil {
		t.Fatalf("collect sell: %v", err)
	}

	// PRE_OPEN → CONTINUOUS should trigger the opening auction.
	result, err := sm.TransitionTo(sessionInstID, sessionTenant, types.SessionContinuous)
	if err != nil {
		t.Fatalf("CONTINUOUS transition: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil AuctionResult from opening auction")
	}
	if result.MatchedVolume != 20 {
		t.Errorf("opening auction matched volume: want 20, got %d", result.MatchedVolume)
	}
	if result.ClearingPrice != 100.0 {
		t.Errorf("opening auction clearing price: want 100.0, got %v", result.ClearingPrice)
	}
}

// TestSession_TransitionRunsClosingAuction: CLOSING_AUCTION→CLOSED executes the closing auction.
func TestSession_TransitionRunsClosingAuction(t *testing.T) {
	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()
	createInstrument(t, &testStores{inst: inst, ord: ord, trd: trd, pos: pos}, sessionInstID, types.TradingStatusActive)

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)

	// Advance through the session cycle.
	steps := []types.MarketSession{
		types.SessionPreOpen,
		types.SessionContinuous,
		types.SessionClosingAuction,
	}
	for _, step := range steps {
		if _, err := sm.TransitionTo(sessionInstID, sessionTenant, step); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}

	// Collect crossing orders during CLOSING_AUCTION.
	// CollectOrder stores the order in the order store (status → PENDING), so no
	// pre-submission is needed for auction mode.
	closeBuy := sessionOrder("close-buy", "buyer", types.OrderSideBuy, 15, 99.0)
	closeSell := sessionOrder("close-sell", "seller", types.OrderSideSell, 15, 99.0)
	if _, err := sm.SubmitOrder(closeBuy, sessionTenant); err != nil {
		t.Fatalf("collect buy: %v", err)
	}
	if _, err := sm.SubmitOrder(closeSell, sessionTenant); err != nil {
		t.Fatalf("collect sell: %v", err)
	}

	// CLOSING_AUCTION → CLOSED should trigger the closing auction.
	result, err := sm.TransitionTo(sessionInstID, sessionTenant, types.SessionClosed)
	if err != nil {
		t.Fatalf("CLOSED transition: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil AuctionResult from closing auction")
	}
	if result.MatchedVolume != 15 {
		t.Errorf("closing auction matched volume: want 15, got %d", result.MatchedVolume)
	}
}
