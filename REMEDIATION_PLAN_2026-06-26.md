# GarudaX — Remediation Plan

**Date:** 2026-06-26
**Basis:** `CODEBASE_REVIEW_2026-06-26.md`
**Delivery vehicle:** the project's own AI pipeline (`./pipeline/run.sh`)
**Task graph:** `tasks-remediation.json` (17 tasks, schema-compatible with `tasks.json`)

---

## Strategic decisions (locked with the owner)

1. **Tenancy → re-scope honestly now, build isolation later.** Do the cheap correctness fixes (adopt the shared tenant library, remove the gateway bypass, forward `tenant_id` to backends) and **document the system truthfully as single-tenant-with-namespacing**. Defer real per-tenant row/query isolation, the `platform.tenants`-backed registry, the provisioner, and `mse_*` schemas until MSE onboarding actually starts. Those become an explicit "MSE prerequisites" backlog, not this plan.
2. **Project is production-bound.** Therefore **P0 financial-correctness bugs are blocking and sit at the top of the plan.** Nothing else ships until money math is provably correct.
3. **Execute via the existing pipeline.** Tasks are authored in the project schema and run with `--resume`, matching how the platform was built.

---

## Phases & sequencing

```
R0  Financial correctness   ──► R3 Integration  (blocks)
R1  Eventing & concurrency  ──► R3 Integration  (blocks)
R2  Tenancy honesty         ──► R3 Integration  (blocks via R011)
R3  Restore the signal
R4  Hygiene                 (independent, run anytime)
```

R0 is the critical path and must lead (everything downstream depends on the shared decimal type). R1, R2, and R4 can run in parallel streams alongside R0. R3 (fresh full-stack integration run) is the gate that closes the plan and must come last.

### Phase R0 — Financial correctness (P0, blocking)
The single most important work. Removes float money math and converges the six divergent `Decimal` copies onto one audited type with checked overflow and defined rounding.

| ID | Role | Task | Blocked by |
|----|------|------|-----------|
| R001 | architect | Spec a shared `pkg/types/decimal` (checked int64/int128, banker's rounding, explicit overflow + divide-by-zero errors) | — |
| R002 | builder | Implement the shared decimal module + unit tests | R001 |
| R003 | builder | Migrate all 6 engine `Decimal` copies onto the shared module via `go.mod replace`; delete the divergent copies | R002 |
| R004 | builder | Replace `securities-service` `float64` money math with the shared decimal type | R002 |
| R005 | test_writer | Reconciliation + property tests: overflow on large notionals, rounding direction, cross-service value reconciliation | R003, R004 |
| R006 | architect | Money-path audit — confirm no residual `float64` or silent truncation remains in any settlement/P&L path | R003, R004 |

### Phase R1 — Eventing & concurrency correctness (P0/P1)
Stops silent event loss and fixes the deadlock/race bugs.

| ID | Role | Task | Blocked by |
|----|------|------|-----------|
| R007 | builder | Make Kafka wiring **fail-fast** when `KAFKA_BROKERS` is unset in non-test mode; restrict the in-process channel adapter to tests only | — |
| R008 | builder | Fix callback-under-lock (clearing + margin engines), the settlement instrument-map data race, and unsynchronized handler fields | — |
| R009 | test_writer | `go test -race` coverage for the engines + concurrency regression tests | R008 |

### Phase R2 — Tenancy honesty (re-scope + cheap enforcement)
Makes the API surface honest and the docs truthful; defers deep isolation.

| ID | Role | Task | Blocked by |
|----|------|------|-----------|
| R010 | builder | Wire the shared `tenant` module into services via `go.mod replace`; delete the orphaned duplicate `platform-control` registry | — |
| R011 | builder | Remove `tenantBypassPrefixes` for real endpoints in the gateway; forward `tenant_id` to gRPC backends in `BackendRequest.Metadata` | R010 |
| R012 | test_writer | Gateway tenant-enforcement tests (reject missing tenant on core trading endpoints) | R011 |
| R013 | docs | Re-scope docs: document as single-tenant-with-namespacing; add an explicit "MSE onboarding prerequisites" backlog; fix drift (service count, roadmap status, phantom `clearing-service`, `garudax/` root name) | R011 |

### Phase R3 — Restore the signal (production-bound)
Re-establishes trust in the build. Must run after R0/R1/R2.

| ID | Role | Task | Blocked by |
|----|------|------|-----------|
| R014 | devops | Re-enable CI: convert `.github/workflows/deploy.yml.disabled` into a build + unit-test gate on merge | — |
| R015 | test_writer | Fresh full-stack integration run (Docker Compose + real Kafka); confirm the 6 historical e2e bugs actually pass against current code | R003, R004, R007, R011 |
| R016 | docs | Update `CLAUDE.md` learned-patterns metrics (or mark them historical); record true current test counts | R015 |

### Phase R4 — Hygiene (independent)

| ID | Role | Task | Blocked by |
|----|------|------|-----------|
| R017 | devops | Delete merged `worktree-agent-*` branches; commit/revert the 4 dirty files; finish or remove the 5 stub Terraform modules + V2–V5 migration stubs; reconcile `corporate-actions` (make it a runnable service or document it as a library) | — |

---

## Explicitly deferred (the "MSE prerequisites" backlog — NOT in this plan)

Per decision #1, these stay parked until MSE onboarding begins:
- Real per-tenant **row-level** `tenant_id` columns + `WHERE tenant_id` filtering across operational tables.
- `platform.tenants`-backed tenant registry (replace the in-memory store) and a **working provisioner** (schemas/IAM/Kafka-admin/Redis/dashboards), not the current `nil`-db dry-run.
- `mse_*` schemas and the equities settlement/corporate-actions/auction/short-sell domain build.
- Per-tenant IRSA roles and KMS CMKs.

Document these in the re-scope task (R013) so the next team inherits an honest gap list.

---

## How to run

This task graph is authored as `tasks-remediation.json`. Two ways to execute via the existing pipeline:

**Option A — merge + resume (recommended; runs exactly this graph):**
```bash
cd /home/vcp/ace-platform
# back up history, then merge the remediation tasks into the live graph
cp tasks.json tasks.backup.$(date -u +%Y%m%d).json
jq -s '.[0].tasks += .[1].tasks
       | .[0].last_updated = "2026-06-26T00:00:00Z"
       | .[0]' tasks.json tasks-remediation.json > tasks.merged.json \
  && mv tasks.merged.json tasks.json
# dry-run first to inspect, then execute pending tasks honoring blockedBy
DRY_RUN=true ./pipeline/run.sh --resume
./pipeline/run.sh --resume
```

**Option B — let the Planner regenerate from intent:**
```bash
./pipeline/run.sh "Remediate the platform per CODEBASE_REVIEW_2026-06-26.md and \
REMEDIATION_PLAN_2026-06-26.md: P0 first — extract one audited shared decimal type and \
remove all float money math; then fix Kafka fail-fast and engine concurrency; adopt the \
shared tenant library and remove the gateway bypass while re-scoping docs to single-tenant; \
re-enable CI and run a fresh full-stack integration; then repo hygiene. Defer real per-tenant \
isolation and the control plane to an MSE-prerequisites backlog."
```

Option A is preferred because it executes the precise, review-derived graph; the Planner in Option B may re-derive different tasks.

### Suggested execution order (if running phases manually)
1. R0 first as the canary (R001→R002), then fan out R003/R004 in parallel.
2. Launch R1 (R007, R008) and R4 (R017) as parallel streams alongside R0.
3. R2 (R010→R011→R012/R013) once R0 is stable.
4. R3 last: R014 anytime, but **R015 only after R003/R004/R007/R011 merge**, then R016.

Keep `MAX_PARALLEL=3` (the pipeline's validated optimum).

---

## Definition of done

- Zero `float64` in any money/settlement/P&L path; one shared decimal type with overflow + rounding tests green.
- `go test -race` clean on all engines.
- Gateway rejects core-endpoint requests with no tenant; `tenant_id` reaches backends.
- A **fresh** integration run (post-fix) reports PASS with real Kafka, replacing the stale 2026-03-29 FAIL.
- CI is on and gating merges.
- Docs match the tree; the deferred multi-tenant work is written down as an explicit backlog.
- Stale branches and stub modules cleaned or finished.
