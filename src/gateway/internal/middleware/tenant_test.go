package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTenantMiddleware_ValidTenant(t *testing.T) {
	mw := TenantMiddleware([]string{"ace-commodities", "mse-equities"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid, ok := TenantFromContext(r.Context())
		if !ok || tid != "ace-commodities" {
			t.Errorf("expected tenant ace-commodities in context, got %q ok=%v", tid, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "ace-commodities")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantMiddleware_MissingHeader(t *testing.T) {
	mw := TenantMiddleware([]string{"ace-commodities"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not have been called")
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenantMiddleware_UnknownTenant(t *testing.T) {
	mw := TenantMiddleware([]string{"ace-commodities"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not have been called")
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	req.Header.Set("X-GarudaX-Tenant", "unknown-tenant")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestTenantMiddleware_HealthBypass(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		mw := TenantMiddleware([]string{"ace-commodities"})
		called := false
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", path, nil)
		// No X-GarudaX-Tenant header — bypass should still allow through
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("path %s: expected 200, got %d", path, rr.Code)
		}
		if !called {
			t.Errorf("path %s: handler was not called", path)
		}
	}
}

// fakeRouteChecker reports a fixed set of known paths for the tenant
// middleware's pre-enforcement 404 guard.
type fakeRouteChecker struct {
	known map[string]bool
}

func (f fakeRouteChecker) RouteExists(path string) bool { return f.known[path] }

func TestTenantMiddleware_PlatformBypass(t *testing.T) {
	// Genuine platform-level prefixes must pass WITHOUT a tenant header.
	bypassPaths := []string{
		"/platform/v1/tenants",
		"/api/v1/platform/v1/tenants",
		"/api/v1/auth/login",
	}
	for _, path := range bypassPaths {
		mw := TenantMiddleware([]string{"ace-commodities"})
		called := false
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("POST", path, nil) // no X-GarudaX-Tenant header
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK || !called {
			t.Errorf("path %s: expected bypass (200, handler called), got %d called=%v",
				path, rr.Code, called)
		}
	}
}

func TestTenantMiddleware_TradingRoutesEnforced(t *testing.T) {
	// Previously-exempt business prefixes must now require the tenant header.
	enforcedPaths := []string{
		"/api/v1/orders",
		"/api/v1/clearing/positions",
		"/api/v1/margin",
		"/api/v1/settlement/cycles",
		"/api/v1/warehouse/inventory",
		"/api/v1/participants",
		"/api/v1/compliance/alerts",
		"/api/v1/market-data/candles",
		"/api/v1/securities/instruments",
		"/api/v1/admin/health",
	}
	for _, path := range enforcedPaths {
		mw := TenantMiddleware([]string{"ace-commodities"})
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("path %s: handler should not be called without a tenant header", path)
		}))
		req := httptest.NewRequest("GET", path, nil) // no tenant header
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("path %s: expected 401 TENANT_REQUIRED, got %d", path, rr.Code)
		}
	}
}

func TestTenantMiddleware_UnknownRouteReturns404(t *testing.T) {
	rc := fakeRouteChecker{known: map[string]bool{"/api/v1/orders": true}}
	mw := TenantMiddleware([]string{"ace-commodities"}, rc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for an unknown path")
	}))

	// Unknown path, no tenant header: should 404 (not 401) so unknown endpoints
	// are not leaked as tenant errors.
	req := httptest.NewRequest("GET", "/api/v1/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown path, got %d", rr.Code)
	}
}

func TestTenantMiddleware_KnownRouteStillEnforcedWithChecker(t *testing.T) {
	rc := fakeRouteChecker{known: map[string]bool{"/api/v1/orders": true}}
	mw := TenantMiddleware([]string{"ace-commodities"}, rc)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without a tenant header")
	}))

	// Known route, no tenant header: still 401 (route exists, header required).
	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for known route without tenant, got %d", rr.Code)
	}
}

func TestTenantFromContext_RoundTrip(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := req.Context()

	// Before: no tenant
	tid, ok := TenantFromContext(ctx)
	if ok || tid != "" {
		t.Errorf("expected no tenant in empty context, got %q ok=%v", tid, ok)
	}

	// Wire through middleware
	mw := TenantMiddleware([]string{"mse-equities"})
	var extracted TenantID
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		extracted, _ = TenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req.Header.Set("X-GarudaX-Tenant", "mse-equities")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if extracted != "mse-equities" {
		t.Errorf("expected mse-equities, got %q", extracted)
	}
}

func TestTenantID_String(t *testing.T) {
	tid := TenantID("ace-commodities")
	if tid.String() != "ace-commodities" {
		t.Errorf("String() = %q, want ace-commodities", tid.String())
	}
}
