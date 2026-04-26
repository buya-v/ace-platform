// Package server — internal tests for entity permission HTTP handlers (Sprint 8 Part B).
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

// newEntityPermissionTestServer creates a test server wired with a fresh
// InMemoryEntityPermissionStore.
func newEntityPermissionTestServer(t *testing.T) (*httptest.Server, store.EntityPermissionStore) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	epStore := store.NewInMemoryEntityPermissionStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	cfg := DefaultConfig()

	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, me, nil, nil,
		nil, nil, nil, nil, cfg,
	)
	srv.SetEntityPermissionStore(epStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, epStore
}

// setEntityPermissionViaHTTP sends a PUT request to set an entity permission and
// returns the response body as a map.
func setEntityPermissionViaHTTP(t *testing.T, ts *httptest.Server, roleID, entityType string, allowCreate, allowView bool) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{
		"role_id":      roleID,
		"entity_type":  entityType,
		"allow_create": allowCreate,
		"allow_view":   allowView,
		"allow_edit":   true,
	}
	resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/entity-permissions", body)
	assertStatus(t, resp, http.StatusOK)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// ── TestSetEntityPermission ───────────────────────────────────────────────────

func TestSetEntityPermission(t *testing.T) {
	ts, _ := newEntityPermissionTestServer(t)

	t.Run("returns 200 with valid body", func(t *testing.T) {
		body := map[string]interface{}{
			"role_id":      "role-admin",
			"entity_type":  "INSTRUMENT",
			"allow_create": true,
			"allow_view":   true,
			"allow_edit":   true,
			"allow_delete": false,
		}
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/entity-permissions", body)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["role_id"] != "role-admin" {
			t.Errorf("role_id: want role-admin, got %v", result["role_id"])
		}
		if result["entity_type"] != "INSTRUMENT" {
			t.Errorf("entity_type: want INSTRUMENT, got %v", result["entity_type"])
		}
		if result["allow_create"] != true {
			t.Errorf("allow_create: want true, got %v", result["allow_create"])
		}
		if result["id"] == nil || result["id"].(string) == "" {
			t.Error("id must be populated")
		}
	})

	t.Run("Set overwrites existing permission", func(t *testing.T) {
		// Set initial.
		setEntityPermissionViaHTTP(t, ts, "role-trader", "ORDER", true, true)
		// Overwrite with different allow_create.
		result := setEntityPermissionViaHTTP(t, ts, "role-trader", "ORDER", false, true)
		if result["allow_create"] != false {
			t.Errorf("allow_create after overwrite: want false, got %v", result["allow_create"])
		}
	})

	t.Run("returns 400 when role_id is missing", func(t *testing.T) {
		body := map[string]interface{}{
			"entity_type": "INSTRUMENT",
		}
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/entity-permissions", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("returns 400 when entity_type is missing", func(t *testing.T) {
		body := map[string]interface{}{
			"role_id": "role-admin",
		}
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/entity-permissions", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("returns 400 on invalid JSON", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/entity-permissions", "not-json")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

// ── TestListByRole ────────────────────────────────────────────────────────────

func TestListByRole(t *testing.T) {
	ts, _ := newEntityPermissionTestServer(t)

	// Seed permissions for role-x and role-y.
	setEntityPermissionViaHTTP(t, ts, "role-x", "INSTRUMENT", true, true)
	setEntityPermissionViaHTTP(t, ts, "role-x", "ORDER", false, true)
	setEntityPermissionViaHTTP(t, ts, "role-y", "TRADE", true, true)

	t.Run("returns all permissions for role-x", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/entity-permissions?role_id=role-x", nil)
		assertStatus(t, resp, http.StatusOK)

		var perms []map[string]interface{}
		decodeBody(t, resp, &perms)

		if len(perms) != 2 {
			t.Errorf("expected 2 permissions for role-x, got %d", len(perms))
		}
		for _, p := range perms {
			if p["role_id"] != "role-x" {
				t.Errorf("unexpected role_id %v in results", p["role_id"])
			}
		}
	})

	t.Run("returns all permissions for role-y", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/entity-permissions?role_id=role-y", nil)
		assertStatus(t, resp, http.StatusOK)

		var perms []map[string]interface{}
		decodeBody(t, resp, &perms)

		if len(perms) != 1 {
			t.Errorf("expected 1 permission for role-y, got %d", len(perms))
		}
	})

	t.Run("returns empty slice for unknown role", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/entity-permissions?role_id=no-such-role", nil)
		assertStatus(t, resp, http.StatusOK)

		var perms []map[string]interface{}
		decodeBody(t, resp, &perms)

		if len(perms) != 0 {
			t.Errorf("expected 0 permissions for unknown role, got %d", len(perms))
		}
	})

	t.Run("returns 400 when role_id is missing", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/entity-permissions", nil)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

// ── TestDeleteEntityPermission ────────────────────────────────────────────────

func TestDeleteEntityPermission(t *testing.T) {
	ts, _ := newEntityPermissionTestServer(t)

	// Seed a permission to delete.
	setEntityPermissionViaHTTP(t, ts, "role-del", "SETTLEMENT", true, true)

	t.Run("returns 204 on success", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete,
			"/api/v1/securities/entity-permissions/role-del/SETTLEMENT", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("ListByRole after DELETE excludes deleted entry", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			"/api/v1/securities/entity-permissions?role_id=role-del", nil)
		assertStatus(t, resp, http.StatusOK)
		var perms []map[string]interface{}
		decodeBody(t, resp, &perms)
		if len(perms) != 0 {
			t.Errorf("expected 0 after delete, got %d", len(perms))
		}
	})

	t.Run("second DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete,
			"/api/v1/securities/entity-permissions/role-del/SETTLEMENT", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE unknown combination returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete,
			"/api/v1/securities/entity-permissions/no-role/NO_ENTITY", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE with invalid path returns 400", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete,
			"/api/v1/securities/entity-permissions/only-one-segment", nil)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

// ── TestEntityPermissionHandlers_Unconfigured ─────────────────────────────────

func TestEntityPermissionHandlers_Unconfigured(t *testing.T) {
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil, store.NewInMemoryCorporateActionStore(), store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(), store.NewInMemorySegmentStore(), store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(), store.NewInMemoryParticipantStore(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, nil, nil,
		nil, nil, nil,
		nil, nil, me, nil, nil,
		nil, nil, nil, nil, cfg,
	)
	// Do NOT call srv.SetEntityPermissionStore() — leave it nil.
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, fmt.Sprintf("/api/v1/securities/entity-permissions?role_id=%s", "role-x")},
		{http.MethodPut, "/api/v1/securities/entity-permissions"},
		{http.MethodDelete, "/api/v1/securities/entity-permissions/role-x/INSTRUMENT"},
	} {
		resp := doJSON(t, ts, tc.method, tc.path, map[string]interface{}{"role_id": "role-x", "entity_type": "INSTRUMENT"})
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: want 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
