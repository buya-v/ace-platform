// Package server — tests for surveillance investigation HTTP handlers.
package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newInvestigationTestServer creates a test server wired with a real InvestigationStore.
func newInvestigationTestServer(t *testing.T) (*httptest.Server, *store.InMemoryInvestigationStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	invStore := store.NewInMemoryInvestigationStore()

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
		invStore,
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
	return ts, invStore
}

// seedInvestigation creates an investigation directly into the store.
func seedInvestigation(t *testing.T, s *store.InMemoryInvestigationStore, inv *types.Investigation) {
	t.Helper()
	if err := s.Create(inv); err != nil {
		t.Fatalf("seedInvestigation %s: %v", inv.ID, err)
	}
}

// createInvestigationViaHTTP creates an investigation via POST and returns its ID.
func createInvestigationViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/investigations", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

// ============================================================
// TestCreateInvestigation
// ============================================================

func TestCreateInvestigation(t *testing.T) {
	ts, _ := newInvestigationTestServer(t)

	payload := map[string]interface{}{
		"id":            "INV-HTTP-1",
		"subject":       "Suspected wash trading on INST-A",
		"instrument_id": "INST-A",
		"assigned_to":   "analyst-007",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/investigations", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] != "INV-HTTP-1" {
		t.Errorf("id: want INV-HTTP-1, got %v", result["id"])
	}
	if result["status"] != "OPEN" {
		t.Errorf("status: want OPEN, got %v", result["status"])
	}
	if result["subject"] != "Suspected wash trading on INST-A" {
		t.Errorf("subject mismatch: got %v", result["subject"])
	}
	if result["opened_at"] == "" || result["opened_at"] == nil {
		t.Error("opened_at must be set on create")
	}
}

func TestCreateInvestigation_MissingFields(t *testing.T) {
	ts, _ := newInvestigationTestServer(t)

	// Missing subject.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/investigations",
		map[string]interface{}{"id": "INV-BAD"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Missing id.
	resp = doJSON(t, ts, http.MethodPost, "/api/v1/securities/investigations",
		map[string]interface{}{"subject": "some subject"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateInvestigation_Duplicate(t *testing.T) {
	ts, _ := newInvestigationTestServer(t)

	payload := map[string]interface{}{
		"id":      "INV-DUP",
		"subject": "First creation",
	}
	createInvestigationViaHTTP(t, ts, payload)

	// Second create with same ID should return 409 Conflict.
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/investigations", payload)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

// ============================================================
// TestListInvestigations_FilterByStatus
// ============================================================

func TestListInvestigations_FilterByStatus(t *testing.T) {
	ts, invStore := newInvestigationTestServer(t)

	// Seed two OPEN and one CLOSED investigation.
	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-A", Subject: "Case A", Status: types.InvestigationOpen,
		OpenedAt: "2026-04-01T00:00:00Z",
	})
	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-B", Subject: "Case B", Status: types.InvestigationOpen,
		OpenedAt: "2026-04-02T00:00:00Z",
	})
	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-C", Subject: "Case C", Status: types.InvestigationClosed,
		OpenedAt: "2026-04-03T00:00:00Z", ClosedAt: "2026-04-10T00:00:00Z",
		Findings: "No breach found.",
	})

	t.Run("no filter returns all 3", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/investigations", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"] != float64(3) {
			t.Errorf("total: want 3, got %v", result["total"])
		}
	})

	t.Run("status=OPEN returns 2", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/investigations?status=OPEN", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"] != float64(2) {
			t.Errorf("total: want 2, got %v", result["total"])
		}
	})

	t.Run("status=CLOSED returns 1", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/investigations?status=CLOSED", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"] != float64(1) {
			t.Errorf("total: want 1, got %v", result["total"])
		}
		data, _ := result["data"].([]interface{})
		if len(data) == 1 {
			item, _ := data[0].(map[string]interface{})
			if item["id"] != "INV-C" {
				t.Errorf("id: want INV-C, got %v", item["id"])
			}
		}
	})
}

// ============================================================
// TestCloseInvestigation
// ============================================================

func TestCloseInvestigation(t *testing.T) {
	ts, invStore := newInvestigationTestServer(t)

	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-TO-CLOSE", Subject: "Volume anomaly", Status: types.InvestigationOpen,
		OpenedAt: "2026-04-24T00:00:00Z",
	})

	// Close with findings.
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/INV-TO-CLOSE/close",
		map[string]string{"findings": "No rule breach confirmed."})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["status"] != "CLOSED" {
		t.Errorf("status: want CLOSED, got %v", result["status"])
	}
	if result["findings"] != "No rule breach confirmed." {
		t.Errorf("findings mismatch: got %v", result["findings"])
	}
	if result["closed_at"] == nil || result["closed_at"] == "" {
		t.Error("closed_at must be set after close")
	}
}

func TestCloseInvestigation_NotFound(t *testing.T) {
	ts, _ := newInvestigationTestServer(t)

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/NO-SUCH-INV/close",
		map[string]string{"findings": "x"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestCloseInvestigation_AlreadyClosed(t *testing.T) {
	ts, invStore := newInvestigationTestServer(t)

	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-ALREADY-CLOSED", Subject: "Already done",
		Status: types.InvestigationClosed,
		OpenedAt: "2026-04-01T00:00:00Z", ClosedAt: "2026-04-10T00:00:00Z",
	})

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/INV-ALREADY-CLOSED/close",
		map[string]string{"findings": "again"})
	// Should return 4xx.
	if resp.StatusCode < 400 {
		t.Errorf("expected 4xx for already-closed investigation, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================
// TestAddEvidence
// ============================================================

func TestAddEvidence(t *testing.T) {
	ts, invStore := newInvestigationTestServer(t)

	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-EVID", Subject: "Evidence gathering", Status: types.InvestigationOpen,
		OpenedAt: "2026-04-24T00:00:00Z",
	})

	// Add first evidence reference.
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/INV-EVID/evidence",
		map[string]string{"evidence": "trade-ref-001"})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	evidence, _ := result["evidence"].([]interface{})
	if len(evidence) != 1 {
		t.Errorf("evidence count: want 1, got %d", len(evidence))
	}
	if len(evidence) > 0 && evidence[0] != "trade-ref-001" {
		t.Errorf("evidence[0]: want trade-ref-001, got %v", evidence[0])
	}

	// Add second evidence reference.
	resp2 := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/INV-EVID/evidence",
		map[string]string{"evidence": "order-ref-999"})
	assertStatus(t, resp2, http.StatusOK)

	var result2 map[string]interface{}
	decodeBody(t, resp2, &result2)

	evidence2, _ := result2["evidence"].([]interface{})
	if len(evidence2) != 2 {
		t.Errorf("evidence count after 2 adds: want 2, got %d", len(evidence2))
	}
}

func TestAddEvidence_MissingField(t *testing.T) {
	ts, invStore := newInvestigationTestServer(t)
	seedInvestigation(t, invStore, &types.Investigation{
		ID: "INV-EV-BAD", Subject: "Test", Status: types.InvestigationOpen,
		OpenedAt: "2026-04-24T00:00:00Z",
	})

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/INV-EV-BAD/evidence",
		map[string]string{"evidence": ""})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestAddEvidence_NotFound(t *testing.T) {
	ts, _ := newInvestigationTestServer(t)

	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/investigations/NO-SUCH/evidence",
		map[string]string{"evidence": "ref-x"})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestInvestigationEndpoints_NotConfigured (503)
// ============================================================

func TestInvestigationEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil, // surveillance, instrument-group, off-book
		nil,           // nodeStore
		nil, nil, nil, // locate, rfq, give-up
		nil, // investigationStore = nil
		nil, // replayStore
		nil, // bondStore
		nil, // strategyStore
		nil, // custodyAccountStore
		nil, // custodyBalanceStore
		nil, // csdTransferStore
		nil, nil, nil, nil, me, nil, nil, nil, nil, nil, cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	httpTS := httptest.NewServer(tenantMW(mux))
	t.Cleanup(httpTS.Close)

	paths := []string{
		"/api/v1/securities/investigations",
		fmt.Sprintf("/api/v1/securities/investigations/%s/close", "some-id"),
		fmt.Sprintf("/api/v1/securities/investigations/%s/evidence", "some-id"),
	}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPost}

	for i, path := range paths {
		resp := doJSON(t, httpTS, methods[i], path, map[string]string{"findings": "x", "evidence": "x"})
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s: expected 503, got %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
