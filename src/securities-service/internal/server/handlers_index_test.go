// Package server — internal tests for index HTTP handlers (Sprint 8 Part A).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newIndexTestServer creates a test server wired with a fresh InMemoryIndexStore.
// The indexStore is returned so tests can pre-seed data without going through HTTP.
func newIndexTestServer(t *testing.T) (*httptest.Server, store.IndexStore) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	indexStore := store.NewInMemoryIndexStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()

	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,  // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // nodeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil,  // tradingCycleStore
		nil,  // dayManager
		me,   // matchingEngine
		nil,  // sessionManager
		nil,  // settlementEngine
		nil,  // producer
		nil,  // privilegeEngine
		nil,  // roleStore
		nil,  // tradingParamSetStore
		cfg,
	)
	srv.SetIndexStore(indexStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, indexStore
}

// validIndexPayload returns a minimal valid payload for creating an index.
func validIndexPayload() map[string]interface{} {
	return map[string]interface{}{
		"name":       "MSE Top 20",
		"base_value": 1000.0,
		"instrument_weights": map[string]float64{
			"inst-a": 0.6,
			"inst-b": 0.4,
		},
	}
}

// createIndexViaHTTP POSTs to the indices endpoint and returns the created map.
func createIndexViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// ── TestCreateIndex ───────────────────────────────────────────────────────────

func TestCreateIndex(t *testing.T) {
	ts, _ := newIndexTestServer(t)

	t.Run("returns 201 with valid body", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices", validIndexPayload())
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] == nil || result["id"].(string) == "" {
			t.Error("created index must have a non-empty id")
		}
		if result["name"] != "MSE Top 20" {
			t.Errorf("name: want MSE Top 20, got %v", result["name"])
		}
		if result["base_value"].(float64) != 1000.0 {
			t.Errorf("base_value: want 1000.0, got %v", result["base_value"])
		}
		if result["created_at"] == nil || result["created_at"].(string) == "" {
			t.Error("created_at must be populated")
		}
	})

	t.Run("assigns id when not provided", func(t *testing.T) {
		payload := validIndexPayload()
		result := createIndexViaHTTP(t, ts, payload)
		if _, ok := result["id"].(string); !ok {
			t.Error("id must be a string")
		}
	})

	t.Run("respects provided id", func(t *testing.T) {
		payload := validIndexPayload()
		payload["id"] = "custom-idx-id"
		payload["name"] = "Custom ID Index"
		result := createIndexViaHTTP(t, ts, payload)
		if result["id"] != "custom-idx-id" {
			t.Errorf("id: want custom-idx-id, got %v", result["id"])
		}
	})

	t.Run("returns 400 on invalid JSON", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices", "not-json")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("returns 409 on duplicate id", func(t *testing.T) {
		payload := validIndexPayload()
		payload["id"] = "dup-idx"
		payload["name"] = "Dup Index"
		createIndexViaHTTP(t, ts, payload)
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices", payload)
		assertStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})
}

// ── TestListIndices ───────────────────────────────────────────────────────────

func TestListIndices(t *testing.T) {
	ts, idxStore := newIndexTestServer(t)

	// Pre-seed three indices directly through the store.
	for _, idx := range []*types.Index{
		{ID: "idx-list-1", Name: "Alpha", BaseValue: 100.0, CurrentValue: 100.0, CreatedAt: "2026-04-26T00:00:00Z"},
		{ID: "idx-list-2", Name: "Beta", BaseValue: 200.0, CurrentValue: 200.0, CreatedAt: "2026-04-26T00:00:00Z"},
		{ID: "idx-list-3", Name: "Gamma", BaseValue: 300.0, CurrentValue: 300.0, CreatedAt: "2026-04-26T00:00:00Z"},
	} {
		if err := idxStore.Create(idx); err != nil {
			t.Fatalf("seed index %s: %v", idx.ID, err)
		}
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/indices", nil)
	assertStatus(t, resp, http.StatusOK)

	var indices []map[string]interface{}
	decodeBody(t, resp, &indices)

	if len(indices) < 3 {
		t.Fatalf("expected at least 3 indices, got %d", len(indices))
	}
}

// ── TestGetIndex ──────────────────────────────────────────────────────────────

func TestGetIndex(t *testing.T) {
	ts, _ := newIndexTestServer(t)

	created := createIndexViaHTTP(t, ts, validIndexPayload())
	id := created["id"].(string)

	t.Run("returns 200 with correct fields", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/indices/%s", id), nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] != id {
			t.Errorf("id: want %s, got %v", id, result["id"])
		}
		if result["name"] != "MSE Top 20" {
			t.Errorf("name: want MSE Top 20, got %v", result["name"])
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/indices/no-such-idx", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestDeleteIndex ───────────────────────────────────────────────────────────

func TestDeleteIndex(t *testing.T) {
	ts, _ := newIndexTestServer(t)

	payload := validIndexPayload()
	payload["id"] = "idx-to-delete"
	payload["name"] = "Delete Me"
	createIndexViaHTTP(t, ts, payload)

	t.Run("returns 204 on success", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/indices/idx-to-delete", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/indices/idx-to-delete", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("second DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/indices/idx-to-delete", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE unknown id returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/indices/no-such-idx", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestCalculateIndex ────────────────────────────────────────────────────────

func TestCalculateIndex(t *testing.T) {
	ts, _ := newIndexTestServer(t)

	payload := validIndexPayload()
	payload["id"] = "idx-calc"
	payload["name"] = "Calc Index"
	payload["base_value"] = 500.0
	payload["current_value"] = 500.0
	createIndexViaHTTP(t, ts, payload)

	t.Run("returns 200 with updated current_value", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices/idx-calc/calculate", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] != "idx-calc" {
			t.Errorf("id: want idx-calc, got %v", result["id"])
		}
		// current_value must be non-zero after calculation.
		if result["current_value"].(float64) == 0 {
			t.Error("current_value should be non-zero after calculate")
		}
		if result["last_calculated_at"] == nil || result["last_calculated_at"].(string) == "" {
			t.Error("last_calculated_at must be populated after calculate")
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices/no-such-idx/calculate", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("JSON round-trip via types.Index", func(t *testing.T) {
		payload2 := map[string]interface{}{
			"id":         "idx-rt",
			"name":       "Round Trip",
			"base_value": 1000.0,
			"instrument_weights": map[string]float64{
				"inst-x": 0.5,
				"inst-y": 0.5,
			},
		}
		createIndexViaHTTP(t, ts, payload2)

		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/indices/idx-rt/calculate", nil)
		assertStatus(t, resp, http.StatusOK)

		var idx types.Index
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
			t.Fatalf("decode into types.Index: %v", err)
		}
		if idx.ID == "" {
			t.Error("Index.ID must not be empty")
		}
		if idx.Name != "Round Trip" {
			t.Errorf("Index.Name: want Round Trip, got %s", idx.Name)
		}
		if idx.LastCalculatedAt == "" {
			t.Error("Index.LastCalculatedAt must be set after calculate")
		}
	})
}

// ── TestIndexHandlers_Unconfigured ────────────────────────────────────────────

// TestIndexHandlers_Unconfigured verifies that handlers return 503 when the
// indexStore is nil (server not configured).
func TestIndexHandlers_Unconfigured(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, me, nil, nil,
		nil, nil, nil, nil, cfg,
	)
	// Do NOT call srv.SetIndexStore() — leave it nil.
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/securities/indices"},
		{http.MethodPost, "/api/v1/securities/indices"},
		{http.MethodGet, "/api/v1/securities/indices/some-id"},
		{http.MethodDelete, "/api/v1/securities/indices/some-id"},
		{http.MethodPost, "/api/v1/securities/indices/some-id/calculate"},
	} {
		resp := doJSON(t, ts, tc.method, tc.path, map[string]interface{}{"name": "x"})
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: want 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
