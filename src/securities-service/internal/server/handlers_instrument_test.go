// Package server — internal tests for instrument HTTP handlers.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// testTenant is the default tenant header value used in all server tests.
const testTenant = "ace-commodities"

// ============================================================
// Test infrastructure
// ============================================================

// newTestServer creates a fully wired httptest.Server using fresh in-memory
// stores. The test server is closed automatically when the test ends.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	return newTestServerWithStores(t, instrStore, orderStore)
}

// newTestServerWithStores creates a test server backed by the provided stores.
// The API handler chain includes TenantMiddleware with testTenant whitelisted.
func newTestServerWithStores(t *testing.T, instrStore store.InstrumentStore, orderStore store.OrderStore) *httptest.Server {
	t.Helper()
	cfg := DefaultConfig()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	srv := New(instrStore, orderStore, tradeStore, positionStore, nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(), store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, me, nil, nil, nil, cfg)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	// Wrap with TenantMiddleware so handler tests exercise the full middleware chain.
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// doJSON sends a JSON request to the test server and returns the response.
// The response body is NOT closed by this function; callers must close it
// (or use decodeBody which closes it).
func doJSON(t *testing.T, ts *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("json encode: %v", err)
		}
	}
	req, err := http.NewRequest(method, ts.URL+path, &buf)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Include tenant header so handlers receive tenant context from TenantMiddleware.
	req.Header.Set("X-GarudaX-Tenant", testTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do: %v", err)
	}
	return resp
}

// decodeBody decodes the response body into dst and closes the body.
func decodeBody(t *testing.T, resp *http.Response, dst interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
}

// assertStatus checks the response status code and fatals with a useful message.
func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("expected HTTP %d, got %d", want, resp.StatusCode)
	}
}

// validInstrumentPayload returns a minimal valid payload for creating an instrument.
func validInstrumentPayload() map[string]interface{} {
	return map[string]interface{}{
		"ticker":      "AAPL",
		"name":        "Apple Inc.",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	}
}

// createInstr creates an instrument via POST and returns its ID.
func createInstr(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	decodeBody(t, resp, &created)
	return created["id"].(string)
}

// ============================================================
// TestCreateInstrument
// ============================================================

func TestCreateInstrument_Success(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", validInstrumentPayload())
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["ticker"] != "AAPL" {
		t.Errorf("expected ticker AAPL, got %v", result["ticker"])
	}
	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["trading_status"] != "ACTIVE" {
		t.Errorf("expected default trading_status ACTIVE, got %v", result["trading_status"])
	}
	if ca, ok := result["created_at"].(string); !ok || ca == "" {
		t.Error("expected non-empty created_at")
	}
}

func TestCreateInstrument_MissingTicker(t *testing.T) {
	ts := newTestServer(t)

	payload := validInstrumentPayload()
	delete(payload, "ticker")

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "MISSING_FIELD" {
		t.Errorf("expected MISSING_FIELD, got %q", errResp.Error.Code)
	}
}

func TestCreateInstrument_MissingName(t *testing.T) {
	ts := newTestServer(t)

	payload := validInstrumentPayload()
	delete(payload, "name")

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateInstrument_MissingAssetClass(t *testing.T) {
	ts := newTestServer(t)

	payload := validInstrumentPayload()
	delete(payload, "asset_class")

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateInstrument_InvalidLotSize(t *testing.T) {
	ts := newTestServer(t)

	t.Run("lot_size=0", func(t *testing.T) {
		payload := validInstrumentPayload()
		payload["lot_size"] = 0
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		var errResp types.ErrorResponse
		decodeBody(t, resp, &errResp)
		if errResp.Error.Code != "INVALID_FIELD" {
			t.Errorf("expected INVALID_FIELD, got %q", errResp.Error.Code)
		}
	})

	t.Run("lot_size negative", func(t *testing.T) {
		payload := validInstrumentPayload()
		payload["lot_size"] = -5
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

func TestCreateInstrument_InvalidTickSize(t *testing.T) {
	ts := newTestServer(t)

	payload := validInstrumentPayload()
	payload["tick_size"] = 0
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/instruments", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD, got %q", errResp.Error.Code)
	}
}

func TestCreateInstrument_InvalidJSON(t *testing.T) {
	ts := newTestServer(t)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/securities/instruments",
		strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GarudaX-Tenant", testTenant)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_JSON" {
		t.Errorf("expected INVALID_JSON, got %q", errResp.Error.Code)
	}
}

// ============================================================
// TestListInstruments
// ============================================================

func TestListInstruments_Empty(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"]
	if !ok {
		t.Fatal("expected 'data' key in response")
	}
	arr, ok := data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be a JSON array, got %T", data)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d items", len(arr))
	}
	if result["total"] != float64(0) {
		t.Errorf("expected total=0, got %v", result["total"])
	}
}

func TestListInstruments_WithFilters(t *testing.T) {
	ts := newTestServer(t)

	instruments := []map[string]interface{}{
		{"ticker": "EQ1", "name": "Equity One", "asset_class": "EQUITY", "lot_size": 100, "tick_size": 0.01},
		{"ticker": "EQ2", "name": "Equity Two", "asset_class": "EQUITY", "lot_size": 100, "tick_size": 0.01},
		{"ticker": "BD1", "name": "Bond One", "asset_class": "BOND", "lot_size": 1, "tick_size": 0.001},
	}
	for _, p := range instruments {
		createInstr(t, ts, p)
	}

	t.Run("filter by asset_class EQUITY", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?asset_class=EQUITY", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 EQUITY instruments, got %d", len(data))
		}
	})

	t.Run("filter by asset_class BOND", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?asset_class=BOND", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 BOND instrument, got %d", len(data))
		}
	})

	t.Run("filter by trading_status ACTIVE", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?trading_status=ACTIVE", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 3 {
			t.Errorf("expected 3 ACTIVE instruments, got %d", len(data))
		}
	})

	t.Run("no match", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?asset_class=ETF", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("expected 0 ETF instruments, got %d", len(data))
		}
	})
}

func TestListInstruments_Pagination(t *testing.T) {
	ts := newTestServer(t)

	for i := 0; i < 5; i++ {
		p := map[string]interface{}{
			"ticker":      fmt.Sprintf("TK%d", i),
			"name":        fmt.Sprintf("Ticker %d", i),
			"asset_class": "EQUITY",
			"lot_size":    100,
			"tick_size":   0.01,
		}
		createInstr(t, ts, p)
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?limit=3", nil)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	data := result["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(data))
	}
	if result["total"] != float64(5) {
		t.Errorf("expected total=5, got %v", result["total"])
	}
}

func TestListInstruments_Search(t *testing.T) {
	ts := newTestServer(t)

	instruments := []map[string]interface{}{
		{"ticker": "AAPL", "name": "Apple Inc.", "asset_class": "EQUITY", "lot_size": 100, "tick_size": 0.01},
		{"ticker": "GOOGL", "name": "Alphabet Inc.", "asset_class": "EQUITY", "lot_size": 100, "tick_size": 0.01},
		{"ticker": "MSFT", "name": "Microsoft Corp.", "asset_class": "EQUITY", "lot_size": 100, "tick_size": 0.01},
	}
	for _, p := range instruments {
		createInstr(t, ts, p)
	}

	t.Run("search by ticker", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?search=aap", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 result for search=aap, got %d", len(data))
		}
	})

	t.Run("search by name", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments?search=inc", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		data := result["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("expected 2 results for search=inc, got %d", len(data))
		}
	})
}

// ============================================================
// TestGetInstrument
// ============================================================

func TestGetInstrument_Success(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/instruments/%s", id), nil)
	assertStatus(t, resp, http.StatusOK)

	var got map[string]interface{}
	decodeBody(t, resp, &got)
	if got["id"] != id {
		t.Errorf("expected id %q, got %v", id, got["id"])
	}
}

func TestGetInstrument_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments/nonexistent-id", nil)
	assertStatus(t, resp, http.StatusNotFound)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %q", errResp.Error.Code)
	}
}

// ============================================================
// TestUpdateInstrumentStatus
// ============================================================

func TestUpdateInstrumentStatus_Success(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	statusPayload := map[string]interface{}{
		"status": "HALTED",
		"reason": "Regulatory review",
	}
	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/instruments/%s/status", id), statusPayload)
	assertStatus(t, resp, http.StatusOK)

	var statusResp map[string]interface{}
	decodeBody(t, resp, &statusResp)
	if statusResp["current_status"] != "HALTED" {
		t.Errorf("expected current_status HALTED, got %v", statusResp["current_status"])
	}
	if statusResp["previous_status"] != "ACTIVE" {
		t.Errorf("expected previous_status ACTIVE, got %v", statusResp["previous_status"])
	}
	if statusResp["instrument_id"] != id {
		t.Errorf("expected instrument_id %q, got %v", id, statusResp["instrument_id"])
	}
	if statusResp["reason"] != "Regulatory review" {
		t.Errorf("expected reason 'Regulatory review', got %v", statusResp["reason"])
	}
}

func TestUpdateInstrumentStatus_AllValidStatuses(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	for _, status := range []string{"HALTED", "SUSPENDED", "DELISTED", "ACTIVE"} {
		t.Run(status, func(t *testing.T) {
			resp := doJSON(t, ts, http.MethodPut,
				fmt.Sprintf("/api/v1/securities/instruments/%s/status", id),
				map[string]interface{}{"status": status})
			assertStatus(t, resp, http.StatusOK)

			var result map[string]interface{}
			decodeBody(t, resp, &result)
			if result["current_status"] != status {
				t.Errorf("expected current_status %q, got %v", status, result["current_status"])
			}
		})
	}
}

func TestUpdateInstrumentStatus_InvalidEnum(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/instruments/%s/status", id),
		map[string]interface{}{"status": "OPEN_FOR_BUSINESS"})
	// Handler returns 400 for invalid enum values.
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_FIELD" {
		t.Errorf("expected INVALID_FIELD, got %q", errResp.Error.Code)
	}
}

func TestUpdateInstrumentStatus_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/instruments/nonexistent-id/status",
		map[string]interface{}{"status": "HALTED"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestUpdateInstrument (PATCH)
// ============================================================

func TestUpdateInstrument_Success(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	resp := doJSON(t, ts, http.MethodPatch,
		fmt.Sprintf("/api/v1/securities/instruments/%s", id),
		map[string]interface{}{"name": "Apple Inc. (Updated)"})
	assertStatus(t, resp, http.StatusOK)

	var got map[string]interface{}
	decodeBody(t, resp, &got)
	if got["name"] != "Apple Inc. (Updated)" {
		t.Errorf("expected updated name, got %v", got["name"])
	}
	if got["ticker"] != "AAPL" {
		t.Errorf("expected ticker AAPL unchanged, got %v", got["ticker"])
	}
}

func TestUpdateInstrument_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodPatch,
		"/api/v1/securities/instruments/nonexistent",
		map[string]interface{}{"name": "Ghost"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// Method not allowed
// ============================================================

func TestMethodNotAllowed_Instruments(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/instruments", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("expected METHOD_NOT_ALLOWED, got %q", errResp.Error.Code)
	}
}

func TestMethodNotAllowed_Instrument(t *testing.T) {
	ts := newTestServer(t)
	id := createInstr(t, ts, validInstrumentPayload())

	resp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/instruments/%s", id), nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)
	resp.Body.Close()
}

// ============================================================
// Health endpoints (via direct handler call)
// ============================================================

func TestHealthzEndpoint(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(store.NewInMemoryInstrumentStore(), store.NewInMemoryOrderStore(), store.NewInMemoryTradeStore(), store.NewInMemoryPositionStore(), nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(), store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, nil, cfg)
	srv.SetReady()

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	srv.healthz(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode healthz body: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %q", result["status"])
	}
}

func TestReadyzEndpoint_NotReady(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(store.NewInMemoryInstrumentStore(), store.NewInMemoryOrderStore(), store.NewInMemoryTradeStore(), store.NewInMemoryPositionStore(), nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(), store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, nil, cfg)
	// NOT calling SetReady()

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	srv.readyz(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when not ready, got %d", w.Code)
	}
}

func TestReadyzEndpoint_Ready(t *testing.T) {
	cfg := DefaultConfig()
	srv := New(store.NewInMemoryInstrumentStore(), store.NewInMemoryOrderStore(), store.NewInMemoryTradeStore(), store.NewInMemoryPositionStore(), nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(), store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(), store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, // locateStore
		nil, // rfqStore
		nil, // giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, nil, cfg)
	srv.SetReady()

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	srv.readyz(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 when ready, got %d", w.Code)
	}
}
