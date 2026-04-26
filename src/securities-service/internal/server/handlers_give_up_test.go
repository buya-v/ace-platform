// Package server — tests for give-up HTTP handlers (P4a).
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


// newGiveUpTestServer creates a test server wired with a real GiveUpStore.
func newGiveUpTestServer(t *testing.T) (*httptest.Server, *store.InMemoryGiveUpStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	guStore := store.NewInMemoryGiveUpStore()

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
		guStore,
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
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, guStore
}

// initiateGiveUp submits a give-up via POST /trades/{tradeID}/give-up and returns the response body.
func initiateGiveUp(t *testing.T, ts *httptest.Server, tradeID, toFirmID int) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{
		"to_firm_id": toFirmID,
	}
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/trades/%d/give-up", tradeID),
		payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// ============================================================
// TestInitiateGiveUp
// ============================================================

func TestInitiateGiveUp(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	result := initiateGiveUp(t, ts, 101, 20)

	if result["status"] != "PENDING" {
		t.Errorf("expected status PENDING, got %v", result["status"])
	}
	if result["id"] == nil {
		t.Error("expected id to be set")
	}

	tradeIDVal := result["trade_id"].(float64)
	if int(tradeIDVal) != 101 {
		t.Errorf("expected trade_id 101, got %v", tradeIDVal)
	}
}

func TestInitiateGiveUp_MissingToFirmID(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	payload := map[string]interface{}{}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trades/101/give-up", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestInitiateGiveUp_InvalidTradeID(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	payload := map[string]interface{}{"to_firm_id": 20}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/trades/not-an-int/give-up", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestAcceptGiveUp
// ============================================================

func TestAcceptGiveUp(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	created := initiateGiveUp(t, ts, 55, 30)
	guID := fmt.Sprintf("%v", created["id"])

	// Accept.
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/give-ups/%s/accept", guID),
		nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "ACCEPTED" {
		t.Errorf("expected status ACCEPTED, got %v", result["status"])
	}
	if result["resolved_at"] == nil || result["resolved_at"] == "" {
		t.Error("expected resolved_at to be set")
	}

	// Accept again — 409 Conflict.
	resp2 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/give-ups/%s/accept", guID),
		nil)
	assertStatus(t, resp2, http.StatusConflict)
	resp2.Body.Close()

	// Accept non-existent → 404.
	resp3 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/give-ups/9999/accept", nil)
	assertStatus(t, resp3, http.StatusNotFound)
	resp3.Body.Close()
}

// ============================================================
// TestRejectGiveUp
// ============================================================

func TestRejectGiveUp(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	created := initiateGiveUp(t, ts, 77, 40)
	guID := fmt.Sprintf("%v", created["id"])

	// Reject with a reason.
	rejectPayload := map[string]interface{}{
		"reason": "counterparty not eligible",
	}
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/give-ups/%s/reject", guID),
		rejectPayload)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "REJECTED" {
		t.Errorf("expected status REJECTED, got %v", result["status"])
	}
	if result["reason"] != "counterparty not eligible" {
		t.Errorf("expected reason 'counterparty not eligible', got %v", result["reason"])
	}
	if result["resolved_at"] == nil || result["resolved_at"] == "" {
		t.Error("expected resolved_at to be set")
	}

	// Reject again — 409 Conflict.
	resp2 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/give-ups/%s/reject", guID),
		rejectPayload)
	assertStatus(t, resp2, http.StatusConflict)
	resp2.Body.Close()

	// Reject non-existent → 404.
	resp3 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/give-ups/9999/reject", rejectPayload)
	assertStatus(t, resp3, http.StatusNotFound)
	resp3.Body.Close()
}

// ============================================================
// TestRejectGiveUp_WithoutReason
// ============================================================

func TestRejectGiveUp_WithoutReason(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	created := initiateGiveUp(t, ts, 88, 50)
	guID := fmt.Sprintf("%v", created["id"])

	// Reject without a reason body — should still succeed (reason is optional).
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/give-ups/%s/reject", guID),
		nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "REJECTED" {
		t.Errorf("expected status REJECTED, got %v", result["status"])
	}
}

// ============================================================
// TestListGiveUps
// ============================================================

func TestListGiveUps(t *testing.T) {
	ts, _ := newGiveUpTestServer(t)

	// Create two give-ups via API.
	initiateGiveUp(t, ts, 200, 10)
	initiateGiveUp(t, ts, 201, 20)

	// List all — expect exactly 2.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/give-ups", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	total := result["total"].(float64)
	if total != 2 {
		t.Errorf("expected 2 give-ups, got %v", total)
	}

	// Data array should be present.
	data, ok := result["data"].([]interface{})
	if !ok || len(data) != 2 {
		t.Errorf("expected data array with 2 items, got %v", result["data"])
	}
}
