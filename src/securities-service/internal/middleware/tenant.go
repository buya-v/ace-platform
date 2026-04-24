// Package middleware provides HTTP middleware for the securities-service.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// tenantContextKey is an unexported type to prevent context key collisions.
type tenantContextKey struct{}

// TenantID is a named string type that carries a validated tenant identifier.
type TenantID string

// String implements fmt.Stringer for ergonomic logging.
func (t TenantID) String() string { return string(t) }

// defaultValidTenants is the fallback tenant list when VALID_TENANTS is not set.
const defaultValidTenants = "ace-commodities,mse-equities"

// tenantHealthPaths are the paths that bypass tenant enforcement (exact match).
var tenantHealthPaths = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// tenantErrorBody is the JSON error shape for tenant errors.
type tenantErrorBody struct {
	Error tenantErrorDetail `json:"error"`
}

type tenantErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeTenantError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(tenantErrorBody{
		Error: tenantErrorDetail{Code: code, Message: message},
	})
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

// TenantMiddleware returns an HTTP middleware that enforces the X-GarudaX-Tenant
// header on every request except health/metrics bypass paths.
//
// validTenants is converted to a map[string]bool at construction time for O(1)
// per-request lookup. The middleware:
//   - Bypasses /healthz, /readyz, /metrics (exact path match only)
//   - Returns 401 TENANT_REQUIRED when the header is absent
//   - Returns 403 UNKNOWN_TENANT when the tenant is not in the whitelist
//   - Stores the validated TenantID in the request context
func TenantMiddleware(validTenants []string) func(http.Handler) http.Handler {
	// Build O(1) lookup map once at construction time.
	tenantMap := make(map[string]bool, len(validTenants))
	for _, t := range validTenants {
		tenantMap[t] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bypass tenant enforcement for health / observability endpoints.
			if tenantHealthPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("X-GarudaX-Tenant")
			if header == "" {
				writeTenantError(w, http.StatusUnauthorized,
					"TENANT_REQUIRED",
					"X-GarudaX-Tenant header is required",
				)
				return
			}

			if !tenantMap[header] {
				writeTenantError(w, http.StatusForbidden,
					"UNKNOWN_TENANT",
					"tenant '"+header+"' is not registered",
				)
				return
			}

			// Store validated tenant ID in request context.
			ctx := context.WithValue(r.Context(), tenantContextKey{}, TenantID(header))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithTenant returns a new context with the given TenantID stored.
// Useful for injecting tenant context in tests.
func WithTenant(ctx context.Context, id TenantID) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, id)
}

// TenantFromContext extracts the TenantID from the context.
// Returns ("", false) when no tenant has been set (e.g. bypass paths).
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	t, ok := ctx.Value(tenantContextKey{}).(TenantID)
	return t, ok
}

// MustTenant extracts the TenantID from the context and panics if not present.
// Use only in handlers that are guaranteed to be behind TenantMiddleware.
func MustTenant(ctx context.Context) TenantID {
	t, ok := TenantFromContext(ctx)
	if !ok {
		panic("middleware: TenantID not present in context — ensure TenantMiddleware is wired")
	}
	return t
}
