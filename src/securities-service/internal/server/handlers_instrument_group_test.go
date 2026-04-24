// Package server — tests for instrument group HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newGroupTestServer creates a test server wired with a real InstrumentGroupStore.
func newGroupTestServer(t *testing.T) (*httptest.Server, *store.InMemoryInstrumentGroupStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	groupStore := store.NewInMemoryInstrumentGroupStore()

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
		nil, nil, nil, nil, nil, nil, nil,
		nil, // surveillanceStore
		groupStore,
		nil, // offBookTradeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		nil, // bondStore
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
	return ts, groupStore
}

// createGroup creates an instrument group via POST and returns its ID.
func createGroupViaHTTP(t *testing.T, ts *httptest.Server, name string, groupType string) string {
	t.Helper()
	payload := map[string]interface{}{
		"name":       name,
		"group_type": groupType,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instrument-groups", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, ok := result["id"].(string)
	if !ok || id == "" {
		t.Fatal("expected non-empty id in create response")
	}
	return id
}

// ============================================================
// TestCreateGroup
// ============================================================

func TestCreateGroup_Success(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	payload := map[string]interface{}{
		"name":          "Blue Chips",
		"description":   "Top 10 market cap stocks",
		"group_type":    "MANUAL",
		"instrument_ids": []string{"INST-A", "INST-B"},
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instrument-groups", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["name"] != "Blue Chips" {
		t.Errorf("expected name Blue Chips, got %v", result["name"])
	}
	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["group_type"] != "MANUAL" {
		t.Errorf("expected group_type MANUAL, got %v", result["group_type"])
	}
}

func TestCreateGroup_MissingName(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	payload := map[string]interface{}{
		"group_type": "MANUAL",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instrument-groups", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateGroup_DefaultsGroupType(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	payload := map[string]interface{}{
		"name": "No Type Group",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instrument-groups", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["group_type"] != "MANUAL" {
		t.Errorf("expected default group_type MANUAL, got %v", result["group_type"])
	}
}

// ============================================================
// TestListGroups
// ============================================================

func TestListGroups_Empty(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instrument-groups", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("expected total 0, got %v", result["total"])
	}
}

func TestListGroups_ReturnsTwoGroups(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	for _, name := range []string{"GroupA", "GroupB"} {
		createGroupViaHTTP(t, ts, name, "MANUAL")
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instrument-groups", nil)
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
		t.Errorf("expected 2 groups in data, got %d", len(data))
	}
}

// ============================================================
// TestDeleteGroup
// ============================================================

func TestDeleteGroup_Success(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	id1 := createGroupViaHTTP(t, ts, "ToDelete", "MANUAL")
	createGroupViaHTTP(t, ts, "ToKeep", "MANUAL")

	resp := doJSON(t, ts, http.MethodDelete,
		fmt.Sprintf("/api/v1/securities/instrument-groups/%s", id1), nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// List — should have 1 remaining.
	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/instrument-groups", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(1) {
		t.Errorf("expected total 1 after delete, got %v", result["total"])
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete,
		"/api/v1/securities/instrument-groups/no-such-group", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestGetGroup
// ============================================================

func TestGetGroup_Success(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	id := createGroupViaHTTP(t, ts, "GetMe", "SECTOR")

	resp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/instrument-groups/%s", id), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["name"] != "GetMe" {
		t.Errorf("expected name GetMe, got %v", result["name"])
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	ts, _ := newGroupTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/instrument-groups/no-such-id", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// Not configured (503) test
// ============================================================

func TestInstrumentGroupEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil, nil, nil, nil, nil,
		nil,  // surveillanceStore
		nil,  // instrumentGroupStore = nil
		nil,  // offBookTradeStore
		nil, nil, nil, // locateStore, rfqStore, giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	for _, path := range []string{
		"/api/v1/securities/instrument-groups",
		"/api/v1/securities/instrument-groups/some-id",
	} {
		resp := doJSON(t, httpTS, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
