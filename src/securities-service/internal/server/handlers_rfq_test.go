// Package server — tests for RFQ HTTP handlers (P4a).
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

// newRFQTestServer creates a test server wired with a real RFQStore.
func newRFQTestServer(t *testing.T) (*httptest.Server, *store.InMemoryRFQStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	rfqStore := store.NewInMemoryRFQStore()

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
		rfqStore,
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
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, rfqStore
}

// submitRFQ creates an RFQ via the API and returns the parsed response body.
func submitRFQ(t *testing.T, ts *httptest.Server, instrumentID int, side string, qty int) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{
		"instrument_id":    instrumentID,
		"requestor_firm_id": 5,
		"quantity":         qty,
		"side":             side,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/rfq", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// ============================================================
// TestSubmitRFQ
// ============================================================

func TestSubmitRFQ(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	result := submitRFQ(t, ts, 42, "BUY", 1000)

	if result["status"] != "OPEN" {
		t.Errorf("expected status OPEN, got %v", result["status"])
	}
	if result["id"] == nil {
		t.Error("expected id to be set")
	}
	if result["created_at"] == nil {
		t.Error("expected created_at to be set")
	}
}

func TestSubmitRFQ_MissingInstrumentID(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	payload := map[string]interface{}{
		"quantity": 100,
		"side":     "BUY",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/rfq", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

func TestSubmitRFQ_InvalidSide(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	payload := map[string]interface{}{
		"instrument_id": 1,
		"quantity":      100,
		"side":          "INVALID",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/rfq", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

func TestSubmitRFQ_ZeroQuantity(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	payload := map[string]interface{}{
		"instrument_id": 1,
		"quantity":      0,
		"side":          "SELL",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/rfq", payload)
	assertStatus(t, resp, http.StatusUnprocessableEntity)
	resp.Body.Close()
}

// ============================================================
// TestRespondToRFQ
// ============================================================

func TestRespondToRFQ(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	// Create an RFQ first.
	created := submitRFQ(t, ts, 7, "SELL", 500)
	rfqID := fmt.Sprintf("%v", created["id"])

	// Respond to it.
	respondPayload := map[string]interface{}{
		"quote_id": "Q-0001",
	}
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/rfq/%s/respond", rfqID),
		respondPayload)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "RESPONDED" {
		t.Errorf("expected status RESPONDED, got %v", result["status"])
	}

	// Respond again — should fail with 409.
	resp2 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/rfq/%s/respond", rfqID),
		respondPayload)
	assertStatus(t, resp2, http.StatusConflict)
	resp2.Body.Close()

	// Missing quote_id → 400.
	created2 := submitRFQ(t, ts, 8, "BUY", 200)
	rfq2ID := fmt.Sprintf("%v", created2["id"])
	resp3 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/rfq/%s/respond", rfq2ID),
		map[string]interface{}{})
	assertStatus(t, resp3, http.StatusBadRequest)
	resp3.Body.Close()

	// Respond to non-existent RFQ → 404.
	resp4 := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/rfq/9999/respond",
		map[string]interface{}{"quote_id": "Q-X"})
	assertStatus(t, resp4, http.StatusNotFound)
	resp4.Body.Close()
}

// ============================================================
// TestCancelRFQ
// ============================================================

func TestCancelRFQ(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	created := submitRFQ(t, ts, 3, "BUY", 300)
	rfqID := fmt.Sprintf("%v", created["id"])

	// Cancel.
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/rfq/%s/cancel", rfqID),
		nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != "CANCELLED" {
		t.Errorf("expected status CANCELLED, got %v", result["status"])
	}

	// Cancel again — should fail with 409.
	resp2 := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/rfq/%s/cancel", rfqID),
		nil)
	assertStatus(t, resp2, http.StatusConflict)
	resp2.Body.Close()

	// Cancel non-existent → 404.
	resp3 := doJSON(t, ts, http.MethodPost, "/api/v1/securities/rfq/9999/cancel", nil)
	assertStatus(t, resp3, http.StatusNotFound)
	resp3.Body.Close()
}

// ============================================================
// TestListRFQ
// ============================================================

func TestListRFQ(t *testing.T) {
	ts, _ := newRFQTestServer(t)

	// Seed two RFQs.
	submitRFQ(t, ts, 10, "BUY", 100)
	submitRFQ(t, ts, 10, "SELL", 200)

	t.Run("list all", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/rfq", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"].(float64) != 2 {
			t.Errorf("expected total 2, got %v", result["total"])
		}
	})

	t.Run("filter by status OPEN", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/rfq?status=OPEN", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"].(float64) != 2 {
			t.Errorf("expected 2 OPEN RFQs, got %v", result["total"])
		}
	})

	t.Run("filter by status CANCELLED returns empty", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/rfq?status=CANCELLED", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"].(float64) != 0 {
			t.Errorf("expected 0 CANCELLED RFQs, got %v", result["total"])
		}
	})
}
