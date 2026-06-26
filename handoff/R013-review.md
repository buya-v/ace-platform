# Review — R013: Re-scope tenancy docs to reality

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

This is a documentation-only task whose entire purpose is to make `README.md` and
`AiX_Project_Knowledge.md` describe reality instead of aspiration. The correct
test for this task is therefore: *are the new claims true against the code on
disk?* I verified each load-bearing factual claim independently — not just against
the prior docs — and all of them check out:

- **14 Go modules (13 services + `shared` lib).** `ls src/*/go.mod` returns exactly
  14 files (auth-service, clearing-engine, compliance-service, corporate-actions,
  fix-gateway, gateway, margin-engine, market-data-service, matching-engine,
  platform-service, securities-service, settlement-engine, shared,
  warehouse-service). The old "11 Go services" line was wrong; the new count is
  right and stated precisely.
- **Gateway enforcement (R011).** `src/gateway/internal/middleware/tenant.go`
  `tenantBypassPrefixes` = `/platform/`, `/api/v1/platform/`, `/api/v1/auth/`
  (exactly as the doc states), with health-path exact-match bypass and a 404
  route-check before tenant enforcement. `src/gateway/cmd/gateway/main.go:472`
  constructs `TenantMiddleware([]string{"ace-commodities","mse-equities"}, rt)` —
  matching the "static two-entry allow-list" claim. The doc does **not** overstate
  enforcement: it correctly says the gateway *validates + forwards* the header and
  that backends do not yet enforce it.
- **In-memory registry.** `src/platform-service/internal/store/store.go`
  `InMemoryTenantStore` is "seeded with the two known platform tenants" — the doc's
  "in-memory, seeded, not `platform.tenants`-backed" is accurate.
- **Schema-only / dry-run provisioner.** `provisioner.go` only runs
  `CREATE SCHEMA IF NOT EXISTS` and is dry-run when `db == nil`; no IAM/Kafka/Redis/
  dashboard provisioning. Matches the doc.
- **`mse_*` schemas absent.** No migration creates them; `mse_` appears only in a
  V33 comment and as computed names in provisioner tests. Matches.
- **Migration range V29–V31** and the `ace_*` renames (V30/V32/V33) match the files
  in `infrastructure/db/migrations/`.

The rewrite also correctly fixes secondary drift: phantom repo root name
(`garudax/` → `ace-platform/`), the Java/Kotlin → Go/TypeScript language stack,
roadmap statuses (0.6/0.7/9 now "In progress (partial)" with explicit done/not-done
splits), and the new **MSE Onboarding Prerequisites** gap list. No new inaccuracies
were introduced.

### Security: PASS

Docs-only change with no code, no secrets, no input surface. Net-positive for
security posture: the previous text asserted full multi-tenant data isolation as
fact, which is dangerous because it invites false confidence in an isolation
boundary that is not actually enforced at the row level. The rewrite explicitly
flags that row-level `tenant_id`/`WHERE` filtering and backend-side enforcement are
**not** built, and ties the live P1 gateway/backend tenancy gaps to the
prerequisites list — aligning the docs with the platform invariant rather than
papering over it.

### Code Quality: PASS

Edits follow the existing Markdown conventions (status tables, callout blockquotes,
✅/⬜ markers). The "target vs today" annotation pattern is applied consistently
across both files and is more useful than deletion would have been, since the
invariants remain the review contract. Scope discipline is correct: the worker
edited only the two named deliverables plus its own handoff, did **not** touch the
protected files (`CLAUDE.md`, `tasks.json`, `pipeline/`), and flagged the stale
`CLAUDE.md` "11 Go services" / `src/clearing-service` references for the PostMortem
step rather than silently editing out of scope. Handoff is thorough and cites the
exact files it verified against.

### Test Coverage: PASS (N/A)

No tests are applicable to a documentation rewrite. The appropriate verification —
cross-checking claims against source — was performed and passed (see Correctness).

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

- The worker's own handoff already flags it: `CLAUDE.md`'s "Current State" header
  still says "11 Go services" and several learned-pattern entries reference the
  removed `src/clearing-service` stub. Reconcile these to "14 Go modules" in the
  PostMortem step so the AI-memory file matches the now-corrected public docs.
- The new **MSE Onboarding Prerequisites** list overlaps with deferred backlog
  items (R012 backend enforcement, row-level `tenant_id`, `platform.tenants`
  registry). Consider cross-linking each prerequisite to its backlog task ID so the
  doc and `tasks.json` stay in sync as items land.
- Per R016: when metrics/test counts are next republished, keep the README service
  count (14) and the AiX language table current.
