// Package server — tests for market replay HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newReplayTestServer creates a test server wired with a real ReplayStore.
func newReplayTestServer(t *testing.T) (*httptest.Server, *store.InMemoryReplayStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	rpStore := store.NewInMemoryReplayStore()

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
		nil, // surveillanceStore
		nil, // instrumentGroupStore
		nil, // offBookTradeStore
		nil, // nodeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		rpStore,
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
	return ts, rpStore
}

// createReplaySessionViaHTTP creates a replay session via POST and returns the session ID.
func createReplaySessionViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/replay/sessions", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

// ============================================================
// TestCreateReplaySession
// ============================================================

func TestCreateReplaySession(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	payload := map[string]interface{}{
		"id":            "REPLAY-HTTP-1",
		"instrument_id": "INST-REPLAY",
		"start_time":    "2026-04-24T09:00:00Z",
		"end_time":      "2026-04-24T17:00:00Z",
		"description":   "Full trading day replay for INST-REPLAY",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/replay/sessions", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "REPLAY-HTTP-1" {
		t.Errorf("id: want REPLAY-HTTP-1, got %v", result["id"])
	}
	if result["instrument_id"] != "INST-REPLAY" {
		t.Errorf("instrument_id: want INST-REPLAY, got %v", result["instrument_id"])
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set on create")
	}
}

func TestCreateReplaySession_MissingFields(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	// Missing instrument_id.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/replay/sessions",
		map[string]interface{}{"id": "REPLAY-BAD"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Missing id.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/replay/sessions",
		map[string]interface{}{"instrument_id": "INST-X"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateReplaySession_Duplicate(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	payload := map[string]interface{}{
		"id":            "REPLAY-DUP",
		"instrument_id": "INST-DUP",
	}
	createReplaySessionViaHTTP(t, ts, payload)

	// Second create with same ID should return 409.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/replay/sessions", payload)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

func TestListReplaySessions(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	// Empty initially.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/replay/sessions", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("initial total: want 0, got %v", result["total"])
	}

	// Create two sessions.
	createReplaySessionViaHTTP(t, ts, map[string]interface{}{"id": "S1", "instrument_id": "I1"})
	createReplaySessionViaHTTP(t, ts, map[string]interface{}{"id": "S2", "instrument_id": "I2"})

	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/replay/sessions", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)
	if result["total"] != float64(2) {
		t.Errorf("total after 2 creates: want 2, got %v", result["total"])
	}
}

// ============================================================
// TestGetReplayEvents — events returned in sequence order
// ============================================================

func TestGetReplayEvents(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	sessionID := createReplaySessionViaHTTP(t, ts, map[string]interface{}{
		"id":            "REPLAY-EV-SESSION",
		"instrument_id": "INST-EV",
	})

	// Add 3 events out of sequence order.
	eventsPayload := []map[string]interface{}{
		{"sequence": 3, "event_type": "TRADE", "occurred_at": "2026-04-24T10:02:00Z"},
		{"sequence": 1, "event_type": "ORDER", "occurred_at": "2026-04-24T09:30:00Z"},
		{"sequence": 2, "event_type": "ORDER", "occurred_at": "2026-04-24T10:01:00Z"},
	}
	for _, ev := range eventsPayload {
		resp := doJSON(t, ts,
			http.MethodPost,
			"/api/v1/securities/replay/sessions/"+sessionID+"/events",
			ev)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	// GetEvents should return them sorted by sequence.
	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/replay/sessions/"+sessionID+"/events", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["total"] != float64(3) {
		t.Errorf("total: want 3, got %v", result["total"])
	}

	events, _ := result["events"].([]interface{})
	if len(events) != 3 {
		t.Fatalf("events count: want 3, got %d", len(events))
	}

	for i, ev := range events {
		item, _ := ev.(map[string]interface{})
		wantSeq := float64(i + 1)
		if item["sequence"] != wantSeq {
			t.Errorf("events[%d].sequence: want %.0f, got %v", i, wantSeq, item["sequence"])
		}
	}

	// First event should be ORDER (sequence 1).
	first, _ := events[0].(map[string]interface{})
	if first["event_type"] != "ORDER" {
		t.Errorf("events[0].event_type: want ORDER, got %v", first["event_type"])
	}

	// Last event should be TRADE (sequence 3).
	last, _ := events[2].(map[string]interface{})
	if last["event_type"] != "TRADE" {
		t.Errorf("events[2].event_type: want TRADE, got %v", last["event_type"])
	}
}

func TestGetReplayEvents_UnknownSession(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/replay/sessions/NO-SESSION/events", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("total: want 0 for unknown session, got %v", result["total"])
	}
}

func TestGetReplayEvents_MissingEventType(t *testing.T) {
	ts, _ := newReplayTestServer(t)

	createReplaySessionViaHTTP(t, ts, map[string]interface{}{
		"id":            "REPLAY-EV-BAD",
		"instrument_id": "INST-BAD",
	})

	// Missing event_type should return 400.
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/replay/sessions/REPLAY-EV-BAD/events",
		map[string]interface{}{"sequence": 1})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestReplayEndpoints_NotConfigured (503)
// ============================================================

func TestReplayEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil, nil, nil, nil, nil,
		nil,        // investigationStore
		nil,        // replayStore = nil
		nil,        // bondStore
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []string{
		"/api/v1/securities/replay/sessions",
		"/api/v1/securities/replay/sessions/some-id",
		"/api/v1/securities/replay/sessions/some-id/events",
	}
	for _, path := range paths {
		resp := doJSON(t, httpTS, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
