// Package server — tests for post-trade params HTTP handlers.
package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newTestServerWithPostTradeParams creates a test server wired with a PostTradeParamsStore.
func newTestServerWithPostTradeParams(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	ptpStore := store.NewInMemoryPostTradeParamsStore()

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
	srv.SetPostTradeParamsStore(ptpStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts
}

// doBrokenJSON sends a request with invalid JSON body to exercise 400 paths.
func doBrokenJSON(t *testing.T, ts *httptest.Server, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewBufferString("{invalid"))
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GarudaX-Tenant", testTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do: %v", err)
	}
	return resp
}

// ============================================================
// TestCreatePostTradeParams
// ============================================================

func TestCreatePostTradeParams(t *testing.T) {
	ts := newTestServerWithPostTradeParams(t)

	t.Run("201 on valid payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"instrument_id":    "INST-TEST",
			"settlement_cycle": "T+2",
			"clearing_firm_id": "CF-001",
			"fee_schedule_id":  "FS-001",
			"penalty_rate_pct": 0.5,
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params", payload)
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["instrument_id"] != "INST-TEST" {
			t.Errorf("expected instrument_id INST-TEST, got %v", result["instrument_id"])
		}
		if id, ok := result["id"].(string); !ok || id == "" {
			t.Error("expected non-empty id in response")
		}
		if result["settlement_cycle"] != "T+2" {
			t.Errorf("expected settlement_cycle T+2, got %v", result["settlement_cycle"])
		}
		if v, ok := result["created_at"].(string); !ok || v == "" {
			t.Error("expected non-empty created_at in response")
		}
	})

	t.Run("400 when instrument_id is missing", func(t *testing.T) {
		payload := map[string]interface{}{
			"settlement_cycle": "T+2",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params", payload)
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

	t.Run("400 on invalid JSON body", func(t *testing.T) {
		resp := doBrokenJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("405 on wrong method", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/post-trade-params", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})

	t.Run("409 on duplicate ID", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":               "ptp-dup",
			"instrument_id":    "INST-DUP",
			"settlement_cycle": "T+2",
		}
		resp1 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params", payload)
		assertStatus(t, resp1, http.StatusCreated)
		resp1.Body.Close()

		resp2 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params", payload)
		assertStatus(t, resp2, http.StatusConflict)
		resp2.Body.Close()
	})
}

// ============================================================
// TestGetPostTradeParamsByInstrument
// ============================================================

func TestGetPostTradeParamsByInstrument(t *testing.T) {
	ts := newTestServerWithPostTradeParams(t)

	// Create a record first.
	payload := map[string]interface{}{
		"instrument_id":    "INST-LOOKUP",
		"settlement_cycle": "T+3",
		"clearing_firm_id": "CF-002",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params", payload)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	t.Run("GET by instrument ID returns matching record", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/post-trade-params/instrument/INST-LOOKUP", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["instrument_id"] != "INST-LOOKUP" {
			t.Errorf("expected instrument_id INST-LOOKUP, got %v", result["instrument_id"])
		}
		if result["settlement_cycle"] != "T+3" {
			t.Errorf("expected settlement_cycle T+3, got %v", result["settlement_cycle"])
		}
	})

	t.Run("404 for unknown instrument", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/post-trade-params/instrument/NO-SUCH", nil)
		assertStatus(t, resp, http.StatusNotFound)

		var errResp map[string]interface{}
		decodeBody(t, resp, &errResp)
		errObj, ok := errResp["error"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected error object, got %T", errResp["error"])
		}
		if errObj["code"] != "NOT_FOUND" {
			t.Errorf("expected code NOT_FOUND, got %v", errObj["code"])
		}
	})

	t.Run("405 on wrong method for instrument lookup", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/post-trade-params/instrument/INST-LOOKUP", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}
