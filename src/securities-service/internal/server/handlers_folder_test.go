// Package server — internal tests for folder HTTP handlers (Sprint 8 Part C).
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

// newFolderTestServer creates a test server wired with a fresh InMemoryFolderStore.
// The folderStore is returned so tests can pre-seed data without going through HTTP.
func newFolderTestServer(t *testing.T) (*httptest.Server, store.FolderStore) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	folderStore := store.NewInMemoryFolderStore()

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
	srv.SetFolderStore(folderStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, folderStore
}

// validFolderPayload returns a minimal valid payload for creating a folder.
func validFolderPayload(name string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
	}
}

// createFolderViaHTTP POSTs to the folders endpoint and returns the created map.
func createFolderViaHTTP(t *testing.T, ts *httptest.Server, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/folders", payload)
	assertStatus(t, resp, http.StatusCreated)
	var result map[string]interface{}
	decodeBody(t, resp, &result)
	return result
}

// ── TestCreateFolder ──────────────────────────────────────────────────────────

func TestCreateFolder(t *testing.T) {
	ts, _ := newFolderTestServer(t)

	t.Run("returns 201 with valid body", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/folders",
			validFolderPayload("Equities"))
		assertStatus(t, resp, http.StatusCreated)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] == nil || result["id"].(string) == "" {
			t.Error("created folder must have a non-empty id")
		}
		if result["name"] != "Equities" {
			t.Errorf("name: want Equities, got %v", result["name"])
		}
		if result["created_at"] == nil || result["created_at"].(string) == "" {
			t.Error("created_at must be populated")
		}
	})

	t.Run("assigns id when not provided", func(t *testing.T) {
		result := createFolderViaHTTP(t, ts, validFolderPayload("Auto ID Folder"))
		if _, ok := result["id"].(string); !ok {
			t.Error("id must be a string")
		}
	})

	t.Run("respects provided id", func(t *testing.T) {
		payload := validFolderPayload("Custom ID Folder")
		payload["id"] = "custom-fld-id"
		result := createFolderViaHTTP(t, ts, payload)
		if result["id"] != "custom-fld-id" {
			t.Errorf("id: want custom-fld-id, got %v", result["id"])
		}
	})

	t.Run("creates child folder with parent_id", func(t *testing.T) {
		parent := createFolderViaHTTP(t, ts, validFolderPayload("Parent Folder"))
		parentID := parent["id"].(string)

		childPayload := validFolderPayload("Child Folder")
		childPayload["parent_id"] = parentID
		child := createFolderViaHTTP(t, ts, childPayload)

		if child["parent_id"] != parentID {
			t.Errorf("parent_id: want %s, got %v", parentID, child["parent_id"])
		}
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		body := map[string]interface{}{
			"parent_id": "some-parent",
		}
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/folders", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("returns 400 on invalid JSON", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/folders", "not-json")
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("returns 409 on duplicate id", func(t *testing.T) {
		payload := validFolderPayload("Dup Folder")
		payload["id"] = "dup-fld"
		createFolderViaHTTP(t, ts, payload)
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/folders", payload)
		assertStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})
}

// ── TestListFolders ───────────────────────────────────────────────────────────

func TestListFolders(t *testing.T) {
	ts, fldStore := newFolderTestServer(t)

	// Pre-seed three folders directly through the store.
	for _, f := range []*types.Folder{
		{ID: "fld-list-1", Name: "Alpha", CreatedAt: "2026-04-26T00:00:00Z"},
		{ID: "fld-list-2", Name: "Beta", ParentID: "fld-list-1", CreatedAt: "2026-04-26T00:00:00Z"},
		{ID: "fld-list-3", Name: "Gamma", CreatedAt: "2026-04-26T00:00:00Z"},
	} {
		if err := fldStore.Create(f); err != nil {
			t.Fatalf("seed folder %s: %v", f.ID, err)
		}
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders", nil)
	assertStatus(t, resp, http.StatusOK)

	var folders []map[string]interface{}
	decodeBody(t, resp, &folders)

	if len(folders) < 3 {
		t.Fatalf("expected at least 3 folders, got %d", len(folders))
	}
}

// ── TestGetFolder ─────────────────────────────────────────────────────────────

func TestGetFolder(t *testing.T) {
	ts, _ := newFolderTestServer(t)

	created := createFolderViaHTTP(t, ts, validFolderPayload("My Folder"))
	id := created["id"].(string)

	t.Run("returns 200 with correct fields", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, fmt.Sprintf("/api/v1/securities/folders/%s", id), nil)
		assertStatus(t, resp, http.StatusOK)

		var result map[string]interface{}
		decodeBody(t, resp, &result)

		if result["id"] != id {
			t.Errorf("id: want %s, got %v", id, result["id"])
		}
		if result["name"] != "My Folder" {
			t.Errorf("name: want My Folder, got %v", result["name"])
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders/no-such-fld", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestDeleteFolder ──────────────────────────────────────────────────────────

func TestDeleteFolder(t *testing.T) {
	ts, _ := newFolderTestServer(t)

	payload := validFolderPayload("Delete Me")
	payload["id"] = "fld-to-delete"
	createFolderViaHTTP(t, ts, payload)

	t.Run("returns 204 on success", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/folders/fld-to-delete", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("GET after DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders/fld-to-delete", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("second DELETE returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/folders/fld-to-delete", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("DELETE unknown id returns 404", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodDelete, "/api/v1/securities/folders/no-such-fld", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

// ── TestListFolderChildren ────────────────────────────────────────────────────

func TestListFolderChildren(t *testing.T) {
	ts, _ := newFolderTestServer(t)

	// Create root.
	rootPayload := validFolderPayload("Root")
	rootPayload["id"] = "root-fld"
	createFolderViaHTTP(t, ts, rootPayload)

	// Create two children.
	child1Payload := validFolderPayload("Child 1")
	child1Payload["parent_id"] = "root-fld"
	child1 := createFolderViaHTTP(t, ts, child1Payload)
	child1ID := child1["id"].(string)

	child2Payload := validFolderPayload("Child 2")
	child2Payload["parent_id"] = "root-fld"
	createFolderViaHTTP(t, ts, child2Payload)

	// Create grandchild under child1.
	grandChildPayload := validFolderPayload("Grandchild")
	grandChildPayload["parent_id"] = child1ID
	createFolderViaHTTP(t, ts, grandChildPayload)

	t.Run("returns direct children of root", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders/root-fld/children", nil)
		assertStatus(t, resp, http.StatusOK)

		var children []map[string]interface{}
		decodeBody(t, resp, &children)

		if len(children) != 2 {
			t.Errorf("expected 2 direct children of root, got %d", len(children))
		}
		for _, c := range children {
			if c["parent_id"] != "root-fld" {
				t.Errorf("child parent_id: want root-fld, got %v", c["parent_id"])
			}
		}
	})

	t.Run("returns grandchild as child of child1", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			fmt.Sprintf("/api/v1/securities/folders/%s/children", child1ID), nil)
		assertStatus(t, resp, http.StatusOK)

		var children []map[string]interface{}
		decodeBody(t, resp, &children)

		if len(children) != 1 {
			t.Errorf("expected 1 grandchild, got %d", len(children))
		}
	})

	t.Run("returns empty slice for folder with no children", func(t *testing.T) {
		// Create a leaf folder with no children.
		leafPayload := validFolderPayload("Leaf")
		leafPayload["id"] = "leaf-fld"
		createFolderViaHTTP(t, ts, leafPayload)

		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders/leaf-fld/children", nil)
		assertStatus(t, resp, http.StatusOK)

		var children []map[string]interface{}
		decodeBody(t, resp, &children)

		if len(children) != 0 {
			t.Errorf("expected 0 children for leaf folder, got %d", len(children))
		}
	})

	t.Run("returns empty slice for unknown parent id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/folders/no-such-fld/children", nil)
		assertStatus(t, resp, http.StatusOK)

		var children []map[string]interface{}
		decodeBody(t, resp, &children)

		if len(children) != 0 {
			t.Errorf("expected 0 children for unknown parent, got %d", len(children))
		}
	})
}

// ── TestFolderHandlers_Unconfigured ───────────────────────────────────────────

func TestFolderHandlers_Unconfigured(t *testing.T) {
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
	// Do NOT call srv.SetFolderStore() — leave it nil.
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
		{http.MethodGet, "/api/v1/securities/folders"},
		{http.MethodPost, "/api/v1/securities/folders"},
		{http.MethodGet, "/api/v1/securities/folders/some-id"},
		{http.MethodDelete, "/api/v1/securities/folders/some-id"},
		{http.MethodGet, "/api/v1/securities/folders/some-id/children"},
	} {
		resp := doJSON(t, ts, tc.method, tc.path, map[string]interface{}{"name": "x"})
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: want 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
