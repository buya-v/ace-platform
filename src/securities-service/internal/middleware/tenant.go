// Package middleware provides HTTP middleware for the securities-service.
package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	tenant "github.com/garudax-platform/tenant"
)

// TenantID is the canonical tenant identifier, re-exported from the shared
// tenant module so the whole platform agrees on one type and one context key.
type TenantID = tenant.TenantID

// defaultValidTenants is the fallback tenant list when VALID_TENANTS is not set.
const defaultValidTenants = "ace-commodities,mse-equities"

// TenantMiddleware returns the shared HTTP middleware that enforces the
// X-GarudaX-Tenant header on every request except the health/metrics bypass
// paths (/healthz, /readyz, /metrics). validTenants is validated against an
// O(1) lookup set built once at construction time:
//   - Missing header → 401 TENANT_REQUIRED
//   - Unknown tenant → 403 UNKNOWN_TENANT
//   - Otherwise the validated TenantID is stored in the request context.
func TenantMiddleware(validTenants []string) func(http.Handler) http.Handler {
	return tenant.TenantMiddleware(validTenants)
}

// WithTenant returns a new context with the given TenantID stored.
// Useful for injecting tenant context in tests.
func WithTenant(ctx context.Context, id TenantID) context.Context {
	return tenant.WithTenant(ctx, id)
}

// TenantFromContext extracts the TenantID from the context.
// Returns ("", false) when no tenant has been set (e.g. bypass paths).
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	return tenant.TenantFromContext(ctx)
}

// MustTenant extracts the TenantID from the context and panics if not present.
// Use only in handlers that are guaranteed to be behind TenantMiddleware.
func MustTenant(ctx context.Context) TenantID {
	return tenant.MustTenant(ctx)
}

// ValidTenantsFromEnv reads the VALID_TENANTS environment variable (comma-separated).
// Falls back to "ace-commodities,mse-equities" if the variable is unset or empty.
func ValidTenantsFromEnv() []string {
	raw := os.Getenv("VALID_TENANTS")
	if raw == "" {
		raw = defaultValidTenants
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
