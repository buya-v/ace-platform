# GarudaX Platform — Codebase Review

**Date:** 2026-06-26
**Reviewer:** Claude (static analysis — no Go/Node toolchain available on this host, so build/test claims could not be re-executed)
**Scope:** `/home/vcp/ace-platform` — 14 Go services, 3 React SPAs, infra, docs, AI pipeline
**Method:** 4 parallel investigation agents (multi-tenancy, Go quality/security, test/integration, frontend/infra/docs) + direct grep/read verification

---

## Executive Summary

GarudaX is a genuinely substantial, AI-pipeline-generated multi-tenant trading platform. The **engineering surface is real** — working SPAs, real Go services, clean DB migrations (V6+), a live nginx+TLS edge — far more than a demo. But three things undercut the headline claims:

1. **Multi-tenancy is a costume, not a system.** The directive demands `tenant_id` on every row/message/call, enforced everywhere. Reality: a single-tenant system (`ace-commodities`) hardcoded throughout, with enforcement bypassed at the gateway and absent in 15 of 17 services. The second tenant (`mse-equities`) does not physically exist.
2. **Financial correctness is at risk.** The flagship equities service does **all money math in `float64`**, and the six copied `Decimal` types have **diverged** with silent overflow and one-directional truncation rounding.
3. **The documentation is confidently stale.** `CLAUDE.md` presents precise metrics (1199 tests, 6 unfixed bugs, 66% coverage) that no longer match the tree. The bugs appear fixed; the counts are ~2–3× too low; and **no integration run has executed since 2026-03-29**, whose verdict was **FAIL**.

**Overall:** Impressive scaffold and breadth; not production-ready. Treat all metrics in `CLAUDE.md` as historical. Priorities below are ordered by risk to real money and real trust.

---

## 1. Multi-Tenancy — Partially Scaffolded, ~85% Aspirational

The multi-tenant *shape* exists (a clean shared package, a renamed schema namespace, a tenant-prefixed topic convention, a registry API) but it is single-tenant hardcoded to `ace-commodities`, enforced almost nowhere at runtime, and the onboarding control plane is an in-memory stub.

| Invariant (per directive) | Reality |
|---|---|
| Shared tenant package adopted by services | **0 / 17** |
| Services enforcing tenant on inbound | **2 / 17** (gateway — but nullified; securities-service — real) |
| Operational tables carrying `tenant_id` | ~10 audit tables only, hardcoded `DEFAULT 'ace-commodities'` |
| Operational queries filtering by `tenant_id` | **0** |
| `ace_*` schema rename matches code | ✅ **yes — the one solid deliverable** |
| `mse_*` (second tenant) schemas | **none exist** |
| Kafka tenant topic prefix | 9/9 services, but compile-time **hardcoded strings**; shared router used by 0 |
| Tenant registry backed by `platform.tenants` table | ❌ in-memory map only |
| Tenant provisioning (schemas/IAM/Kafka/Redis/dashboards) | dry-run stub; only schema-DDL coded, and it's disabled |

**Key evidence:**
- A genuine shared package exists — `src/shared/internal/tenant/{context,middleware,grpc}.go` with `TENANT_REQUIRED`/`UNKNOWN_TENANT` enforcement and a tenant-aware Kafka router (`src/shared/kafka/tenant_router.go`). **But it is a separate `go.mod` module with no `replace` wiring, so zero services import it.** The 2 services that do enforce tenancy use duplicate local copies.
- **The gateway neuters its own tenant middleware:** `src/gateway/internal/middleware/tenant.go:27-49` defines `tenantBypassPrefixes` that exempt essentially every real endpoint (`/api/v1/orders`, `/clearing/`, `/margin`, `/settlement/`, `/warehouse/`, `/participants`, `/compliance/`, `/market-data/`, `/securities/`, `/auth/`, `/admin/`). Core trading traffic is accepted with no tenant header — directly contradicting the directive's "reject any call without tenant context."
- Even when the gateway resolves a tenant, it does **not forward it** to gRPC backends (`src/gateway/internal/proxy/backend.go:17-24` carries user/request/roles, no `tenant_id`).
- Migrations are the strongest piece: `platform.tenants` + `platform.audit` exist (V29/V31), `ace_*` renames are real and idempotent (V30/V32/V33), and all 8 operational services query the new `ace_*` schemas with **zero old-name references**. But **no query filters by `tenant_id`** — isolation rests entirely on a namespace that only ever holds one tenant.
- **Two competing, orphaned control planes:** `platform-service` (gateway proxies to it) backs onto an in-memory map, inits the provisioner with `nil` db (dry-run); `platform-control` is a second, richer in-memory registry the gateway never routes to. Dead duplicate.

**Verdict:** A single-tenant system wearing a multi-tenant costume. Onboarding `mse-equities` as currently planned would surface most of this work as not-yet-done.

---

## 2. Financial Correctness — HIGH Risk

This is the most serious code-level category for a trading platform.

- **`securities-service` (the MSE equities flagship) does all money math in `float64`.** There is no `decimal.go` in the service. Position cost, market value, P&L, settlement cash, and bond accrual are floats:
  - `securities-service/internal/engine/engine.go:678-681` — `AvgCost`/`MarketValue`/`UnrealizedPnl` in float64 (same in `auction.go:346-363`)
  - `securities-service/internal/settlement/engine.go:79`, `equities.go:186` — `NetAmount = trade.Price * float64(trade.Quantity)`
  - `securities-service/internal/settlement/engine.go:207-220` — accrued bond interest `days * couponRate * parValue / 365.0`
  - It also crosses a type boundary: matching/clearing/settlement engines use int64 `Decimal`, securities uses `float64` → values will not reconcile.
- **The six copied `Decimal` types have diverged** (docs claim "identical Decimal(18,4)"). Different APIs and rounding per service: matching has `MulUint64`/`Sub` but **no `Add`**; warehouse has `Add`/`Sub` only (no `Mul`/`Div`/`Cmp`); margin/settlement add `MulDecimal`/`DivInt64`/`Max`; clearing adds `Negate`/`IsNeg`. Same conceptual type, six behaviors.
- **Silent overflow + truncation rounding bias:** `margin-engine/.../decimal.go:98` `MulDecimal` computes `(d.value * other.value) / decimalScale` where both operands are already ×10⁴ — the intermediate product **overflows int64 for large notionals and wraps silently** (no check, no panic). All division truncates toward zero with no rounding (`DivInt64`, `DivInt`, `ParseDecimal` beyond 4 digits) → systematic one-directional money bias that fails to reconcile to the cent over many trades. `DivInt/DivInt64` return zero on divide-by-zero, masking errors (e.g. VWAP with zero volume silently → 0).

**Fix direction:** Extract one audited `pkg/types/decimal.go` shared module (int128 or checked-int64 with banker's rounding and explicit overflow errors), delete the float path in securities-service, and adopt it across all engines. This is the right time for the "extract shared decimal" trigger the postmortems already flagged (5+ services duplicating).

---

## 3. Cross-Service Eventing — Default Wiring Doesn't Cross Processes

- The "channel-based Kafka" pattern silently falls back to **in-process Go channels** when `KAFKA_BROKERS` is unset: `src/*/internal/kafka/wiring.go` (e.g. `settlement-engine/.../wiring.go:20-31`). Each service's channels are private to its own process, so `matching → clearing → margin → settlement` events are **silently dropped** in any multi-process deployment without a broker. This is exactly why the e2e `TestFullTradingLifecycle` subtests failed.
- **Partially remediated:** a real `segmentio/kafka-go` writer now exists (`src/gateway/internal/kafka/kafka_writer.go`, commit T116, 2026-03-31), used when `KAFKA_BROKERS` is set. But the default path still falls back to channels, and this also **breaks the advertised "zero-dependency Go module" claim** (9 `go.mod` files now pull in `kafka-go`).

**Fix direction:** In any non-test deployment, fail fast if `KAFKA_BROKERS` is unset rather than silently degrading to in-process channels. Reserve the channel adapter strictly for unit tests.

---

## 4. Concurrency — MED Risk

- **Callback invoked while holding the engine mutex** (deadlock risk): `clearing-engine/internal/engine/engine.go:~110` calls `e.tradeHandler(...)` inside the `e.mu` critical section; same in `margin-engine/internal/margincall/service.go:~88`. If the handler publishes to Kafka/store or re-enters the engine, it stalls/deadlocks. Fire callbacks after unlock.
- **Map read without the guarding lock** (data race): `settlement-engine/internal/engine/engine.go:71-79` `getInstrumentConfig` reads `e.instruments` unlocked while `RegisterInstrument` writes under `e.mu` — classic Go "concurrent map read and map write" crash if registration races a settlement cycle.
- **Unsynchronized handler fields** (low/med): `SetTradeHandler`/`SetCycleHandler`/`SetMarginHandler` write handler fields with no lock; readers run on cycle goroutines. Safe only if set strictly before serving; use `atomic.Value` to be correct.

---

## 5. Test & Integration Reality — Confidence Is Currently Unknown

- **Latest integration verdict is FAIL, and it's ~3 months old:** `handoff/integration-run-20260329-110902.md` → FAIL (1188 pass / 6 fail / 5 skip). **No integration run exists after 2026-03-29**, despite many merges since (MSE-1..5, T116 real-Kafka, gateway/compliance fixes). The whole-platform signal for the current code is simply absent.
- **The 6 "known bugs" appear fixed in source** (gateway 404-before-auth via `RouteChecker` in `auth.go:35-70`; compliance returns an object in `server.go:186-188`; real Kafka added) — but **no run has confirmed** the fixes, and the `CLAUDE.md` narrative still says they persisted unfixed. Misleading as of today.
- **e2e tests provide near-zero default CI signal:** all 21 e2e funcs call `skipIfGatewayUnavailable` first (`tests/e2e/e2e_test.go:144-155`); 110 skip statements total. Without a live full stack, everything skips → green proves almost nothing about cross-service behavior.
- **Unit test counts are real but inflated by mechanical duplication.** Actual: ~2770 `func Test*` + 509 subtests across 247 files (the documented "828/1199" is now a ~2–3× *undercount*, not inflation). However, the per-service Kafka tests are **byte-identical copies** across 9 services (`event_test.go` 1427 bytes, `producer_test.go` 3552 bytes everywhere) — meaningful but redundant 9×. Entrypoint/wiring packages (`cmd/`, `config/`, `server`) sit at 0–18%, exactly where integration bugs live.

**Fix direction:** Run a fresh full-stack integration run (Docker Compose + real Kafka) before trusting any "PASS." Don't count duplicated Kafka template tests toward coverage goals.

---

## 6. Frontend, Infra & Deployment — More Real Than Documented

**Frontend (genuinely real apps, not scaffolds):**
- web-ui (38 src files): trading terminal, protected routes, JWT refresh, real WebSocket.
- admin-ui (82 src files): 23 pages, role-guarded routes, **full tenant switcher** (`TenantContext` fetching `/platform/v1/tenants`, injects `X-GarudaX-Tenant` on every non-platform call). The api.ts docs claim (29 fns) is stale — actual **60** exported functions.
- demo-runner (30 src files): data-driven scenario runner against the real gateway.
- The 4820 TS file count is inflated by `node_modules`; real source is ~150 `.ts/.tsx` total.

**Infra (mixed):**
- `docker-compose.yml` (20 services) — every build context has a real Dockerfile; runnable local stack.
- Terraform ~37% real: EKS/RDS/MSK modules are production-grade, **but 5 modules are 0-byte stubs** (vpc, elasticache, iam, s3, security-groups) yet wired into `main.tf` → `terraform apply` would not stand up a working network. **Not deployable as-is.**
- Migrations: **no V8/V9 conflict** (false alarm — distinct files). Caveat: **V2–V5 are 0-byte stubs**; real DDL starts at V6.
- nginx (`infrastructure/nginx/garudax.asla.mn`, 240 lines) is real and deployed on the host with Certbot TLS, reverse-proxying root/admin/trade/demo subdomains to local ports. This is a working demo/staging host.
- CI/CD is **off**: `.github/workflows/deploy.yml.disabled`. K8s/Helm manifests exist but are aspirational.

---

## 7. Repository Hygiene & Doc Drift

- **119 git branches; ~96% are orphaned `worktree-agent-*`** from the AI pipeline (111 merged, ~8 abandoned). Bulk-delete the merged ones now.
- **Doc drift is the bigger risk than the code:** README/CLAUDE.md say root is `garudax/` (actual: `ace-platform`); claim "11 Go services" (actual: 14, incl. undocumented corporate-actions, fix-gateway, platform-control); reference a `src/clearing-service/` stub that **no longer exists**; mark Phase 9 FIX gateway / 0.7 control-plane / 0.8 corporate-actions "Pending" though code exists. `AiX_Project_Knowledge.md` is the most accurate of the three docs.
- `corporate-actions/` is listed as a service but has **no `package main`/`main.go`/Dockerfile** — it's an importable library, not a runnable service.
- 4 uncommitted working-tree files (nginx config, tsbuildinfo, patterns.md, test-results) — commit or revert.
- The reported T062 "stranded unmerged branch" is **obsolete** — it's merged and live.

---

## Prioritized Recommendations

**P0 — correctness / money (do before any real trading or MSE onboarding):**
1. Replace `securities-service` float money math with a shared, audited decimal type.
2. Extract one `pkg/types/decimal.go` with checked overflow + defined rounding; converge all six divergent copies onto it.
3. Make Kafka wiring fail-fast without a broker in non-test mode (stop silent in-process fallback).
4. Fix callback-under-lock (clearing, margin) and the settlement instrument-map race.

**P1 — close the multi-tenancy gap (or honestly re-scope the claim):**
5. Wire the shared `tenant` module into services (add `replace` directives), or vendor it; delete the duplicate `platform-control` registry.
6. Remove the gateway `tenantBypassPrefixes` for real endpoints and forward `tenant_id` to backends.
7. Back the tenant registry with `platform.tenants`; enable the provisioner (real db, not `nil`).
8. Decide explicitly: either implement real per-tenant row/query isolation, or document the system as single-tenant-with-namespacing until MSE work begins.

**P2 — restore trust in the signal:**
9. Run a fresh full-stack integration run (Compose + Kafka); update `CLAUDE.md` metrics or mark them historical.
10. Re-enable CI/CD; gate merges on at least a build + unit run.

**P3 — hygiene:**
11. Delete merged `worktree-agent-*` branches; commit/revert the 4 dirty files.
12. Reconcile README/CLAUDE.md service count, roadmap status, and the phantom `clearing-service`; finish or remove the 5 stub Terraform modules and V2–V5 migration stubs.

---

## What's Genuinely Good

- Real, non-trivial implementations across 13 Go services and 3 React SPAs — not vaporware.
- Clean, idempotent SQL migrations (V6+) with a coherent `ace_*` namespace; the schema rename matches the code with zero stragglers.
- A well-built (if unadopted) shared tenant/observability library and a correct tenant-topic convention design.
- A working TLS edge serving all three SPAs; a runnable Docker Compose stack.
- A disciplined, self-documenting AI pipeline whose postmortem log honestly recorded many of these same issues as they arose.
