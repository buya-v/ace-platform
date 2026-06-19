package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer returns a seeded API mounted on an httptest server.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	api := NewAPI(NewSeededRegistry())
	api.SetReady()
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	return resp
}

func decodeTenant(t *testing.T, resp *http.Response) Tenant {
	t.Helper()
	defer resp.Body.Close()
	var tn Tenant
	if err := json.NewDecoder(resp.Body).Decode(&tn); err != nil {
		t.Fatalf("decode tenant: %v", err)
	}
	return tn
}

func TestHealthAndReady(t *testing.T) {
	api := NewAPI(NewSeededRegistry())
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	// Not ready until SetReady.
	resp := do(t, srv, http.MethodGet, "/readyz", nil)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("readyz before ready: want 503, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	api.SetReady()
	resp = do(t, srv, http.MethodGet, "/readyz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("readyz after ready: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = do(t, srv, http.MethodGet, "/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestListTenants(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out struct {
		Tenants []Tenant `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Tenants) != 2 {
		t.Fatalf("want 2 tenants, got %d", len(out.Tenants))
	}
}

func TestListTenants_StatusFilter(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants?status=ACTIVE", nil)
	defer resp.Body.Close()
	var out struct {
		Tenants []Tenant `json:"tenants"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Tenants) != 1 || out.Tenants[0].ID != "ace-commodities" {
		t.Fatalf("status filter wrong: %+v", out.Tenants)
	}
}

func TestListTenants_BadStatusFilter(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants?status=BOGUS", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for bad filter, got %d", resp.StatusCode)
	}
}

func TestGetTenant_FoundAndNotFound(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants/ace-commodities", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("found: want 200, got %d", resp.StatusCode)
	}
	tn := decodeTenant(t, resp)
	if tn.ID != "ace-commodities" {
		t.Errorf("wrong tenant: %s", tn.ID)
	}

	resp = do(t, srv, http.MethodGet, "/platform/v1/tenants/ghost", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("not found: want 404, got %d", resp.StatusCode)
	}
}

func TestCreateTenant_Success(t *testing.T) {
	srv := newTestServer(t)
	body := CreateTenantRequest{ID: "darkhan-meat", DisplayName: "Darkhan Meat Exchange"}
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
	tn := decodeTenant(t, resp)
	if tn.Status != StatusOnboarding {
		t.Errorf("new tenant should be ONBOARDING, got %s", tn.Status)
	}
}

func TestCreateTenant_ValidationAndConflict(t *testing.T) {
	srv := newTestServer(t)

	// Missing name -> 422.
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants", CreateTenantRequest{ID: "noname"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing name: want 422, got %d", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	resp.Body.Close()
	if er.Error.Code != "VALIDATION_ERROR" || len(er.Error.Details) == 0 {
		t.Errorf("want validation details, got %+v", er.Error)
	}

	// Bad slug -> 422.
	resp = do(t, srv, http.MethodPost, "/platform/v1/tenants", CreateTenantRequest{ID: "Bad Slug", DisplayName: "X"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad slug: want 422, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Duplicate -> 409.
	resp = do(t, srv, http.MethodPost, "/platform/v1/tenants", CreateTenantRequest{ID: "ace-commodities", DisplayName: "Dup"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCreateTenant_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/platform/v1/tenants", strings.NewReader("{not json"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for bad json, got %d", resp.StatusCode)
	}
}

func TestCreateTenant_FlagshipConflict(t *testing.T) {
	srv := newTestServer(t)
	body := CreateTenantRequest{ID: "another-flagship", DisplayName: "Another", Flagship: true}
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("flagship conflict: want 409, got %d", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Error.Code != "FLAGSHIP_CONFLICT" {
		t.Errorf("want FLAGSHIP_CONFLICT, got %s", er.Error.Code)
	}
}

func TestPatchTenant(t *testing.T) {
	srv := newTestServer(t)
	newName := "ACE Commodities (Renamed)"
	resp := do(t, srv, http.MethodPatch, "/platform/v1/tenants/ace-commodities",
		UpdateTenantRequest{DisplayName: &newName})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch: want 200, got %d", resp.StatusCode)
	}
	tn := decodeTenant(t, resp)
	if tn.DisplayName != newName {
		t.Errorf("name not updated: %s", tn.DisplayName)
	}
}

func TestPatchTenant_NotFound(t *testing.T) {
	srv := newTestServer(t)
	n := "X"
	resp := do(t, srv, http.MethodPatch, "/platform/v1/tenants/ghost", UpdateTenantRequest{DisplayName: &n})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestLifecycleActions_SuspendActivate(t *testing.T) {
	srv := newTestServer(t)

	// ace-commodities is ACTIVE -> suspend.
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants/ace-commodities/suspend",
		StatusChangeRequest{Actor: "ops", Reason: "maintenance"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("suspend: want 200, got %d", resp.StatusCode)
	}
	if tn := decodeTenant(t, resp); tn.Status != StatusSuspended {
		t.Fatalf("want SUSPENDED, got %s", tn.Status)
	}

	// Suspended -> activate.
	resp = do(t, srv, http.MethodPost, "/platform/v1/tenants/ace-commodities/activate", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activate: want 200, got %d", resp.StatusCode)
	}
	if tn := decodeTenant(t, resp); tn.Status != StatusActive {
		t.Fatalf("want ACTIVE, got %s", tn.Status)
	}
}

func TestLifecycleAction_IllegalTransition(t *testing.T) {
	srv := newTestServer(t)
	// mse-equities is ONBOARDING; suspend is illegal (must activate first).
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants/mse-equities/suspend", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("illegal transition: want 409, got %d", resp.StatusCode)
	}
	var er ErrorResponse
	json.NewDecoder(resp.Body).Decode(&er)
	if er.Error.Code != "INVALID_TRANSITION" {
		t.Errorf("want INVALID_TRANSITION, got %s", er.Error.Code)
	}
}

func TestStatusEndpoint_Put(t *testing.T) {
	srv := newTestServer(t)
	// mse-equities ONBOARDING -> ACTIVE via generic status endpoint.
	resp := do(t, srv, http.MethodPut, "/platform/v1/tenants/mse-equities/status",
		StatusChangeRequest{Status: "active", Actor: "admin"}) // lowercase normalised
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status put: want 200, got %d", resp.StatusCode)
	}
	if tn := decodeTenant(t, resp); tn.Status != StatusActive {
		t.Fatalf("want ACTIVE, got %s", tn.Status)
	}
}

func TestStatusEndpoint_InvalidTarget(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodPut, "/platform/v1/tenants/mse-equities/status",
		StatusChangeRequest{Status: "BOGUS"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for bogus status, got %d", resp.StatusCode)
	}
}

func TestDecommissionAndAudit(t *testing.T) {
	srv := newTestServer(t)
	// Decommission ace-commodities (ACTIVE -> DECOMMISSIONED).
	resp := do(t, srv, http.MethodPost, "/platform/v1/tenants/ace-commodities/decommission",
		StatusChangeRequest{Actor: "admin", Reason: "sunset"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decommission: want 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Audit should show the decommission entry.
	resp = do(t, srv, http.MethodGet, "/platform/v1/tenants/ace-commodities/audit", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit: want 200, got %d", resp.StatusCode)
	}
	var out struct {
		Audit []AuditEntry `json:"audit"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.Audit) == 0 {
		t.Fatal("expected audit entries")
	}
	last := out.Audit[len(out.Audit)-1]
	if last.Action != "tenant.decommissioned" || last.Actor != "admin" {
		t.Errorf("wrong final audit entry: %+v", last)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodDelete, "/platform/v1/tenants/ace-commodities", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", resp.StatusCode)
	}
}

func TestUnknownSubResource(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants/ace-commodities/bogus", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for unknown sub-resource, got %d", resp.StatusCode)
	}
}

func TestItem_EmptyID(t *testing.T) {
	srv := newTestServer(t)
	// Trailing slash with no id should 404.
	resp := do(t, srv, http.MethodGet, "/platform/v1/tenants/", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for empty id, got %d", resp.StatusCode)
	}
}

func TestValidationError_String(t *testing.T) {
	err := &ValidationError{Fields: []string{"id is required"}}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("Error() should include field messages, got %q", err.Error())
	}
}

func TestCollectionMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	resp := do(t, srv, http.MethodDelete, "/platform/v1/tenants", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", resp.StatusCode)
	}
}
