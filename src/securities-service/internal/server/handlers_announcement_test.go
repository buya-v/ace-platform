// Package server — tests for announcement HTTP handlers.
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
)

// newTestServerWithAnnouncement creates a test server wired with a real
// AnnouncementStore and AuditStore so that the announcement endpoints are
// reachable (not 503).
func newTestServerWithAnnouncement(t *testing.T) *httptest.Server {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	annStore := store.NewInMemoryAnnouncementStore()
	auditStore := store.NewInMemoryAuditStore()

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
		annStore,
		auditStore,
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
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// ============================================================
// TestCreateAnnouncement
// ============================================================

func TestCreateAnnouncement_Success(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	payload := map[string]interface{}{
		"title":    "Market Opening",
		"body":     "Trading begins at 09:00 today.",
		"audience": "PUBLIC",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/announcements", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["title"] != "Market Opening" {
		t.Errorf("expected title 'Market Opening', got %v", result["title"])
	}
	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["tenant_id"] != testTenant {
		t.Errorf("expected tenant_id %q, got %v", testTenant, result["tenant_id"])
	}
	if result["audience"] != "PUBLIC" {
		t.Errorf("expected audience PUBLIC, got %v", result["audience"])
	}
}

func TestCreateAnnouncement_MissingTitle(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	payload := map[string]interface{}{
		"body":     "Missing title body",
		"audience": "PUBLIC",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/announcements", payload)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)

	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected error object in response, got %v", errResp)
	}
	if errObj["code"] != "MISSING_FIELD" {
		t.Errorf("expected code MISSING_FIELD, got %v", errObj["code"])
	}
}

func TestCreateAnnouncement_MissingBody(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	payload := map[string]interface{}{
		"title":    "Title Only",
		"audience": "PUBLIC",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/announcements", payload)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// ============================================================
// TestListAnnouncements
// ============================================================

// TestListAnnouncements creates 2 announcements via POST and verifies that a
// subsequent GET returns both.
func TestListAnnouncements(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	payloads := []map[string]interface{}{
		{"title": "Notice One", "body": "Body one.", "audience": "PUBLIC"},
		{"title": "Notice Two", "body": "Body two.", "audience": "PARTICIPANT"},
	}
	for _, p := range payloads {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/announcements", p)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/announcements", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array in response, got %T", result["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 announcements, got %d", len(data))
	}
	if result["total"] != float64(2) {
		t.Errorf("expected total=2, got %v", result["total"])
	}
}

func TestListAnnouncements_Empty(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/announcements", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected 0 announcements, got %d", len(data))
	}
}

func TestCreateAnnouncement_DefaultsAudienceToPublic(t *testing.T) {
	ts := newTestServerWithAnnouncement(t)

	// Omit audience — handler must default to PUBLIC.
	payload := map[string]interface{}{
		"title": "Default Audience",
		"body":  "Audience omitted.",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/announcements", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["audience"] != "PUBLIC" {
		t.Errorf("expected default audience PUBLIC, got %v", result["audience"])
	}
}
