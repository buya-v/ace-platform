// Package server — tests for instrument DELETE (soft-delete / flag for deletion).
package server

import (
	"net/http"
	"testing"
)

// ============================================================
// TestDeleteInstrument_FlagForDeletion
// ============================================================

// TestDeleteInstrument_FlagForDeletion creates an instrument, issues a DELETE
// request, and verifies the response contains deletion_status="FLAGGED" and a
// non-empty deletion_date.  The instrument is NOT removed from the store.
func TestDeleteInstrument_FlagForDeletion(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "DEL1",
		"name":        "Delete Me Instrument",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	// Issue DELETE.
	resp := doJSON(t, ts, http.MethodDelete,
		"/api/v1/securities/instruments/"+instrID, nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	// DeletionStatus must be FLAGGED.
	if result["deletion_status"] != "FLAGGED" {
		t.Errorf("expected deletion_status=FLAGGED, got %v", result["deletion_status"])
	}
	// DeletionDate must be non-empty (30 days from now).
	if dd, ok := result["deletion_date"].(string); !ok || dd == "" {
		t.Errorf("expected non-empty deletion_date, got %v", result["deletion_date"])
	}
	// Instrument id must be preserved.
	if result["id"] != instrID {
		t.Errorf("expected id %q, got %v", instrID, result["id"])
	}
}

// TestDeleteInstrument_NotFound verifies that deleting a non-existent instrument
// returns HTTP 404.
func TestDeleteInstrument_NotFound(t *testing.T) {
	ts := newTestServer(t)

	resp := doJSON(t, ts, http.MethodDelete,
		"/api/v1/securities/instruments/no-such-id", nil)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestDeleteInstrument_StillReadable verifies that a flagged instrument can
// still be retrieved via GET (soft-delete, not hard-delete).
func TestDeleteInstrument_StillReadable(t *testing.T) {
	ts := newTestServer(t)

	instrID := createInstr(t, ts, map[string]interface{}{
		"ticker":      "DEL2",
		"name":        "Soft Delete Instrument",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	})

	// Flag for deletion.
	delResp := doJSON(t, ts, http.MethodDelete,
		"/api/v1/securities/instruments/"+instrID, nil)
	assertStatus(t, delResp, http.StatusOK)
	delResp.Body.Close()

	// Must still be retrievable.
	getResp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/instruments/"+instrID, nil)
	assertStatus(t, getResp, http.StatusOK)

	var got map[string]interface{}
	decodeBody(t, getResp, &got)
	if got["deletion_status"] != "FLAGGED" {
		t.Errorf("expected deletion_status=FLAGGED after GET, got %v", got["deletion_status"])
	}
}
