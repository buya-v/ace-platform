package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/garudax-platform/securities-service/internal/middleware"
)

// echoTenantHandler is a simple handler that writes the tenant ID (or "MISSING") to the response.
func echoTenantHandler(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.TenantFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("MISSING"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(tenant.String()))
}

func applyMiddleware(validTenants []string) http.Handler {
	mw := middleware.TenantMiddleware(validTenants)
	return mw(http.HandlerFunc(echoTenantHandler))
}

func TestTenantMiddleware_MissingHeader(t *testing.T) {
	handler := applyMiddleware([]string{"ace-commodities"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/securities/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestTenantMiddleware_UnknownTenant(t *testing.T) {
	handler := applyMiddleware([]string{"ace-commodities"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/securities/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "rogue-tenant")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTenantMiddleware_ValidTenant(t *testing.T) {
	handler := applyMiddleware([]string{"ace-commodities", "mse-equities"})

	for _, tenant := range []string{"ace-commodities", "mse-equities"} {
		t.Run(tenant, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/securities/orders", nil)
			req.Header.Set("X-GarudaX-Tenant", tenant)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
			if body := rec.Body.String(); body != tenant {
				t.Errorf("expected body %q, got %q", tenant, body)
			}
		})
	}
}

func TestTenantMiddleware_BypassHealthPaths(t *testing.T) {
	handler := applyMiddleware([]string{"ace-commodities"})

	// Health paths should bypass tenant enforcement — no header needed.
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			// Deliberately no X-GarudaX-Tenant header.
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			// echoTenantHandler returns 500 if no tenant in context, but bypass
			// paths skip middleware so the handler runs without tenant context.
			// We only care that we did NOT get 401 (tenant middleware rejection).
			if rec.Code == http.StatusUnauthorized {
				t.Errorf("health path %s should bypass tenant enforcement, got 401", path)
			}
		})
	}
}

func TestTenantFromContext_NotPresent(t *testing.T) {
	ctx := context.Background()
	tenant, ok := middleware.TenantFromContext(ctx)
	if ok {
		t.Errorf("expected ok=false for empty context, got tenant=%q", tenant)
	}
	if tenant != "" {
		t.Errorf("expected empty TenantID, got %q", tenant)
	}
}

func TestWithTenant(t *testing.T) {
	ctx := middleware.WithTenant(context.Background(), middleware.TenantID("ace-commodities"))
	tenant, ok := middleware.TenantFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true after WithTenant")
	}
	if tenant != "ace-commodities" {
		t.Errorf("expected ace-commodities, got %q", tenant)
	}
}

func TestMustTenant_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustTenant to panic on missing tenant, got nil")
		}
	}()
	middleware.MustTenant(context.Background())
}

func TestMustTenant_OK(t *testing.T) {
	ctx := middleware.WithTenant(context.Background(), middleware.TenantID("mse-equities"))
	tenant := middleware.MustTenant(ctx)
	if tenant != "mse-equities" {
		t.Errorf("expected mse-equities, got %q", tenant)
	}
}

func TestValidTenantsFromEnv_Default(t *testing.T) {
	// Unset the env var to test default.
	os.Unsetenv("VALID_TENANTS")
	tenants := middleware.ValidTenantsFromEnv()
	if len(tenants) == 0 {
		t.Fatal("expected default tenants, got empty slice")
	}
	// Default should include ace-commodities.
	found := false
	for _, t := range tenants {
		if t == "ace-commodities" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ace-commodities in default tenants, got %v", tenants)
	}
}

func TestValidTenantsFromEnv_Custom(t *testing.T) {
	t.Setenv("VALID_TENANTS", "tenant-a, tenant-b , tenant-c")
	tenants := middleware.ValidTenantsFromEnv()
	if len(tenants) != 3 {
		t.Fatalf("expected 3 tenants, got %d: %v", len(tenants), tenants)
	}
	want := []string{"tenant-a", "tenant-b", "tenant-c"}
	for i, w := range want {
		if tenants[i] != w {
			t.Errorf("tenants[%d]: want %q, got %q", i, w, tenants[i])
		}
	}
}

func TestTenantID_String(t *testing.T) {
	id := middleware.TenantID("ace-commodities")
	if id.String() != "ace-commodities" {
		t.Errorf("String(): want ace-commodities, got %q", id.String())
	}
}
