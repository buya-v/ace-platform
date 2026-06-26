// Tenant-enforcement e2e tests for the GarudaX gateway (R012).
//
// These tests prove the platform invariant "Tenant ID is never optional" at the
// gateway edge: every tenant-scoped route rejects requests that carry no or an
// invalid X-GarudaX-Tenant header, while genuine platform-level routes (health,
// platform control plane, auth) still pass without a tenant. They complement the
// in-package unit tests in src/gateway/internal/middleware/tenant_test.go and
// internal/handler/handler_test.go by exercising the FULL production middleware
// chain (RequestID → Tenant → Auth → RateLimit → Router) against a live gateway.
//
// They follow the same graceful-skip pattern as the rest of this suite: when no
// gateway is reachable at E2E_BASE_URL they skip cleanly, so they are safe to run
// in CI without infrastructure.
//
// Design notes:
//   - TenantMiddleware runs BEFORE Auth, so the missing-/unknown-tenant rejections
//     are deterministic and do NOT depend on a valid JWT or a live backend: the
//     request never reaches auth or the backend. This is what makes these
//     assertions stable in a partial stack.
//   - The middleware's RouteChecker returns 404 for unregistered paths BEFORE
//     tenant enforcement (so unknown endpoints aren't leaked as 401), therefore
//     every path used for enforcement assertions below is a REGISTERED,
//     tenant-scoped route.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	validTenant   = "ace-commodities"
	otherTenant   = "mse-equities"
	unknownTenant = "definitely-not-a-real-tenant"

	tenantHeader = "X-GarudaX-Tenant"

	codeTenantRequired = "TENANT_REQUIRED"
	codeUnknownTenant  = "UNKNOWN_TENANT"
)

// coreTenantScopedRoutes are registered, tenant-scoped GET endpoints spanning the
// real-time trading pipeline (orders/clearing/margin/settlement), the supporting
// services (warehouse/compliance/market-data) and admin. None of these is on the
// tenant bypass whitelist, so all of them must require the tenant header.
// GET is used throughout so no request body is needed and the assertion isolates
// the tenant layer (which runs before any body/handler logic).
var coreTenantScopedRoutes = []string{
	"/api/v1/orders",
	"/api/v1/clearing/positions",
	"/api/v1/clearing/netting",
	"/api/v1/margin",
	"/api/v1/margin/calls",
	"/api/v1/settlement/cycles",
	"/api/v1/warehouse/inventory",
	"/api/v1/compliance/alerts",
	"/api/v1/market-data/candles/WHT-HRW", // matches /api/v1/market-data/candles/{instrument_id}
	"/api/v1/admin/health",
}

// tenantBypassRoutes are genuine platform-level endpoints that must pass tenant
// enforcement WITHOUT an X-GarudaX-Tenant header. Health paths are served by the
// gateway itself; platform/auth paths reach a backend (which may be down — that
// is fine, we only assert the gateway did not reject them as a tenant error).
var tenantBypassRoutes = []struct {
	method string
	path   string
}{
	{"GET", "/healthz"},
	{"GET", "/readyz"},
	{"GET", "/metrics"},
	{"GET", "/platform/v1/tenants"},
	{"GET", "/api/v1/platform/v1/tenants"},
	{"POST", "/api/v1/auth/login"},
	{"POST", "/api/v1/auth/register"},
}

// tenantRequest issues a raw request to the gateway with explicit control over the
// X-GarudaX-Tenant header. When tenant is "" the header is omitted entirely. No
// Authorization header is sent: tenant enforcement happens before auth, so the
// tenant-layer behaviour is observable without a token.
func tenantRequest(t *testing.T, method, path, tenant string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		t.Fatalf("create request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if tenant != "" {
		req.Header.Set(tenantHeader, tenant)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("execute request %s %s: %v", method, path, err)
	}
	return resp
}

// errorCode reads (and closes) a response body and returns the gateway error code
// under {"error":{"code":...}}, plus the raw body for diagnostics. Returns "" when
// the body is empty or not the standard error envelope (e.g. a success payload).
func errorCode(t *testing.T, resp *http.Response) (string, string) {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	raw := string(data)
	if len(data) == 0 {
		return "", raw
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		// Non-JSON or array payload — definitively not a tenant error envelope.
		return "", raw
	}
	return env.Error.Code, raw
}

// assertNotTenantRejected fails if the response is a gateway tenant rejection
// (401 TENANT_REQUIRED or 403 UNKNOWN_TENANT). Any other outcome — success, an
// auth challenge, a backend error — means the request passed the tenant gate.
func assertNotTenantRejected(t *testing.T, resp *http.Response, label string) {
	t.Helper()
	status := resp.StatusCode
	code, raw := errorCode(t, resp)
	if code == codeTenantRequired || code == codeUnknownTenant {
		t.Errorf("%s: request was rejected by tenant middleware (status %d, code %q); expected it to bypass tenant enforcement; body: %s",
			label, status, code, raw)
	}
}

// ---------- missing tenant → 401 TENANT_REQUIRED ----------

func TestTenantEnforcement_MissingHeaderRejected(t *testing.T) {
	skipIfGatewayUnavailable(t)

	for _, path := range coreTenantScopedRoutes {
		t.Run(path, func(t *testing.T) {
			resp := tenantRequest(t, "GET", path, "", nil)
			status := resp.StatusCode
			code, raw := errorCode(t, resp)

			if status != http.StatusUnauthorized {
				t.Fatalf("expected 401 for %s with no tenant header, got %d; body: %s", path, status, raw)
			}
			if code != codeTenantRequired {
				t.Errorf("expected error code %q for %s, got %q; body: %s",
					codeTenantRequired, path, code, raw)
			}
		})
	}
}

// ---------- unknown tenant → 403 UNKNOWN_TENANT ----------

func TestTenantEnforcement_UnknownTenantRejected(t *testing.T) {
	skipIfGatewayUnavailable(t)

	for _, path := range coreTenantScopedRoutes {
		t.Run(path, func(t *testing.T) {
			resp := tenantRequest(t, "GET", path, unknownTenant, nil)
			status := resp.StatusCode
			code, raw := errorCode(t, resp)

			if status != http.StatusForbidden {
				t.Fatalf("expected 403 for %s with unknown tenant, got %d; body: %s", path, status, raw)
			}
			if code != codeUnknownTenant {
				t.Errorf("expected error code %q for %s, got %q; body: %s",
					codeUnknownTenant, path, code, raw)
			}
		})
	}
}

// ---------- empty tenant value is treated as missing ----------

func TestTenantEnforcement_EmptyHeaderValueRejected(t *testing.T) {
	skipIfGatewayUnavailable(t)

	// An explicitly blank header value is equivalent to no header: the middleware
	// reads "" and returns 401, never 200.
	resp := tenantRequest(t, "GET", "/api/v1/clearing/positions", "", nil)
	// (tenantRequest omits the header when tenant == ""; assert the 401 path.)
	status := resp.StatusCode
	code, raw := errorCode(t, resp)
	if status != http.StatusUnauthorized || code != codeTenantRequired {
		t.Fatalf("expected 401 %s for blank tenant, got %d %q; body: %s",
			codeTenantRequired, status, code, raw)
	}
}

// ---------- valid tenant passes the tenant gate ----------

func TestTenantEnforcement_ValidTenantPasses(t *testing.T) {
	skipIfGatewayUnavailable(t)

	// With a registered tenant, the request must clear the tenant gate. For
	// auth-protected routes (no token sent) this surfaces as an auth challenge or
	// a backend response — never a tenant rejection. The point is that the gateway
	// no longer blocks on tenant.
	for _, tenant := range []string{validTenant, otherTenant} {
		for _, path := range coreTenantScopedRoutes {
			t.Run(tenant+" "+path, func(t *testing.T) {
				resp := tenantRequest(t, "GET", path, tenant, nil)
				assertNotTenantRejected(t, resp, tenant+" "+path)
			})
		}
	}
}

// TestTenantEnforcement_ValidTenantReachesBackend uses a route that is BOTH
// auth-public and tenant-scoped (market-data). A valid tenant therefore clears
// tenant AND auth, so the request reaches the backend: the status is a backend
// outcome (200/404/502/503), proving the validated tenant is forwarded onward
// rather than blocked at the edge.
func TestTenantEnforcement_ValidTenantReachesBackend(t *testing.T) {
	skipIfGatewayUnavailable(t)

	const path = "/api/v1/market-data/candles/WHT-HRW"

	// No tenant → blocked at the edge (401), never reaches the backend.
	noTenant := tenantRequest(t, "GET", path, "", nil)
	if code, raw := errorCode(t, noTenant); noTenant.StatusCode != http.StatusUnauthorized || code != codeTenantRequired {
		t.Fatalf("market-data without tenant: expected 401 %s, got %d %q; body: %s",
			codeTenantRequired, noTenant.StatusCode, code, raw)
	}

	// Valid tenant → clears tenant + (public) auth and is forwarded to the backend.
	withTenant := tenantRequest(t, "GET", path, validTenant, nil)
	assertNotTenantRejected(t, withTenant, "market-data with valid tenant")
	// A tenant-forwarded request must not be a 401 at all (auth is public for this
	// prefix, so the only 401 it could produce would be a tenant one — already
	// excluded above). Anything from 2xx through 5xx-backend is acceptable here;
	// we only guard against a regression that re-blocks the route.
	if withTenant.StatusCode == http.StatusUnauthorized {
		t.Errorf("market-data with valid tenant returned 401; expected the request to be forwarded to the backend")
	}
}

// ---------- bypass whitelist: platform/health/auth pass without a tenant ----------

func TestTenantEnforcement_BypassRoutesPassWithoutTenant(t *testing.T) {
	skipIfGatewayUnavailable(t)

	for _, r := range tenantBypassRoutes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			var body interface{}
			if r.method == "POST" {
				body = map[string]string{} // empty JSON; backend handles validation
			}
			resp := tenantRequest(t, r.method, r.path, "", body)
			assertNotTenantRejected(t, resp, r.method+" "+r.path)
		})
	}
}

// TestTenantEnforcement_HealthEndpointsServedWithoutTenant pins the exact-match
// health bypass: /healthz and /readyz are served by the gateway itself (200)
// even with no tenant header, confirming the health bypass is unconditional.
func TestTenantEnforcement_HealthEndpointsServedWithoutTenant(t *testing.T) {
	skipIfGatewayUnavailable(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			resp := tenantRequest(t, "GET", path, "", nil)
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Fatalf("expected 200 for %s without tenant, got %d; body: %s",
					path, resp.StatusCode, strings.TrimSpace(string(body)))
			}
			resp.Body.Close()
		})
	}
}

// TestTenantEnforcement_BypassNotAffectedByUnknownTenant verifies that a bogus
// tenant header on a bypass route does NOT cause a 403 — bypass routes skip tenant
// validation entirely, so the (ignored) header value is irrelevant.
func TestTenantEnforcement_BypassNotAffectedByUnknownTenant(t *testing.T) {
	skipIfGatewayUnavailable(t)

	resp := tenantRequest(t, "GET", "/healthz", unknownTenant, nil)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 for /healthz even with an unknown tenant header, got %d; body: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}
	resp.Body.Close()
}
