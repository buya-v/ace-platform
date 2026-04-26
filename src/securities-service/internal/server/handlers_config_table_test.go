// Package server — tests for config-table HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newTestServerWithConfigTable creates a test server wired with a ConfigTableStore.
func newTestServerWithConfigTable(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	ctStore := store.NewInMemoryConfigTableStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)

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
		nil, // replayStore
		nil, // bondStore
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, // watchListStore
		nil, // ipRestrictionStore
		nil, // passwordPolicyStore
		nil, // tradingCycleStore
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		nil, // privilegeEngine
		nil, // roleStore
		nil, // tradingParamSetStore
		cfg,
	)
	srv.SetConfigTableStore(ctStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts
}

// ============================================================
// TestCreateConfigTable
// ============================================================

func TestCreateConfigTable(t *testing.T) {
	ts := newTestServerWithConfigTable(t)

	t.Run("201 on valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":       "Standard Fee Schedule",
			"table_type": "FEE_SCHEDULE",
			"rows": []map[string]interface{}{
				{"tier": "A", "rate": 0.001},
			},
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["name"] != "Standard Fee Schedule" {
			t.Errorf("expected name 'Standard Fee Schedule', got %v", result["name"])
		}
		if result["table_type"] != "FEE_SCHEDULE" {
			t.Errorf("expected table_type FEE_SCHEDULE, got %v", result["table_type"])
		}
		if id, ok := result["id"].(string); !ok || id == "" {
			t.Error("expected non-empty id in response")
		}
		if v, ok := result["created_at"].(string); !ok || v == "" {
			t.Error("expected non-empty created_at")
		}
	})

	t.Run("400 when name is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"table_type": "FEE_SCHEDULE",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp, http.StatusBadRequest)

		var errResp map[string]interface{}
		decodeBody(t, resp, &errResp)
		errObj, ok := errResp["error"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected error object, got %T", errResp["error"])
		}
		if errObj["code"] != "VALIDATION_ERROR" {
			t.Errorf("expected code VALIDATION_ERROR, got %v", errObj["code"])
		}
	})

	t.Run("400 when table_type is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"name": "No Type Table",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		resp := doBrokenJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("405 on wrong method", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/config-tables", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})

	t.Run("409 on duplicate ID", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":         "ct-dup",
			"name":       "Dup Table",
			"table_type": "CUSTOM",
		}
		resp1 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp1, http.StatusCreated)
		resp1.Body.Close()

		resp2 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp2, http.StatusConflict)
		resp2.Body.Close()
	})
}

// ============================================================
// TestListByType
// ============================================================

func TestListByType(t *testing.T) {
	ts := newTestServerWithConfigTable(t)

	// Create 2 FEE_SCHEDULE and 1 TAX_RATE tables.
	for _, p := range []map[string]interface{}{
		{"name": "Fee A", "table_type": "FEE_SCHEDULE"},
		{"name": "Fee B", "table_type": "FEE_SCHEDULE"},
		{"name": "Tax A", "table_type": "TAX_RATE"},
	} {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", p)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	t.Run("GET without filter returns all tables", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 3 {
			t.Errorf("expected 3 tables total, got %d", len(result))
		}
	})

	t.Run("GET with table_type=FEE_SCHEDULE returns only fee tables", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables?table_type=FEE_SCHEDULE", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 2 {
			t.Errorf("expected 2 FEE_SCHEDULE tables, got %d", len(result))
		}
		for _, item := range result {
			tbl := item.(map[string]interface{})
			if tbl["table_type"] != "FEE_SCHEDULE" {
				t.Errorf("unexpected table_type %v in FEE_SCHEDULE list", tbl["table_type"])
			}
		}
	})

	t.Run("GET with table_type=TAX_RATE returns 1 table", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables?table_type=TAX_RATE", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 1 {
			t.Errorf("expected 1 TAX_RATE table, got %d", len(result))
		}
	})

	t.Run("GET with unknown type returns empty list", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables?table_type=NONEXISTENT", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 0 {
			t.Errorf("expected 0 tables for unknown type, got %d", len(result))
		}
	})

	t.Run("GET single by ID returns correct table", func(t *testing.T) {
		// Create a known-ID table to look up.
		payload := map[string]interface{}{
			"id":         "ct-known",
			"name":       "Known Table",
			"table_type": "HOLIDAY",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()

		resp2 := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables/ct-known", nil)
		assertStatus(t, resp2, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp2, &result)
		if result["id"] != "ct-known" {
			t.Errorf("expected id ct-known, got %v", result["id"])
		}
		if result["table_type"] != "HOLIDAY" {
			t.Errorf("expected table_type HOLIDAY, got %v", result["table_type"])
		}
	})

	t.Run("GET unknown ID returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables/no-such-id", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ============================================================
// TestConfigTableDeleteEndpoint (covers DELETE handler)
// ============================================================

func TestConfigTableDeleteEndpoint(t *testing.T) {
	ts := newTestServerWithConfigTable(t)

	// Create a table to delete.
	payload := map[string]interface{}{
		"id":         "ct-del",
		"name":       "To Delete",
		"table_type": "CUSTOM",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/config-tables", payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	t.Run("DELETE removes table", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/config-tables/ct-del", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/config-tables/ct-del", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE non-existent returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/config-tables/no-such", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}
