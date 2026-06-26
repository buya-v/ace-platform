# GarudaX — Multi-Tenant AI-Native Trading Platform

GarudaX is an **AI-native operating platform** for regulated trading venues, **architected for multi-tenancy**. The long-term design hosts each trading venue as an isolated tenant on a shared spine — matching, clearing, margin, settlement, market data, compliance, identity. **MSE is the incoming flagship tenant** whose needs drive platform-level design decisions.

> **GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.**

> **⚠️ Current state (be honest about it).** Today GarudaX runs **single-tenant with namespacing**, not full multi-tenant isolation. What is actually shipped: a Postgres `ace_*` schema namespace (migrations V30/V32/V33), a `platform.*` control-plane schema, and a **tenant-aware API surface** — the gateway requires and validates an `X-GarudaX-Tenant` header on every business route against a static two-entry allow-list (`ace-commodities`, `mse-equities`) and forwards the resolved tenant downstream (see R010/R011). There is exactly **one live tenant: `ace-commodities`**. `mse-equities` is registered as `ONBOARDING` but has no schemas, no data, and no domain code yet. Several isolation mechanisms the invariants describe are **not yet built** — see [MSE Onboarding Prerequisites](#mse-onboarding-prerequisites). This README marks aspirational architecture as *target* and shipped behaviour as *today*.

**Target:** Mongolia (Ulaanbaatar, MNT currency)
**Delivery:** AI agent-driven development (Self-Learning Softhouse pipeline)
**Stack:** Go (engines & services) · React/TypeScript (frontends) · PostgreSQL 15 · TimescaleDB · Kafka · Redis · EKS/Istio · Terraform

---

## Tenants

| Tenant | Venue | Status | Domain | Settlement |
|--------|-------|--------|--------|------------|
| **`ace-commodities`** | ACE Commodity Exchange | **ACTIVE** | Wheat, barley, cattle, cashmere, wool — physical delivery via eWR | T+0 / physical delivery, daily mark-to-market |
| **`mse-equities`** | Mongolian Stock Exchange | **ONBOARDING (flagship)** | Equities, bonds, ETFs — corporate actions, auctions, short selling | T+2 book-entry via MCSD, FRC reporting |

`ace-commodities` is live and working. `mse-equities` is the incoming **flagship tenant** and drives platform-level design decisions: when a design choice creates friction between the flagship's needs and a secondary tenant's needs, the flagship wins.

Per-tenant configuration lives under [`venues/`](venues/) — `venues/ace-commodities/config.json` and `venues/mse-equities/config.json` declare each venue's trading calendar, settlement profile, circuit breakers, compliance policy, and feature flags.

---

## Platform Invariants (target architecture)

These are the **design invariants** that govern multi-tenant work. They are the contract every new tenancy task is reviewed against — but most are **not yet fully enforced in code**. Each invariant below is annotated with its current status.

1. **Tenant as a first-class construct.** *Target:* every database row, Kafka message, S3 object, metric, log line, cache key, and IAM role carries an explicit `tenant_id`; a query without a tenant filter is a bug. *Today:* tenant is carried at the **API edge** (gateway header validation/forwarding, shared `pkg/tenant` context + interceptors) and at the **schema** level (`ace_*` namespace). **Row-level `tenant_id` columns and `WHERE tenant_id = …` filtering are NOT in place** on domain tables — only immutable audit/event records are annotated (V32). See backlog.
2. **Data isolation.** *Target:* one tenant cannot read, write, or infer another tenant's data; Postgres schemas are namespaced per tenant (`ace_exchange`, `mse_exchange`, …). *Today:* isolation is **schema-namespace-only** and there is only the `ace_*` namespace — `mse_*` schemas do not exist. With a single live tenant there is no cross-tenant surface to breach yet, but the row-level guards that would enforce it under two live tenants are not built.
3. **Operational isolation.** *Target:* a tenant can halt trading, roll back, or trip a breaker without coordinating with another. *Today:* there is one live tenant, so this is **untested** — shared-infra blast-radius isolation has not been exercised.
4. **Governance isolation.** *Partially today:* per-tenant config lives under [`venues/`](venues/) (`venues/ace-commodities/config.json`, `venues/mse-equities/config.json`); platform-wide config is separate.
5. **Identity is platform-level.** *Today:* `auth` is a platform-level schema and the gateway bypasses tenant enforcement on `/api/v1/auth/` (login happens before a tenant is selected). Per-tenant JWT claims and multi-tenant role binding are **design intent, not yet implemented** — there is a single tenant to authorise against.

---

## Architecture

- **Cloud:** AWS dual-region active/passive (Tokyo primary, Singapore DR) — ADR-001, unchanged by the multi-tenant pivot
- **Orchestration:** EKS + Istio service mesh with mTLS
- **Database:** PostgreSQL 15 (OLTP) + TimescaleDB (tick data) + Redis 7 (cache). *Today:* the `ace_*` schema namespace plus the `platform.*` control-plane schema (V29/V31). *Target:* one namespaced schema set per tenant.
- **Messaging:** Apache Kafka 3.5 via MSK — topic convention `{tenant_id}.{domain}.{event}` (the shared `pkg/tenant` Kafka router applies it; topics for additional tenants are provisioned on onboarding).
- **Identity:** platform-level `auth`. *Target:* IRSA roles scoped per tenant (`garudax-{tenant_id}-{service}`) and KMS CMKs per tenant — the Terraform variables are tenant-aware but **per-tenant IRSA/KMS are not yet provisioned** (single shared set today).
- **IaC:** Terraform 1.6+ with a per-tenant workspace *convention* (not yet instantiated for a second tenant).

See [`docs/platform-architecture.md`](docs/platform-architecture.md) and [`docs/adr/`](docs/adr/) for the target design, and [MSE Onboarding Prerequisites](#mse-onboarding-prerequisites) for the gap to it.

---

## Repository Structure

> The repository directory is `ace-platform/` (its original name); the product/platform is **GarudaX**. The two names refer to the same tree.

```
ace-platform/                    # repo dir (product name: GarudaX)
├── CLAUDE.md                    # AI pipeline memory + learned patterns (read first)
├── AiX_Project_Knowledge.md     # Project knowledge base (all phases/tasks)
├── GarudaX_Strategy_Directive.md# Multi-tenant pivot directive (platform invariants)
├── tasks.json                   # Current task graph (machine-readable)
├── handoff/                     # Agent-to-agent communication
│   ├── <task-id>.md             # Worker completion summaries
│   └── <task-id>-review.md      # Reviewer verdicts
├── venues/                      # Per-tenant venue configuration
│   ├── ace-commodities/         # Commodity exchange tenant (ACTIVE)
│   └── mse-equities/            # MSE flagship tenant (ONBOARDING)
├── docs/
│   ├── adr/                     # Architecture Decision Records
│   ├── platform-architecture.md # Multi-tenant platform design
│   └── specs/                   # Phase architecture specs
├── infrastructure/
│   ├── terraform/               # vpc, eks, rds, msk modules (per-tenant aware)
│   └── db/migrations/           # Flyway SQL (V1–V30+: core, platform, tenant renames)
├── src/                         # Application source — 14 Go modules + TS bots + React SPAs
│   │                            # (14 = count of go.mod files; 13 services + `shared` lib)
│   ├── matching-engine/         # CLOB + order matching
│   ├── clearing-engine/         # Novation, netting, default fund
│   ├── margin-engine/           # SPAN margin
│   ├── settlement-engine/       # DvP settlement, P&L
│   ├── auth-service/            # Platform identity & IAM
│   ├── compliance-service/      # KYC/AML, surveillance, MCSD/FRC
│   ├── market-data-service/     # Ticks, candles, streaming
│   ├── warehouse-service/       # eWR (ace-commodities)
│   ├── securities-service/      # Equities domain (mse-equities, not yet a live tenant)
│   ├── corporate-actions/       # Dividends/splits/rights (built, not yet wired to a binary)
│   ├── platform-service/        # Tenant registry (in-memory today) & provisioner
│   ├── fix-gateway/             # FIX 4.4 broker connectivity (Phase 9, partial)
│   ├── gateway/                 # Tenant-aware API gateway (header enforcement)
│   ├── shared/                  # Shared tenant context middleware (pkg/tenant) + Kafka router
│   ├── admin-bot/ architect-bot/# AI ops bots (TypeScript MCP servers — not Go)
│   └── web-ui/ admin-ui/ demo-runner/  # React SPAs
├── tests/                       # Unit, e2e, Playwright, load
└── deploy/                      # K8s manifests, Helm charts
```

---

## Roadmap

| Phase | Name | Status |
|-------|------|--------|
| 0     | Foundation & Infrastructure (cloud arch, IaC, DB schema, auth) | Complete |
| 1–7   | Exchange engines, supporting services, securities module, frontends, AI bot | Complete |
| **0.5** | **Multi-tenant platform specs** — architecture, migrations V29–V31, tenant context design | **Complete** |
| **0.6** | **`ace-commodities` retrofit** — schema renames, tenant context middleware, code updates, Kafka topic migration | **In progress (partial)** — `ace_*` schema renames shipped (V30/V32/V33); shared `pkg/tenant` middleware + gateway header enforcement shipped (R010/R011). Row-level `tenant_id` + Kafka topic cutover **not done**. |
| **0.7** | **Platform control plane** — tenant registry service, lifecycle workflows, platform-admin API/UI | **In progress (partial)** — `platform-service` exists with an **in-memory** registry (seeded, not `platform.tenants`-backed) and a **schema-only** provisioner. Lifecycle/IAM/Kafka/Redis/dashboard provisioning + platform-admin UI **not done**. |
| **0.8** | **`mse-equities` flagship build** — equities domain, corporate actions, FRC reporting, MCSD integration | Pending — `securities-service` and `corporate-actions` modules **exist** but are not wired to a live `mse-equities` tenant; no `mse_*` schemas. |
| 9     | FIX protocol gateway (tenant-aware broker connectivity) | **In progress (partial)** — `fix-gateway` module exists. |
| 10    | AI bot expansion (tenant-scoped operations) | Pending |

The numbered phases (0–8) delivered the original ACE exchange foundation, now reinterpreted as the `ace-commodities` tenant. The Phase 0.5+ cluster delivers the platform pivot: `ace-commodities` retrofit must complete before `mse-equities` flagship builder work so the platform is clean when MSE onboards. "Code exists" ≠ "tenant is live" — see [MSE Onboarding Prerequisites](#mse-onboarding-prerequisites) for what still gates a second live tenant.

---

## MSE Onboarding Prerequisites

Standing up a **second live tenant** (`mse-equities`) requires the isolation machinery the [Platform Invariants](#platform-invariants-target-architecture) describe but that is **not built yet**. This is the explicit gap between today's single-tenant-with-namespacing reality and the multi-tenant target. None of the items below should be assumed to exist:

1. **Row-level tenant scoping.** Add `tenant_id` columns to domain tables (orders, trades, positions, margin, settlement, participants, …) and enforce `WHERE tenant_id = …` on every query — ideally via Postgres Row-Level Security plus application-layer guards. Today only immutable audit/event rows carry `tenant_id` (V32); domain isolation is schema-namespace-only.
2. **`platform.tenants`-backed registry.** Replace `platform-service`'s in-memory, seeded `InMemoryTenantStore` with a store backed by the real `platform.tenants` table (DDL exists in V29) so tenant identity/status/routing is authoritative and durable.
3. **A working provisioner.** Extend the current schema-only, dry-run-by-default provisioner into a repeatable workflow that creates, for a new tenant: Postgres schemas, IAM/IRSA roles, KMS CMKs, Kafka topics, Redis keyspaces, and monitoring dashboards — as one transactional operation with rollback.
4. **`mse_*` schemas.** Author the migrations that create the `mse_exchange` / `mse_*` namespace and the equities domain tables. These do not exist (only referenced in comments).
5. **Per-tenant IRSA & KMS.** Instantiate the per-tenant Terraform workspace and provision `garudax-mse-equities-{service}` IRSA roles and per-tenant KMS CMKs. The Terraform variables are tenant-aware but a second tenant has never been applied.
6. **Backend-side tenant enforcement.** The gateway validates and forwards `X-GarudaX-Tenant`, but backend services do not yet enforce it (no gRPC interceptor wired on backend servers; no cross-tenant isolation tests). Wire the shared `pkg/tenant` interceptors backend-side (R012 follow-up).
7. **WebSocket & demo tenant resolution.** `/api/v1/ws/` now requires the tenant header, but browsers cannot set custom headers on the WS handshake — needs a query-param/subprotocol resolver before MSE streaming works.

Until these land, treat GarudaX as a single-tenant commodity exchange (`ace-commodities`) with a tenant-aware edge, not a running multi-tenant platform.

---

## Agent Workflow

This project uses the **Self-Learning Softhouse** pattern (see [`CLAUDE.md`](CLAUDE.md)):

1. **Planner** reads requirements + learned patterns → produces `tasks.json`
2. **Orchestrator** spawns worker agents in isolated git worktrees
3. **Workers** (Coder/QA/Docs/Security) write code + `handoff/` summaries
4. **Reviewer** approves or rejects with notes — and rejects any task that does not carry tenant context through its API surface
5. **PostMortem** extracts patterns → appends to `CLAUDE.md`

Tenant-awareness is a **review and design policy**, not yet a runtime sandbox: the Reviewer rejects API surfaces without tenant context, but there is no enforced data-access isolation between agents today (there is one live tenant). Runtime cross-tenant isolation is part of the [MSE Onboarding Prerequisites](#mse-onboarding-prerequisites).

---

## Getting Started

```bash
# Clone (repo dir is `ace-platform`; product name is GarudaX)
git clone <repo-url> && cd ace-platform

# Inspect tenant configuration
cat venues/ace-commodities/config.json
cat venues/mse-equities/config.json

# Check current task state
cat tasks.json | jq '.tasks[] | select(.status != "done") | {id, title, status}'

# Resume from where agents left off
# 1. Read CLAUDE.md (+ GarudaX_Strategy_Directive.md for platform invariants)
# 2. Check tasks.json for incomplete tasks
# 3. Read handoff/ for completed context
```

---

## License

Proprietary — GarudaX Multi-Tenant AI-Native Trading Platform
