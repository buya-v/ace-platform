// Package server — whitebox handler tests using httptest.
// Uses package server (not server_test) so that unexported handler methods
// and route registration are accessible without modifying production code.
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/garudax-platform/platform-service/internal/provisioning"
	"github.com/garudax-platform/platform-service/internal/store"
	"github.com/garudax-platform/platform-service/internal/types"
)

// newHandlerTestServer creates an httptest.Server whose mux is registered
// via the server's own registerRoutes method (unexported but accessible here).
func newHandlerTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	tenantStore := store.NewInMemoryTenantStore()
	p := provisioning.New(nil)
	srv := NewWithProvisioner(tenantStore, DefaultConfig(), p)
	srv.SetReady()

	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	return httptest.NewServer(mux)
}

// doJSON is a helper that sends a JSON request and returns the response.
func doJSON(t *testing.T, ts *httptest.Server, method, path string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, bodyReader)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http request %s %s: %v", method, path, err)
	}
	return resp
}

// decodeJSON is a helper that decodes the response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response JSON: %v", err)
	}
}

// --- Tests ---

// TestListTenants verifies GET /platform/v1/tenants returns 200 with both seeded tenants.
func TestListTenants(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/platform/v1/tenants", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var tenants []types.Tenant
	decodeJSON(t, resp, &tenants)

	if len(tenants) != 2 {
		t.Fatalf("got %d tenants, want 2", len(tenants))
	}

	ids := map[string]bool{}
	for _, tn := range tenants {
		ids[tn.ID] = true
	}
	for _, wantID := range []string{"ace-commodities", "mse-equities"} {
		if !ids[wantID] {
			t.Errorf("response missing tenant %q", wantID)
		}
	}
}

// TestGetTenant_Found verifies GET /platform/v1/tenants/ace-commodities returns 200.
func TestGetTenant_Found(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/platform/v1/tenants/ace-commodities", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var tn types.Tenant
	decodeJSON(t, resp, &tn)

	if tn.ID != "ace-commodities" {
		t.Errorf("ID = %q, want ace-commodities", tn.ID)
	}
}

// TestGetTenant_NotFound verifies GET /platform/v1/tenants/nonexistent returns 404.
func TestGetTenant_NotFound(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/platform/v1/tenants/nonexistent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestCreateTenant_Success verifies POST /platform/v1/tenants with a valid body returns 201.
func TestCreateTenant_Success(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"id":   "test-exchange",
		"name": "Test Exchange",
	}
	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var result map[string]interface{}
	decodeJSON(t, resp, &result)

	tenant, ok := result["tenant"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing 'tenant' key")
	}
	if tenant["id"] != "test-exchange" {
		t.Errorf("tenant.id = %q, want test-exchange", tenant["id"])
	}
	if tenant["status"] != types.TenantStatusOnboarding {
		t.Errorf("tenant.status = %q, want ONBOARDING", tenant["status"])
	}

	prov, ok := result["provisioning"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing 'provisioning' key")
	}
	if prov["status"] != "PROVISIONED" {
		t.Errorf("provisioning.status = %q, want PROVISIONED", prov["status"])
	}

	schemas, ok := prov["schemas_created"].([]interface{})
	if !ok || len(schemas) != 8 {
		t.Errorf("provisioning.schemas_created count = %d, want 8", len(schemas))
	}

	topics, ok := prov["topic_prefixes"].([]interface{})
	if !ok || len(topics) != 8 {
		t.Errorf("provisioning.topic_prefixes count = %d, want 8", len(topics))
	}
}

// TestCreateTenant_MissingName verifies POST without a name returns 400.
func TestCreateTenant_MissingName(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"id": "no-name-exchange",
	}
	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants", body)
	// Handler returns 400 VALIDATION_ERROR when name is missing.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestCreateTenant_InvalidSlug verifies POST with an invalid id slug returns 400.
func TestCreateTenant_InvalidSlug(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"id":   "Bad Slug!",
		"name": "Bad Slug Exchange",
	}
	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	var errResp types.ErrorResponse
	decodeJSON(t, resp, &errResp)
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", errResp.Error.Code)
	}
}

// TestCreateTenant_DuplicateID verifies POST with an existing ID returns 409.
func TestCreateTenant_DuplicateID(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"id":   "ace-commodities",
		"name": "Duplicate Exchange",
	}
	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants", body)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}

	var errResp types.ErrorResponse
	decodeJSON(t, resp, &errResp)
	if errResp.Error.Code != "TENANT_ALREADY_EXISTS" {
		t.Errorf("error code = %q, want TENANT_ALREADY_EXISTS", errResp.Error.Code)
	}
}

// TestUpdateTenantStatus_Activate verifies PUT /platform/v1/tenants/{id}/status with ACTIVE returns 200.
func TestUpdateTenantStatus_Activate(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	// mse-equities is seeded as ONBOARDING; promote it to ACTIVE.
	body := map[string]string{"status": "ACTIVE"}
	resp := doJSON(t, ts, http.MethodPut, "/platform/v1/tenants/mse-equities/status", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var tn types.Tenant
	decodeJSON(t, resp, &tn)
	if tn.Status != types.TenantStatusActive {
		t.Errorf("Status = %q, want ACTIVE", tn.Status)
	}
}

// TestUpdateTenantStatus_InvalidStatus verifies PUT with an unknown status value returns 400.
func TestUpdateTenantStatus_InvalidStatus(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]string{"status": "INVALID_STATUS_VALUE"}
	resp := doJSON(t, ts, http.MethodPut, "/platform/v1/tenants/ace-commodities/status", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	var errResp types.ErrorResponse
	decodeJSON(t, resp, &errResp)
	if errResp.Error.Code != "INVALID_STATUS" {
		t.Errorf("error code = %q, want INVALID_STATUS", errResp.Error.Code)
	}
}

// TestUpdateTenantStatus_NotFound verifies PUT on a non-existent tenant returns 404.
func TestUpdateTenantStatus_NotFound(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]string{"status": "ACTIVE"}
	resp := doJSON(t, ts, http.MethodPut, "/platform/v1/tenants/nonexistent/status", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestListTenants_ContentType verifies that list responses carry application/json.
func TestListTenants_ContentType(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/platform/v1/tenants", nil)
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestHealthEndpoints verifies /healthz returns 200 on the health mux.
func TestHealthEndpoints(t *testing.T) {
	tenantStore := store.NewInMemoryTenantStore()
	p := provisioning.New(nil)
	srv := NewWithProvisioner(tenantStore, DefaultConfig(), p)

	// Test healthz via a direct httptest request.
	wHealthz := httptest.NewRecorder()
	rHealthz := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.healthz(wHealthz, rHealthz)
	if wHealthz.Code != http.StatusOK {
		t.Errorf("healthz status = %d, want 200", wHealthz.Code)
	}

	// Test readyz when NOT ready.
	wNotReady := httptest.NewRecorder()
	rNotReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	srv.readyz(wNotReady, rNotReady)
	if wNotReady.Code != http.StatusServiceUnavailable {
		t.Errorf("readyz (not ready) status = %d, want 503", wNotReady.Code)
	}

	// Test readyz when ready.
	srv.SetReady()
	wReady := httptest.NewRecorder()
	rReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	srv.readyz(wReady, rReady)
	if wReady.Code != http.StatusOK {
		t.Errorf("readyz (ready) status = %d, want 200", wReady.Code)
	}
}
