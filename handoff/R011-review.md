APPROVED

# Review — R011: Make gateway tenant enforcement real

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The change closes the live P1 multi-tenancy defect exactly as the platform invariant and the repeatedly-documented learned pattern require.

- `tenantBypassPrefixes` is narrowed from ~18 entries (which swallowed nearly every business path: orders, clearing, margin, settlement, warehouse, participants, compliance, market-data, securities, admin, ws, tickets, bot, screening, risk-scores) down to genuine platform-level prefixes only: `/platform/`, `/api/v1/platform/`, `/api/v1/auth/`, plus the existing exact-match health paths. This matches the learned-pattern Action verbatim.
- Verified the middleware-chain ordering is unchanged and correct: `RequestID → Tracing → Metrics → BodyLimit → Tenant → Auth → RateLimit → Router` (`main.go:486-491`), so Tenant still runs before Auth and the resolved tenant is in context for all downstream handlers.
- Verified every referenced symbol exists and the code compiles against `main`: `RouteChecker`/`RouteExists` are defined in `auth.go` (same package) and implemented by `router.Router` (`router.go:58`); `tenant.HeaderName` (`= "X-GarudaX-Tenant"`), `TenantID.String()`, `WithTenant`, and `TenantFromContext` all exist in the shared module (`src/shared/pkg/tenant/`), wired via the relative `replace` directive in `src/gateway/go.mod:22`.
- The tenant propagation claim holds: `proxy/httpclient.go:176-177` forwards `req.Metadata` to backends as HTTP headers via `Header.Set`, so injecting under `TenantHeaderName` reaches downstream services as the canonical header. The `meta` map is built fresh per request from the validated context value (not copied from raw client headers), so there is no client-spoofing path around the whitelist.
- The pre-enforcement 404 guard is a genuinely necessary addition, not gold-plating: because Tenant now sits in front of Auth and rejects header-less requests with 401, an unknown path would otherwise return 401 TENANT_REQUIRED before Auth's existing 404 guard could fire — reintroducing the documented "auth-before-routing 401-vs-404" bug at the tenant layer. The guard is correctly ordered after the bypass checks and before the header/whitelist checks, so unknown paths 404 regardless of header presence while bypassed platform paths still pass through.

**Sound judgment call, correctly resolved:** the task parenthetical listed `/auth` among paths to enforce, but the worker kept `/api/v1/auth/` bypassed, citing (a) the platform directive ("Auth is platform-level; all other schemas are per-tenant"), (b) login/register/refresh occur before a tenant is resolved, and (c) the authoritative learned pattern explicitly lists `/api/v1/auth/` as a bypass to KEEP. This is the correct reading — enforcing tenant on login would deadlock the auth flow. The deviation is documented in the handoff. `/api/v1/admin/` is correctly NOT bypassed (admin actions are tenant-scoped).

### Security: PASS

This is the security fix. Handlers are no longer invoked with an unresolved tenant on business routes; 401 (missing header) and 403 (unknown tenant) are now enforced on the previously-exempt paths, satisfying the "Tenant ID is never optional" invariant. The variadic `RouteChecker` is backward-compatible and does not weaken any existing guard. No secrets, no injection surface, no auth bypass introduced. The 404 guard also prevents leaking endpoint existence as a tenant error.

### Code Quality: PASS

- Mirrors the existing `Auth` middleware's `RouteChecker` pattern, so the approach is idiomatic for this codebase.
- The variadic `routeChecker ...RouteChecker` is a deliberate backward-compat choice that keeps existing call sites and tests compiling unchanged; only `routeChecker[0]` is consumed, which is acceptable for an optional dependency.
- Comments thoroughly document the security rationale and the "do not re-add business prefixes here" warning, which guards against regression of exactly the defect being fixed.
- Diff is minimal and focused; pre-existing gofmt drift in unrelated lines was correctly left untouched.

### Test Coverage: PASS

Critical paths are covered with meaningful assertions:
- `TestTenantMiddleware_PlatformBypass` — 3 platform paths pass with no header.
- `TestTenantMiddleware_TradingRoutesEnforced` — 10 formerly-exempt business paths now return 401 (the regression guard for this exact defect).
- `TestTenantMiddleware_UnknownRouteReturns404` and `_KnownRouteStillEnforcedWithChecker` — distinguish the 404 (unknown path) vs 401 (known path, header required) behaviors.
- `TestForwardInjectsTenantMetadata` — exercises injection end-to-end through the real middleware for both generic forward and SubmitOrder, asserting the forwarded value equals the validated tenant.
- The 3 pre-existing `TestTenantMiddleware_*` tests that were FAILing on `main` now pass.
- e2e clients in both chokepoints send the default `ace-commodities` header so the full-stack suite behaves like a real single-tenant client rather than regressing to 401 on every authed call.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **User-to-tenant binding is still open (defer to R012).** The whitelist validates that the supplied tenant is *registered*, but nothing binds the authenticated JWT subject to the requested tenant — a user authenticated for `ace-commodities` could send `X-GarudaX-Tenant: mse-equities` and pass. This is genuine cross-tenant authorization and is correctly out of R011's stated scope (require + validate the header), but it should be the headline item for R012's cross-tenant isolation tests and backend-side gRPC interceptor enforcement.
2. **WebSocket regression (`/api/v1/ws/`).** Browsers cannot set custom headers on the WS handshake, so live web-ui streams will now fail tenant enforcement. The worker correctly refused to re-add `ws` to the bypass list (that would reopen the hole) and flagged a dedicated follow-up for query-param/subprotocol tenant resolution. Track this as a real task before WS streaming ships, not just a note.
3. **Demo-reset paths** (`/api/v1/admin/demo/`, `/api/v1/securities/demo/`) now require a header — confirm the demo-runner client sends one (R012/demo task).
4. Minor: the variadic accepts multiple `RouteChecker` values but silently uses only the first. A single explicit optional param via a constructor option, or a doc note that extras are ignored, would be marginally clearer. Non-blocking.
