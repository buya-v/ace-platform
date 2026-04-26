// Package server — tests for node hierarchy HTTP handlers.
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

// newNodeTestServer creates a test server wired with a real InMemoryNodeStore.
func newNodeTestServer(t *testing.T) (*httptest.Server, *store.InMemoryNodeStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	me := engine.NewMatchingEngine(instrStore, orderStore, tradeStore, positionStore, nil, nil, nil)
	nodeStore := store.NewInMemoryNodeStore()

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
		nodeStore,
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
	return ts, nodeStore
}

// createNodeViaHTTP sends POST /api/v1/securities/nodes and returns the created node ID.
func createNodeViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) string {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/nodes", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	id, _ := result["id"].(string)
	return id
}

// validNodePayload returns a minimal valid node creation payload.
func validNodePayload(firmID, name string) map[string]interface{} {
	return map[string]interface{}{
		"firm_id":     firmID,
		"name":        name,
		"permissions": []string{"READ", "WRITE"},
	}
}

// ============================================================
// TestCreateNode — 201
// ============================================================

func TestCreateNode(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	payload := validNodePayload("FIRM-A", "Trading Desk")
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/nodes", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if id, ok := result["id"].(string); !ok || id == "" {
		t.Error("expected non-empty id in response")
	}
	if result["firm_id"] != "FIRM-A" {
		t.Errorf("firm_id: want FIRM-A, got %v", result["firm_id"])
	}
	if result["name"] != "Trading Desk" {
		t.Errorf("name: want 'Trading Desk', got %v", result["name"])
	}
	perms, _ := result["permissions"].([]interface{})
	if len(perms) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(perms))
	}
	if result["created_at"] == nil || result["created_at"] == "" {
		t.Error("created_at must be set")
	}
}

func TestCreateNode_MissingFirmID(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/nodes",
		map[string]interface{}{"name": "Desk"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateNode_MissingName(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/nodes",
		map[string]interface{}{"firm_id": "FIRM-A"})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateNode_DefaultEmptyPermissions(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	// Omit permissions — handler should default to empty slice.
	payload := map[string]interface{}{
		"firm_id": "FIRM-B",
		"name":    "No Perms Desk",
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/nodes", payload)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	perms, _ := result["permissions"].([]interface{})
	if len(perms) != 0 {
		t.Errorf("expected 0 permissions when omitted, got %d", len(perms))
	}
}

// ============================================================
// TestListNodesByFirm
// ============================================================

func TestListNodesByFirm(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	// Create nodes for two firms.
	createNodeViaHTTP(t, ts, validNodePayload("FIRM-1", "Desk-A"))
	createNodeViaHTTP(t, ts, validNodePayload("FIRM-1", "Desk-B"))
	createNodeViaHTTP(t, ts, validNodePayload("FIRM-2", "Desk-C"))

	t.Run("filter by FIRM-1 returns 2 nodes", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/nodes?firm_id=FIRM-1", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"] != float64(2) {
			t.Errorf("FIRM-1 total: want 2, got %v", result["total"])
		}
	})

	t.Run("filter by FIRM-2 returns 1 node", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/nodes?firm_id=FIRM-2", nil)
		assertStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		decodeBody(t, resp, &result)
		if result["total"] != float64(1) {
			t.Errorf("FIRM-2 total: want 1, got %v", result["total"])
		}
	})

	t.Run("missing firm_id param returns 400", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/nodes", nil)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

// ============================================================
// TestGetNodePermissions — returns inherited permissions
// ============================================================

func TestGetNodePermissions(t *testing.T) {
	ts, nodeStore := newNodeTestServer(t)

	// Create parent with permissions.
	parentID := createNodeViaHTTP(t, ts, map[string]interface{}{
		"firm_id":     "FIRM-A",
		"name":        "Parent Desk",
		"permissions": []string{"TRADE", "REPORT"},
	})

	// Create child that inherits from parent.
	childID := createNodeViaHTTP(t, ts, map[string]interface{}{
		"firm_id":        "FIRM-A",
		"parent_node_id": parentID,
		"name":           "Child Desk",
		"permissions":    []string{"SETTLE"},
	})

	// Verify store has both nodes.
	_, err := nodeStore.Get(parentID)
	if err != nil {
		t.Fatalf("parent node not in store: %v", err)
	}

	// Get child effective permissions — should include parent's perms + own.
	resp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/nodes/%s/permissions", childID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["node_id"] != childID {
		t.Errorf("node_id: want %s, got %v", childID, result["node_id"])
	}

	perms, _ := result["permissions"].([]interface{})
	// Child should inherit TRADE and REPORT from parent, plus its own SETTLE.
	if len(perms) < 3 {
		t.Errorf("expected at least 3 effective permissions (inherited + own), got %d: %v", len(perms), perms)
	}
}

func TestGetNodePermissions_LeafNodeOwnPermsOnly(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	// Node with no parent — effective perms = own perms only.
	nodeID := createNodeViaHTTP(t, ts, map[string]interface{}{
		"firm_id":     "FIRM-A",
		"name":        "Solo Desk",
		"permissions": []string{"READ_ONLY"},
	})

	resp := doJSON(t, ts, http.MethodGet,
		fmt.Sprintf("/api/v1/securities/nodes/%s/permissions", nodeID), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	perms, _ := result["permissions"].([]interface{})
	if len(perms) != 1 {
		t.Errorf("expected 1 permission, got %d: %v", len(perms), perms)
	}
}

func TestGetNodePermissions_NotFound(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	resp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/nodes/no-such-id/permissions", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// ============================================================
// TestSetNodePermissions — 200
// ============================================================

func TestSetNodePermissions(t *testing.T) {
	ts, nodeStore := newNodeTestServer(t)

	nodeID := createNodeViaHTTP(t, ts, map[string]interface{}{
		"firm_id":     "FIRM-A",
		"name":        "Amend Desk",
		"permissions": []string{"READ"},
	})

	// Replace permissions.
	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/nodes/%s/permissions", nodeID),
		map[string]interface{}{
			"permissions": []string{"TRADE", "CANCEL", "REPORT"},
		})
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	perms, _ := result["permissions"].([]interface{})
	if len(perms) != 3 {
		t.Errorf("expected 3 permissions after set, got %d: %v", len(perms), perms)
	}

	// Verify store reflects the change via store.Get.
	node, err := nodeStore.Get(nodeID)
	if err != nil {
		t.Fatalf("Get node after SetPermissions: %v", err)
	}
	if len(node.Permissions) != 3 {
		t.Errorf("store: expected 3 permissions, got %d", len(node.Permissions))
	}
}

func TestSetNodePermissions_NotFound(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	resp := doJSON(t, ts, http.MethodPut,
		"/api/v1/securities/nodes/no-such-id/permissions",
		map[string]interface{}{"permissions": []string{"TRADE"}})
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestSetNodePermissions_EmptyList(t *testing.T) {
	ts, _ := newNodeTestServer(t)

	nodeID := createNodeViaHTTP(t, ts, map[string]interface{}{
		"firm_id":     "FIRM-A",
		"name":        "Clear Perms Desk",
		"permissions": []string{"TRADE"},
	})

	// Setting empty permissions is valid — clears all.
	resp := doJSON(t, ts, http.MethodPut,
		fmt.Sprintf("/api/v1/securities/nodes/%s/permissions", nodeID),
		map[string]interface{}{"permissions": []string{}})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// ============================================================
// TestNodeEndpoints_NotConfigured (503)
// ============================================================

func TestNodeEndpoints_NotConfigured(t *testing.T) {
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
		nil, nil, nil,
		nil, // nodeStore = nil
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
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
		{http.MethodGet, "/api/v1/securities/nodes?firm_id=X"},
		{http.MethodPost, "/api/v1/securities/nodes"},
		{http.MethodGet, "/api/v1/securities/nodes/node-id/permissions"},
		{http.MethodPut, "/api/v1/securities/nodes/node-id/permissions"},
	}

	for _, tc := range paths {
		resp := doJSON(t, httpTS, tc.method, tc.path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: expected 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
