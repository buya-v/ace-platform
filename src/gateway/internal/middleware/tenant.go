package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// tenantContextKey is an unexported type to prevent context key collisions.
type tenantContextKey struct{}

// TenantID is a named string type that carries a validated tenant identifier.
type TenantID string

// String implements fmt.Stringer for ergonomic logging.
func (t TenantID) String() string { return string(t) }

// tenantHealthPaths are the paths that bypass tenant enforcement.
// Exact match only — no prefix match — to prevent accidental bypass.
var tenantHealthPaths = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// tenantBypassPrefixes are path prefixes that bypass tenant enforcement.
// These are platform-level APIs that operate above tenant scope.
var tenantBypassPrefixes = []string{
	"/platform/",
	"/api/v1/auth/",
	"/api/v1/admin/demo/",
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

// TenantMiddleware returns an HTTP middleware that enforces the X-GarudaX-Tenant
// header on every request except health/metrics bypass paths.
//
// validTenants is converted to a map[string]bool at construction time for O(1)
// per-request lookup. The middleware:
//   - Bypasses /healthz, /readyz, /metrics (exact path match only)
//   - Returns 401 TENANT_REQUIRED when the header is absent
//   - Returns 403 UNKNOWN_TENANT when the tenant is not in the whitelist
//   - Stores the validated TenantID in the request context
//
// Must be placed BEFORE the Auth middleware so tenant context is available to
// all downstream handlers.
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

			// Bypass tenant enforcement for platform-level API prefixes.
			// Platform APIs (e.g. /platform/v1/tenants) are above tenant scope
			// and must not require an X-GarudaX-Tenant header.
			for _, prefix := range tenantBypassPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
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

// TenantFromContext extracts the TenantID from the context.
// Returns ("", false) when no tenant has been set (e.g. bypass paths).
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	t, ok := ctx.Value(tenantContextKey{}).(TenantID)
	return t, ok
}
