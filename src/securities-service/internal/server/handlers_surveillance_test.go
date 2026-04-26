// Package server — tests for surveillance HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newSurveillanceTestServer creates a test server wired with a real SurveillanceStore.
func newSurveillanceTestServer(t *testing.T) (*httptest.Server, *store.InMemorySurveillanceStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	survStore := store.NewInMemorySurveillanceStore()

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, // tickTableStore
		nil, // tradeCorrectionStore
		nil, // throttleStore
		nil, // throttleConfigStore
		nil, // announcementStore
		nil, // auditStore
		nil, // pendingChangeStore
		nil, // referencePriceStore
		survStore,
		nil, // instrumentGroupStore
		nil, // offBookTradeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		nil, // bondStore
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, survStore
}

// seedAlert creates a surveillance alert directly into the store.
func seedAlert(t *testing.T, s *store.InMemorySurveillanceStore, alert *types.SurveillanceAlert) {
	t.Helper()
	if err := s.CreateAlert(alert); err != nil {
		t.Fatalf("seedAlert %s: %v", alert.ID, err)
	}
}

// ============================================================
// TestListAlerts
// ============================================================

func TestListAlerts_Empty(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/alerts", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["total"] != float64(0) {
		t.Errorf("expected total 0, got %v", result["total"])
	}
}

func TestListAlerts_ReturnsTwoAlerts(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "a1", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "alert 1", CreatedAt: "2026-01-01T00:00:00Z",
	})
	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "a2", InstrumentID: "I2", AlertType: types.AlertTypePriceSpike,
		Status: types.AlertStatusOpen, Message: "alert 2", CreatedAt: "2026-01-01T00:00:00Z",
	})

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/alerts", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["total"] != float64(2) {
		t.Errorf("expected total 2, got %v", result["total"])
	}
	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 alerts in data, got %d", len(data))
	}
}

func TestListAlerts_FilterByStatus(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "open-1", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "open", CreatedAt: "2026-01-01T00:00:00Z",
	})
	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "res-1", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusResolved, Message: "resolved", CreatedAt: "2026-01-01T00:00:00Z",
		ResolvedAt: "2026-01-01T01:00:00Z",
	})

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/alerts?status=OPEN", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(1) {
		t.Errorf("expected 1 OPEN alert, got %v", result["total"])
	}
}

// ============================================================
// TestResolveAlert
// ============================================================

func TestResolveAlert_Success(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "to-resolve", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusOpen, Message: "needs resolution", CreatedAt: "2026-01-01T00:00:00Z",
	})

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/surveillance/alerts/to-resolve/resolve",
		map[string]string{"resolved_by": "analyst-99"})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "resolved" {
		t.Errorf("expected status resolved, got %v", result["status"])
	}

	// Verify in store.
	alerts, _ := survStore.ListAlerts(store.SurveillanceAlertFilters{Status: types.AlertStatusResolved})
	if len(alerts) != 1 {
		t.Errorf("expected 1 resolved alert in store, got %d", len(alerts))
	}
}

func TestResolveAlert_NotFound(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/surveillance/alerts/no-such-alert/resolve",
		map[string]string{"resolved_by": "analyst"})
	// Should return 4xx (404 or 400 depending on store error message).
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for not-found alert, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestResolveAlert_AlreadyResolved(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	seedAlert(t, survStore, &types.SurveillanceAlert{
		ID: "already-res", InstrumentID: "I1", AlertType: types.AlertTypeLargeTrade,
		Status: types.AlertStatusResolved, Message: "already done", CreatedAt: "2026-01-01T00:00:00Z",
		ResolvedAt: "2026-01-01T01:00:00Z", ResolvedBy: "first-analyst",
	})

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/surveillance/alerts/already-res/resolve",
		map[string]string{"resolved_by": "second-analyst"})
	// Should return 400 (already resolved).
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for already-resolved alert, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================
// TestSetThreshold
// ============================================================

func TestSetThreshold_Success(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	payload := map[string]interface{}{
		"alert_type": "LARGE_TRADE",
		"value":      500.0,
	}
	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/surveillance/thresholds/INST-THRESH",
		payload)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["instrument_id"] != "INST-THRESH" {
		t.Errorf("expected instrument_id INST-THRESH, got %v", result["instrument_id"])
	}
	if result["value"] != float64(500) {
		t.Errorf("expected value 500, got %v", result["value"])
	}

	// Verify stored.
	thresholds, _ := survStore.GetThresholds("INST-THRESH")
	if len(thresholds) != 1 {
		t.Errorf("expected 1 threshold in store, got %d", len(thresholds))
	}
}

func TestSetThreshold_MissingAlertType(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/surveillance/thresholds/INST-X",
		map[string]interface{}{"value": 100.0})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestGetThresholds
// ============================================================

func TestGetThresholds_Empty(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/thresholds/INST-NONE", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("expected 0 thresholds, got %v", result["total"])
	}
}

func TestGetThresholds_ReturnsTwoThresholds(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	survStore.SetThreshold(&types.SurveillanceThreshold{
		InstrumentID: "INST-T", AlertType: types.AlertTypeLargeTrade, Value: 1000,
	})
	survStore.SetThreshold(&types.SurveillanceThreshold{
		InstrumentID: "INST-T", AlertType: types.AlertTypePriceSpike, Value: 200,
	})

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/surveillance/thresholds/INST-T", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(2) {
		t.Errorf("expected 2 thresholds, got %v", result["total"])
	}
	if result["instrument_id"] != "INST-T" {
		t.Errorf("expected instrument_id INST-T, got %v", result["instrument_id"])
	}
}

// ============================================================
// Not configured (503) tests
// ============================================================

func TestSurveillanceEndpoints_NotConfigured(t *testing.T) {
	// Build server without surveillance store.
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // surveillanceStore = nil
		nil, nil,
		nil, nil, nil, // locateStore, rfqStore, giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []string{
		"/api/v1/securities/surveillance/alerts",
		fmt.Sprintf("/api/v1/securities/surveillance/alerts/%s/resolve", "some-id"),
		"/api/v1/securities/surveillance/thresholds/INST-1",
	}
	methods := []string{http.MethodGet, http.MethodPut, http.MethodGet}

	for i, path := range paths {
		resp := doJSON(t, httpTS, methods[i], path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
