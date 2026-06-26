package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	tenant "github.com/garudax-platform/tenant"
)

// TenantID is the canonical tenant identifier, re-exported from the shared
// tenant module so the gateway, downstream services, and domain logic all agree
// on one type and one context key. Tenant context set here is readable via
// tenant.TenantFromContext / tenant.MustTenant anywhere in the platform.
type TenantID = tenant.TenantID

// TenantHeaderName is the canonical HTTP header that carries the tenant
// identifier, re-exported from the shared tenant module. Handlers forward the
// resolved tenant to backends under this header so downstream services read the
// same value the gateway validated.
const TenantHeaderName = tenant.HeaderName

// tenantHealthPaths are the paths that bypass tenant enforcement.
// Exact match only — no prefix match — to prevent accidental bypass.
var tenantHealthPaths = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// tenantBypassPrefixes are path prefixes that bypass tenant enforcement.
// These are GENUINELY platform-level APIs that operate above tenant scope: the
// platform control plane (tenant lifecycle) and authentication (platform-level,
// the user has not yet selected a tenant when logging in). Every other route —
// orders, clearing, margin, settlement, warehouse, participants, compliance,
// market-data, securities, admin — is tenant-scoped and MUST carry the
// X-GarudaX-Tenant header per the platform invariant ("Tenant ID is never
// optional"). Do not re-add business-path prefixes here.
var tenantBypassPrefixes = []string{
	"/platform/",
	"/api/v1/platform/",
	"/api/v1/auth/",
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
// header on every tenant-scoped request.
//
// validTenants is converted to a map[string]bool at construction time for O(1)
// per-request lookup. The middleware:
//   - Bypasses /healthz, /readyz, /metrics (exact path match only)
//   - Bypasses platform-level prefixes (/platform/, /api/v1/platform/, /api/v1/auth/)
//   - Returns 404 NOT_FOUND for paths matching no registered route (when a
//     RouteChecker is supplied), so unknown endpoints are not leaked as 401
//   - Returns 401 TENANT_REQUIRED when the header is absent
//   - Returns 403 UNKNOWN_TENANT when the tenant is not in the whitelist
//   - Stores the validated TenantID in the request context
//
// An optional RouteChecker may be supplied; when present it is consulted before
// tenant enforcement so requests to nonexistent paths receive 404 (matching the
// behaviour of the Auth middleware, which sits behind this one).
//
// Must be placed BEFORE the Auth middleware so tenant context is available to
// all downstream handlers.
func TenantMiddleware(validTenants []string, routeChecker ...RouteChecker) func(http.Handler) http.Handler {
	// Build O(1) lookup map once at construction time.
	tenantMap := make(map[string]bool, len(validTenants))
	for _, t := range validTenants {
		tenantMap[t] = true
	}

	var rc RouteChecker
	if len(routeChecker) > 0 {
		rc = routeChecker[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORS preflight requests always pass through — no tenant context needed.
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

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

			// Unknown paths (no registered route) get 404 before tenant
			// enforcement so a nonexistent endpoint is not reported as a tenant
			// error. This mirrors the Auth middleware's pre-routing 404 guard.
			if rc != nil && !rc.RouteExists(r.URL.Path) {
				writeTenantError(w, http.StatusNotFound,
					"NOT_FOUND",
					"Endpoint not found",
				)
				return
			}

			header := r.Header.Get(tenant.HeaderName)
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

			// Store validated tenant ID in request context using the shared
			// context key so downstream handlers and services resolve it.
			ctx := tenant.WithTenant(r.Context(), tenant.TenantID(header))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFromContext extracts the TenantID from the context.
// Returns ("", false) when no tenant has been set (e.g. bypass paths).
func TenantFromContext(ctx context.Context) (TenantID, bool) {
	return tenant.TenantFromContext(ctx)
}
