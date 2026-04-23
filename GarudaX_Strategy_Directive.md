# Strategic Directive — GarudaX Platform Evolution

**To:** Planner Agent (read before next planning pass)
**From:** Architecture / Product direction
**Effective:** Immediately — applies to all new tasks and all in-flight work not yet merged
**Status:** Supersedes prior product framing in `AiX_Project_Knowledge.md` §1 (Project Overview)

---

## 1. WHAT CHANGES

The product is no longer a single-purpose commodity exchange ("ACE Platform"). The product is **GarudaX** — a multi-tenant, AI-native operating platform that hosts regulated trading venues. The commodity exchange is now one tenant on that platform. The Mongolian Stock Exchange (MSE) is the incoming **flagship tenant** and drives platform-level design decisions.

**Renaming rule:** Internally, the codebase root is `garudax/`. The former ACE Platform is now the tenant `ace-commodities` (equivalently: `venues/ace-commodities/`). MSE will be the tenant `mse-equities`. All branding, docs, READMEs, and CLI output must reflect this.

---

## 2. NON-NEGOTIABLE ARCHITECTURAL CONSTRAINTS

These are platform invariants. Any task that cannot honour all of these is rejected by the Reviewer Agent without further discussion.

### 2.1 Tenant as a first-class construct
Every runtime artefact — every database row, every Kafka message, every S3 object, every metric, every log line, every cache key, every IAM role — carries an explicit `tenant_id`. There are no "default tenant" shortcuts, no NULL tenant IDs, no implicit tenant resolution from user context. A query without a tenant filter is a bug.

### 2.2 Tenant isolation guarantees
Three layers of isolation must hold simultaneously:

- **Data isolation.** One tenant cannot read, write, or infer another tenant's trading, clearing, participant, or audit data through any API, query, or side channel. Postgres schemas are namespaced per tenant (`ace_exchange`, `mse_exchange`, etc.). Cross-tenant queries require an explicit, logged, and reviewable platform-admin role that no tenant service account holds.
- **Operational isolation.** A tenant can halt trading, roll back a deployment, or run a circuit breaker without requiring coordination with any other tenant. Shared infrastructure (Kafka, Redis, observability) must tolerate one tenant's outage without degrading another.
- **Governance isolation.** Tenant-specific configuration (trading calendar, listing rules, circuit breaker thresholds, KYC policies, position limits) lives in per-tenant config stores. Platform-wide config is separate and changes to it require a platform-governance workflow, not a tenant workflow.

### 2.3 MSE-flagship priority
When a design choice creates friction between the flagship tenant's needs and a secondary tenant's needs, the flagship wins. This means the commodity exchange (`ace-commodities`) will require rework in several places. That rework is expected and acceptable. Builder agents must not avoid MSE-driven changes to preserve ACE-flagship convenience.

### 2.4 Preservation of completed foundation work
The following completed deliverables are not re-done. They are extended, not replaced:

- **T001** — AWS dual-region architecture (ADR-001). Accepted as-is. Extension: node groups and RDS instances must be labelled with a platform-level role (shared infra vs tenant-specific) but the topology does not change.
- **T002** — Terraform modules. Accepted as-is. Extension: module inputs must accept a `tenant_id` where applicable; state backend layout gets a per-tenant workspace convention (see §3.1).
- **T004** — Core database schema (V1–V5 migrations). Accepted as-is, *but the schema interpretation changes*: what was "the ACE database" is now "the `ace-commodities` tenant schema inside the GarudaX platform database." V6+ migrations will introduce the platform-level `platform.*` schema (see §3.2). ACE's V1–V5 schemas are renamed (by SQL migration, not code rewrite) into the `ace_*` namespace.

---

## 3. REQUIRED PLATFORM-LEVEL COMPONENTS (NEW TASKS)

The following are new task domains the Planner must add to `tasks.json`. Priority, dependencies, and estimates are at the Planner's discretion; what follows is the *content* each task domain must cover.

### 3.1 Platform control plane
- Tenant registry service — canonical source of truth for tenant identity, status (active / suspended / onboarding), and routing.
- Tenant lifecycle workflows — provisioning a new tenant (schemas, IAM roles, Kafka topics, Redis keyspaces, observability dashboards) must be a single reviewable and repeatable operation.
- Platform-admin API and UI — distinct from any tenant's operational surfaces; only platform operators have access.

### 3.2 Platform data model
- New Postgres schema `platform.tenants` — tenant ID, name, status, onboarding metadata, flagship flag, governance-tier.
- New Postgres schema `platform.audit` — platform-level audit events (tenant lifecycle, cross-tenant admin actions) — separate from each tenant's own `*_compliance.audit_events` chain.
- Flyway migrations: V6 onwards introduces `platform.*`; a V7 migration renames existing `reference / participants / exchange / clearing / compliance / warehouse / market_data` to `ace_reference / ace_participants / ...`. `auth` is evaluated separately (see §3.4).

### 3.3 Tenant context propagation
- Every inbound request (HTTP, FIX, Kafka consumer) must resolve to a `tenant_id` before any domain logic runs. No service accepts traffic with an unresolved tenant.
- gRPC / HTTP middleware — rejects any call without a tenant context. Tenant context travels in a signed header (not a query param).
- Kafka — topic naming convention `{tenant_id}.{domain}.{event}`. A consumer subscribed to a tenant's topic cannot accidentally receive another tenant's events.
- Observability — every metric, log, and trace carries `tenant_id` as a required label. Dashboards are tenant-scoped by default; platform-level dashboards are explicit opt-in for platform operators.

### 3.4 Identity, auth, and isolation
- `auth` schema is platform-level, not tenant-level. Users may have roles in multiple tenants; the JWT carries claims per tenant. Session tokens cannot grant cross-tenant access implicitly.
- IRSA roles are scoped per tenant: `garudax-{tenant_id}-{service}` — e.g., `garudax-mse-equities-matching-engine`. No service role spans tenants.
- KMS CMKs are per tenant. A tenant's data at rest is encrypted with that tenant's key, and key rotation is per tenant.

### 3.5 Agent fabric — tenant-aware by design
The AI agents that run the platform (InfraOps, Deploy, Incident, Security, Data Health, Observability) must carry tenant context through their entire execution. An agent responding to an incident on `mse-equities` has no read access to `ace-commodities` data during that invocation. Post-mortems are scoped per tenant by default; cross-tenant pattern extraction is a distinct, explicit operation.

---

## 4. MSE-FLAGSHIP TENANT REQUIREMENTS

These are specific to bringing `mse-equities` online as the flagship tenant. They are additions to the Phase 1+ task graph.

### 4.1 Equities-specific domain extensions
Equities differ from commodities in several concrete ways. Each needs explicit design and implementation:
- **Settlement cycle** — T+1 or T+2 against a central securities depository; commodity is physical delivery or daily cash mark-to-market. The clearing engine must support both models as separate settlement profiles per tenant.
- **Corporate actions** — dividends, splits, rights issues, mergers. Has no commodity equivalent. New domain service required.
- **Short selling rules and locate** — must be implementable per tenant's regulatory regime.
- **Opening and closing auctions** — equities use call auctions at session boundaries; commodity CLOB is continuous. Matching engine must support both session types.
- **Listing workflow** — equities listing (IPO, secondary listing) has a different review process than commodity product listing.

### 4.2 MSE regulatory integration
- Financial Regulatory Commission (FRC) of Mongolia reporting interfaces — separate reporting surface from commodity exchange reporting. Both land in the platform's `platform.audit` schema but use tenant-specific report formats.
- Mongolian Central Securities Depository integration — `mse-equities` tenant only.

### 4.3 MSE ownership of platform roadmap
Until further notice, any design decision that affects the platform's default behaviour for trading calendars, surveillance policy, or participant onboarding UX is resolved in favour of MSE's preferred approach, documented as the platform default, and offered to other tenants as a configurable override. Planner agents must flag such decisions explicitly in handoff documents so this precedent is visible.

---

## 5. RETROFIT TASKS FOR `ace-commodities`

The commodity tenant is live and working. The following migrations are required but must be executed without downtime for the existing commodity operations:

- Schema rename `reference → ace_reference` etc. — performed via Flyway; application code updated to reference new schema names in the same deployment.
- Kafka topic rename — new topics published under `ace-commodities.*` naming convention; dual-write transition period, then cutover.
- IRSA role rename — new roles provisioned with new names, services migrated, old roles removed.
- Audit event migration — existing hash-chained audit events remain immutable in their original location but are annotated with `tenant_id = 'ace-commodities'` at the schema level; new events land directly in the namespaced location.

Estimated effort for the retrofit is non-trivial. Planner should budget it as its own task cluster and sequence it *before* MSE-specific builder work so the platform is clean when MSE onboards.

---

## 6. WHAT IS EXPLICITLY OUT OF SCOPE

To keep this directive bounded:

- **No re-platforming of cloud architecture.** ADR-001 stands. Dual-region Tokyo / Singapore.
- **No change to the underlying tech stack.** Go for matching engine, Java/Kotlin for services, React frontend, PostgreSQL 15, TimescaleDB, Redis, Kafka, EKS.
- **No change to the agent collaboration pattern.** The Self-Learning Softhouse (Planner → Orchestrator → Workers → Reviewer → PostMortem) stays. What changes is that every agent is tenant-aware.
- **No commitment to any tenant beyond `ace-commodities` and `mse-equities`** in this directive. Future venues (bonds, carbon, FX) are in the product vision but do not generate tasks.

---

## 7. UPDATES TO PROJECT MEMORY

After the Planner produces the revised task graph, the PostMortem Agent must make the following updates:

- **`CLAUDE.md`** — append to Learned Patterns: "Every task from directive-2026-04 forward assumes multi-tenancy. Tenant ID is a first-class argument to every domain operation." Update file layout convention to show `venues/ace-commodities/` and `venues/mse-equities/` under the project root.
- **`AiX_Project_Knowledge.md`** — §1 Project Overview rewritten: product is GarudaX, tenants are ace-commodities (live) and mse-equities (onboarding). §2 phase plan gains a new Phase 0.5 for the retrofit cluster. §3 completed tasks gains annotations that T001/T002/T004 apply to the platform, not just ACE.
- **`tasks.json`** — new task cluster added (platform control plane, tenant context propagation, identity scoping, agent fabric tenancy, ace-commodities retrofit, mse-equities flagship build). Existing pending tasks (T003, T005, T006, T007, T008, T015, T027, T028, T029) reviewed: any that would break multi-tenancy as written are marked `status: rejected` and re-queued with new descriptions that honour §2 constraints.

---

## 8. SUCCESS CRITERIA

This directive is considered successfully absorbed when:

1. The next `tasks.json` produced by the Planner contains explicit platform-control-plane tasks and explicit tenant-isolation tasks.
2. No new task description references "the database" or "the exchange" without qualifying which tenant.
3. The `ace-commodities` retrofit cluster is sequenced before `mse-equities` flagship builder work.
4. The Reviewer Agent has a new rejection rule: "task does not carry tenant context through its API surface" — and uses it.
5. At least one task in the new graph is a platform-admin-only task (demonstrating platform vs tenant role separation has been internalised).

---

## 9. ONE-LINE SUMMARY FOR EVERY AGENT

> **GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.**

If any task, review, or commit message conflicts with that sentence, it is wrong.
