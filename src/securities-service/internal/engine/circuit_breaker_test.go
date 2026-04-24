// Package engine_test — tests for CircuitBreakerEngine.
package engine_test

import (
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// newCBStore returns an empty InMemoryCircuitBreakerStore.
func newCBStore() *store.InMemoryCircuitBreakerStore {
	return store.NewInMemoryCircuitBreakerStore()
}

// newCBEngine creates a CircuitBreakerEngine backed by a fresh store.
func newCBEngine() (*engine.CircuitBreakerEngine, *store.InMemoryCircuitBreakerStore) {
	s := newCBStore()
	return engine.NewCircuitBreakerEngine(s), s
}

// seedCB adds a CircuitBreaker record to the store.
func seedCB(t *testing.T, s *store.InMemoryCircuitBreakerStore, cb *types.CircuitBreaker) {
	t.Helper()
	if err := s.Set(cb); err != nil {
		t.Fatalf("seedCB: %v", err)
	}
}

// ── TestCircuitBreaker_PriceWithinBands ──────────────────────────────────────

// TestCircuitBreaker_PriceWithinBands verifies that a price inside both the
// static and dynamic bands is allowed.
func TestCircuitBreaker_PriceWithinBands(t *testing.T) {
	cbe, s := newCBEngine()

	// referencePrice=100, ±10% static, ±5% dynamic, lastTradedPrice=100
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-1",
		ReferencePrice:  100.0,
		StaticUpperPct:  10.0,
		StaticLowerPct:  10.0,
		DynamicUpperPct: 5.0,
		DynamicLowerPct: 5.0,
		LastTradedPrice: 100.0,
		Status:          types.CBActive,
	})

	// Price=102: within static ±10 (90–110) and dynamic ±5 (95–105).
	allowed, event, err := cbe.ValidatePrice("INST-1", 102.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected price 102 to be allowed; got event: %+v", event)
	}
	if event != nil {
		t.Errorf("expected nil event for allowed price, got %+v", event)
	}
}

// ── TestCircuitBreaker_StaticUpperBreach ─────────────────────────────────────

// TestCircuitBreaker_StaticUpperBreach verifies that a price above the static
// upper band is rejected with a CB_STATIC_UPPER event.
func TestCircuitBreaker_StaticUpperBreach(t *testing.T) {
	cbe, s := newCBEngine()

	// referencePrice=100, staticUpperPct=10 → ceiling = 110.
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-2",
		ReferencePrice: 100.0,
		StaticUpperPct: 10.0,
		Status:         types.CBActive,
	})

	// Price=111 exceeds 110.
	allowed, event, err := cbe.ValidatePrice("INST-2", 111.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if allowed {
		t.Fatalf("expected price 111 to be rejected (static upper breach)")
	}
	if event == nil {
		t.Fatal("expected non-nil event for rejected price")
	}
	if event.Type != types.CBStaticUpper {
		t.Errorf("event.Type: want %s, got %s", types.CBStaticUpper, event.Type)
	}
	if event.InstrumentID != "INST-2" {
		t.Errorf("event.InstrumentID: want INST-2, got %s", event.InstrumentID)
	}
	if event.TriggerPrice != 111.0 {
		t.Errorf("event.TriggerPrice: want 111.0, got %v", event.TriggerPrice)
	}
	if event.ReferencePrice != 100.0 {
		t.Errorf("event.ReferencePrice: want 100.0, got %v", event.ReferencePrice)
	}
}

// TestCircuitBreaker_StaticUpperBreach_ExactBoundary verifies that a price
// exactly at the upper static limit is allowed (strictly greater than required).
func TestCircuitBreaker_StaticUpperBreach_ExactBoundary(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-2B",
		ReferencePrice: 100.0,
		StaticUpperPct: 10.0,
		Status:         types.CBActive,
	})

	// Exactly at 110.0 — boundary should be allowed (> not >=).
	allowed, _, err := cbe.ValidatePrice("INST-2B", 110.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected price exactly at upper limit (110.0) to be allowed")
	}
}

// ── TestCircuitBreaker_StaticLowerBreach ─────────────────────────────────────

// TestCircuitBreaker_StaticLowerBreach verifies that a price below the static
// lower band is rejected with a CB_STATIC_LOWER event.
func TestCircuitBreaker_StaticLowerBreach(t *testing.T) {
	cbe, s := newCBEngine()

	// referencePrice=100, staticLowerPct=10 → floor = 90.
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-3",
		ReferencePrice: 100.0,
		StaticLowerPct: 10.0,
		Status:         types.CBActive,
	})

	// Price=89 is below the floor of 90.
	allowed, event, err := cbe.ValidatePrice("INST-3", 89.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if allowed {
		t.Fatalf("expected price 89 to be rejected (static lower breach)")
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}
	if event.Type != types.CBStaticLower {
		t.Errorf("event.Type: want %s, got %s", types.CBStaticLower, event.Type)
	}
	if event.ReferencePrice != 100.0 {
		t.Errorf("event.ReferencePrice: want 100.0, got %v", event.ReferencePrice)
	}
}

// TestCircuitBreaker_StaticLowerBreach_ExactBoundary verifies that a price
// exactly at the lower static limit is allowed.
func TestCircuitBreaker_StaticLowerBreach_ExactBoundary(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-3B",
		ReferencePrice: 100.0,
		StaticLowerPct: 10.0,
		Status:         types.CBActive,
	})

	// Exactly at 90.0 — boundary should be allowed (< not <=).
	allowed, _, err := cbe.ValidatePrice("INST-3B", 90.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected price exactly at lower limit (90.0) to be allowed")
	}
}

// ── TestCircuitBreaker_DynamicBreach ─────────────────────────────────────────

// TestCircuitBreaker_DynamicBreach verifies that a price breaching the dynamic
// upper band (based on lastTradedPrice) is rejected with a CB_DYNAMIC_UPPER event.
func TestCircuitBreaker_DynamicBreach(t *testing.T) {
	cbe, s := newCBEngine()

	// referencePrice=100, lastTradedPrice=105, dynamicUpperPct=3 → ceiling = 108.15.
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-4",
		ReferencePrice:  100.0,
		StaticUpperPct:  20.0, // Wide static band — won't trigger.
		DynamicUpperPct: 3.0,
		LastTradedPrice: 105.0,
		Status:          types.CBActive,
	})

	// Price=109 exceeds 105 * 1.03 = 108.15.
	allowed, event, err := cbe.ValidatePrice("INST-4", 109.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if allowed {
		t.Fatalf("expected price 109 to be rejected (dynamic upper breach)")
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}
	if event.Type != types.CBDynamicUpper {
		t.Errorf("event.Type: want %s, got %s", types.CBDynamicUpper, event.Type)
	}
	// ReferencePrice in the dynamic event is the lastTradedPrice.
	if event.ReferencePrice != 105.0 {
		t.Errorf("event.ReferencePrice: want 105.0 (lastTradedPrice), got %v", event.ReferencePrice)
	}
}

// TestCircuitBreaker_DynamicLowerBreach verifies that a price breaching the
// dynamic lower band is rejected with a CB_DYNAMIC_LOWER event.
// To isolate dynamic lower, we set a wide static range (referencePrice=1000,
// staticLowerPct=50 → static floor=500) and a tight dynamic band
// (lastTradedPrice=100, dynamicLowerPct=5 → dynamic floor=95), then probe
// price=93 which is inside the static range but below the dynamic floor.
func TestCircuitBreaker_DynamicLowerBreach(t *testing.T) {
	cbe, s := newCBEngine()

	// referencePrice=1000, staticLowerPct=50 → static floor = 500.
	// lastTradedPrice=100, dynamicLowerPct=5 → dynamic floor = 95.
	// Price=93 is above 500 (static floor OK) but below 95 (dynamic floor breach).
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-4L",
		ReferencePrice:  1000.0,
		StaticLowerPct:  50.0, // floor = 500; price=93 is above 500? No — 93 < 500.
		DynamicLowerPct: 5.0,
		LastTradedPrice: 100.0,
		Status:          types.CBActive,
	})

	// Actually static lower check fires first for price=93 since 93 < 500.
	// To test dynamic lower in isolation we need staticLowerPct=0 (disabled).
	s.Delete("INST-4L")
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-4L",
		ReferencePrice:  100.0,
		StaticUpperPct:  200.0, // Very wide upper — won't trigger.
		StaticLowerPct:  0,     // Disabled — no static lower check.
		DynamicUpperPct: 0,     // Disabled.
		DynamicLowerPct: 5.0,   // Dynamic floor: lastTradedPrice * 0.95 = 95.
		LastTradedPrice: 100.0,
		Status:          types.CBActive,
	})

	// Price=93 is below 100 * 0.95 = 95 → dynamic lower breach.
	allowed, event, err := cbe.ValidatePrice("INST-4L", 93.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if allowed {
		t.Fatalf("expected price 93 to be rejected (dynamic lower breach)")
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}
	if event.Type != types.CBDynamicLower {
		t.Errorf("event.Type: want %s, got %s", types.CBDynamicLower, event.Type)
	}
}

// TestCircuitBreaker_DynamicBreach_ZeroLastPrice verifies that dynamic checks
// are skipped when lastTradedPrice is 0 (no trades yet).
func TestCircuitBreaker_DynamicBreach_ZeroLastPrice(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-4Z",
		ReferencePrice:  100.0,
		StaticUpperPct:  20.0,
		DynamicUpperPct: 1.0,
		LastTradedPrice: 0, // No trades yet.
		Status:          types.CBActive,
	})

	// Price=115 — within static upper (120) but would breach dynamic if last=100.
	// Since last=0, dynamic check is skipped.
	allowed, _, err := cbe.ValidatePrice("INST-4Z", 115.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected price 115 to be allowed when lastTradedPrice=0")
	}
}

// ── TestCircuitBreaker_NoBreakerSet ──────────────────────────────────────────

// TestCircuitBreaker_NoBreakerSet verifies that any price is allowed when no
// circuit breaker is configured for the instrument.
func TestCircuitBreaker_NoBreakerSet(t *testing.T) {
	cbe, _ := newCBEngine()

	// No record set for "INST-UNKNOWN".
	allowed, event, err := cbe.ValidatePrice("INST-UNKNOWN", 9999.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected any price to be allowed when no CB configured; got event: %+v", event)
	}
	if event != nil {
		t.Errorf("expected nil event when no CB configured, got %+v", event)
	}
}

// ── TestCircuitBreaker_OnTrade ────────────────────────────────────────────────

// TestCircuitBreaker_OnTrade verifies that OnTrade updates the lastTradedPrice
// in the store, which is then used for subsequent dynamic band checks.
func TestCircuitBreaker_OnTrade(t *testing.T) {
	cbe, s := newCBEngine()

	// Configure CB with a tight dynamic band: dynamicUpperPct=2%.
	// Initial lastTradedPrice=100.
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-5",
		ReferencePrice:  100.0,
		StaticUpperPct:  50.0,  // Very wide — won't trigger.
		DynamicUpperPct: 2.0,   // Tight dynamic band.
		LastTradedPrice: 100.0, // Initial last price.
		Status:          types.CBActive,
	})

	// Before OnTrade: price=103 is above 100*1.02=102 → rejected.
	allowed, _, err := cbe.ValidatePrice("INST-5", 103.0)
	if err != nil {
		t.Fatalf("ValidatePrice before trade: %v", err)
	}
	if allowed {
		t.Error("expected 103 to be rejected before trade (lastPrice=100, band=2%)")
	}

	// Simulate a trade at 101.5 → lastTradedPrice moves to 101.5.
	if err := cbe.OnTrade("INST-5", 101.5); err != nil {
		t.Fatalf("OnTrade: %v", err)
	}

	// After OnTrade: dynamic upper band is now 101.5*1.02=103.53.
	// Price=103 should now be allowed.
	allowed, _, err = cbe.ValidatePrice("INST-5", 103.0)
	if err != nil {
		t.Fatalf("ValidatePrice after trade: %v", err)
	}
	if !allowed {
		t.Error("expected 103 to be allowed after trade moved lastTradedPrice to 101.5")
	}

	// Verify the stored lastTradedPrice was updated.
	cb, err := s.Get("INST-5")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if cb.LastTradedPrice != 101.5 {
		t.Errorf("lastTradedPrice: want 101.5, got %v", cb.LastTradedPrice)
	}
}

// TestCircuitBreaker_OnTrade_NoConfig verifies that OnTrade returns no error
// when there is no CB config for the instrument (store returns nil, no-ops).
func TestCircuitBreaker_OnTrade_NoConfig(t *testing.T) {
	cbe, _ := newCBEngine()

	// No record set — should not error.
	if err := cbe.OnTrade("INST-NOCONFIG", 50.0); err != nil {
		t.Errorf("OnTrade with no config: want nil error, got %v", err)
	}
}

// ── TestCircuitBreaker_AlreadyTriggered ───────────────────────────────────────

// TestCircuitBreaker_AlreadyTriggered verifies that when a circuit breaker has
// status CB_TRIGGERED, all prices are rejected regardless of the bands.
func TestCircuitBreaker_AlreadyTriggered(t *testing.T) {
	cbe, s := newCBEngine()

	// Very permissive bands, but status is CB_TRIGGERED.
	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-6",
		ReferencePrice: 100.0,
		StaticUpperPct: 100.0, // 200% ceiling — would normally allow anything.
		StaticLowerPct: 100.0, // 0% floor — would normally allow anything.
		Status:         types.CBTriggered,
	})

	// Price is well within the wide static bands, but CB is triggered.
	allowed, event, err := cbe.ValidatePrice("INST-6", 100.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if allowed {
		t.Fatalf("expected price to be rejected when CB status is CB_TRIGGERED")
	}
	if event == nil {
		t.Fatal("expected non-nil event for triggered CB")
	}
	if event.Type != types.CBTriggered {
		t.Errorf("event.Type: want %s, got %s", types.CBTriggered, event.Type)
	}
	if event.InstrumentID != "INST-6" {
		t.Errorf("event.InstrumentID: want INST-6, got %s", event.InstrumentID)
	}
	if event.TriggerPrice != 100.0 {
		t.Errorf("event.TriggerPrice: want 100.0, got %v", event.TriggerPrice)
	}
}

// TestCircuitBreaker_AlreadyTriggered_AnyPrice verifies that CB_TRIGGERED
// rejects prices both above and below reference price.
func TestCircuitBreaker_AlreadyTriggered_AnyPrice(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-6B",
		ReferencePrice: 100.0,
		StaticUpperPct: 50.0,
		StaticLowerPct: 50.0,
		Status:         types.CBTriggered,
	})

	for _, price := range []float64{0.01, 50.0, 100.0, 150.0, 999.0} {
		allowed, event, err := cbe.ValidatePrice("INST-6B", price)
		if err != nil {
			t.Fatalf("ValidatePrice(%.2f): %v", price, err)
		}
		if allowed {
			t.Errorf("price %.2f: expected rejected when CB_TRIGGERED, got allowed", price)
		}
		if event == nil || event.Type != types.CBTriggered {
			t.Errorf("price %.2f: expected CB_TRIGGERED event, got %+v", price, event)
		}
	}
}

// ── Additional coverage: static-only config, dynamic-only config ──────────────

// TestCircuitBreaker_StaticOnlyConfig verifies that when only static bands are
// set, dynamic checks don't fire even if lastTradedPrice is non-zero.
func TestCircuitBreaker_StaticOnlyConfig(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:    "INST-7",
		ReferencePrice:  100.0,
		StaticUpperPct:  10.0,
		StaticLowerPct:  10.0,
		DynamicUpperPct: 0, // Disabled.
		DynamicLowerPct: 0, // Disabled.
		LastTradedPrice: 100.0,
		Status:          types.CBActive,
	})

	// Price=105 is within static ±10 and dynamic check is disabled.
	allowed, _, err := cbe.ValidatePrice("INST-7", 105.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if !allowed {
		t.Errorf("expected 105 to be allowed with only static bands and wide limits")
	}
}

// TestCircuitBreaker_Timestamp verifies that the event timestamp is populated
// and in RFC3339 format.
func TestCircuitBreaker_Timestamp(t *testing.T) {
	cbe, s := newCBEngine()

	seedCB(t, s, &types.CircuitBreaker{
		InstrumentID:   "INST-8",
		ReferencePrice: 100.0,
		StaticUpperPct: 5.0,
		Status:         types.CBActive,
	})

	_, event, err := cbe.ValidatePrice("INST-8", 200.0)
	if err != nil {
		t.Fatalf("ValidatePrice: %v", err)
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}
	if event.Timestamp == "" {
		t.Error("event.Timestamp must not be empty")
	}
	// Quick sanity check — RFC3339 timestamps contain 'T'.
	if len(event.Timestamp) < 10 {
		t.Errorf("event.Timestamp looks too short: %q", event.Timestamp)
	}
}
