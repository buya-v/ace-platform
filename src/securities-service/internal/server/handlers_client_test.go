// Package server — tests for client entity HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newTestServerWithClient creates a test server wired with a ClientStore.
func newTestServerWithClient(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	clientStore := store.NewInMemoryClientStore()

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
	srv.SetClientStore(clientStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts
}

// ============================================================
// TestCreateClient
// ============================================================

func TestCreateClient(t *testing.T) {
	ts := newTestServerWithClient(t)

	t.Run("201 on valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"firm_id":     "FIRM-A",
			"name":        "Alice Corp",
			"client_type": "INSTITUTIONAL",
			"nationality": "MN",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["name"] != "Alice Corp" {
			t.Errorf("expected name 'Alice Corp', got %v", result["name"])
		}
		if result["firm_id"] != "FIRM-A" {
			t.Errorf("expected firm_id FIRM-A, got %v", result["firm_id"])
		}
		if result["client_type"] != "INSTITUTIONAL" {
			t.Errorf("expected client_type INSTITUTIONAL, got %v", result["client_type"])
		}
		if id, ok := result["id"].(string); !ok || id == "" {
			t.Error("expected non-empty id in response")
		}
		if v, ok := result["created_at"].(string); !ok || v == "" {
			t.Error("expected non-empty created_at in response")
		}
	})

	t.Run("400 when firm_id is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "No Firm Client",
			"client_type": "INDIVIDUAL",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
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

	t.Run("400 when name is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"firm_id":     "FIRM-A",
			"client_type": "INDIVIDUAL",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("400 when client_type is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"firm_id": "FIRM-A",
			"name":    "No Type Client",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		resp := doBrokenJSON(t, ts, http.MethodPost, "/api/v1/securities/clients")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("405 on wrong method", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/clients", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})

	t.Run("409 on duplicate ID", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":          "cli-dup",
			"firm_id":     "FIRM-B",
			"name":        "Dup Client",
			"client_type": "PROPRIETARY",
		}
		resp1 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp1, http.StatusCreated)
		resp1.Body.Close()

		resp2 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp2, http.StatusConflict)
		resp2.Body.Close()
	})
}

// ============================================================
// TestListByFirm
// ============================================================

func TestListByFirm(t *testing.T) {
	ts := newTestServerWithClient(t)

	// Create 2 clients for FIRM-X and 1 for FIRM-Y.
	clients := []map[string]interface{}{
		{"firm_id": "FIRM-X", "name": "Client One", "client_type": "INDIVIDUAL"},
		{"firm_id": "FIRM-X", "name": "Client Two", "client_type": "INSTITUTIONAL"},
		{"firm_id": "FIRM-Y", "name": "Client Three", "client_type": "PROPRIETARY"},
	}
	for _, c := range clients {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", c)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	t.Run("GET without firm_id returns all clients", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 3 {
			t.Errorf("expected 3 clients total, got %d", len(result))
		}
	})

	t.Run("GET with firm_id=FIRM-X returns only 2 clients", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients?firm_id=FIRM-X", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 2 {
			t.Errorf("expected 2 clients for FIRM-X, got %d", len(result))
		}
		for _, item := range result {
			cli := item.(map[string]interface{})
			if cli["firm_id"] != "FIRM-X" {
				t.Errorf("unexpected firm_id %v in FIRM-X list", cli["firm_id"])
			}
		}
	})

	t.Run("GET with firm_id=FIRM-Y returns 1 client", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients?firm_id=FIRM-Y", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 1 {
			t.Errorf("expected 1 client for FIRM-Y, got %d", len(result))
		}
	})

	t.Run("GET with unknown firm_id returns empty list", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients?firm_id=NO-SUCH-FIRM", nil)
		assertStatus(t, resp, http.StatusOK)

		var result []interface{}
		decodeBody(t, resp, &result)
		if len(result) != 0 {
			t.Errorf("expected 0 clients for unknown firm, got %d", len(result))
		}
	})

	t.Run("GET single client by ID returns correct record", func(t *testing.T) {
		// Create a known-ID client.
		payload := map[string]interface{}{
			"id":          "cli-known",
			"firm_id":     "FIRM-Z",
			"name":        "Known Client",
			"client_type": "INSTITUTIONAL",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()

		resp2 := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients/cli-known", nil)
		assertStatus(t, resp2, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp2, &result)
		if result["id"] != "cli-known" {
			t.Errorf("expected id cli-known, got %v", result["id"])
		}
		if result["name"] != "Known Client" {
			t.Errorf("expected name 'Known Client', got %v", result["name"])
		}
	})

	t.Run("GET unknown client ID returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients/no-such-id", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ============================================================
// TestDeleteClient
// ============================================================

func TestDeleteClient(t *testing.T) {
	ts := newTestServerWithClient(t)

	// Create a client to delete.
	payload := map[string]interface{}{
		"id":          "cli-del",
		"firm_id":     "FIRM-DEL",
		"name":        "To Delete",
		"client_type": "INDIVIDUAL",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	t.Run("DELETE returns 204 No Content", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/clients/cli-del", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/clients/cli-del", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE non-existent returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/clients/no-such-client", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("405 on wrong method for item endpoint", func(t *testing.T) {
		// Create a client to test wrong-method on item endpoint.
		p2 := map[string]interface{}{
			"id":          "cli-method",
			"firm_id":     "FIRM-M",
			"name":        "Method Test",
			"client_type": "PROPRIETARY",
		}
		r := doJSON(t, ts, http.MethodPost, "/api/v1/securities/clients", p2)
		assertStatus(t, r, http.StatusCreated)
		r.Body.Close()

		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/clients/cli-method", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}
