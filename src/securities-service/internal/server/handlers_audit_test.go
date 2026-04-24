// Package server — tests for audit trail HTTP handler.
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

// newTestServerWithAudit creates a test server wired with a real AuditStore
// and AnnouncementStore so that audit and announcement endpoints are reachable.
func newTestServerWithAudit(t *testing.T) (*httptest.Server, *store.InMemoryAuditStore) {
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
		annStore,
		auditStore,
		nil, // pendingChangeStore
		nil, // referencePriceStore
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
	handler := tenantMW(mux)

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts, auditStore
}

// logAuditEntry is a helper that logs an AuditEntry directly into the store.
func logAuditEntry(t *testing.T, s *store.InMemoryAuditStore, entry types.AuditEntry) {
	t.Helper()
	if err := s.Log(entry); err != nil {
		t.Fatalf("Log audit entry %s: %v", entry.ID, err)
	}
}

// ============================================================
// TestAuditTrail_List
// ============================================================

// TestAuditTrail_List logs entries directly into the store and verifies that
// GET /api/v1/securities/audit-trail returns them.
func TestAuditTrail_List(t *testing.T) {
	ts, auditStore := newTestServerWithAudit(t)

	entries := []types.AuditEntry{
		{ID: "ht-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "trader-1", Timestamp: "2026-04-24T10:00:00Z"},
		{ID: "ht-2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "system", Timestamp: "2026-04-24T10:01:00Z"},
		{ID: "ht-3", EntityType: "INSTRUMENT", EntityID: "i1", Action: "UPDATE", ActorID: "admin", Timestamp: "2026-04-24T10:02:00Z"},
	}
	for _, e := range entries {
		logAuditEntry(t, auditStore, e)
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/audit-trail", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 3 {
		t.Errorf("expected 3 audit entries, got %d", len(data))
	}
	if result["total"] != float64(3) {
		t.Errorf("expected total=3, got %v", result["total"])
	}
}

func TestAuditTrail_Empty(t *testing.T) {
	ts, _ := newTestServerWithAudit(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/audit-trail", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected 0 entries, got %d", len(data))
	}
}

// ============================================================
// TestAuditTrail_FilterEntityType
// ============================================================

// TestAuditTrail_FilterEntityType verifies that GET ?entity_type=ORDER returns
// only ORDER entries.
func TestAuditTrail_FilterEntityType(t *testing.T) {
	ts, auditStore := newTestServerWithAudit(t)

	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fe-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "u1", Timestamp: "2026-04-24T10:00:00Z"})
	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fe-2", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "u1", Timestamp: "2026-04-24T10:01:00Z"})
	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fe-3", EntityType: "INSTRUMENT", EntityID: "i1", Action: "UPDATE", ActorID: "u2", Timestamp: "2026-04-24T10:02:00Z"})

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/audit-trail?entity_type=ORDER", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 1 {
		t.Errorf("expected 1 ORDER entry, got %d", len(data))
	}

	entry, ok := data[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected entry to be object, got %T", data[0])
	}
	if entry["entity_type"] != "ORDER" {
		t.Errorf("expected entity_type ORDER, got %v", entry["entity_type"])
	}
	if result["total"] != float64(1) {
		t.Errorf("expected total=1, got %v", result["total"])
	}
}

func TestAuditTrail_FilterActorID(t *testing.T) {
	ts, auditStore := newTestServerWithAudit(t)

	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fa-1", EntityType: "ORDER", EntityID: "o1", Action: "CREATE", ActorID: "admin", Timestamp: "2026-04-24T10:00:00Z"})
	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fa-2", EntityType: "ORDER", EntityID: "o2", Action: "CANCEL", ActorID: "trader", Timestamp: "2026-04-24T10:01:00Z"})
	logAuditEntry(t, auditStore, types.AuditEntry{ID: "fa-3", EntityType: "TRADE", EntityID: "t1", Action: "UPDATE", ActorID: "admin", Timestamp: "2026-04-24T10:02:00Z"})

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/audit-trail?actor_id=admin", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", result["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 admin entries, got %d", len(data))
	}
	for _, item := range data {
		entry := item.(map[string]interface{})
		if entry["actor_id"] != "admin" {
			t.Errorf("unexpected actor_id %v in admin filter result", entry["actor_id"])
		}
	}
}

func TestAuditTrail_MethodNotAllowed(t *testing.T) {
	ts, _ := newTestServerWithAudit(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/audit-trail", nil)
	assertStatus(t, resp, http.StatusMethodNotAllowed)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("expected METHOD_NOT_ALLOWED, got %q", errResp.Error.Code)
	}
}

func TestAuditTrail_FilterEntityID(t *testing.T) {
	ts, auditStore := newTestServerWithAudit(t)

	logAuditEntry(t, auditStore, types.AuditEntry{ID: "feid-1", EntityType: "ORDER", EntityID: "target-order", Action: "CREATE", ActorID: "u1", Timestamp: "2026-04-24T10:00:00Z"})
	logAuditEntry(t, auditStore, types.AuditEntry{ID: "feid-2", EntityType: "ORDER", EntityID: "other-order", Action: "CANCEL", ActorID: "u1", Timestamp: "2026-04-24T10:01:00Z"})

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/audit-trail?entity_id=target-order", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("expected 1 result for entity_id=target-order, got %d", len(data))
	}
}
