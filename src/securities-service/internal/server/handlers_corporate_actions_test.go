// Package server — tests for corporate actions HTTP handlers.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// ============================================================
// Corporate action test helpers
// ============================================================

// caStores groups the mutable stores needed for white-box corporate action tests.
type caStores struct {
	instrument    *store.InMemoryInstrumentStore
	order         *store.InMemoryOrderStore
	trade         *store.InMemoryTradeStore
	position      *store.InMemoryPositionStore
	corporateAct  *store.InMemoryCorporateActionStore
	entitlement   *store.InMemoryEntitlementStore
}

// newCAStores creates fresh in-memory store instances.
func newCAStores() caStores {
	return caStores{
		instrument:   store.NewInMemoryInstrumentStore(),
		order:        store.NewInMemoryOrderStore(),
		trade:        store.NewInMemoryTradeStore(),
		position:     store.NewInMemoryPositionStore(),
		corporateAct: store.NewInMemoryCorporateActionStore(),
		entitlement:  store.NewInMemoryEntitlementStore(),
	}
}

// newTestServerWithCA creates a test httptest.Server wired to the given stores.
// This allows the test to retain direct access to position/entitlement stores
// for verification without going through the HTTP layer.
func newTestServerWithCA(t *testing.T, s caStores) *httptest.Server {
	t.Helper()
	cfg := DefaultConfig()
	me := engine.NewMatchingEngine(s.instrument, s.order, s.trade, s.position, nil, nil, nil)
	srv := New(s.instrument, s.order, s.trade, s.position, nil,
		s.corporateAct, s.entitlement, store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // nodeStore
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil, nil, me, nil, nil, nil, nil, nil, nil, cfg)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// doJSONCA sends a JSON request to a URL string and returns the response.
// Used to target custom test servers.
func doJSONCA(t *testing.T, baseURL, method, path string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("json encode: %v", err)
		}
	}
	req, err := http.NewRequest(method, baseURL+path, &buf)
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

// createInstrCA creates an instrument on the given test server and returns its ID.
func createInstrCA(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", validInstrumentPayload())
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	return created["id"].(string)
}

// announceCA POSTs a corporate action to the test server and returns its ID.
func announceCA(t *testing.T, ts *httptest.Server, instrID, actionType string,
	details map[string]interface{}) string {
	t.Helper()
	payload := map[string]interface{}{
		"instrument_id": instrID,
		"action_type":   actionType,
		"details":       details,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/corporate-actions", payload)
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	return created["id"].(string)
}

// ============================================================
// TestAnnounceCorporateAction_Success
// ============================================================

func TestAnnounceCorporateAction_Success(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrCA(t, ts)

	payload := map[string]interface{}{
		"instrument_id":     instrID,
		"action_type":       "CA_DIVIDEND",
		"announcement_date": "2026-04-01",
		"ex_date":           "2026-04-10",
		"record_date":       "2026-04-11",
		"payment_date":      "2026-04-20",
		"details": map[string]interface{}{
			"dividend_amount": 2.50,
		},
	}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/corporate-actions", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["instrument_id"] != instrID {
		t.Errorf("expected instrument_id %q, got %v", instrID, result["instrument_id"])
	}
	if result["action_type"] != "CA_DIVIDEND" {
		t.Errorf("expected action_type CA_DIVIDEND, got %v", result["action_type"])
	}
	if result["status"] != "ANNOUNCED" {
		t.Errorf("expected status ANNOUNCED, got %v", result["status"])
	}
	if ca, ok := result["created_at"].(string); !ok || ca == "" {
		t.Error("expected non-empty created_at")
	}
}

// ============================================================
// TestAnnounceCorporateAction_MissingInstrument
// ============================================================

func TestAnnounceCorporateAction_MissingInstrument(t *testing.T) {
	ts := newTestServer(t)

	// instrument_id is empty string — triggers "instrument_id is required".
	payload := map[string]interface{}{
		"instrument_id": "",
		"action_type":   "CA_DIVIDEND",
	}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/corporate-actions", payload)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	if errResp["error"] == nil {
		t.Error("expected error body in response")
	}
	errObj := errResp["error"].(map[string]interface{})
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %v", errObj["code"])
	}
}

// ============================================================
// TestAnnounceCorporateAction_InvalidType
// ============================================================

func TestAnnounceCorporateAction_InvalidType(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrCA(t, ts)

	// action_type is empty — triggers "action_type is required".
	payload := map[string]interface{}{
		"instrument_id": instrID,
		"action_type":   "",
	}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/corporate-actions", payload)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	if errResp["error"] == nil {
		t.Error("expected error body in response")
	}
	errObj := errResp["error"].(map[string]interface{})
	if errObj["code"] != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %v", errObj["code"])
	}
}

// ============================================================
// TestListCorporateActions
// ============================================================

func TestListCorporateActions(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrCA(t, ts)

	// Create two corporate actions with different types.
	announceCA(t, ts, instrID, "CA_DIVIDEND", map[string]interface{}{"dividend_amount": 1.0})
	announceCA(t, ts, instrID, "CA_STOCK_SPLIT", map[string]interface{}{"split_ratio": 2.0})

	t.Run("list all returns 2", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/corporate-actions", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		count := result["count"].(float64)
		if count != 2 {
			t.Errorf("expected count=2, got %v", count)
		}
		actions := result["corporate_actions"].([]interface{})
		if len(actions) != 2 {
			t.Errorf("expected 2 corporate_actions, got %d", len(actions))
		}
	})

	t.Run("filter by action_type CA_DIVIDEND", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/corporate-actions?action_type=CA_DIVIDEND", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		count := result["count"].(float64)
		if count != 1 {
			t.Errorf("expected count=1 for CA_DIVIDEND filter, got %v", count)
		}
		actions := result["corporate_actions"].([]interface{})
		first := actions[0].(map[string]interface{})
		if first["action_type"] != "CA_DIVIDEND" {
			t.Errorf("expected CA_DIVIDEND, got %v", first["action_type"])
		}
	})

	t.Run("filter by action_type CA_STOCK_SPLIT", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/corporate-actions?action_type=CA_STOCK_SPLIT", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		count := result["count"].(float64)
		if count != 1 {
			t.Errorf("expected count=1 for CA_STOCK_SPLIT filter, got %v", count)
		}
	})

	t.Run("filter by instrument_id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			fmt.Sprintf("/api/v1/securities/corporate-actions?instrument_id=%s", instrID), nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		count := result["count"].(float64)
		if count != 2 {
			t.Errorf("expected count=2 for instrument_id filter, got %v", count)
		}
	})

	t.Run("filter by non-existent action_type returns 0", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/corporate-actions?action_type=CA_MERGER", nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		count := result["count"].(float64)
		if count != 0 {
			t.Errorf("expected count=0 for CA_MERGER filter, got %v", count)
		}
	})
}

// ============================================================
// TestGetCorporateAction_Found
// ============================================================

func TestGetCorporateAction_Found(t *testing.T) {
	ts := newTestServer(t)
	instrID := createInstrCA(t, ts)
	caID := announceCA(t, ts, instrID, "CA_DIVIDEND", map[string]interface{}{"dividend_amount": 3.0})

	resp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/corporate-actions/%s", caID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != caID {
		t.Errorf("expected id %q, got %v", caID, result["id"])
	}
	if result["action_type"] != "CA_DIVIDEND" {
		t.Errorf("expected action_type CA_DIVIDEND, got %v", result["action_type"])
	}
	if result["status"] != "ANNOUNCED" {
		t.Errorf("expected status ANNOUNCED, got %v", result["status"])
	}
}

// ============================================================
// TestGetCorporateAction_NotFound
// ============================================================

func TestGetCorporateAction_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/corporate-actions/nonexistent-ca-id", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errObj := errResp["error"].(map[string]interface{})
	if errObj["code"] != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND code, got %v", errObj["code"])
	}
}

// ============================================================
// TestProcessDividend
// ============================================================

// TestProcessDividend verifies that processing CA_DIVIDEND creates one
// entitlement per participant holding the instrument, with value = qty * amount.
func TestProcessDividend(t *testing.T) {
	s := newCAStores()
	ts := newTestServerWithCA(t, s)

	// Create instrument via HTTP.
	instrID := createInstrCA(t, ts)

	// Seed positions directly (two participants holding different quantities).
	p1, _ := s.position.GetOrCreate("participant-alpha", instrID)
	p1.Quantity = 100
	if err := s.position.Update(p1); err != nil {
		t.Fatalf("Update position p1: %v", err)
	}

	p2, _ := s.position.GetOrCreate("participant-beta", instrID)
	p2.Quantity = 200
	if err := s.position.Update(p2); err != nil {
		t.Fatalf("Update position p2: %v", err)
	}

	// Announce dividend with amount=5.0 per share.
	caID := announceCA(t, ts, instrID, "CA_DIVIDEND",
		map[string]interface{}{"dividend_amount": 5.0})

	// Process the corporate action.
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/corporate-actions/%s/process", caID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["action_type"] != "CA_DIVIDEND" {
		t.Errorf("expected action_type CA_DIVIDEND, got %v", result["action_type"])
	}
	entitlementsCreated := result["entitlements_created"].(float64)
	if entitlementsCreated != 2 {
		t.Errorf("expected 2 entitlements created, got %v", entitlementsCreated)
	}

	// Verify CA status is COMPLETED.
	getResp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/corporate-actions/%s", caID), nil)
	assertStatus(t, getResp, http.StatusOK)
	var caResult map[string]interface{}
	decodeBody(t, getResp, &caResult)
	if caResult["status"] != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %v", caResult["status"])
	}

	// Verify entitlement values via store directly.
	entitlements, err := s.entitlement.ListByAction(caID)
	if err != nil {
		t.Fatalf("ListByAction: %v", err)
	}
	if len(entitlements) != 2 {
		t.Fatalf("expected 2 entitlements in store, got %d", len(entitlements))
	}

	totalValue := 0.0
	for _, e := range entitlements {
		expectedValue := float64(e.Quantity) * 5.0
		if e.EntitlementValue != expectedValue {
			t.Errorf("participant %s: expected value %.2f (qty=%d * 5.0), got %.2f",
				e.ParticipantID, expectedValue, e.Quantity, e.EntitlementValue)
		}
		totalValue += e.EntitlementValue
	}
	// participant-alpha: 100 * 5.0 = 500
	// participant-beta:  200 * 5.0 = 1000
	// total: 1500
	if totalValue != 1500.0 {
		t.Errorf("expected total entitlement value 1500.0, got %.2f", totalValue)
	}
}

// ============================================================
// TestProcessStockSplit
// ============================================================

// TestProcessStockSplit verifies that processing CA_STOCK_SPLIT with ratio=2
// doubles all position quantities for the instrument.
func TestProcessStockSplit(t *testing.T) {
	s := newCAStores()
	ts := newTestServerWithCA(t, s)

	// Create instrument.
	instrID := createInstrCA(t, ts)

	// Seed positions for two participants.
	p1, _ := s.position.GetOrCreate("participant-x", instrID)
	p1.Quantity = 50
	if err := s.position.Update(p1); err != nil {
		t.Fatalf("Update position p1: %v", err)
	}

	p2, _ := s.position.GetOrCreate("participant-y", instrID)
	p2.Quantity = 150
	if err := s.position.Update(p2); err != nil {
		t.Fatalf("Update position p2: %v", err)
	}

	// Announce stock split with ratio=2.
	caID := announceCA(t, ts, instrID, "CA_STOCK_SPLIT",
		map[string]interface{}{"split_ratio": 2.0})

	// Process.
	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/corporate-actions/%s/process", caID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["action_type"] != "CA_STOCK_SPLIT" {
		t.Errorf("expected action_type CA_STOCK_SPLIT, got %v", result["action_type"])
	}
	positionsAdjusted := result["positions_adjusted"].(float64)
	if positionsAdjusted != 2 {
		t.Errorf("expected 2 positions_adjusted, got %v", positionsAdjusted)
	}

	// Verify positions were doubled.
	pX, err := s.position.GetOrCreate("participant-x", instrID)
	if err != nil {
		t.Fatalf("GetOrCreate participant-x: %v", err)
	}
	if pX.Quantity != 100 {
		t.Errorf("participant-x: expected quantity 100 after split, got %d", pX.Quantity)
	}

	pY, err := s.position.GetOrCreate("participant-y", instrID)
	if err != nil {
		t.Fatalf("GetOrCreate participant-y: %v", err)
	}
	if pY.Quantity != 300 {
		t.Errorf("participant-y: expected quantity 300 after split, got %d", pY.Quantity)
	}

	// Verify CA is COMPLETED.
	getResp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/corporate-actions/%s", caID), nil)
	assertStatus(t, getResp, http.StatusOK)
	var caResult map[string]interface{}
	decodeBody(t, getResp, &caResult)
	if caResult["status"] != "COMPLETED" {
		t.Errorf("expected status COMPLETED, got %v", caResult["status"])
	}
}
