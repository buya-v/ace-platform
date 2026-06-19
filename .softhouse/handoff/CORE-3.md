# CORE-3 — Implement Platform Control Plane APIs

**Status:** success
**Role:** coder
**Branch/worktree:** line/CORE-3

## Summary

Built the GarudaX **Platform Control Plane** as a self-contained, zero-dependency Go
module at `src/platform-control/` (stdlib only, matching the established service
pattern). It delivers the Tenant Registry and platform-admin API for creating,
suspending, activating, modifying, and decommissioning tenant environments (venues).

The file layout follows the task hint (flat package at the module root):

| File | Responsibility |
|---|---|
| `registry.go` | `TenantRegistry` — thread-safe in-memory store + lifecycle logic |
| `api.go` | `API` — platform-admin HTTP surface (`http.Handler`) |
| `types.go` | Domain types, lifecycle/tier constants, transition table, DTOs |
| `main.go` | Service entrypoint (seeded registry, API :8096, health :9096) |
| `registry_test.go`, `api_test.go` | Unit + HTTP integration tests |
| `Dockerfile` | Static non-root build |

## Key decisions

- **Relationship to existing `src/platform-service`:** `platform-service` (run
  `20260423-control-plane`) already does basic tenant CRUD + provisioning. Rather than
  duplicate it, this module is the *control plane* proper: it adds the pieces that
  service lacks — a **lifecycle state machine**, the **single-flagship invariant**, and
  an **audit trail**. The two are complementary; if consolidation is desired later,
  fold platform-service's provisioner behind this registry.
- **Lifecycle state machine** (`allowedTransitions` in `types.go`):
  `ONBOARDING → ACTIVE → SUSPENDED ⇄ ACTIVE`, any non-terminal state → `DECOMMISSIONED`,
  which is terminal. Illegal transitions and no-op same-state transitions return
  `409 INVALID_TRANSITION`. This mirrors the `status` CHECK constraint in
  `infrastructure/db/migrations/V31__platform_control_schemas.sql`.
- **Single-flagship invariant:** creating a second flagship returns `409
  FLAGSHIP_CONFLICT`; a new flagship is permitted only after the prior one is
  decommissioned. Matches the partial unique index `idx_platform_tenants_flagship`.
- **Decommissioned tenants are immutable** (`PATCH` → `409 TENANT_DECOMMISSIONED`).
- **Config versioning:** modifying `governance_tier` or `asset_classes` bumps
  `config_version` (descriptive-only edits do not).
- **Audit trail:** every mutating action appends an `AuditEntry` (sequence, action,
  from/to status, actor, reason, timestamp) — the app-side mirror of `platform.audit`.
- **Platform-level, not tenant-scoped:** routes live under `/platform/v1/*` with **no
  tenant middleware** — this service *is* the platform (per the platform invariant).

## API surface (`/platform/v1/tenants`)

- `GET    /tenants[?status=ACTIVE]` — list (optional status filter)
- `POST   /tenants` — create (always starts `ONBOARDING`; validates slug, name, tier, cycle)
- `GET    /tenants/{id}` — fetch
- `PATCH  /tenants/{id}` — modify display_name/description/governance_tier/asset_classes/regulatory_body
- `POST   /tenants/{id}/suspend|/activate|/decommission` — convenience lifecycle actions (optional `{actor, reason}` body)
- `PUT    /tenants/{id}/status` — generic transition `{status, actor, reason}`
- `GET    /tenants/{id}/audit` — per-tenant audit trail
- `GET    /healthz`, `GET /readyz`

Error envelope is `{ "error": { code, message, details[] } }`. Sentinel errors map to
404 / 409 / 422 as appropriate.

## Verification

```
cd src/platform-control
go vet ./...        # clean
gofmt -l .          # clean
go test ./... -race # ok — 37 tests pass
```

- Overall coverage **79.0%**; business-logic (`registry.go` + `api.go`) **~89%**.
- Untested remainder is `main.go` (network server wiring), consistent with the project
  convention of excluding `cmd/`/server bootstrap from coverage targets.

## Suggested follow-ups

- **DB-backed store:** swap the in-memory map for a `*sql.DB` implementation behind the
  same `TenantRegistry` methods, persisting to `platform.tenants` + `platform.audit`
  (V31). The API layer needs no changes.
- **Gateway wiring:** add `/platform/v1/*` → platform-control (port 8096) in
  `src/gateway`, skipping tenant middleware (same treatment platform-service got).
- **docker-compose:** add a `platform-control` service (8096/9096) with a healthcheck.
- **Admin UI:** the existing `src/admin-ui/src/pages/PlatformAdmin.tsx` can point its
  status-change calls at the new lifecycle endpoints to get state-machine validation.
- **Consolidation:** reconcile `platform-control` vs `platform-service` to avoid the
  naming-confusion class flagged in CLAUDE.md learned patterns.
