// Package server — internal tests for pending change HTTP handlers.
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
)

// newPendingChangeTestServer creates a test server with a wired PendingChangeStore.
func newPendingChangeTestServer(t *testing.T) (*httptest.Server, *store.InMemoryPendingChangeStore) {
	t.Helper()
	instrStore := store.NewInMemoryInstrumentStore()
	orderStore := store.NewInMemoryOrderStore()
	tradeStore := store.NewInMemoryTradeStore()
	positionStore := store.NewInMemoryPositionStore()
	pcStore := store.NewInMemoryPendingChangeStore()

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
		nil, nil, nil, nil, nil, nil,
		pcStore,
		nil,
		nil, nil, nil,
		nil, nil, nil, // nodeStore
		nil, // locateStore, rfqStore, giveUpStore
		nil, nil, nil, // investigationStore, replayStore, bondStore
		nil, nil, nil, nil, // strategyStore, custodyAccountStore, custodyBalanceStore, csdTransferStore
		nil, nil, nil, // watchListStore, ipRestrictionStore, passwordPolicyStore
		nil, me, nil, nil, nil,
		nil, nil, // privilegeEngine, roleStore
		nil, // tradingParamSetStore
		cfg,
	)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	tenantMW := middleware.TenantMiddleware([]string{testTenant})
	ts := httptest.NewServer(tenantMW(mux))
	t.Cleanup(ts.Close)
	return ts, pcStore
}

// submitPendingChange posts a pending change and returns the response.
func submitPendingChange(t *testing.T, ts *httptest.Server, body map[string]interface{}) *http.Response {
	t.Helper()
	return doJSON(t, ts, http.MethodPost, "/api/v1/securities/pending-changes", body)
}

func validPendingChangePayload(submittedBy string) map[string]interface{} {
	return map[string]interface{}{
		"entity_type":  "INSTRUMENT",
		"entity_id":    "inst-123",
		"change_type":  "UPDATE",
		"payload":      map[string]interface{}{"lot_size": 200},
		"submitted_by": submittedBy,
	}
}

// ============================================================
// TestSubmitPendingChange
// ============================================================

func TestSubmitPendingChange_Success(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-1"))
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	decodeBody(t, resp, &result)

	if result["id"] == nil || result["id"].(string) == "" {
		t.Error("expected non-empty id in response")
	}
	if result["status"] != "PENDING_APPROVAL" {
		t.Errorf("expected status PENDING_APPROVAL, got %v", result["status"])
	}
	if result["entity_type"] != "INSTRUMENT" {
		t.Errorf("expected entity_type INSTRUMENT, got %v", result["entity_type"])
	}
	if result["submitted_by"] != "maker-1" {
		t.Errorf("expected submitted_by maker-1, got %v", result["submitted_by"])
	}
}

func TestSubmitPendingChange_MissingEntityType(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	body := map[string]interface{}{
		// entity_type deliberately omitted
		"change_type": "UPDATE",
		"payload":     map[string]interface{}{"field": "val"},
	}
	resp := submitPendingChange(t, ts, body)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "MISSING_FIELD" {
		t.Errorf("expected code MISSING_FIELD, got %v", errBlock["code"])
	}
}

func TestSubmitPendingChange_MissingPayload(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	body := map[string]interface{}{
		"entity_type": "INSTRUMENT",
		"change_type": "UPDATE",
		// payload deliberately omitted
	}
	resp := submitPendingChange(t, ts, body)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "MISSING_FIELD" {
		t.Errorf("expected code MISSING_FIELD, got %v", errBlock["code"])
	}
}

func TestSubmitPendingChange_InvalidChangeType(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	body := map[string]interface{}{
		"entity_type": "INSTRUMENT",
		"change_type": "INVALIDOP",
		"payload":     map[string]interface{}{"field": "val"},
	}
	resp := submitPendingChange(t, ts, body)
	assertStatus(t, resp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, resp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "INVALID_FIELD" {
		t.Errorf("expected code INVALID_FIELD, got %v", errBlock["code"])
	}
}

// ============================================================
// TestApprovePendingChange
// ============================================================

func TestApprovePendingChange_Success(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	// Submit a change as maker-1.
	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-1"))
	assertStatus(t, resp, http.StatusCreated)

	var created map[string]interface{}
	decodeBody(t, resp, &created)
	id := created["id"].(string)

	// Approve as reviewer-1 (different from submitter).
	approveBody := map[string]interface{}{"reviewer_id": "reviewer-1"}
	appResp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/pending-changes/%s/approve", id),
		approveBody,
	)
	assertStatus(t, appResp, http.StatusOK)

	var approved map[string]interface{}
	decodeBody(t, appResp, &approved)
	if approved["status"] != "APPROVED" {
		t.Errorf("expected status APPROVED, got %v", approved["status"])
	}
	if approved["reviewed_by"] != "reviewer-1" {
		t.Errorf("expected reviewed_by reviewer-1, got %v", approved["reviewed_by"])
	}
}

func TestApprovePendingChange_SameUser(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	// Submit as maker-1.
	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-1"))
	assertStatus(t, resp, http.StatusCreated)

	var created map[string]interface{}
	decodeBody(t, resp, &created)
	id := created["id"].(string)

	// Attempt to approve with the same user (four-eyes violation).
	approveBody := map[string]interface{}{"reviewer_id": "maker-1"}
	appResp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/pending-changes/%s/approve", id),
		approveBody,
	)
	assertStatus(t, appResp, http.StatusForbidden)

	var errResp map[string]interface{}
	decodeBody(t, appResp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "FORBIDDEN" {
		t.Errorf("expected code FORBIDDEN, got %v", errBlock["code"])
	}
}

func TestApprovePendingChange_NotFound(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	approveBody := map[string]interface{}{"reviewer_id": "reviewer-1"}
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/pending-changes/nonexistent-id/approve",
		approveBody,
	)
	assertStatus(t, resp, http.StatusNotFound)
}

// ============================================================
// TestRejectPendingChange
// ============================================================

func TestRejectPendingChange_Success(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-1"))
	assertStatus(t, resp, http.StatusCreated)

	var created map[string]interface{}
	decodeBody(t, resp, &created)
	id := created["id"].(string)

	rejectBody := map[string]interface{}{
		"reviewer_id": "reviewer-1",
		"comment":     "does not meet compliance requirements",
	}
	rejResp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/pending-changes/%s/reject", id),
		rejectBody,
	)
	assertStatus(t, rejResp, http.StatusOK)

	var rejected map[string]interface{}
	decodeBody(t, rejResp, &rejected)
	if rejected["status"] != "REJECTED" {
		t.Errorf("expected status REJECTED, got %v", rejected["status"])
	}
	if rejected["review_comment"] != "does not meet compliance requirements" {
		t.Errorf("expected review_comment set, got %v", rejected["review_comment"])
	}
	if rejected["reviewed_by"] != "reviewer-1" {
		t.Errorf("expected reviewed_by reviewer-1, got %v", rejected["reviewed_by"])
	}
}

func TestRejectPendingChange_NoComment(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-1"))
	assertStatus(t, resp, http.StatusCreated)

	var created map[string]interface{}
	decodeBody(t, resp, &created)
	id := created["id"].(string)

	// Reject without a comment — should be 400.
	rejectBody := map[string]interface{}{
		"reviewer_id": "reviewer-1",
		// comment deliberately omitted / empty
		"comment": "",
	}
	rejResp := doJSON(t, ts, http.MethodPost,
		fmt.Sprintf("/api/v1/securities/pending-changes/%s/reject", id),
		rejectBody,
	)
	assertStatus(t, rejResp, http.StatusBadRequest)

	var errResp map[string]interface{}
	decodeBody(t, rejResp, &errResp)
	errBlock := errResp["error"].(map[string]interface{})
	if errBlock["code"] != "MISSING_FIELD" {
		t.Errorf("expected code MISSING_FIELD, got %v", errBlock["code"])
	}
}

func TestRejectPendingChange_NotFound(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	rejectBody := map[string]interface{}{
		"reviewer_id": "reviewer-1",
		"comment":     "not found test",
	}
	resp := doJSON(t, ts, http.MethodPost,
		"/api/v1/securities/pending-changes/no-such-id/reject",
		rejectBody,
	)
	assertStatus(t, resp, http.StatusNotFound)
}

// ============================================================
// TestListPendingChanges
// ============================================================

func TestListPendingChanges_FilterByStatus(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	// Submit three changes.
	for i := 0; i < 3; i++ {
		body := map[string]interface{}{
			"entity_type":  "INSTRUMENT",
			"entity_id":    fmt.Sprintf("inst-%d", i),
			"change_type":  "UPDATE",
			"payload":      map[string]interface{}{"lot_size": 100},
			"submitted_by": fmt.Sprintf("maker-%d", i),
		}
		resp := submitPendingChange(t, ts, body)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	// List all — expect 3.
	listResp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/pending-changes", nil)
	assertStatus(t, listResp, http.StatusOK)

	var listResult map[string]interface{}
	decodeBody(t, listResp, &listResult)
	total := listResult["total"].(float64)
	if total != 3 {
		t.Errorf("expected total 3, got %v", total)
	}

	// Filter by PENDING_APPROVAL — all 3 should match.
	filteredResp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/pending-changes?status=PENDING_APPROVAL", nil)
	assertStatus(t, filteredResp, http.StatusOK)

	var filteredResult map[string]interface{}
	decodeBody(t, filteredResp, &filteredResult)
	filteredTotal := filteredResult["total"].(float64)
	if filteredTotal != 3 {
		t.Errorf("expected total 3 for PENDING_APPROVAL filter, got %v", filteredTotal)
	}

	// Filter by APPROVED — expect 0.
	approvedResp := doJSON(t, ts, http.MethodGet,
		"/api/v1/securities/pending-changes?status=APPROVED", nil)
	assertStatus(t, approvedResp, http.StatusOK)

	var approvedResult map[string]interface{}
	decodeBody(t, approvedResp, &approvedResult)
	approvedTotal := approvedResult["total"].(float64)
	if approvedTotal != 0 {
		t.Errorf("expected total 0 for APPROVED filter, got %v", approvedTotal)
	}

	data := listResult["data"].([]interface{})
	if len(data) != 3 {
		t.Errorf("expected data array length 3, got %d", len(data))
	}
}

func TestListPendingChanges_EmptyStore(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	resp := doJSON(t, ts, http.MethodGet, "/api/v1/securities/pending-changes", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	decodeBody(t, resp, &result)
	total := result["total"].(float64)
	if total != 0 {
		t.Errorf("expected total 0, got %v", total)
	}
	data := result["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty data array, got length %d", len(data))
	}
}

// ============================================================
// JSON serialisation sanity
// ============================================================

func TestSubmitPendingChange_ResponseShape(t *testing.T) {
	ts, _ := newPendingChangeTestServer(t)

	resp := submitPendingChange(t, ts, validPendingChangePayload("maker-shape"))
	assertStatus(t, resp, http.StatusCreated)

	// Decode into typed struct to verify field names match json tags.
	var raw json.RawMessage
	decodeBody(t, resp, &raw)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	for _, required := range []string{"id", "entity_type", "entity_id", "change_type", "payload", "submitted_by", "status", "submitted_at"} {
		if _, ok := fields[required]; !ok {
			t.Errorf("response missing required field %q", required)
		}
	}
}
