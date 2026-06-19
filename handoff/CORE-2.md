# CORE-2 — Refactor Auth & IAM for Platform Tenancy

**Status:** success
**Role:** coder
**Branch/worktree:** line/CORE-2

## Summary

Reworked the auth-service so JWTs carry **tenant-scoped claims and permissions**
per `docs/platform-architecture.md §5.2`, and so per-tenant role assignments are
sourced from the platform-level `platform.tenant_user_roles` table (migration
**V31**, see [[CORE-3]] / DB-1). The `auth` schema stays platform-level — it is
NOT nested under a tenant (honoring the V29/V30/V31 decision).

## JWT structure (now emitted)

```json
{
  "sub": "...", "iss": "garudax-auth", "name": "...", "email": "...",
  "tenant_roles": { "ace-commodities": ["admin","trader"], "mse-equities": ["viewer"] },
  "platform_roles": ["platform-admin"],
  "active_tenant": "mse-equities",
  "permissions": ["market:read", ...],   // resolved for active_tenant's roles
  "role": "viewer",                        // legacy: primary role in active tenant
  "iat":..., "exp":..., "jti":"...", "type":"access"
}
```

## Key decisions

- **Grants sourced from `platform.tenant_user_roles`.** Added
  `Store.GetTenantRoles(userID)` + `AssignTenantRole(...)` to the auth `Store`
  interface, implemented for InMemory, Postgres (`platform.tenant_user_roles`
  with `ON CONFLICT` upsert), and the Redis wrapper (delegates to inner). The
  auth service is the manager of these rows per V31's grant
  (`garudax_auth_svc` gets INSERT/UPDATE).
- **Platform vs tenant scope.** Assignments with `tenant_id = 'platform'`
  (`types.PlatformScope`) surface as `platform_roles`, never inside
  `tenant_roles`. Only `platform-admin` exists today.
- **Active tenant + access enforcement.** `LoginWithTenant` (and the
  `X-GarudaX-Tenant` header / `active_tenant` body field on `/login`) selects the
  active tenant. Selecting a tenant the user has no role in returns
  `ErrTenantAccessDenied` → HTTP 403. `platform-admin` bypasses this check (still
  needs a tenant to scope the session). Final authorization remains the gateway's
  job via the header (§5.3) — the token just advertises capability.
- **Backward compatibility (no breakage).** `GenerateAccessToken(user, jti)` is
  retained as a shim that scopes the user's top-level role to the **DefaultTenant
  (`ace-commodities`)**, so all ~20 existing call sites (jwt/rsa/key-rotation
  tests, `KeySet`) and single-tenant logins keep working. Users with no explicit
  assignments fall back to `{ace-commodities: [user.role]}`.
- **New tenant-scoped roles/permissions** added per §5.2: `clearing_admin`,
  `exchange_admin` (+ `settlement:manage`, `instrument:manage`, `trading:halt`).
  `PermissionsForRoles` returns the deduped union for the active tenant.

## Verification

```
cd src/auth-service
go build ./...      # clean
go vet ./...        # clean
go test ./...       # all pass — internal/auth 87.6% coverage
```

New tests: `internal/auth/tenant_claims_test.go` (8 tests — multi-tenant claim
emission, tenant-access denial, platform-admin bypass, legacy fallback,
active-tenant resolution, permission dedup, idempotent assignment) and a Redis
delegation test for the tenant-role methods.

## Suggested follow-ups

- **Gateway (`src/gateway/internal/auth/jwt.go` + `middleware/auth.go`):** extend
  its `Claims` to parse `tenant_roles`/`platform_roles`/`active_tenant` and
  enforce §5.3 step 6–8 (look up `tenant_roles[X-GarudaX-Tenant]`, 403 if empty;
  platform-admin bypass). It currently only reads `role`/`roles`.
- **Admin/platform API:** expose an endpoint to call `AssignTenantRole` so
  operators can grant per-tenant roles (today only programmatic).
- **`name` claim:** `auth.users` has no `name` column; `User.Name` is currently
  always empty. Add a column + Register field if the display name is needed.
- **mse-equities (Phase 0.8):** seed `platform.tenant_user_roles` for MSE users.
