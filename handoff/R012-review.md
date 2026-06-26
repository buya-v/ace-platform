APPROVED

# Review — R012: Gateway tenant enforcement tests

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The test file asserts behavior that matches the actual R011 implementation, verified
against source rather than the handoff alone:

- **Status/code constants match `src/gateway/internal/middleware/tenant.go`.** Missing
  header → 401 `TENANT_REQUIRED` (tenant.go:131-136); unknown tenant → 403
  `UNKNOWN_TENANT` (tenant.go:139-144). The test constants (`codeTenantRequired`,
  `codeUnknownTenant`) and asserted statuses are exact.
- **Valid-tenant set matches the live wiring.** `main.go:472` constructs
  `TenantMiddleware([]string{"ace-commodities","mse-equities"}, rt)`, so the test's
  `validTenant`/`otherTenant` will genuinely clear the gate. If the worker had guessed
  a tenant not in the registry, `ValidTenantPasses` would fail — it won't.
- **Every enforcement path is a registered route.** This is the subtle correctness
  trap: the middleware consults `RouteChecker.RouteExists` and returns 404 *before*
  tenant enforcement (tenant.go:122-128), so a typo'd path would 404 and silently mask
  the 401/403 assertion. I cross-checked all 10 entries in `coreTenantScopedRoutes`
  against `routes.go` — every one is registered (orders, clearing/positions,
  clearing/netting, margin, margin/calls, settlement/cycles, warehouse/inventory,
  compliance/alerts, market-data/candles/{instrument_id}, admin/health). The
  market-data path with a parameter segment matches because `RouteExists` uses
  `matchPath` (router.go:58-65), which is method-agnostic and resolves `{param}`.
- **Bypass-route assertions are correctly weak.** `/platform/...`,
  `/api/v1/platform/...`, `/api/v1/auth/...` bypass via `tenantBypassPrefixes`;
  `/healthz`,`/readyz`,`/metrics` via exact-match `tenantHealthPaths`. The bypass tests
  only assert "not a tenant rejection," so a 404/502/503 from a down backend won't
  cause a false failure — the right call.
- **Tenant-before-auth ordering is exploited correctly.** Because Tenant runs before
  Auth, the missing/unknown assertions need no JWT and no live backend, which makes
  them deterministic in a partial stack. The `ValidTenantReachesBackend` test correctly
  distinguishes edge-block (401 with no tenant) from forwarding (valid tenant → backend
  outcome) behaviourally, since e2e cannot inspect the injected backend header (that is
  covered by R011's `TestForwardInjectsTenantMetadata` unit test).

### Security: PASS

This is a regression guard for a security-relevant invariant (the multi-tenancy
"Tenant ID is never optional" rule, whose violation was the live P1 defect across three
FAIL runs). It strengthens posture rather than introducing risk. No secrets, no
injection surface, no credentials. The `errorCode` helper safely handles array-vs-object
payloads (a `json.Unmarshal` into a struct of an array body returns "" → treated as
not-a-tenant-error), avoiding the array/object pitfall seen elsewhere in the suite. The
handoff documents an R009-style soundness proof (re-add the bug shape → suite fails on
the broken binary → restore), and the diff confirms **no `src/` changes are committed**
— only `tests/e2e/tenant_enforcement_test.go` and `handoff/R012.md`.

### Code Quality: PASS

- Follows the established graceful-skip pattern (`skipIfGatewayUnavailable`) and reuses
  the shared `baseURL` from `e2e_test.go`.
- **No identifier collisions.** Verified `e2e_test.go` (newClient/apiClient) and
  `lifecycle_test.go` (lc-prefixed, gatewayURL) do not declare `tenantRequest`,
  `errorCode`, `assertNotTenantRejected`, `validTenant`, `tenantHeader`,
  `codeTenantRequired`, `coreTenantScopedRoutes`, or `tenantBypassRoutes` — the package
  compiles.
- The self-contained `tenantRequest` helper (rather than extending `apiClient`) is
  justified: it must control header omission and send no `Authorization`, which the
  existing client cannot do. Imports are all used; comments are accurate and explain the
  non-obvious ordering/route-existence reasoning.

### Test Coverage: PASS

Eight tests cover the full enforcement matrix: missing→401 (×10 routes), unknown→403
(×10), valid passes (×10 ×2 tenants), valid forwarded to backend, bypass routes pass
without tenant, health served without tenant, and bypass unaffected by a bogus tenant.
Assertions are meaningful (status + error code), not "runs without error." These
complement, not duplicate, the R011 in-package unit tests by exercising the assembled
production chain against a real binary.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **`TestTenantEnforcement_EmptyHeaderValueRejected` is misnamed/redundant.** Despite
   the name and comment ("empty header value"), `tenantRequest` *omits* the header when
   `tenant == ""` (it never calls `req.Header.Set(tenantHeader, "")`). So this test
   exercises the same omitted-header path already covered by `MissingHeaderRejected`,
   not an explicitly-blank header value. Either set the blank header explicitly to test
   the distinct case, or rename to reflect that it asserts the missing-header path. (Go's
   `Header.Get` returns "" for both, so behaviour is identical — purely a clarity nit.)
2. **Wire into the R014 CI gate** (already noted in the handoff): point `E2E_BASE_URL` at
   the Compose gateway so these run for real, and report `PARTIAL` rather than `PASS`
   when they skip, per the e2e-canary pattern.
3. **Backend-side enforcement remains untested** (out of scope here): these prove the
   gateway *forwards* the validated tenant, not that backend gRPC/HTTP servers *require*
   it. Track as the R011 follow-up for backend interceptor tests.
