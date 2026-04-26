// Package server — tests for CSD (custody account, balance, transfer) HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// csdTestStores bundles all three CSD stores for convenience.
type csdTestStores struct {
	accounts  *store.InMemoryCustodyAccountStore
	balances  *store.InMemoryCustodyBalanceStore
	transfers *store.InMemoryCSDTransferStore
}

// newCSDTestServer creates a test server wired with all CSD stores.
func newCSDTestServer(t *testing.T) (*httptest.Server, csdTestStores) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)

	stores := csdTestStores{
		accounts:  store.NewInMemoryCustodyAccountStore(),
		balances:  store.NewInMemoryCustodyBalanceStore(),
		transfers: store.NewInMemoryCSDTransferStore(),
	}

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
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, // investigationStore
		nil, // replayStore
		nil, // bondStore
		nil, // strategyStore
		stores.accounts,
		stores.balances,
		stores.transfers,
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
	return ts, stores
}

// ── Helper payloads and factories ─────────────────────────────────────────────

func validAccountPayload(id, firmID string) map[string]interface{} {
	return map[string]interface{}{
		"id":        id,
		"firm_id":   firmID,
		"name":      "Main Custody Account",
		"currency":  "MNT",
		"tenant_id": testTenant,
	}
}

func validTransferPayload(id, fromID, toID string, transferType string) map[string]interface{} {
	payload := map[string]interface{}{
		"id":             id,
		"from_account_id": fromID,
		"to_account_id":  toID,
		"instrument_id":  "WHEAT-FUT-JAN",
		"quantity":       100,
		"transfer_type":  transferType,
		"tenant_id":      testTenant,
	}
	if transferType == "DVP" {
		payload["settlement_amount"] = 50000.0
	}
	return payload
}

func createAccountViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

func createTransferViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

// ============================================================
// TestCreateCustodyAccount — 201
// ============================================================

func TestCreateCustodyAccount(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts",
		validAccountPayload("ACCT-HTTP-1", "FIRM-A"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "ACCT-HTTP-1" {
		t.Errorf("id: want ACCT-HTTP-1, got %v", result["id"])
	}
	if result["firm_id"] != "FIRM-A" {
		t.Errorf("firm_id: want FIRM-A, got %v", result["firm_id"])
	}
	if result["name"] != "Main Custody Account" {
		t.Errorf("name: want 'Main Custody Account', got %v", result["name"])
	}
	if result["currency"] != "MNT" {
		t.Errorf("currency: want MNT, got %v", result["currency"])
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set on create")
	}
}

func TestCreateCustodyAccount_AutoID(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	payload := validAccountPayload("", "FIRM-B")
	delete(payload, "id")
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["id"] == nil || result["id"] == "" {
		t.Error("auto-generated id must not be empty")
	}
}

func TestCreateCustodyAccount_MissingFields(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	// Missing firm_id.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts",
		map[string]interface{}{"id": "ACCT-BAD", "name": "X"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Missing name.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts",
		map[string]interface{}{"id": "ACCT-BAD2", "firm_id": "FIRM-A"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateCustodyAccount_Duplicate(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	createAccountViaHTTP(t, ts, validAccountPayload("ACCT-DUP", "FIRM-A"))

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/accounts",
		validAccountPayload("ACCT-DUP", "FIRM-A"))
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

// ============================================================
// TestListAccountsByFirm
// ============================================================

func TestListAccountsByFirm(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	// Empty initially.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("initial total: want 0, got %v", result["total"])
	}

	// Create accounts for two firms.
	createAccountViaHTTP(t, ts, validAccountPayload("ACCT-F1A", "FIRM-1"))
	createAccountViaHTTP(t, ts, validAccountPayload("ACCT-F1B", "FIRM-1"))
	createAccountViaHTTP(t, ts, validAccountPayload("ACCT-F2A", "FIRM-2"))

	// List all — 3 accounts.
	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)
	if result["total"] != float64(3) {
		t.Errorf("all total: want 3, got %v", result["total"])
	}

	// Filter by FIRM-1 — 2 accounts.
	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts?firm_id=FIRM-1", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)
	if result["total"] != float64(2) {
		t.Errorf("FIRM-1 total: want 2, got %v", result["total"])
	}

	// Filter by FIRM-2 — 1 account.
	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts?firm_id=FIRM-2", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)
	if result["total"] != float64(1) {
		t.Errorf("FIRM-2 total: want 1, got %v", result["total"])
	}
}

// ============================================================
// TestGetBalances
// ============================================================

func TestGetBalances(t *testing.T) {
	ts, stores := newCSDTestServer(t)

	// Create an account.
	createAccountViaHTTP(t, ts, validAccountPayload("ACCT-BAL-1", "FIRM-A"))

	// Initially empty balances.
	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts/ACCT-BAL-1/balances", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["total"] != float64(0) {
		t.Errorf("initial balance total: want 0, got %v", result["total"])
	}

	// Seed balances directly in the store.
	stores.balances.GetOrUpdate("ACCT-BAL-1", "WHEAT-FUT", 500, 120.0)   //nolint:errcheck
	stores.balances.GetOrUpdate("ACCT-BAL-1", "BARLEY-FUT", 200, 80.0)   //nolint:errcheck
	stores.balances.GetOrUpdate("ACCT-BAL-2", "WHEAT-FUT", 1000, 120.0)  //nolint:errcheck

	// Get balances for ACCT-BAL-1 — should have 2.
	resp = doJSON(t, ts, http.MethodGet, "/api/v1/securities/csd/accounts/ACCT-BAL-1/balances", nil)
	assertStatus(t, resp, http.StatusOK)
	decodeBody(t, resp, &result)
	if result["total"] != float64(2) {
		t.Errorf("ACCT-BAL-1 balance total: want 2, got %v", result["total"])
	}
	data, _ := result["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("data length: want 2, got %d", len(data))
	}

	// Verify balance fields.
	for _, item := range data {
		bal := item.(map[string]interface{})
		if bal["account_id"] != "ACCT-BAL-1" {
			t.Errorf("account_id: want ACCT-BAL-1, got %v", bal["account_id"])
		}
	}
}

// ============================================================
// TestCreateTransfer_DVP — 201
// ============================================================

func TestCreateTransfer_DVP(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		validTransferPayload("TXN-HTTP-1", "ACCT-FROM", "ACCT-TO", "DVP"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "TXN-HTTP-1" {
		t.Errorf("id: want TXN-HTTP-1, got %v", result["id"])
	}
	if result["transfer_type"] != "DVP" {
		t.Errorf("transfer_type: want DVP, got %v", result["transfer_type"])
	}
	if result["status"] != string(types.CSDTransferPending) {
		t.Errorf("status: want CSD_PENDING, got %v", result["status"])
	}
	if result["quantity"] != float64(100) {
		t.Errorf("quantity: want 100, got %v", result["quantity"])
	}
	if result["settlement_amount"] != float64(50000) {
		t.Errorf("settlement_amount: want 50000, got %v", result["settlement_amount"])
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set on create")
	}
}

func TestCreateTransfer_FOP(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		validTransferPayload("TXN-FOP-1", "ACCT-FROM", "ACCT-TO", "FOP"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["transfer_type"] != "FOP" {
		t.Errorf("transfer_type: want FOP, got %v", result["transfer_type"])
	}
}

func TestCreateTransfer_MissingFields(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	// Missing from_account_id.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		map[string]interface{}{"id": "TXN-BAD", "to_account_id": "B", "instrument_id": "I", "quantity": 10})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Missing instrument_id.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		map[string]interface{}{"id": "TXN-BAD2", "from_account_id": "A", "to_account_id": "B", "quantity": 10})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Zero quantity.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		map[string]interface{}{"from_account_id": "A", "to_account_id": "B", "instrument_id": "I", "quantity": 0})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// DVP with no settlement_amount.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		map[string]interface{}{"from_account_id": "A", "to_account_id": "B", "instrument_id": "I",
			"quantity": 10, "transfer_type": "DVP", "settlement_amount": 0})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateTransfer_Duplicate(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	createTransferViaHTTP(t, ts, validTransferPayload("TXN-DUP", "A", "B", "FOP"))

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers",
		validTransferPayload("TXN-DUP", "A", "B", "FOP"))
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

// ============================================================
// TestCompleteTransfer — 200
// ============================================================

func TestCompleteTransfer(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	// Create a PENDING transfer.
	txnID := createTransferViaHTTP(t, ts, validTransferPayload("TXN-COMPLETE", "ACCT-FROM", "ACCT-TO", "DVP"))

	// Complete it.
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/csd/transfers/"+txnID+"/complete", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != string(types.CSDTransferCompleted) {
		t.Errorf("status after complete: want CSD_COMPLETED, got %v", result["status"])
	}

	// Completing again returns 409 (invalid state).
	resp = doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/csd/transfers/"+txnID+"/complete", nil)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

func TestCompleteTransfer_NotFound(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/csd/transfers/NO-TXN/complete", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestFailTransfer — 200
// ============================================================

func TestFailTransfer(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	// Create a PENDING transfer.
	txnID := createTransferViaHTTP(t, ts, validTransferPayload("TXN-FAIL", "ACCT-FROM", "ACCT-TO", "FOP"))

	// Fail it with a reason.
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/csd/transfers/"+txnID+"/fail",
		map[string]interface{}{"reason": "insufficient balance"})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	if result["status"] != string(types.CSDTransferFailed) {
		t.Errorf("status after fail: want CSD_FAILED, got %v", result["status"])
	}
	if result["fail_reason"] != "insufficient balance" {
		t.Errorf("fail_reason: want 'insufficient balance', got %v", result["fail_reason"])
	}

	// Failing again returns 409 (invalid state).
	resp = doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/csd/transfers/"+txnID+"/fail",
		map[string]interface{}{"reason": "again"})
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

func TestFailTransfer_NotFound(t *testing.T) {
	ts, _ := newCSDTestServer(t)

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/csd/transfers/NO-TXN/fail",
		map[string]interface{}{"reason": "test"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestCSDEndpoints_NotConfigured (503)
// ============================================================

func TestCSDEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil, nil, nil, nil,
		nil, nil,
		nil,  // bondStore
		nil,  // strategyStore
		nil,  // custodyAccountStore = nil
		nil,  // custodyBalanceStore = nil
		nil,  // csdTransferStore = nil
		nil, me, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/securities/csd/accounts"},
		{http.MethodPost, "/api/v1/securities/csd/accounts"},
		{http.MethodGet, "/api/v1/securities/csd/accounts/x"},
		{http.MethodPost, "/api/v1/securities/csd/transfers"},
		{http.MethodGet, "/api/v1/securities/csd/transfers/x"},
		{http.MethodPost, "/api/v1/securities/csd/transfers/x/complete"},
		{http.MethodPost, "/api/v1/securities/csd/transfers/x/fail"},
	}

	for _, tc := range paths {
		resp := doJSON(t, httpTS, tc.method, tc.path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: expected 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
