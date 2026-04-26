// Package server — internal tests for RBAC role HTTP handlers.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/securities-service/internal/engine"
	"github.com/garudax-platform/securities-service/internal/middleware"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// newRoleTestServer creates a test server wired with a fresh InMemoryRoleStore
// and an InMemoryParticipantStore.  The roleStore is returned so tests can
// pre-seed roles without going through HTTP.
func newRoleTestServer(t *testing.T) (*httptest.Server, store.RoleStore) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	participantStore := store.NewInMemoryParticipantStore()
	roleStore := store.NewInMemoryRoleStore()

	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	pe := engine.NewPrivilegeEngine(participantStore, roleStore)

	cfg := DefaultConfig()
	srv := New(
		instrStore, orderStore, tradeStore, positionStore,
		nil,  // settlementStore
		store.NewInMemoryCorporateActionStore(),
		store.NewInMemoryEntitlementStore(),
		store.NewInMemoryMarketStore(),
		store.NewInMemorySegmentStore(),
		store.NewInMemoryCircuitBreakerStore(),
		store.NewInMemoryFirmStore(),
		participantStore,
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
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil,    // tradingCycleStore
		nil,    // dayManager
		me,     // matchingEngine
		nil,    // sessionManager
		nil,    // settlementEngine
		nil,    // producer
		pe,     // privilegeEngine
		roleStore,
		nil, // tradingParamSetStore
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, roleStore
}

// createRoleViaHTTP POSTs to /api/v1/securities/roles and returns the created role.
func createRoleViaHTTP(t *testing.T, ts *httptest.Server, name string, perms []string) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{
		"name":        name,
		"permissions": perms,
		"description": "test role " + name,
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/roles", body)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// seedThreeRoles creates ADMIN, TRADER, and VIEWER roles via the store directly.
func seedThreeRoles(t *testing.T, rs store.RoleStore) {
	t.Helper()
	roles := []*types.Role{
		{ID: "r-admin", Name: "ADMIN", Permissions: []string{types.PermAdminRoleManage, types.PermAdminAuditView}, CreatedAt: "2026-04-26T00:00:00Z", UpdatedAt: "2026-04-26T00:00:00Z"},
		{ID: "r-trader", Name: "TRADER", Permissions: []string{types.PermTradeEquity, types.PermTradeBond}, CreatedAt: "2026-04-26T00:00:00Z", UpdatedAt: "2026-04-26T00:00:00Z"},
		{ID: "r-viewer", Name: "VIEWER", Permissions: []string{types.PermAdminAuditView}, CreatedAt: "2026-04-26T00:00:00Z", UpdatedAt: "2026-04-26T00:00:00Z"},
	}
	for _, r := range roles {
		if err := rs.Create(r); err != nil {
			t.Fatalf("seedThreeRoles: Create %s: %v", r.Name, err)
		}
	}
}

// ── TestCreateRole ────────────────────────────────────────────────────────────

func TestCreateRole(t *testing.T) {
	ts, _ := newRoleTestServer(t)

	t.Run("returns 201 with valid body", func(t *testing.T) {
		body := map[string]interface{}{
			"name":        "ANALYST",
			"permissions": []string{types.PermTradeEquity},
			"description": "equity analyst",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/roles", body)
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] == nil || result["id"].(string) == "" {
			t.Error("created role must have a non-empty id")
		}
		if result["name"] != "ANALYST" {
			t.Errorf("name: want ANALYST, got %v", result["name"])
		}
		perms, ok := result["permissions"].([]interface{})
		if !ok || len(perms) != 1 {
			t.Errorf("permissions: want 1, got %v", result["permissions"])
		}
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		body := map[string]interface{}{"permissions": []string{}}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/roles", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("nil permissions defaults to empty slice", func(t *testing.T) {
		body := map[string]interface{}{"name": "EMPTY-PERMS"}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/roles", body)
		assertStatus(t, resp, http.StatusCreated)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		perms, ok := result["permissions"].([]interface{})
		if !ok {
			t.Errorf("permissions should be an array, got %T", result["permissions"])
		}
		if len(perms) != 0 {
			t.Errorf("permissions: want empty, got %v", perms)
		}
	})
}

// ── TestListRoles ─────────────────────────────────────────────────────────────

func TestListRoles(t *testing.T) {
	ts, rs := newRoleTestServer(t)
	seedThreeRoles(t, rs)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/roles", nil)
	assertStatus(t, resp, http.StatusOK)

	var roles []map[string]interface{}
	decodeBody(t, resp, &roles)

	if len(roles) < 3 {
		t.Fatalf("want at least 3 seeded roles, got %d", len(roles))
	}

	names := make(map[string]bool)
	for _, r := range roles {
		if name, ok := r["name"].(string); ok {
			names[name] = true
		}
	}
	for _, want := range []string{"ADMIN", "TRADER", "VIEWER"} {
		if !names[want] {
			t.Errorf("seeded role %s missing from GET /roles", want)
		}
	}
}

// ── TestGetRole ───────────────────────────────────────────────────────────────

func TestGetRole(t *testing.T) {
	ts, _ := newRoleTestServer(t)

	// Create a role first.
	created := createRoleViaHTTP(t, ts, "COMPLIANCE-OFFICER",
		[]string{types.PermAdminAuditView, types.PermSurveillanceView})
	id := created["id"].(string)

	t.Run("returns 200 with correct fields", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/roles/%s", id), nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] != id {
			t.Errorf("id: want %s, got %v", id, result["id"])
		}
		if result["name"] != "COMPLIANCE-OFFICER" {
			t.Errorf("name: want COMPLIANCE-OFFICER, got %v", result["name"])
		}
		perms, ok := result["permissions"].([]interface{})
		if !ok || len(perms) != 2 {
			t.Errorf("permissions: want 2, got %v", result["permissions"])
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/roles/no-such-role", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestUpdateRole ────────────────────────────────────────────────────────────

func TestUpdateRole(t *testing.T) {
	ts, _ := newRoleTestServer(t)

	created := createRoleViaHTTP(t, ts, "TO-UPDATE", []string{types.PermTradeEquity})
	id := created["id"].(string)

	t.Run("returns 200 with updated fields", func(t *testing.T) {
		updatedName := "UPDATED-ROLE"
		updatedDesc := "updated description"
		body := map[string]interface{}{
			"name":        updatedName,
			"description": updatedDesc,
			"permissions": []string{types.PermTradeBond, types.PermTradeETF},
		}
		resp := doJSON(t, ts, http.MethodPut, fmt.Sprintf("/api/v1/securities/roles/%s", id), body)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["name"] != updatedName {
			t.Errorf("name: want %s, got %v", updatedName, result["name"])
		}
		perms, ok := result["permissions"].([]interface{})
		if !ok || len(perms) != 2 {
			t.Errorf("permissions: want 2 after update, got %v", result["permissions"])
		}
		// Old permission should be gone.
		for _, p := range perms {
			if p == types.PermTradeEquity {
				t.Errorf("old permission %s should have been replaced", types.PermTradeEquity)
			}
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		body := map[string]interface{}{"name": "x"}
		resp := doJSON(t, ts, http.MethodPut, "/api/v1/securities/roles/no-such-role", body)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("subsequent GET reflects updated values", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/roles/%s", id), nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["name"] != "UPDATED-ROLE" {
			t.Errorf("GET after PUT: name want UPDATED-ROLE, got %v", result["name"])
		}
	})
}

// ── TestDeleteRole ────────────────────────────────────────────────────────────

func TestDeleteRole(t *testing.T) {
	ts, _ := newRoleTestServer(t)

	created := createRoleViaHTTP(t, ts, "TO-DELETE", []string{})
	id := created["id"].(string)

	t.Run("returns 204", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, fmt.Sprintf("/api/v1/securities/roles/%s", id), nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/roles/%s", id), nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("second DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, fmt.Sprintf("/api/v1/securities/roles/%s", id), nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE unknown id returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/roles/no-such-role", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestRoleStore_Unconfigured ─────────────────────────────────────────────────

// TestRoleHandlers_Unconfigured verifies that handlers return 503 when the
// roleStore is nil (server not configured with RBAC).
func TestRoleHandlers_Unconfigured(t *testing.T) {
	// Build a server with nil roleStore and nil privilegeEngine.
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
		nil, // producer
		nil, // privilegeEngine
		nil, // roleStore — intentionally nil
		nil, // tradingParamSetStore
		cfg,
	)
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
		{http.MethodGet, "/api/v1/securities/roles"},
		{http.MethodPost, "/api/v1/securities/roles"},
		{http.MethodGet, "/api/v1/securities/roles/some-id"},
		{http.MethodPut, "/api/v1/securities/roles/some-id"},
		{http.MethodDelete, "/api/v1/securities/roles/some-id"},
	} {
		resp := doJSON(t, ts, tc.method, tc.path, map[string]interface{}{"name": "x"})
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: want 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// ── JSON round-trip verification ──────────────────────────────────────────────

// TestCreateRole_JSONRoundTrip creates a role via HTTP and decodes the response
// into a types.Role to verify all JSON fields are present and correctly tagged.
func TestCreateRole_JSONRoundTrip(t *testing.T) {
	ts, _ := newRoleTestServer(t)

	body := map[string]interface{}{
		"name":        "ROUND-TRIP",
		"permissions": []string{types.PermTradeEquity, types.PermTradeBond},
		"description": "round trip test",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/roles", body)
	assertStatus(t, resp, http.StatusCreated)

	var role types.Role
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		t.Fatalf("decode into types.Role: %v", err)
	}

	if role.ID == "" {
		t.Error("Role.ID must not be empty")
	}
	if role.Name != "ROUND-TRIP" {
		t.Errorf("Role.Name: want ROUND-TRIP, got %s", role.Name)
	}
	if role.Description != "round trip test" {
		t.Errorf("Role.Description: want 'round trip test', got %s", role.Description)
	}
	if len(role.Permissions) != 2 {
		t.Errorf("Role.Permissions: want 2, got %d", len(role.Permissions))
	}
	if role.CreatedAt == "" {
		t.Error("Role.CreatedAt must not be empty")
	}
	if role.UpdatedAt == "" {
		t.Error("Role.UpdatedAt must not be empty")
	}
}
