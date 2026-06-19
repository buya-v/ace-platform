// Package server — additional whitebox API tests for the Platform-admin control
// plane. These complement handlers_test.go by covering the tenant onboarding
// config endpoint, PATCH lifecycle updates, status-transition variants, and the
// method-not-allowed / malformed-input / store-failure error paths.
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/garudax-platform/platform-service/internal/config"
	"github.com/garudax-platform/platform-service/internal/provisioning"
	"github.com/garudax-platform/platform-service/internal/store"
	"github.com/garudax-platform/platform-service/internal/types"
)

// newServerWithConfigDir builds a test server whose ConfigLoader reads venue
// config from venuesDir. Used to exercise the tenant config endpoint without
// depending on the process working directory.
func newServerWithConfigDir(t *testing.T, venuesDir string) *Server {
	t.Helper()
	tenantStore := store.NewInMemoryTenantStore()
	p := provisioning.New(nil)
	cl := config.NewConfigLoader(venuesDir)
	srv := NewWithConfig(tenantStore, DefaultConfig(), p, cl)
	srv.SetReady()
	return srv
}

// writeVenueConfig writes a config.json for tenantID under venuesDir and returns
// the directory. Helper for the config-endpoint tests.
func writeVenueConfig(t *testing.T, venuesDir, tenantID string, cfg map[string]interface{}) {
	t.Helper()
	dir := filepath.Join(venuesDir, tenantID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir venue dir: %v", err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal venue config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write venue config: %v", err)
	}
}

// --- Tenant config endpoint (onboarding) ---

// TestTenantConfig_Found verifies GET /platform/v1/tenants/{id}/config returns the
// venue configuration with 200.
func TestTenantConfig_Found(t *testing.T) {
	venuesDir := t.TempDir()
	writeVenueConfig(t, venuesDir, "ace-commodities", map[string]interface{}{
		"tenant_id": "ace-commodities",
		"name":      "ACE Commodity Exchange",
		"settlement": map[string]interface{}{
			"default_cycle": "T+0",
		},
	})
	srv := newServerWithConfigDir(t, venuesDir)

	w := newRequest(t, srv, http.MethodGet, "/platform/v1/tenants/ace-commodities/config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg["tenant_id"] != "ace-commodities" {
		t.Errorf("config.tenant_id = %v, want ace-commodities", cfg["tenant_id"])
	}
	if cfg["name"] != "ACE Commodity Exchange" {
		t.Errorf("config.name = %v, want ACE Commodity Exchange", cfg["name"])
	}
}

// TestTenantConfig_NotFound verifies a config request for a tenant with no config
// file returns 404 CONFIG_NOT_FOUND.
func TestTenantConfig_NotFound(t *testing.T) {
	venuesDir := t.TempDir() // empty: no config files
	srv := newServerWithConfigDir(t, venuesDir)

	w := newRequest(t, srv, http.MethodGet, "/platform/v1/tenants/ghost-venue/config", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}

	var errResp types.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "CONFIG_NOT_FOUND" {
		t.Errorf("error code = %q, want CONFIG_NOT_FOUND", errResp.Error.Code)
	}
}

// TestTenantConfig_MethodNotAllowed verifies a non-GET to the config endpoint
// returns 405.
func TestTenantConfig_MethodNotAllowed(t *testing.T) {
	venuesDir := t.TempDir()
	writeVenueConfig(t, venuesDir, "ace-commodities", map[string]interface{}{"tenant_id": "ace-commodities"})
	srv := newServerWithConfigDir(t, venuesDir)

	w := newRequest(t, srv, http.MethodPost, "/platform/v1/tenants/ace-commodities/config", map[string]string{"x": "y"})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// --- PATCH tenant (lifecycle management) ---

// TestUpdateTenant_Success verifies PATCH /platform/v1/tenants/{id} updates the
// allowed fields (name, governance_tier) and returns the updated tenant.
func TestUpdateTenant_Success(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"name":            "ACE Renamed Exchange",
		"governance_tier": "PREMIUM",
	}
	resp := doJSON(t, ts, http.MethodPatch, "/platform/v1/tenants/ace-commodities", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var tn types.Tenant
	decodeJSON(t, resp, &tn)
	if tn.Name != "ACE Renamed Exchange" {
		t.Errorf("Name = %q, want ACE Renamed Exchange", tn.Name)
	}
	if tn.GovernanceTier != "PREMIUM" {
		t.Errorf("GovernanceTier = %q, want PREMIUM", tn.GovernanceTier)
	}
}

// TestUpdateTenant_IgnoresUnknownFields verifies PATCH only mutates name and
// governance_tier — fields like status or id must be ignored (status is changed
// only via the dedicated /status endpoint).
func TestUpdateTenant_IgnoresUnknownFields(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{
		"name":   "Renamed",
		"status": "DECOMMISSIONED", // must NOT take effect via PATCH
		"id":     "hacked-id",      // must NOT take effect
	}
	resp := doJSON(t, ts, http.MethodPatch, "/platform/v1/tenants/ace-commodities", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var tn types.Tenant
	decodeJSON(t, resp, &tn)
	if tn.ID != "ace-commodities" {
		t.Errorf("ID = %q, want ace-commodities (PATCH must not change id)", tn.ID)
	}
	if tn.Status != types.TenantStatusActive {
		t.Errorf("Status = %q, want ACTIVE (PATCH must not change status)", tn.Status)
	}
	if tn.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", tn.Name)
	}
}

// TestUpdateTenant_NotFound verifies PATCH on an unknown tenant returns 404.
func TestUpdateTenant_NotFound(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	body := map[string]interface{}{"name": "x"}
	resp := doJSON(t, ts, http.MethodPatch, "/platform/v1/tenants/nonexistent", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}

	var errResp types.ErrorResponse
	decodeJSON(t, resp, &errResp)
	if errResp.Error.Code != "TENANT_NOT_FOUND" {
		t.Errorf("error code = %q, want TENANT_NOT_FOUND", errResp.Error.Code)
	}
}

// TestUpdateTenant_InvalidJSON verifies a malformed PATCH body returns 400.
func TestUpdateTenant_InvalidJSON(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequestRaw(t, srv, http.MethodPatch, "/platform/v1/tenants/ace-commodities", "{not json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// --- Status transitions (lifecycle) ---

// TestUpdateTenantStatus_Transitions verifies every valid lifecycle status is
// accepted and persisted.
func TestUpdateTenantStatus_Transitions(t *testing.T) {
	for _, status := range []string{
		types.TenantStatusActive,
		types.TenantStatusSuspended,
		types.TenantStatusOnboarding,
		types.TenantStatusDecommissioned,
	} {
		status := status
		t.Run(status, func(t *testing.T) {
			ts := newHandlerTestServer(t)
			defer ts.Close()

			resp := doJSON(t, ts, http.MethodPut,
				"/platform/v1/tenants/ace-commodities/status",
				map[string]string{"status": status})
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 for transition to %s", resp.StatusCode, status)
			}
			var tn types.Tenant
			decodeJSON(t, resp, &tn)
			if tn.Status != status {
				t.Errorf("Status = %q, want %q", tn.Status, status)
			}
		})
	}
}

// TestUpdateTenantStatus_InvalidJSON verifies a malformed status body returns 400.
func TestUpdateTenantStatus_InvalidJSON(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequestRaw(t, srv, http.MethodPut, "/platform/v1/tenants/ace-commodities/status", "}{")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// TestUpdateTenantStatus_WrongMethod verifies a non-PUT to the /status endpoint
// returns 405.
func TestUpdateTenantStatus_WrongMethod(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequest(t, srv, http.MethodPost, "/platform/v1/tenants/ace-commodities/status",
		map[string]string{"status": "ACTIVE"})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// --- Onboarding / create variants ---

// TestCreateTenant_AppliesDefaults verifies a create with only id+name applies
// the documented defaults: status=ONBOARDING, governance_tier=STANDARD,
// flagship=false, onboarding_metadata={}.
func TestCreateTenant_AppliesDefaults(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants",
		map[string]interface{}{"id": "new-venue", "name": "New Venue"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var result map[string]interface{}
	decodeJSON(t, resp, &result)
	tenant := result["tenant"].(map[string]interface{})
	if tenant["status"] != types.TenantStatusOnboarding {
		t.Errorf("status = %v, want ONBOARDING", tenant["status"])
	}
	if tenant["governance_tier"] != "STANDARD" {
		t.Errorf("governance_tier = %v, want STANDARD", tenant["governance_tier"])
	}
	if tenant["flagship"] != false {
		t.Errorf("flagship = %v, want false", tenant["flagship"])
	}
	if md, ok := tenant["onboarding_metadata"].(map[string]interface{}); !ok || len(md) != 0 {
		t.Errorf("onboarding_metadata = %v, want empty object", tenant["onboarding_metadata"])
	}
}

// TestCreateTenant_HonoursProvidedFields verifies flagship, governance_tier, and
// onboarding_metadata supplied in the request are carried through.
func TestCreateTenant_HonoursProvidedFields(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants", map[string]interface{}{
		"id":                  "flagship-venue",
		"name":                "Flagship Venue",
		"flagship":            true,
		"governance_tier":     "FLAGSHIP",
		"onboarding_metadata": map[string]interface{}{"region": "MN"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var result map[string]interface{}
	decodeJSON(t, resp, &result)
	tenant := result["tenant"].(map[string]interface{})
	if tenant["flagship"] != true {
		t.Errorf("flagship = %v, want true", tenant["flagship"])
	}
	if tenant["governance_tier"] != "FLAGSHIP" {
		t.Errorf("governance_tier = %v, want FLAGSHIP", tenant["governance_tier"])
	}
	md, _ := tenant["onboarding_metadata"].(map[string]interface{})
	if md["region"] != "MN" {
		t.Errorf("onboarding_metadata.region = %v, want MN", md["region"])
	}
}

// TestCreateTenant_MissingID verifies create without an id returns 400.
func TestCreateTenant_MissingID(t *testing.T) {
	ts := newHandlerTestServer(t)
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/platform/v1/tenants",
		map[string]interface{}{"name": "No ID"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var errResp types.ErrorResponse
	decodeJSON(t, resp, &errResp)
	if errResp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("error code = %q, want VALIDATION_ERROR", errResp.Error.Code)
	}
}

// TestCreateTenant_InvalidJSON verifies a malformed create body returns 400.
func TestCreateTenant_InvalidJSON(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequestRaw(t, srv, http.MethodPost, "/platform/v1/tenants", "not-json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var errResp types.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("error code = %q, want INVALID_REQUEST", errResp.Error.Code)
	}
}

// --- Method-not-allowed on the tenant collection / item routes ---

// TestTenants_MethodNotAllowed verifies an unsupported method on the collection
// route returns 405.
func TestTenants_MethodNotAllowed(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequest(t, srv, http.MethodDelete, "/platform/v1/tenants", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// TestTenant_MethodNotAllowed verifies an unsupported method on the item route
// returns 405.
func TestTenant_MethodNotAllowed(t *testing.T) {
	srv := newServerOnly(t)
	w := newRequest(t, srv, http.MethodDelete, "/platform/v1/tenants/ace-commodities", nil)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

// --- Store-failure error paths (500) ---

// faultyStore implements store.TenantStore but fails List and the post-update
// Get, letting us exercise the handlers' 500 INTERNAL_ERROR branches.
type faultyStore struct {
	failList   bool
	failGet    bool
	failUpdate bool
}

func (f *faultyStore) List() ([]types.Tenant, error) {
	if f.failList {
		return nil, fmt.Errorf("boom: list failed")
	}
	return []types.Tenant{}, nil
}

func (f *faultyStore) Get(id string) (*types.Tenant, error) {
	if f.failGet {
		return nil, fmt.Errorf("boom: get failed")
	}
	return &types.Tenant{ID: id, Name: "ok", Status: types.TenantStatusActive}, nil
}

func (f *faultyStore) Create(t *types.Tenant) error { return nil }

func (f *faultyStore) Update(id string, updates map[string]interface{}) error {
	if f.failUpdate {
		return fmt.Errorf("boom: update failed")
	}
	return nil
}

func (f *faultyStore) UpdateStatus(id, status string) error { return nil }

// newServerWithStore builds a ready server backed by the given store.
func newServerWithStore(t *testing.T, st store.TenantStore) *Server {
	t.Helper()
	srv := NewWithProvisioner(st, DefaultConfig(), provisioning.New(nil))
	srv.SetReady()
	return srv
}

// TestListTenants_StoreError verifies a store List failure surfaces as 500.
func TestListTenants_StoreError(t *testing.T) {
	srv := newServerWithStore(t, &faultyStore{failList: true})
	w := newRequest(t, srv, http.MethodGet, "/platform/v1/tenants", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var errResp types.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("error code = %q, want INTERNAL_ERROR", errResp.Error.Code)
	}
}

// TestUpdateTenant_ReadBackError verifies that when Update succeeds but the
// subsequent Get fails, the handler returns 500 INTERNAL_ERROR.
func TestUpdateTenant_ReadBackError(t *testing.T) {
	srv := newServerWithStore(t, &faultyStore{failGet: true})
	w := newRequest(t, srv, http.MethodPatch, "/platform/v1/tenants/some-tenant",
		map[string]interface{}{"name": "x"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// TestUpdateTenantStatus_ReadBackError verifies the status handler returns 500
// when the post-update read-back fails.
func TestUpdateTenantStatus_ReadBackError(t *testing.T) {
	srv := newServerWithStore(t, &faultyStore{failGet: true})
	w := newRequest(t, srv, http.MethodPut, "/platform/v1/tenants/some-tenant/status",
		map[string]string{"status": types.TenantStatusActive})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// --- Shared request helpers (httptest.ResponseRecorder against the real mux) ---

// newServerOnly returns a ready in-memory-backed server (no httptest.Server).
func newServerOnly(t *testing.T) *Server {
	t.Helper()
	return newServerWithStore(t, store.NewInMemoryTenantStore())
}

// newRequest issues a request with an optional JSON body through the server's
// registered routes and returns the recorded response.
func newRequest(t *testing.T, srv *Server, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		raw = string(b)
	}
	return newRequestRaw(t, srv, method, path, raw)
}

// newRequestRaw issues a request with a raw (possibly malformed) string body
// through the server's registered routes and returns the recorded response.
func newRequestRaw(t *testing.T, srv *Server, method, path, rawBody string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	srv.registerRoutes(mux)

	req := httptest.NewRequest(method, path, strings.NewReader(rawBody))
	if rawBody != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
