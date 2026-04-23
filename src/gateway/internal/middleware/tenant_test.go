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
