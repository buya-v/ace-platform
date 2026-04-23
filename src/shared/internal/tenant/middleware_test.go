package tenant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/shared/internal/tenant"
)

// captureHandler is an http.Handler that records the tenant extracted from
// context so tests can assert on it without coupling to request body parsing.
type captureHandler struct {
	tenantID tenant.TenantID
	ok       bool
}

func (h *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.tenantID, h.ok = tenant.TenantFromContext(r.Context())
	w.WriteHeader(http.StatusOK)
}

// validTenants used across all tests.
var validTenants = []string{"ace-commodities", "mse-equities"}

// newMiddlewareStack wires TenantMiddleware around a captureHandler and
// returns the handler and the capture pointer for assertions.
func newMiddlewareStack() (http.Handler, *captureHandler) {
	inner := &captureHandler{}
	mw := tenant.TenantMiddleware(validTenants)
	return mw(inner), inner
}

// parseErrorCode decodes a JSON error response and returns the "code" field.
func parseErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("parseErrorCode: failed to parse JSON body %q: %v", body, err)
	}
	return resp.Error.Code
}

// --- middleware tests ---

func TestTenantMiddleware_ValidTenant(t *testing.T) {
	handler, capture := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "ace-commodities")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !capture.ok {
		t.Fatal("TenantFromContext returned ok=false — tenant was not injected")
	}
	if capture.tenantID != "ace-commodities" {
		t.Fatalf("expected tenant ace-commodities, got %q", capture.tenantID)
	}
}

func TestTenantMiddleware_MissingHeader(t *testing.T) {
	handler, _ := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	// Intentionally no X-GarudaX-Tenant header.
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if code := parseErrorCode(t, rr.Body.Bytes()); code != "TENANT_REQUIRED" {
		t.Fatalf("expected error code TENANT_REQUIRED, got %q", code)
	}
}

func TestTenantMiddleware_UnknownTenant(t *testing.T) {
	handler, _ := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/api/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "rogue-exchange")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	if code := parseErrorCode(t, rr.Body.Bytes()); code != "UNKNOWN_TENANT" {
		t.Fatalf("expected error code UNKNOWN_TENANT, got %q", code)
	}
}

func TestTenantMiddleware_HealthBypass(t *testing.T) {
	handler, capture := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	// No X-GarudaX-Tenant header — must still reach the inner handler.
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Tenant should NOT be injected for bypassed paths.
	if capture.ok {
		t.Fatal("expected no tenant in context for /healthz bypass, but got one")
	}
}

func TestTenantMiddleware_ReadyzBypass(t *testing.T) {
	handler, _ := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /readyz bypass, got %d", rr.Code)
	}
}

func TestTenantMiddleware_MetricsBypass(t *testing.T) {
	handler, _ := newMiddlewareStack()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /metrics bypass, got %d", rr.Code)
	}
}

// --- context helpers tests ---

func TestTenantFromContext_RoundTrip(t *testing.T) {
	ctx := context.Background()
	ctx = tenant.WithTenant(ctx, tenant.TenantID("mse-equities"))

	id, ok := tenant.TenantFromContext(ctx)
	if !ok {
		t.Fatal("TenantFromContext returned ok=false after WithTenant")
	}
	if id != "mse-equities" {
		t.Fatalf("round-trip mismatch: expected mse-equities, got %q", id)
	}
}

func TestTenantFromContext_Missing(t *testing.T) {
	id, ok := tenant.TenantFromContext(context.Background())
	if ok {
		t.Fatal("TenantFromContext returned ok=true on empty context")
	}
	if id != "" {
		t.Fatalf("expected empty TenantID, got %q", id)
	}
}

func TestMustTenant_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustTenant did not panic on context without tenant")
		}
	}()
	tenant.MustTenant(context.Background())
}
