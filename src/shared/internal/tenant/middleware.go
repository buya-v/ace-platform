package tenant

import (
	"encoding/json"
	"net/http"
)

// healthPaths is the set of paths that bypass tenant validation.
// These endpoints must be reachable without a tenant header for liveness
// probes, readiness probes, and Prometheus scraping.
var healthPaths = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// errorBody is the canonical error response shape for this service.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError writes a JSON error response with the given HTTP status code.
// It always sets Content-Type: application/json.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{
		Error: errorDetail{Code: code, Message: message},
	})
}

// TenantMiddleware returns an HTTP middleware that enforces tenant context on
// every request except the health/metrics bypass paths (/healthz, /readyz, /metrics).
//
// The middleware reads the X-GarudaX-Tenant header and validates the tenant_id
// against the provided whitelist. On success it calls WithTenant(ctx, id) and
// calls next. On failure it writes a JSON error response and does not call next.
//
// validTenants is converted to a map[string]bool for O(1) lookup so per-request
// cost is independent of the number of registered tenants.
//
// Error responses:
//   - Missing header → 401 TENANT_REQUIRED
//   - Unknown tenant → 403 UNKNOWN_TENANT
func TenantMiddleware(validTenants []string) func(http.Handler) http.Handler {
	// Build O(1) lookup map once at construction time.
	tenantMap := make(map[string]bool, len(validTenants))
	for _, t := range validTenants {
		tenantMap[t] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Bypass tenant enforcement for health / observability endpoints.
			if healthPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			header := r.Header.Get("X-GarudaX-Tenant")
			if header == "" {
				writeError(w, http.StatusUnauthorized,
					"TENANT_REQUIRED",
					"X-GarudaX-Tenant header is required",
				)
				return
			}

			// The header carries only the tenant_id in this simplified middleware.
			// Full HMAC validation (including timestamp) is handled by the
			// platform-layer gateway before traffic reaches individual services;
			// internal services receive a pre-verified tenant_id.
			tenantID := header

			if !tenantMap[tenantID] {
				writeError(w, http.StatusForbidden,
					"UNKNOWN_TENANT",
					"tenant '"+tenantID+"' is not registered",
				)
				return
			}

			// Inject validated tenant into the request context.
			ctx := WithTenant(r.Context(), TenantID(tenantID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
