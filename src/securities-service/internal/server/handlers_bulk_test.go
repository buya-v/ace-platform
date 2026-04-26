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

// ============================================================
// TestCSVImport_Valid — 3 rows created
// ============================================================

// TestCSVImport_Valid sends a CSV with 3 valid rows and expects created=3.
func TestCSVImport_Valid(t *testing.T) {
	ts := newTestServer(t)

	csvBody := strings.Join([]string{
		"ticker,name,asset_class,exchange_code,lot_size,tick_size,currency",
		"CSV1,CSV One Corp,EQUITY,MSE,100,0.01,MNT",
		"CSV2,CSV Two Corp,EQUITY,MSE,200,0.05,MNT",
		"CSV3,CSV Three Corp,BOND,MSE,50,0.10,MNT",
	}, "\n")

	req, err := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/securities/bulk/instruments/csv",
		strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "text/csv")
	req.Header.Set("X-GarudaX-Tenant", testTenant)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)

	var result types.BulkUploadResult
	decodeBody(t, resp, &result)

	if result.Total != 3 {
		t.Errorf("total: want 3, got %d", result.Total)
	}
	if result.Created != 3 {
		t.Errorf("created: want 3, got %d", result.Created)
	}
	if result.Failed != 0 {
		t.Errorf("failed: want 0, got %d: %v", result.Failed, result.Errors)
	}
}

// ============================================================
// TestCSVImport_PartialFail — 1 error
// ============================================================

// TestCSVImport_PartialFail sends 3 rows where the middle one has a missing ticker.
// Expects created=2, failed=1.
func TestCSVImport_PartialFail(t *testing.T) {
	ts := newTestServer(t)

	// Row 2 has no ticker column — results in empty ticker, causing validation failure.
	csvBody := strings.Join([]string{
		"ticker,name,asset_class,exchange_code,lot_size,tick_size",
		"PFCSV1,PF One,EQUITY,MSE,100,0.01",
		",No Ticker Corp,EQUITY,MSE,100,0.01",
		"PFCSV3,PF Three,EQUITY,MSE,100,0.01",
	}, "\n")

	req, err := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/securities/bulk/instruments/csv",
		strings.NewReader(csvBody))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "text/csv")
	req.Header.Set("X-GarudaX-Tenant", testTenant)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	assertStatus(t, resp, http.StatusOK)

	var result types.BulkUploadResult
	decodeBody(t, resp, &result)

	if result.Failed != 1 {
		t.Errorf("failed: want 1, got %d", result.Failed)
	}
	if result.Created != 2 {
		t.Errorf("created: want 2, got %d", result.Created)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors length: want 1, got %d", len(result.Errors))
	}
}

// ============================================================
// TestMassAmend_Success — 2 updated
// ============================================================

// TestMassAmend_Success pre-creates 2 instruments via bulk upload, then amends
// both via the mass-amend endpoint. Expects updated=2.
func TestMassAmend_Success(t *testing.T) {
	ts := newTestServer(t)

	// Create two instruments.
	createPayload := []interface{}{
		validBulkItem("AMEND1"),
		validBulkItem("AMEND2"),
	}
	createResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments", createPayload)
	assertStatus(t, createResp, http.StatusOK)
	var createResult types.BulkUploadResult
	decodeBody(t, createResp, &createResult)
	if createResult.Created != 2 {
		t.Fatalf("setup: expected 2 created, got %d", createResult.Created)
	}

	// Retrieve the IDs by listing instruments.
	listResp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/instruments", nil)
	assertStatus(t, listResp, http.StatusOK)
	var listResult map[string]interface{}
	decodeBody(t, listResp, &listResult)
	data, _ := listResult["data"].([]interface{})
	if len(data) < 2 {
		t.Fatalf("expected at least 2 instruments, got %d", len(data))
	}

	// Collect IDs.
	var ids []string
	for _, item := range data {
		instr, _ := item.(map[string]interface{})
		ticker, _ := instr["ticker"].(string)
		if ticker == "AMEND1" || ticker == "AMEND2" {
			id, _ := instr["id"].(string)
			ids = append(ids, id)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 AMEND instrument IDs, got %d", len(ids))
	}

	// Mass amend both.
	amendPayload := []interface{}{
		map[string]interface{}{"id": ids[0], "name": "Amended Name One", "lot_size": 200},
		map[string]interface{}{"id": ids[1], "name": "Amended Name Two", "lot_size": 300},
	}
	amendResp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments/amend", amendPayload)
	assertStatus(t, amendResp, http.StatusOK)

	var amendResult BulkAmendResult
	decodeBody(t, amendResp, &amendResult)

	if amendResult.Total != 2 {
		t.Errorf("total: want 2, got %d", amendResult.Total)
	}
	if amendResult.Updated != 2 {
		t.Errorf("updated: want 2, got %d", amendResult.Updated)
	}
	if amendResult.Failed != 0 {
		t.Errorf("failed: want 0, got %d: %v", amendResult.Failed, amendResult.Errors)
	}
}

// ============================================================
// TestMassAmend_NotFound — error in result
// ============================================================

// TestMassAmend_NotFound attempts to amend a non-existent instrument ID.
// The endpoint returns 200 with failed=1 and an error entry in the result.
func TestMassAmend_NotFound(t *testing.T) {
	ts := newTestServer(t)

	amendPayload := []interface{}{
		map[string]interface{}{"id": "NO-SUCH-INSTR", "name": "Ghost Corp", "lot_size": 100},
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments/amend", amendPayload)
	assertStatus(t, resp, http.StatusOK)

	var result BulkAmendResult
	decodeBody(t, resp, &result)

	if result.Total != 1 {
		t.Errorf("total: want 1, got %d", result.Total)
	}
	if result.Failed != 1 {
		t.Errorf("failed: want 1, got %d", result.Failed)
	}
	if result.Updated != 0 {
		t.Errorf("updated: want 0, got %d", result.Updated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors length: want 1, got %d", len(result.Errors))
	}
}

// TestMassAmend_MissingID verifies that an amend item without an id is rejected
// as a per-item error (not an HTTP error).
func TestMassAmend_MissingID(t *testing.T) {
	ts := newTestServer(t)

	amendPayload := []interface{}{
		map[string]interface{}{"name": "No ID Corp", "lot_size": 100},
	}
	resp := doJSON(t, ts, http.MethodPost, "/api/v1/securities/bulk/instruments/amend", amendPayload)
	assertStatus(t, resp, http.StatusOK)

	var result BulkAmendResult
	decodeBody(t, resp, &result)

	if result.Failed != 1 {
		t.Errorf("failed: want 1, got %d", result.Failed)
	}
}
