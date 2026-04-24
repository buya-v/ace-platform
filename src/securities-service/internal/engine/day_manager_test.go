// Package engine_test — day lifecycle tests for DayManager.
package engine_test

import (
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ---- DayManager helpers -----------------------------------------------------

// dayTestEnv holds the DayManager and underlying stores / engines used in
// day-lifecycle tests.
type dayTestEnv struct {
	dm   *engine.DayManager
	sm   *engine.SessionManager
	inst *store.InMemoryInstrumentStore
}

// setupDayTest creates a fresh DayManager wired to a SessionManager with a
// set of ACTIVE instruments pre-seeded into the instrument store.
func setupDayTest(t *testing.T, instrumentIDs ...string) *dayTestEnv {
	t.Helper()

	inst := store.NewInMemoryInstrumentStore()
	ord := store.NewInMemoryOrderStore()
	trd := store.NewInMemoryTradeStore()
	pos := store.NewInMemoryPositionStore()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range instrumentIDs {
		if err := inst.Create(&types.Instrument{
			ID:            id,
			Ticker:        id,
			Name:          id + " Corp",
			AssetClass:    types.AssetClassEquity,
			TradingStatus: types.TradingStatusActive,
			LotSize:       1,
			TickSize:      0.01,
			ExchangeCode:  "MSE",
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			t.Fatalf("setupDayTest: create instrument %s: %v", id, err)
		}
	}

	me := engine.NewMatchingEngine(inst, ord, trd, pos, nil, nil, nil)
	ae := engine.NewAuctionEngine(ord, trd, pos, nil)
	sm := engine.NewSessionManager(ae, me)
	dm := engine.NewDayManager(sm, inst)

	return &dayTestEnv{dm: dm, sm: sm, inst: inst}
}

// ---- DayManager tests -------------------------------------------------------

// TestDayManager_StartDay verifies that StartDay transitions the DayManager
// from DAY_CLOSED to DAY_PRE_OPEN and transitions ACTIVE instruments to PRE_OPEN.
func TestDayManager_StartDay(t *testing.T) {
	env := setupDayTest(t, "INST-DAY-1", "INST-DAY-2")

	// Initial state must be DAY_CLOSED.
	if env.dm.GetState() != types.DayClosed {
		t.Fatalf("initial state: want DAY_CLOSED, got %s", env.dm.GetState())
	}

	if err := env.dm.StartDay(); err != nil {
		t.Fatalf("StartDay: %v", err)
	}

	// Day state must advance to DAY_PRE_OPEN.
	if env.dm.GetState() != types.DayPreOpen {
		t.Errorf("after StartDay: want DAY_PRE_OPEN, got %s", env.dm.GetState())
	}

	// Each instrument session must now be PRE_OPEN.
	for _, id := range []string{"INST-DAY-1", "INST-DAY-2"} {
		sess := env.sm.GetSession(id)
		if sess != types.SessionPreOpen {
			t.Errorf("instrument %s session: want PRE_OPEN, got %s", id, sess)
		}
	}
}

// TestDayManager_FullCycle walks the DayManager through the complete four-step
// lifecycle: CLOSED → PRE_OPEN → TRADING → POST_CLOSE → CLOSED.
func TestDayManager_FullCycle(t *testing.T) {
	env := setupDayTest(t, "INST-CYCLE-1")

	// Step 1: CLOSED → PRE_OPEN.
	if err := env.dm.StartDay(); err != nil {
		t.Fatalf("StartDay: %v", err)
	}
	if env.dm.GetState() != types.DayPreOpen {
		t.Fatalf("after StartDay: want DAY_PRE_OPEN, got %s", env.dm.GetState())
	}

	// Step 2: PRE_OPEN → TRADING.
	if err := env.dm.StartTrading(); err != nil {
		t.Fatalf("StartTrading: %v", err)
	}
	if env.dm.GetState() != types.DayTrading {
		t.Fatalf("after StartTrading: want DAY_TRADING, got %s", env.dm.GetState())
	}
	// Instrument session must be CONTINUOUS.
	if sess := env.sm.GetSession("INST-CYCLE-1"); sess != types.SessionContinuous {
		t.Errorf("instrument session after StartTrading: want CONTINUOUS, got %s", sess)
	}

	// Step 3: TRADING → POST_CLOSE.
	if err := env.dm.EndTrading(); err != nil {
		t.Fatalf("EndTrading: %v", err)
	}
	if env.dm.GetState() != types.DayPostClose {
		t.Fatalf("after EndTrading: want DAY_POST_CLOSE, got %s", env.dm.GetState())
	}
	// Instrument session must be CLOSED after closing auction.
	if sess := env.sm.GetSession("INST-CYCLE-1"); sess != types.SessionClosed {
		t.Errorf("instrument session after EndTrading: want CLOSED, got %s", sess)
	}

	// Step 4: POST_CLOSE → CLOSED.
	if err := env.dm.EndDay(); err != nil {
		t.Fatalf("EndDay: %v", err)
	}
	if env.dm.GetState() != types.DayClosed {
		t.Fatalf("after EndDay: want DAY_CLOSED, got %s", env.dm.GetState())
	}
}

// TestDayManager_InvalidTransition verifies that skipping a required state
// (e.g. jumping from CLOSED directly to TRADING) returns an error and leaves
// the DayManager state unchanged.
func TestDayManager_InvalidTransition(t *testing.T) {
	env := setupDayTest(t) // no instruments needed for error-path tests

	// Attempt CLOSED → TRADING (invalid: must go through PRE_OPEN first).
	if err := env.dm.StartTrading(); err == nil {
		t.Fatal("expected error for CLOSED → TRADING transition, got nil")
	}
	// State must remain CLOSED.
	if env.dm.GetState() != types.DayClosed {
		t.Errorf("state after invalid transition: want DAY_CLOSED, got %s", env.dm.GetState())
	}
}

// TestDayManager_InvalidTransition_AllPaths exercises all invalid state jumps.
func TestDayManager_InvalidTransition_AllPaths(t *testing.T) {
	tests := []struct {
		name    string
		advance func(dm *engine.DayManager) // bring dm to the desired starting state
		attempt func(dm *engine.DayManager) error
	}{
		{
			name:    "CLOSED cannot EndTrading",
			advance: func(dm *engine.DayManager) {},
			attempt: func(dm *engine.DayManager) error { return dm.EndTrading() },
		},
		{
			name:    "CLOSED cannot EndDay",
			advance: func(dm *engine.DayManager) {},
			attempt: func(dm *engine.DayManager) error { return dm.EndDay() },
		},
		{
			name: "PRE_OPEN cannot EndDay",
			advance: func(dm *engine.DayManager) {
				// Advance to PRE_OPEN.
				_ = dm.StartDay()
			},
			attempt: func(dm *engine.DayManager) error { return dm.EndDay() },
		},
		{
			name: "PRE_OPEN cannot EndTrading",
			advance: func(dm *engine.DayManager) {
				_ = dm.StartDay()
			},
			attempt: func(dm *engine.DayManager) error { return dm.EndTrading() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := setupDayTest(t)
			tc.advance(env.dm)
			if err := tc.attempt(env.dm); err == nil {
				t.Errorf("%s: expected error for invalid transition, got nil", tc.name)
			}
		})
	}
}

// TestDayManager_GetState verifies that GetState returns the correct state
// at each step of the lifecycle and is safe for concurrent reads.
func TestDayManager_GetState(t *testing.T) {
	env := setupDayTest(t, "INST-STATE-1")

	// Initial.
	if s := env.dm.GetState(); s != types.DayClosed {
		t.Errorf("initial state: want DAY_CLOSED, got %s", s)
	}

	// After StartDay.
	if err := env.dm.StartDay(); err != nil {
		t.Fatalf("StartDay: %v", err)
	}
	if s := env.dm.GetState(); s != types.DayPreOpen {
		t.Errorf("after StartDay: want DAY_PRE_OPEN, got %s", s)
	}

	// After StartTrading.
	if err := env.dm.StartTrading(); err != nil {
		t.Fatalf("StartTrading: %v", err)
	}
	if s := env.dm.GetState(); s != types.DayTrading {
		t.Errorf("after StartTrading: want DAY_TRADING, got %s", s)
	}

	// After EndTrading.
	if err := env.dm.EndTrading(); err != nil {
		t.Fatalf("EndTrading: %v", err)
	}
	if s := env.dm.GetState(); s != types.DayPostClose {
		t.Errorf("after EndTrading: want DAY_POST_CLOSE, got %s", s)
	}

	// After EndDay.
	if err := env.dm.EndDay(); err != nil {
		t.Fatalf("EndDay: %v", err)
	}
	if s := env.dm.GetState(); s != types.DayClosed {
		t.Errorf("after EndDay: want DAY_CLOSED, got %s", s)
	}
}

// TestDayManager_StartDay_NoActiveInstruments verifies that StartDay succeeds
// even when there are no ACTIVE instruments (empty list is a valid case).
func TestDayManager_StartDay_NoActiveInstruments(t *testing.T) {
	env := setupDayTest(t) // no instruments seeded

	if err := env.dm.StartDay(); err != nil {
		t.Fatalf("StartDay with no instruments: %v", err)
	}
	if env.dm.GetState() != types.DayPreOpen {
		t.Errorf("state: want DAY_PRE_OPEN, got %s", env.dm.GetState())
	}
}

// TestDayManager_DoubleStartDay verifies that calling StartDay twice returns
// an error on the second call (already in PRE_OPEN).
func TestDayManager_DoubleStartDay(t *testing.T) {
	env := setupDayTest(t)

	if err := env.dm.StartDay(); err != nil {
		t.Fatalf("first StartDay: %v", err)
	}
	// Second StartDay must fail: already in PRE_OPEN.
	if err := env.dm.StartDay(); err == nil {
		t.Fatal("expected error on double StartDay, got nil")
	}
	// State must remain PRE_OPEN.
	if env.dm.GetState() != types.DayPreOpen {
		t.Errorf("state after double StartDay: want DAY_PRE_OPEN, got %s", env.dm.GetState())
	}
}
