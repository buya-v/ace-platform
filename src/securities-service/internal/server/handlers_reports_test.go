// Package server — tests for FRC reporting HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ============================================================
// FRC report test helpers
// ============================================================

// frcStores groups the stores needed for white-box FRC tests.
type frcStores struct {
	instrument *store.InMemoryInstrumentStore
	order      *store.InMemoryOrderStore
	trade      *store.InMemoryTradeStore
	position   *store.InMemoryPositionStore
	caStore    *store.InMemoryCorporateActionStore
	entStore   *store.InMemoryEntitlementStore
}

// newFRCStores creates fresh in-memory store instances for FRC tests.
func newFRCStores() frcStores {
	return frcStores{
		instrument: store.NewInMemoryInstrumentStore(),
		order:      store.NewInMemoryOrderStore(),
		trade:      store.NewInMemoryTradeStore(),
		position:   store.NewInMemoryPositionStore(),
		caStore:    store.NewInMemoryCorporateActionStore(),
		entStore:   store.NewInMemoryEntitlementStore(),
	}
}

// newFRCServer creates an httptest.Server backed by the given frcStores.
func newFRCServer(t *testing.T, s frcStores) *httptest.Server {
	t.Helper()
	cfg := DefaultConfig()
	me := engine.NewMatchingEngine(s.instrument, s.order, s.trade, s.position, nil, nil, nil)
	srv := New(s.instrument, s.order, s.trade, s.position, nil,
		s.caStore, s.entStore, store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, me, nil, nil, nil, cfg)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// seedInstrumentFRC creates an instrument directly in the store and returns its ID.
func seedInstrumentFRC(t *testing.T, s frcStores, ticker string) string {
	t.Helper()
	inst := &types.Instrument{
		ID:            "inst-frc-" + ticker,
		Ticker:        ticker,
		Name:          ticker + " Corp",
		AssetClass:    types.AssetClassEquity,
		TradingStatus: types.TradingStatusActive,
		LotSize:       100,
		TickSize:      0.01,
	}
	if err := s.instrument.Create(inst); err != nil {
		t.Fatalf("seedInstrumentFRC: %v", err)
	}
	return inst.ID
}

// seedTradeFRC creates a trade directly in the trade store for a given instrument and date.
func seedTradeFRC(t *testing.T, s frcStores, id, instrID string, qty int, price float64, tradeDate string) {
	t.Helper()
	trade := &types.SecurityTrade{
		ID:             id,
		BuyOrderID:     "buy-" + id,
		SellOrderID:    "sell-" + id,
		InstrumentID:   instrID,
		Price:          price,
		Quantity:       qty,
		TradeDate:      tradeDate,
		SettlementDate: "2026-05-01",
		Status:         types.TradeStatusPending,
		CreatedAt:      tradeDate + "T00:00:00Z",
	}
	if err := s.trade.Create(trade); err != nil {
		t.Fatalf("seedTradeFRC: %v", err)
	}
}

// ============================================================
// TestFRCDailySummary
// ============================================================

// TestFRCDailySummary creates an instrument with two trades on today's date
// and verifies that the DAILY_SUMMARY report returns the correct
// trade_count, total_volume, and total_value.
func TestFRCDailySummary(t *testing.T) {
	s := newFRCStores()
	ts := newFRCServer(t, s)

	today := time.Now().UTC().Format("2006-01-02")
	instrID := seedInstrumentFRC(t, s, "FRC1")

	// Two trades on today's date.
	seedTradeFRC(t, s, "tr-frc-1", instrID, 100, 50.0, today)
	seedTradeFRC(t, s, "tr-frc-2", instrID, 200, 75.0, today)

	// One trade on a different date — should NOT appear in today's summary.
	seedTradeFRC(t, s, "tr-frc-old", instrID, 300, 10.0, "2020-01-01")

	path := fmt.Sprintf("/api/v1/securities/reports/frc?type=DAILY_SUMMARY&date=%s", today)
	resp := doJSON(t, ts, http.MethodGet, path, nil)
	assertStatus(t, resp, http.StatusOK)

	var report map[string]interface{}
	decodeBody(t, resp, &report)

	if report["report_type"] != "DAILY_SUMMARY" {
		t.Errorf("expected report_type DAILY_SUMMARY, got %v", report["report_type"])
	}
	if report["report_date"] != today {
		t.Errorf("expected report_date %q, got %v", today, report["report_date"])
	}
	if _, ok := report["id"].(string); !ok {
		t.Error("expected non-empty report id")
	}
	if _, ok := report["generated_at"].(string); !ok {
		t.Error("expected non-empty generated_at")
	}

	data := report["data"].(map[string]interface{})
	tradeCount := data["trade_count"].(float64)
	totalVolume := data["total_volume"].(float64)
	totalValue := data["total_value"].(float64)

	if tradeCount != 2 {
		t.Errorf("expected trade_count=2, got %v", tradeCount)
	}
	// total_volume = 100 + 200 = 300
	if totalVolume != 300 {
		t.Errorf("expected total_volume=300, got %v", totalVolume)
	}
	// total_value = 100*50 + 200*75 = 5000 + 15000 = 20000
	if totalValue != 20000.0 {
		t.Errorf("expected total_value=20000.0, got %v", totalValue)
	}
}

// ============================================================
// TestFRCLargeTrader
// ============================================================

// TestFRCLargeTrader seeds positions, one with qty > 1000 and one with qty <= 1000,
// and verifies that only the large position appears in the LARGE_TRADER report.
func TestFRCLargeTrader(t *testing.T) {
	s := newFRCStores()
	ts := newFRCServer(t, s)

	instrID := seedInstrumentFRC(t, s, "LT1")

	// Large trader: qty=1500 > 1000 → should appear.
	posLarge, _ := s.position.GetOrCreate("large-participant", instrID)
	posLarge.Quantity = 1500
	if err := s.position.Update(posLarge); err != nil {
		t.Fatalf("Update large position: %v", err)
	}

	// Small trader: qty=500 ≤ 1000 → should NOT appear.
	posSmall, _ := s.position.GetOrCreate("small-participant", instrID)
	posSmall.Quantity = 500
	if err := s.position.Update(posSmall); err != nil {
		t.Fatalf("Update small position: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	path := fmt.Sprintf("/api/v1/securities/reports/frc?type=LARGE_TRADER&date=%s", today)
	resp := doJSON(t, ts, http.MethodGet, path, nil)
	assertStatus(t, resp, http.StatusOK)

	var report map[string]interface{}
	decodeBody(t, resp, &report)

	if report["report_type"] != "LARGE_TRADER" {
		t.Errorf("expected report_type LARGE_TRADER, got %v", report["report_type"])
	}

	data := report["data"].(map[string]interface{})
	count := data["count"].(float64)
	if count != 1 {
		t.Errorf("expected count=1 large trader, got %v", count)
	}

	positions := data["large_trader_positions"].([]interface{})
	if len(positions) != 1 {
		t.Fatalf("expected 1 large_trader_positions entry, got %d", len(positions))
	}

	entry := positions[0].(map[string]interface{})
	if entry["participant_id"] != "large-participant" {
		t.Errorf("expected participant_id large-participant, got %v", entry["participant_id"])
	}
	if entry["instrument_id"] != instrID {
		t.Errorf("expected instrument_id %q, got %v", instrID, entry["instrument_id"])
	}
	qty := entry["quantity"].(float64)
	if qty != 1500 {
		t.Errorf("expected quantity=1500, got %v", qty)
	}
}

// ============================================================
// TestFRCSuspiciousActivity
// ============================================================

// TestFRCSuspiciousActivity verifies that SUSPICIOUS_ACTIVITY report
// returns an empty list (placeholder implementation).
func TestFRCSuspiciousActivity(t *testing.T) {
	ts := newTestServer(t)

	today := time.Now().UTC().Format("2006-01-02")
	path := fmt.Sprintf("/api/v1/securities/reports/frc?type=SUSPICIOUS_ACTIVITY&date=%s", today)
	resp := doJSON(t, ts, http.MethodGet, path, nil)
	assertStatus(t, resp, http.StatusOK)

	var report map[string]interface{}
	decodeBody(t, resp, &report)

	if report["report_type"] != "SUSPICIOUS_ACTIVITY" {
		t.Errorf("expected report_type SUSPICIOUS_ACTIVITY, got %v", report["report_type"])
	}

	data := report["data"].(map[string]interface{})
	count := data["count"].(float64)
	if count != 0 {
		t.Errorf("expected count=0 (placeholder), got %v", count)
	}

	suspiciousActivity := data["suspicious_activity"].([]interface{})
	if len(suspiciousActivity) != 0 {
		t.Errorf("expected empty suspicious_activity list, got %d entries", len(suspiciousActivity))
	}
}

// ============================================================
// TestFRCMissingType
// ============================================================

// TestFRCMissingType verifies that omitting the type query parameter
// returns a 400 validation error.
func TestFRCMissingType(t *testing.T) {
	ts := newTestServer(t)

	t.Run("missing type parameter", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/reports/frc", nil)
		assertStatus(t, resp, http.StatusBadRequest)

		var errResp map[string]interface{}
		decodeBody(t, resp, &errResp)
		errObj := errResp["error"].(map[string]interface{})
		if errObj["code"] != "VALIDATION_ERROR" {
			t.Errorf("expected VALIDATION_ERROR, got %v", errObj["code"])
		}
	})

	t.Run("unknown type parameter", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/reports/frc?type=UNKNOWN_REPORT", nil)
		assertStatus(t, resp, http.StatusBadRequest)

		var errResp map[string]interface{}
		decodeBody(t, resp, &errResp)
		errObj := errResp["error"].(map[string]interface{})
		if errObj["code"] != "VALIDATION_ERROR" {
			t.Errorf("expected VALIDATION_ERROR, got %v", errObj["code"])
		}
	})
}

// ============================================================
// TestFRCDailySummary_DefaultsToToday
// ============================================================

// TestFRCDailySummary_DefaultsToToday verifies that omitting the date parameter
// defaults to today's date and returns a valid (possibly empty) summary.
func TestFRCDailySummary_DefaultsToToday(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/reports/frc?type=DAILY_SUMMARY", nil)
	assertStatus(t, resp, http.StatusOK)

	var report map[string]interface{}
	decodeBody(t, resp, &report)

	today := time.Now().UTC().Format("2006-01-02")
	data := report["data"].(map[string]interface{})
	if data["date"] != today {
		t.Errorf("expected date=%q (today), got %v", today, data["date"])
	}
}

// ============================================================
// TestFRCMethodNotAllowed
// ============================================================

func TestFRCMethodNotAllowed(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/reports/frc?type=DAILY_SUMMARY", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}
