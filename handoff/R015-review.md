APPROVED

# Review — R015: Fresh full-stack integration run

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Evaluation

### Correctness: PASS

The task was to perform a *fresh* full-stack integration run with a real Kafka broker and a live gateway so e2e tests no longer graceful-skip. The worker did exactly this and — importantly — caught that every on-disk service image predated the R007–R022 remediation (built 2026-06-22). They rebuilt all 12 backend binaries from current source and re-ran, so the reported results reflect `main`, not stale artifacts. This is the right instinct and avoids a false-green run against old code.

The single code change is correct and I verified it against source:
- `src/gateway/internal/middleware/tenant.go` confirms `/api/v1/orders` is **not** in `tenantBypassPrefixes` (only `/platform/`, `/api/v1/platform/`, `/api/v1/auth/` + health). Post-R011 the tenant middleware runs ahead of the order handler, so the header-less raw request in `TestOrderNegativeCases` was rejected with 401 `TENANT_REQUIRED` before the body was parsed — the test asserts 400, so it was genuinely failing.
- The added `X-GarudaX-Tenant: ace-commodities` matches the value the shared client helper sets at `tests/e2e/e2e_test.go:72`, so the fix is consistent with the rest of the suite.

This is a genuine fix (it makes the case reach the intended JSON-decode 400 path), not a green-washing change. The worker is also commendably honest that bug class #3 (cross-service Kafka propagation) is **OPEN/unimplemented**, not fixed, and that `TestFullTradingLifecycle` passes only vacuously (instrument not seeded) — the verdict is appropriately nuanced rather than a blanket PASS.

### Security: PASS

No security regressions. The change adds a required tenant header to a test request. The run actively *validates* the platform tenant invariant (404 for unknown path, 401 for missing tenant, 403 for unknown tenant) on a live gateway, which strengthens confidence in the R011 enforcement. The worker correctly scoped the still-open cross-tenant-authz gap (JWT subject not bound to requested tenant) out of this task. No secrets, no injection, no auth bypass introduced.

### Code Quality: PASS

The change is minimal, well-commented, and explains *why* (R011 ordering) at the call site. The worker stayed strictly within the assigned scope (`tests/` + `handoff/` only) and verified that `src/`, `CLAUDE.md`, `tasks.json`, and `infrastructure/` were untouched — the diff confirms this. Rather than hacking repo source to work around the infra blockers, they used temp `/tmp` artifacts and filed concretely-scoped follow-up tasks (R024/R025/R026) with file:line evidence. This matches the project's "audit tasks must emit named follow-ups, not prose" pattern.

### Test Coverage: PASS

Appropriate for an integration-run task (no new feature surface to cover). The one test edit restores the *meaningfulness* of an existing negative assertion (it now exercises the JSON-decode 400 path instead of being short-circuited at the tenant layer). Coverage is reported with the documented dual methodology (65.0% statement-weighted / 69.5% business-logic-only), consistent with the ~66% baseline, and the race tier (all 4 engines clean under `-race`) is included.

I independently confirmed the three reported blockers are real, which gives high confidence in the rest of the report:
- Migrations are not zero-padded — `V10__`/`V11__` sort before `V2__exchange_schema.sql` lexicographically (R025).
- `src/matching-engine/go.mod` declares `replace github.com/garudax-platform/decimal => ../shared/pkg/types/decimal` while `docker-compose.yml` sets `context: ./src/matching-engine`, putting `../shared` outside the build context (R026).

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **R025/R026 are P1 and block any reproducible clean deploy** — a fresh DB cannot initialize and committed Dockerfiles cannot rebuild from source. These should be drained before further feature work; they are arguably higher urgency than the P2 R024. Recommend the PostMortem/Planner add them to the `deferred` backlog with P1 severity immediately.
2. **R024 (Kafka wire-up)** is the substantive open item: until `internal/kafka` is wired into engine `cmd/*/main.go`, `TestFullTradingLifecycle` cannot meaningfully assert cross-service propagation. Pair the wire-up with seeding the `WHEAT-2026-07` instrument (or aligning the test to the registered `WHT-HRW-2026M07-UB`) so the lifecycle stops passing vacuously.
3. Consider promoting the "rebuild images from source" step into the R014 CI gate — it would have caught the 06-22 image drift and the Dockerfile-context break automatically rather than at integration time.
