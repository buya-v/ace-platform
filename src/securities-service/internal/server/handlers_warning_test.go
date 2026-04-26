// Package server — internal tests for warning HTTP handlers (Sprint 8 Part D).
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

// newWarningTestServer creates a test server wired with a fresh InMemoryWarningStore.
// The warningStore is returned so tests can pre-seed data without going through HTTP.
func newWarningTestServer(t *testing.T) (*httptest.Server, store.WarningStore) {
	t.Helper()

	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	warnStore := store.NewInMemoryWarningStore()

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
	srv.SetWarningStore(warnStore)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, warnStore
}

// seedWarnings creates the given warnings directly in the store.
func seedWarnings(t *testing.T, ws store.WarningStore, warnings []*types.Warning) {
	t.Helper()
	for _, w := range warnings {
		if err := ws.Create(w); err != nil {
			t.Fatalf("seedWarnings: Create %s: %v", w.ID, err)
		}
	}
}

// ── TestListWarnings ──────────────────────────────────────────────────────────

func TestListWarnings(t *testing.T) {
	ts, warnStore := newWarningTestServer(t)

	// Seed two unacknowledged warnings.
	seedWarnings(t, warnStore, []*types.Warning{
		{
			ID:          "warn-list-1",
			WarningType: types.WarnDeleteActive,
			EntityType:  "INSTRUMENT",
			EntityID:    "inst-1",
			Message:     "Active instrument flagged for deletion",
			Severity:    "HIGH",
			CreatedAt:   "2026-04-26T00:00:00Z",
		},
		{
			ID:          "warn-list-2",
			WarningType: types.WarnLargeOrder,
			EntityType:  "ORDER",
			EntityID:    "ord-1",
			Message:     "Large order detected",
			Severity:    "MEDIUM",
			CreatedAt:   "2026-04-26T00:00:00Z",
		},
	})

	t.Run("default lists unacknowledged warnings", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings", nil)
		assertStatus(t, resp, http.StatusOK)

		var warnings []map[string]interface{}
		decodeBody(t, resp, &warnings)

		if len(warnings) < 2 {
			t.Fatalf("expected at least 2 unacknowledged warnings, got %d", len(warnings))
		}
	})

	t.Run("acknowledged=false lists unacknowledged warnings", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings?acknowledged=false", nil)
		assertStatus(t, resp, http.StatusOK)

		var warnings []map[string]interface{}
		decodeBody(t, resp, &warnings)

		if len(warnings) < 2 {
			t.Fatalf("expected at least 2 unacknowledged warnings, got %d", len(warnings))
		}
		// All returned warnings should have no acknowledged_by.
		for _, w := range warnings {
			if ack, ok := w["acknowledged_by"]; ok && ack != "" {
				t.Errorf("warning %v should not have acknowledged_by, got %v", w["id"], ack)
			}
		}
	})

	t.Run("acknowledged=true returns empty when none are acknowledged", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings?acknowledged=true", nil)
		assertStatus(t, resp, http.StatusOK)

		var warnings []map[string]interface{}
		decodeBody(t, resp, &warnings)

		// No warnings have been acknowledged yet in this test.
		if len(warnings) != 0 {
			t.Errorf("expected 0 acknowledged warnings, got %d", len(warnings))
		}
	})

	t.Run("returns 405 on non-GET methods", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/warnings", nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ── TestAcknowledgeWarning ────────────────────────────────────────────────────

func TestAcknowledgeWarning(t *testing.T) {
	ts, warnStore := newWarningTestServer(t)

	seedWarnings(t, warnStore, []*types.Warning{
		{
			ID:          "warn-ack-1",
			WarningType: types.WarnHaltDuringAuction,
			EntityType:  "MARKET",
			EntityID:    "mkt-1",
			Message:     "Halt during auction",
			Severity:    "HIGH",
			CreatedAt:   "2026-04-26T00:00:00Z",
		},
		{
			ID:          "warn-ack-2",
			WarningType: types.WarnRoleDeletion,
			EntityType:  "ROLE",
			EntityID:    "role-1",
			Message:     "Role deletion pending",
			Severity:    "LOW",
			CreatedAt:   "2026-04-26T00:00:00Z",
		},
	})

	t.Run("returns 204 on successful acknowledge", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/warnings/warn-ack-1/acknowledge", nil)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("acknowledged warning appears in acknowledged=true list", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings?acknowledged=true", nil)
		assertStatus(t, resp, http.StatusOK)

		var warnings []map[string]interface{}
		decodeBody(t, resp, &warnings)

		found := false
		for _, w := range warnings {
			if w["id"] == "warn-ack-1" {
				found = true
				break
			}
		}
		if !found {
			t.Error("warn-ack-1 should appear in acknowledged=true list after acknowledgement")
		}
	})

	t.Run("acknowledged warning removed from unacknowledged list", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings?acknowledged=false", nil)
		assertStatus(t, resp, http.StatusOK)

		var warnings []map[string]interface{}
		decodeBody(t, resp, &warnings)

		for _, w := range warnings {
			if w["id"] == "warn-ack-1" {
				t.Error("warn-ack-1 should NOT appear in unacknowledged list after acknowledgement")
			}
		}
	})

	t.Run("acknowledge with X-User-ID header", func(t *testing.T) {
		// Acknowledge warn-ack-2 with a custom user ID.
		req, err := http.NewRequest(http.MethodPost,
			ts.URL+"/api/v1/securities/warnings/warn-ack-2/acknowledge", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("X-GarudaX-Tenant", testTenant)
		req.Header.Set("X-User-ID", "operator-42")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("want 204, got %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Verify acknowledged_by in the list.
		listResp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings?acknowledged=true", nil)
		assertStatus(t, listResp, http.StatusOK)
		var warnings []map[string]interface{}
		decodeBody(t, listResp, &warnings)

		for _, w := range warnings {
			if w["id"] == "warn-ack-2" {
				if w["acknowledged_by"] != "operator-42" {
					t.Errorf("acknowledged_by: want operator-42, got %v", w["acknowledged_by"])
				}
				return
			}
		}
		t.Error("warn-ack-2 not found in acknowledged list")
	})

	t.Run("returns 404 for unknown warning id", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodPost,
			"/api/v1/securities/warnings/no-such-warn/acknowledge", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("returns 404 for invalid item path (no action suffix)", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/warnings/some-id", nil)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("returns 405 for wrong method on acknowledge", func(t *testing.T) {
		resp := doJSON(t, ts, http.MethodGet,
			fmt.Sprintf("/api/v1/securities/warnings/%s/acknowledge", "warn-ack-1"), nil)
		assertStatus(t, resp, http.StatusMethodNotAllowed)
		resp.Body.Close()
	})
}

// ── TestWarningHandlers_Unconfigured ──────────────────────────────────────────

func TestWarningHandlers_Unconfigured(t *testing.T) {
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
	// Do NOT call srv.SetWarningStore() — leave it nil.
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
		{http.MethodGet, "/api/v1/securities/warnings"},
		{http.MethodPost, "/api/v1/securities/warnings/some-id/acknowledge"},
	} {
		resp := doJSON(t, ts, tc.method, tc.path, nil)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s %s: want 503, got %d", tc.method, tc.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
