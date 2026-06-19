# GarudaX — Multi-Tenant AI-Native Trading Platform

GarudaX is a **multi-tenant, AI-native operating platform** that hosts regulated trading venues. Each tenant is an independent exchange with its own trading rules, participants, settlement model, and regulatory requirements. The platform provides the shared spine — matching, clearing, margin, settlement, market data, compliance, identity — and each venue is provisioned, configured, and operated as an isolated tenant on top of it.

> **GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.**

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

## Platform Invariants

These are non-negotiable. Any change that cannot honour all of them is rejected:

1. **Tenant as a first-class construct.** Every database row, Kafka message, S3 object, metric, log line, cache key, and IAM role carries an explicit `tenant_id`. There are no default-tenant shortcuts and no NULL tenant IDs. A query without a tenant filter is a bug.
2. **Data isolation.** One tenant cannot read, write, or infer another tenant's data through any API or side channel. Postgres schemas are namespaced per tenant (`ace_exchange`, `mse_exchange`, …). Cross-tenant access requires an explicit, logged platform-admin role no tenant service account holds.
3. **Operational isolation.** A tenant can halt trading, roll back a deployment, or trip a circuit breaker without coordinating with any other tenant. Shared infrastructure tolerates one tenant's outage without degrading another.
4. **Governance isolation.** Tenant-specific config (calendar, listing rules, breaker thresholds, KYC policy, position limits) lives in per-tenant stores. Platform-wide config is separate and changes through a platform-governance workflow.
5. **Identity is platform-level.** `auth` is a platform schema. A user may hold roles in multiple tenants; the JWT carries per-tenant claims. Session tokens never grant cross-tenant access implicitly.

---

## Architecture

- **Cloud:** AWS dual-region active/passive (Tokyo primary, Singapore DR) — ADR-001, unchanged by the multi-tenant pivot
- **Orchestration:** EKS + Istio service mesh with mTLS
- **Database:** PostgreSQL 15 (OLTP, per-tenant schemas + `platform.*` control-plane schema) + TimescaleDB (tick data) + Redis 7 (cache)
- **Messaging:** Apache Kafka 3.5 via MSK — topic convention `{tenant_id}.{domain}.{event}`
- **Identity:** platform-level `auth`; IRSA roles scoped per tenant (`garudax-{tenant_id}-{service}`); KMS CMKs per tenant
- **IaC:** Terraform 1.6+ with per-tenant workspace convention

See [`docs/platform-architecture.md`](docs/platform-architecture.md) and [`docs/adr/`](docs/adr/) for details.

---

## Repository Structure

```
garudax/
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
├── src/                         # Application source (Go services + React SPAs)
│   ├── matching-engine/         # CLOB + order matching
│   ├── clearing-engine/         # Novation, netting, default fund
│   ├── margin-engine/           # SPAN margin
│   ├── settlement-engine/       # DvP settlement, P&L
│   ├── auth-service/            # Platform identity & IAM
│   ├── compliance-service/      # KYC/AML, surveillance
│   ├── market-data-service/     # Ticks, candles, streaming
│   ├── warehouse-service/       # eWR (ace-commodities)
│   ├── securities-service/      # Equities domain (mse-equities)
│   ├── platform-service/        # Tenant registry & control plane
│   ├── gateway/                 # Tenant-aware API gateway
│   ├── admin-bot/               # AI ops bot (MCP)
│   ├── web-ui / admin-ui / demo-runner  # React SPAs
│   └── shared/                  # Shared tenant context middleware
├── tests/                       # Unit, e2e, Playwright, load
└── deploy/                      # K8s manifests, Helm charts
```

---

## Roadmap

| Phase | Name | Status |
|-------|------|--------|
| 0     | Foundation & Infrastructure (cloud arch, IaC, DB schema, auth) | Complete |
| 1–7   | Exchange engines, supporting services, securities module, frontends, AI bot | Complete |
| **0.5** | **Multi-tenant platform specs** — architecture, migrations V29–V30, tenant context design | **Complete** |
| **0.6** | **`ace-commodities` retrofit** — schema renames, tenant context middleware, code updates, Kafka topic migration | **Next** |
| **0.7** | **Platform control plane** — tenant registry service, lifecycle workflows, platform-admin API/UI | Pending |
| **0.8** | **`mse-equities` flagship build** — equities domain, corporate actions, FRC reporting, MCSD integration | Pending |
| 9     | FIX protocol gateway (tenant-aware broker connectivity) | Pending |
| 10    | AI bot expansion (tenant-scoped operations) | Pending |

The numbered phases (0–8) delivered the original ACE exchange foundation, now reinterpreted as the `ace-commodities` tenant. The Phase 0.5+ cluster delivers the platform pivot: `ace-commodities` retrofit must complete before `mse-equities` flagship builder work so the platform is clean when MSE onboards.

---

## Agent Workflow

This project uses the **Self-Learning Softhouse** pattern (see [`CLAUDE.md`](CLAUDE.md)):

1. **Planner** reads requirements + learned patterns → produces `tasks.json`
2. **Orchestrator** spawns worker agents in isolated git worktrees
3. **Workers** (Coder/QA/Docs/Security) write code + `handoff/` summaries
4. **Reviewer** approves or rejects with notes — and rejects any task that does not carry tenant context through its API surface
5. **PostMortem** extracts patterns → appends to `CLAUDE.md`

Every agent is tenant-aware: an agent acting on `mse-equities` has no read access to `ace-commodities` data during that invocation.

---

## Getting Started

```bash
# Clone
git clone <repo-url> && cd garudax

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
