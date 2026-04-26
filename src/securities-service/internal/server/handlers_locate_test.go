// Package server — tests for locate HTTP handlers (P4a).
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

// newLocateTestServer creates a test server wired with real Locate, Instrument,
// and Order stores so that all locate and short-sell order paths are reachable.
func newLocateTestServer(t *testing.T) (*httptest.Server, *store.InMemoryLocateStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	locStore := store.NewInMemoryLocateStore()

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
		locStore,
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
		nil, // dayManager
		me,
		nil, // sessionManager
		nil, // settlementEngine
		nil, // producer
		nil, // privilegeEngine
		nil, // roleStore
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, locStore
}

// ============================================================
// TestRequestLocate_Success
// ============================================================

func TestRequestLocate_Success(t *testing.T) {
	ts, _ := newLocateTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":   99,
		"borrower_firm_id": 10,
		"quantity":        500,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["status"] != "PENDING" {
		t.Errorf("expected status PENDING, got %v", result["status"])
	}
	if result["id"] == nil {
		t.Error("expected id to be set in response")
	}
}

func TestRequestLocate_MissingInstrumentID(t *testing.T) {
	ts, _ := newLocateTestServer(t)

	payload := map[string]interface{}{
		"borrower_firm_id": 10,
		"quantity":         100,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

func TestRequestLocate_ZeroQuantity(t *testing.T) {
	ts, _ := newLocateTestServer(t)

	payload := map[string]interface{}{
		"instrument_id":   5,
		"borrower_firm_id": 10,
		"quantity":         0,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

// ============================================================
// TestApproveLocate
// ============================================================

func TestApproveLocate(t *testing.T) {
	ts, locStore := newLocateTestServer(t)

	// Seed a locate directly.
	payload := map[string]interface{}{
		"instrument_id":   42,
		"borrower_firm_id": 5,
		"quantity":        200,
	}
	createResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", payload)
	assertStatus(t, createResp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, createResp, &created)
	locateID := fmt.Sprintf("%v", created["id"])

	// Approve via HTTP.
	approvePayload := map[string]interface{}{
		"lender_firm_id": "LENDER-A",
	}
	approveResp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/locates/%s/approve", locateID),
		approvePayload)
	assertStatus(t, approveResp, http.StatusOK)

	var approved map[string]interface{}
	decodeBody(t, approveResp, &approved)
	if approved["status"] != "APPROVED" {
		t.Errorf("expected status APPROVED, got %v", approved["status"])
	}

	// Approving again should return 409 Conflict.
	resp2 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/locates/%s/approve", locateID),
		approvePayload)
	assertStatus(t, resp2, http.StatusConflict)
	resp2.Body.Close()

	// Approve non-existent locate → 404.
	resp3 := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/locates/9999/approve",
		approvePayload)
	assertStatus(t, resp3, http.StatusNotFound)
	resp3.Body.Close()

	// Suppress unused variable warning.
	_ = locStore
}

// ============================================================
// TestShortSellWithLocate / TestShortSellWithoutLocate
// ============================================================

// createActiveInstrForLocate creates an ACTIVE instrument via the API and returns its ID.
func createActiveInstrForLocate(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	payload := map[string]interface{}{
		"ticker":      "SHRT",
		"name":        "Short Corp",
		"asset_class": "EQUITY",
		"lot_size":    1,
		"tick_size":   0.01,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	return created["id"].(string)
}

func TestShortSellWithLocate(t *testing.T) {
	ts, locStore := newLocateTestServer(t)

	instrID := createActiveInstrForLocate(t, ts)

	// Create and approve a locate.
	locReq := map[string]interface{}{
		"instrument_id":   1, // int type in store
		"borrower_firm_id": 10,
		"quantity":        10,
	}
	locResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", locReq)
	assertStatus(t, locResp, http.StatusCreated)
	var locCreated map[string]interface{}
	decodeBody(t, locResp, &locCreated)
	locIDNum := locCreated["id"]
	locIDStr := fmt.Sprintf("%v", locIDNum)

	// Approve it directly via store to avoid extra HTTP round-trip.
	if err := locStore.Approve(locIDStr, "LENDER"); err != nil {
		t.Fatalf("store Approve: %v", err)
	}

	// Submit SHORT_SELL with the approved locate.
	orderPayload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "SHORT_SELL",
		"order_type":     "LIMIT",
		"quantity":       1,
		"price":          10.00,
		"locate_id":      locIDStr,
	}
	orderResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", orderPayload)
	// Should be accepted (201 Created or 200 — any 2xx).
	if orderResp.StatusCode != http.StatusCreated && orderResp.StatusCode != http.StatusOK {
		var body map[string]interface{}
		decodeBody(t, orderResp, &body)
		t.Fatalf("expected 2xx for SHORT_SELL with valid locate, got %d: %v", orderResp.StatusCode, body)
	}
	orderResp.Body.Close()

	// Locate should now be USED — cannot submit another SHORT_SELL with same locate.
	order2Payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "SHORT_SELL",
		"order_type":     "LIMIT",
		"quantity":       1,
		"price":          10.00,
		"locate_id":      locIDStr,
	}
	order2Resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", order2Payload)
	assertStatus(t, order2Resp, http.StatusUnprocessableEntity)
	order2Resp.Body.Close()
}

func TestShortSellWithoutLocate(t *testing.T) {
	ts, _ := newLocateTestServer(t)

	instrID := createActiveInstrForLocate(t, ts)

	payload := map[string]interface{}{
		"instrument_id":  instrID,
		"participant_id": "P-001",
		"side":           "SHORT_SELL",
		"order_type":     "LIMIT",
		"quantity":       1,
		"price":          10.00,
		// No locate_id.
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/orders", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)

	errObj, _ := errResp["error"].(map[string]interface{})
	if errObj == nil || errObj["code"] != "LOCATE_REQUIRED" {
		t.Errorf("expected LOCATE_REQUIRED error code, got %v", errObj)
	}
}

// ============================================================
// TestListLocates
// ============================================================

func TestListLocates(t *testing.T) {
	ts, _ := newLocateTestServer(t)

	// Create two locates.
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", map[string]interface{}{
		"instrument_id": 1, "borrower_firm_id": 10, "quantity": 100,
	}).Body.Close()
	doJSON(t, ts, http.MethodPost, "/api/v1/securities/locates", map[string]interface{}{
		"instrument_id": 2, "borrower_firm_id": 20, "quantity": 200,
	}).Body.Close()

	// List all.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/locates", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	total := result["total"].(float64)
	if total != 2 {
		t.Errorf("expected total 2, got %v", total)
	}

	// Filter by firm_id=10.
	resp2 := doJSON(t, ts, http.MethodGet, "/api/v1/securities/locates?firm_id=10", nil)
	assertStatus(t, resp2, http.StatusOK)
	var result2 map[string]interface{}
	decodeBody(t, resp2, &result2)
	if result2["total"].(float64) != 1 {
		t.Errorf("expected 1 locate for firm 10, got %v", result2["total"])
	}
}
