// Package server — tests for the surveillance dashboard HTTP handler.
package server

import (
	"net/http"
	"testing"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// ============================================================
// TestDashboard_Empty
// ============================================================

// TestDashboard_Empty verifies that when no alerts exist the dashboard returns
// a well-formed response with all counts at zero and empty slices.
func TestDashboard_Empty(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/dashboard", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	// total_alerts must be present with all sub-counts zero.
	totals, ok := result["total_alerts"].(map[string]interface{})
	if !ok {
		t.Fatalf("total_alerts: expected object, got %T (%v)", result["total_alerts"], result["total_alerts"])
	}
	if totals["open"] != float64(0) {
		t.Errorf("total_alerts.open: want 0, got %v", totals["open"])
	}
	if totals["investigating"] != float64(0) {
		t.Errorf("total_alerts.investigating: want 0, got %v", totals["investigating"])
	}
	if totals["resolved"] != float64(0) {
		t.Errorf("total_alerts.resolved: want 0, got %v", totals["resolved"])
	}

	// alerts_by_type must be an empty object (not null).
	byType, ok := result["alerts_by_type"].(map[string]interface{})
	if !ok {
		t.Fatalf("alerts_by_type: expected object, got %T", result["alerts_by_type"])
	}
	if len(byType) != 0 {
		t.Errorf("alerts_by_type: expected empty map, got %v", byType)
	}

	// top_instruments must be present and empty.
	topInstr, ok := result["top_instruments"].([]interface{})
	if !ok {
		t.Fatalf("top_instruments: expected array, got %T", result["top_instruments"])
	}
	if len(topInstr) != 0 {
		t.Errorf("top_instruments: expected empty, got len=%d", len(topInstr))
	}

	// recent_alerts must be present and empty.
	recent, ok := result["recent_alerts"].([]interface{})
	if !ok {
		t.Fatalf("recent_alerts: expected array, got %T", result["recent_alerts"])
	}
	if len(recent) != 0 {
		t.Errorf("recent_alerts: expected empty, got len=%d", len(recent))
	}
}

// ============================================================
// TestDashboard_WithAlerts
// ============================================================

// TestDashboard_WithAlerts seeds 5 alerts with varied statuses and types,
// then verifies that the dashboard counts match exactly.
func TestDashboard_WithAlerts(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	// Seed 5 alerts: 2 OPEN, 2 INVESTIGATING, 1 RESOLVED.
	// Types: 2×LARGE_TRADE, 2×SPOOFING, 1×LAYERING.
	alerts := []types.SurveillanceAlert{
		{
			ID: "d-a1", InstrumentID: "INST-A", AlertType: types.AlertTypeLargeTrade,
			Status: types.AlertStatusOpen, Message: "large trade 1",
			CreatedAt: "2026-04-26T10:00:00Z",
		},
		{
			ID: "d-a2", InstrumentID: "INST-A", AlertType: types.AlertTypeLargeTrade,
			Status: types.AlertStatusInvestigating, Message: "large trade 2",
			CreatedAt: "2026-04-26T10:01:00Z",
		},
		{
			ID: "d-a3", InstrumentID: "INST-B", AlertType: types.AlertTypeSpoofing,
			Status: types.AlertStatusOpen, Message: "spoofing 1",
			CreatedAt: "2026-04-26T10:02:00Z",
		},
		{
			ID: "d-a4", InstrumentID: "INST-B", AlertType: types.AlertTypeSpoofing,
			Status: types.AlertStatusInvestigating, Message: "spoofing 2",
			CreatedAt: "2026-04-26T10:03:00Z",
		},
		{
			ID: "d-a5", InstrumentID: "INST-C", AlertType: types.AlertTypeLayering,
			Status: types.AlertStatusResolved, Message: "layering 1",
			CreatedAt: "2026-04-26T10:04:00Z",
		},
	}
	for i := range alerts {
		seedAlert(t, survStore, &alerts[i])
	}

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/dashboard", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	// ── status counts ──
	totals, ok := result["total_alerts"].(map[string]interface{})
	if !ok {
		t.Fatalf("total_alerts: expected object, got %T", result["total_alerts"])
	}
	if totals["open"] != float64(2) {
		t.Errorf("total_alerts.open: want 2, got %v", totals["open"])
	}
	if totals["investigating"] != float64(2) {
		t.Errorf("total_alerts.investigating: want 2, got %v", totals["investigating"])
	}
	if totals["resolved"] != float64(1) {
		t.Errorf("total_alerts.resolved: want 1, got %v", totals["resolved"])
	}

	// ── alerts by type ──
	byType, ok := result["alerts_by_type"].(map[string]interface{})
	if !ok {
		t.Fatalf("alerts_by_type: expected object, got %T", result["alerts_by_type"])
	}
	if byType[string(types.AlertTypeLargeTrade)] != float64(2) {
		t.Errorf("alerts_by_type[LARGE_TRADE]: want 2, got %v", byType[string(types.AlertTypeLargeTrade)])
	}
	if byType[string(types.AlertTypeSpoofing)] != float64(2) {
		t.Errorf("alerts_by_type[SPOOFING]: want 2, got %v", byType[string(types.AlertTypeSpoofing)])
	}
	if byType[string(types.AlertTypeLayering)] != float64(1) {
		t.Errorf("alerts_by_type[LAYERING]: want 1, got %v", byType[string(types.AlertTypeLayering)])
	}

	// ── recent_alerts count ──
	recent, ok := result["recent_alerts"].([]interface{})
	if !ok {
		t.Fatalf("recent_alerts: expected array, got %T", result["recent_alerts"])
	}
	if len(recent) != 5 {
		t.Errorf("recent_alerts: want 5, got %d", len(recent))
	}
}

// ============================================================
// TestDashboard_TopInstruments
// ============================================================

// TestDashboard_TopInstruments verifies that instruments are ranked by descending
// alert count and that the list is correctly ordered.
func TestDashboard_TopInstruments(t *testing.T) {
	ts, survStore := newSurveillanceTestServer(t)

	// INST-HOT → 3 alerts, INST-WARM → 2 alerts, INST-COOL → 1 alert.
	seed := func(id, instrID string, alertType types.AlertType, ts string) {
		seedAlert(t, survStore, &types.SurveillanceAlert{
			ID: id, InstrumentID: instrID, AlertType: alertType,
			Status: types.AlertStatusOpen, Message: "test", CreatedAt: ts,
		})
	}
	seed("ti-1", "INST-HOT", types.AlertTypeLargeTrade, "2026-04-26T09:00:00Z")
	seed("ti-2", "INST-HOT", types.AlertTypeSpoofing, "2026-04-26T09:01:00Z")
	seed("ti-3", "INST-HOT", types.AlertTypeLayering, "2026-04-26T09:02:00Z")
	seed("ti-4", "INST-WARM", types.AlertTypeLargeTrade, "2026-04-26T09:03:00Z")
	seed("ti-5", "INST-WARM", types.AlertTypePriceSpike, "2026-04-26T09:04:00Z")
	seed("ti-6", "INST-COOL", types.AlertTypeLargeTrade, "2026-04-26T09:05:00Z")

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/dashboard", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	topInstr, ok := result["top_instruments"].([]interface{})
	if !ok {
		t.Fatalf("top_instruments: expected array, got %T", result["top_instruments"])
	}
	if len(topInstr) != 3 {
		t.Fatalf("top_instruments: expected 3 entries, got %d", len(topInstr))
	}

	// First entry must be INST-HOT with 3 alerts.
	first, ok := topInstr[0].(map[string]interface{})
	if !ok {
		t.Fatalf("top_instruments[0]: expected object, got %T", topInstr[0])
	}
	if first["instrument_id"] != "INST-HOT" {
		t.Errorf("top_instruments[0].instrument_id: want INST-HOT, got %v", first["instrument_id"])
	}
	if first["alert_count"] != float64(3) {
		t.Errorf("top_instruments[0].alert_count: want 3, got %v", first["alert_count"])
	}

	// Second entry must be INST-WARM with 2 alerts.
	second, ok := topInstr[1].(map[string]interface{})
	if !ok {
		t.Fatalf("top_instruments[1]: expected object, got %T", topInstr[1])
	}
	if second["instrument_id"] != "INST-WARM" {
		t.Errorf("top_instruments[1].instrument_id: want INST-WARM, got %v", second["instrument_id"])
	}
	if second["alert_count"] != float64(2) {
		t.Errorf("top_instruments[1].alert_count: want 2, got %v", second["alert_count"])
	}

	// Third entry must be INST-COOL with 1 alert.
	third, ok := topInstr[2].(map[string]interface{})
	if !ok {
		t.Fatalf("top_instruments[2]: expected object, got %T", topInstr[2])
	}
	if third["instrument_id"] != "INST-COOL" {
		t.Errorf("top_instruments[2].instrument_id: want INST-COOL, got %v", third["instrument_id"])
	}
	if third["alert_count"] != float64(1) {
		t.Errorf("top_instruments[2].alert_count: want 1, got %v", third["alert_count"])
	}
}

// ============================================================
// TestDashboard_NotConfigured
// ============================================================

// TestDashboard_NotConfigured verifies that the dashboard endpoint returns
// HTTP 503 when the surveillance store is not wired into the server.
func TestDashboard_NotConfigured(t *testing.T) {
	// Reuse the handler test helper that builds a server without a surveillance store.
	// We exercise the no-surveillance code path used by TestSurveillanceEndpoints_NotConfigured.
	ts := newTestServer(t) // surveillance store is nil in newTestServer

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/surveillance/dashboard", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("dashboard without surveillance store: want 503, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// ============================================================
// TestDashboard_MethodNotAllowed
// ============================================================

// TestDashboard_MethodNotAllowed verifies that non-GET methods are rejected.
func TestDashboard_MethodNotAllowed(t *testing.T) {
	ts, _ := newSurveillanceTestServer(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		resp := doJSON(t, ts, method, "/api/v1/securities/surveillance/dashboard", nil)
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s /surveillance/dashboard: want 405, got %d", method, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// ============================================================
// helpers — reuse store.SurveillanceAlertFilters for direct store validation
// ============================================================

// verifyAlertCountInStore is a local helper for counting alerts of a given status.
func verifyAlertCountInStore(t *testing.T, ss *store.InMemorySurveillanceStore, status types.AlertStatus, want int) {
	t.Helper()
	alerts, err := ss.ListAlerts(store.SurveillanceAlertFilters{Status: status})
	if err != nil {
		t.Fatalf("ListAlerts(status=%s): %v", status, err)
	}
	if len(alerts) != want {
		t.Errorf("store alert count for status %s: want %d, got %d", status, want, len(alerts))
	}
}
