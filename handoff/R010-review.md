APPROVED

# Review — R010: Adopt shared tenant module

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The task — promote the shared tenant primitives into a standalone zero-dependency
module and have services adopt them via a filesystem `replace` — is implemented
correctly and the refactor is self-consistent. I verified each potential break point
rather than trusting the handoff:

- **New module is well-formed.** `src/shared/pkg/tenant/` becomes
  `module github.com/garudax-platform/tenant` (`go 1.22`, stdlib-only), receiving
  `context.go`, `middleware.go`, `observability.go` and their two test files via
  `git rename` (100%/98%/99% similarity). This mirrors the validated decimal-module
  precedent exactly.
- **The alias shim is complete for its consumers.** `internal/tenant/alias.go`
  re-exports `TenantID` (type alias → shared context key is identical, not merely
  equal), `HeaderName`, `WithTenant`, `TenantFromContext`, `MustTenant`,
  `TenantMiddleware`, and a local `buildTenantSet`. I checked every symbol the
  surviving `grpc.go` needs (`buildTenantSet`, `WithTenant`, `TenantFromContext`,
  `TenantID`) — all present. The type-alias choice means the kafka router, gRPC
  interceptors, gateway, and securities all read/write one and the same context value.
- **No orphaned references to the moved observability helpers.** `observability.go`
  exports `TenantLogger`/`TenantMetricLabels`, which `alias.go` does NOT re-export.
  This is safe: a repo-wide grep shows those two functions are only called from the
  test file that moved alongside them. The four `internal/observability/*.go` files
  that import `internal/tenant` use only `TenantFromContext` (re-exported). No break.
- **kafka/tenant_router.go** uses only `TenantID`, `MustTenant`, `WithTenant`,
  `TenantFromContext` — all re-exported. Zero edits required, as claimed.
- **Securities-service** middleware is reduced to a thin shim delegating to the
  shared package; its public API (`TenantMiddleware`, `WithTenant`, `TenantFromContext`,
  `MustTenant`, `TenantID`, `ValidTenantsFromEnv`) is preserved, so its ~60 call sites
  and `tenant_test.go` compile unchanged.
- **platform-control deletion is in-scope and sanctioned, not scope creep.** This was
  the highest-risk part of the diff (deleting ~1500 lines of tested code). I confirmed
  against `CODEBASE_REVIEW_2026-06-26.md:43` ("Two competing, orphaned control
  planes… `platform-control`… **Dead duplicate**") and the remediation plan P1 item #5
  ("delete the duplicate `platform-control` registry"). `platform-service` is the live
  registry the gateway proxies to (`docker-compose.yml:432`, `PLATFORM_SERVICE_ADDR`)
  and retains its full implementation. The deletion is correct.

The three failing gateway tests (`TestTenantMiddleware_ValidTenant/_MissingHeader/
_UnknownTenant`) are NOT a regression: all three exercise `/api/v1/orders`, which is in
`tenantBypassPrefixes` on `main`, so the middleware bypasses enforcement and returns 200.
This is the pre-existing R011 defect; the worker correctly left the bypass list untouched
and verified the same three tests fail identically on committed HEAD.

### Security: PASS

- R010 relocates primitives only; it does not weaken tenant enforcement. The shared
  module preserves the exact 401 `TENANT_REQUIRED` / 403 `UNKNOWN_TENANT` semantics.
- Unifying the gateway onto the shared context key removes a latent footgun (the
  gateway previously stored tenant under a private key; now domain logic and
  observability can resolve the same value).
- Deleting the dead `platform-control` registry reduces attack surface.
- New module is zero-dependency stdlib — no new supply-chain exposure, no `go.sum`.
- No secrets, no injection, no auth bypass introduced. The platform multi-tenancy
  invariant is advanced (consolidated primitives), and the residual enforcement gap is
  explicitly and correctly deferred to R011/R012.

### Code Quality: PASS

- Follows the established decimal-module convention precisely (standalone `go 1.22`
  zero-dep module under `src/shared/pkg/...`, consumed via relative `replace`).
- `alias.go` is clearly documented and explains why the gRPC interceptors stay behind
  in the `shared` module (grpc dependency).
- Migrations done as true renames keep history and minimize diff noise.
- The gateway change is minimal and surgical — only the duplicated primitives are
  swapped; gateway-specific routing policy (`tenantBypassPrefixes`, CORS) is left for
  R011, which is the right call.

### Test Coverage: PASS

- This is a behavior-preserving extraction; the relocated tests
  (`middleware_test.go`, `tenant_context_test.go`) move with the code and continue to
  exercise it under the new module path. `grpc_test.go` stays and validates the gRPC
  path against the aliased symbols. Securities and gateway middleware retain their
  existing tenant tests, now exercising the shared implementation through shims.
- Critical paths (header missing/unknown/valid, health bypass, context round-trip,
  observability label injection, gRPC unary/stream enforcement) remain covered.
- No new tests were strictly required for a primitive relocation, and none regress.

## Required Fixes (if REJECTED)
None.

## Suggestions (non-blocking)
1. **Add a cross-package interop assertion.** A small test proving that a tenant set by
   the gateway/securities middleware is readable via `tenant.TenantFromContext` (i.e.
   the shared context key truly unifies the two) would lock in the central benefit of
   this refactor and guard against a future accidental re-introduction of a private key.
2. **go.mod hygiene.** In `src/gateway/go.mod` the new
   `github.com/garudax-platform/tenant v0.0.0` line sits inside the block of `// indirect`
   requirements without its own grouping; it is a direct dependency. A `go mod tidy`
   pass would tidy the grouping (cosmetic; builds fine as-is).
3. **Lingering `platform-control` references.** `tasks.json`, `tasks-remediation.json`,
   `docs/platform-architecture.md`, `GarudaX_Strategy_Directive.md`,
   `CODEBASE_REVIEW_2026-06-26.md`, and `REMEDIATION_PLAN_2026-06-26.md` still mention
   the now-deleted service. The worker correctly flagged these as out-of-scope
   (R013/R017 docs + hygiene); ensure those follow-ups scrub the references so the
   deletion doesn't leave dangling docs.
4. **Carry R011 next.** R010 is the enabler; the tenant invariant remains violated at
   the gateway until `tenantBypassPrefixes` is narrowed and the header is required on
   tenant-scoped routes. The three failing gateway tests already encode the target
   behavior. R011 should be prioritized (it is the sole cause of the FAIL integration
   verdict).
