// Package server — tests for bulk instrument upload HTTP handler (P3b).
package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/garudax-platform/securities-service/internal/types"
)

// validBulkItem returns a minimal valid bulk instrument item.
func validBulkItem(ticker string) map[string]interface{} {
	return map[string]interface{}{
		"ticker":      ticker,
		"name":        ticker + " Corp",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
		"currency":    "MNT",
	}
}

// ============================================================
// TestBulkUpload_AllValid
// ============================================================

// TestBulkUpload_AllValid posts 3 valid instruments and verifies
// created=3, failed=0 in the response.
func TestBulkUpload_AllValid(t *testing.T) {
	ts := newTestServer(t)

	payload := []interface{}{
		validBulkItem("BULK1"),
		validBulkItem("BULK2"),
		validBulkItem("BULK3"),
	}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments", payload)
	// Bulk endpoint always returns 200 (partial success is not an HTTP error).
	assertStatus(t, resp, http.StatusOK)

	var result types.BulkUploadResult
	decodeBody(t, resp, &result)

	if result.Total != 3 {
		t.Errorf("expected total=3, got %d", result.Total)
	}
	if result.Created != 3 {
		t.Errorf("expected created=3, got %d", result.Created)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed=0, got %d", result.Failed)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

// ============================================================
// TestBulkUpload_PartialFail
// ============================================================

// TestBulkUpload_PartialFail posts 3 items where 1 has a missing ticker.
// Expects created=2, failed=1.
func TestBulkUpload_PartialFail(t *testing.T) {
	ts := newTestServer(t)

	// Item at index 1 has no ticker — validation should fail for that entry only.
	badItem := map[string]interface{}{
		"name":        "No Ticker Corp",
		"asset_class": "EQUITY",
		"lot_size":    100,
		"tick_size":   0.01,
	}

	payload := []interface{}{
		validBulkItem("PFOK1"),
		badItem,
		validBulkItem("PFOK3"),
	}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments", payload)
	assertStatus(t, resp, http.StatusOK)

	var result types.BulkUploadResult
	decodeBody(t, resp, &result)

	if result.Total != 3 {
		t.Errorf("expected total=3, got %d", result.Total)
	}
	if result.Created != 2 {
		t.Errorf("expected created=2, got %d", result.Created)
	}
	if result.Failed != 1 {
		t.Errorf("expected failed=1, got %d", result.Failed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error entry, got %d", len(result.Errors))
	}
	if len(result.Errors) > 0 && result.Errors[0].Index != 1 {
		t.Errorf("expected error at index 1, got index %d", result.Errors[0].Index)
	}
}

// ============================================================
// TestBulkUpload_EmptyArray
// ============================================================

// TestBulkUpload_EmptyArray sends a non-array JSON body and expects 400
// with INVALID_JSON error code. (A genuine empty array [] would be valid and
// return 200 with total=0; the handler's json.Unmarshal into []bulkInstrumentItem
// rejects object bodies.)
func TestBulkUpload_EmptyArray(t *testing.T) {
	ts := newTestServer(t)

	// Send an object instead of an array — this causes json.Decoder to return
	// an error when unmarshalling into []bulkInstrumentItem, triggering 400.
	req, err := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/securities/bulk/instruments",
		strings.NewReader(`{"not": "an array"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GarudaX-Tenant", testTenant)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp types.ErrorResponse
	decodeBody(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_JSON" {
		t.Errorf("expected INVALID_JSON, got %q", errResp.Error.Code)
	}
}

// TestBulkUpload_LotSizeInvalid verifies that lot_size=0 causes a per-item failure.
func TestBulkUpload_LotSizeInvalid(t *testing.T) {
	ts := newTestServer(t)

	badLot := map[string]interface{}{
		"ticker":      "BADK",
		"name":        "Bad Lot Corp",
		"asset_class": "EQUITY",
		"lot_size":    0,
		"tick_size":   0.01,
	}
	payload := []interface{}{badLot}

	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments", payload)
	assertStatus(t, resp, http.StatusOK)

	var result types.BulkUploadResult
	decodeBody(t, resp, &result)

	if result.Failed != 1 {
		t.Errorf("expected failed=1, got %d", result.Failed)
	}
}
