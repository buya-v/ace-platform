# GarudaX Platform Architecture Specification

**Document ID:** GARUDAX-PLATFORM-ARCH-001
**Version:** 1.0
**Date:** 2026-04-23
**Status:** APPROVED
**Authority:** GarudaX_Strategy_Directive.md

> **GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.**

---

## Table of Contents

1. [Overview](#1-overview)
2. [Tenant Model](#2-tenant-model)
3. [Data Isolation](#3-data-isolation)
4. [Tenant Context Propagation](#4-tenant-context-propagation)
5. [Identity & Auth](#5-identity--auth)
6. [Platform Control Plane](#6-platform-control-plane)
7. [Operational Isolation](#7-operational-isolation)
8. [Governance Isolation](#8-governance-isolation)
9. [Agent Fabric Tenancy](#9-agent-fabric-tenancy)
10. [MSE Flagship Requirements](#10-mse-flagship-requirements)
11. [Migration Strategy](#11-migration-strategy)
12. [Appendix](#12-appendix)

---

## 1. Overview

GarudaX is a multi-tenant, AI-native operating platform that hosts regulated trading venues. Each tenant is a venue -- an independent exchange with its own asset classes, trading rules, settlement cycles, regulatory regime, and operational lifecycle.

### Tenants

| Tenant ID | Display Name | Asset Classes | Status | Role |
|---|---|---|---|---|
| `ace-commodities` | ACE Commodity Exchange | Wheat, barley, cattle, cashmere, wool | `ACTIVE` | First tenant, commodity exchange, live in production |
| `mse-equities` | Mongolian Stock Exchange | Equities, bonds, ETFs | `ONBOARDING` | Flagship tenant, drives all platform-level design decisions |

### What is shared (platform-level)

- Identity and authentication (`auth` schema)
- Platform audit log (`platform.audit`)
- Tenant registry (`platform.tenants`)
- Infrastructure: EKS clusters, Kafka brokers, Redis cluster, PostgreSQL instance, S3 buckets
- Observability stack: Prometheus, Grafana, OpenTelemetry Collector
- AI agent fabric: Planner, Orchestrator, Workers, Reviewer, PostMortem

### What is isolated (per-tenant)

- Database schemas (all domain data)
- Kafka topics
- Redis keyspaces
- KMS encryption keys
- IRSA service roles
- Trading calendars, circuit breakers, market phases
- Compliance/regulatory configuration
- Settlement profiles

### Design invariants

1. Every database row, Kafka message, S3 object, metric, log line, cache key, and IAM role carries an explicit `tenant_id`.
2. There are no "default tenant" shortcuts. No NULL tenant IDs. No implicit tenant resolution from user context.
3. A query without a tenant filter is a bug.
4. When MSE's needs conflict with ACE's convenience, MSE wins.
5. Cross-tenant data access requires an explicit, logged, and reviewable `platform-admin` role.

---

## 2. Tenant Model

### 2.1 `platform.tenants` Table

```sql
CREATE SCHEMA IF NOT EXISTS platform;

CREATE TABLE platform.tenants (
    tenant_id           VARCHAR(64) PRIMARY KEY,           -- Lowercase slug: 'ace-commodities', 'mse-equities'
    display_name        VARCHAR(255) NOT NULL,              -- Human-readable: 'ACE Commodity Exchange'
    description         TEXT,
    status              VARCHAR(20) NOT NULL DEFAULT 'ONBOARDING'
                        CHECK (status IN ('ONBOARDING', 'ACTIVE', 'SUSPENDED', 'DECOMMISSIONED')),
    is_flagship         BOOLEAN NOT NULL DEFAULT FALSE,     -- MSE = true; only one tenant can be flagship
    governance_tier     VARCHAR(10) NOT NULL DEFAULT 'STANDARD'
                        CHECK (governance_tier IN ('FLAGSHIP', 'STANDARD', 'SANDBOX')),
    -- Asset classes supported by this tenant
    asset_classes       TEXT[] NOT NULL DEFAULT '{}',        -- e.g., ARRAY['COMMODITY'] or ARRAY['EQUITY','BOND','ETF']
    -- Settlement profile
    default_settlement_cycle VARCHAR(5) NOT NULL DEFAULT 'T+0',  -- 'T+0', 'T+1', 'T+2'
    -- Regional/regulatory
    primary_currency    CHAR(3) NOT NULL DEFAULT 'MNT',
    timezone            VARCHAR(50) NOT NULL DEFAULT 'Asia/Ulaanbaatar',
    regulatory_body     VARCHAR(100),                        -- 'FRC' for MSE, 'MCGA' for ACE, etc.
    -- KMS
    kms_cmk_arn         VARCHAR(255),                        -- Per-tenant AWS KMS CMK ARN
    -- Metadata
    onboarding_metadata JSONB NOT NULL DEFAULT '{}',         -- Flexible key-value for onboarding state
    config_version      INT NOT NULL DEFAULT 1,              -- Incremented on config changes
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at        TIMESTAMPTZ,
    suspended_at        TIMESTAMPTZ,
    decommissioned_at   TIMESTAMPTZ
);

-- Only one flagship tenant allowed
CREATE UNIQUE INDEX idx_tenants_flagship ON platform.tenants (is_flagship) WHERE is_flagship = TRUE;
CREATE INDEX idx_tenants_status ON platform.tenants (status);
```

### 2.2 Tenant ID Format

- **Format:** Lowercase alphanumeric slug with hyphens. Pattern: `^[a-z][a-z0-9-]{2,62}[a-z0-9]$`
- **Examples:** `ace-commodities`, `mse-equities`, `carbon-exchange`, `bond-market`
- **Rules:**
  - 4-64 characters
  - Starts with a letter
  - Ends with a letter or digit
  - Only lowercase letters, digits, and hyphens
  - No consecutive hyphens
  - Immutable after creation (cannot be renamed)

### 2.3 Tenant Lifecycle

```
ONBOARDING ─────→ ACTIVE ─────→ SUSPENDED ─────→ DECOMMISSIONED
     │                │               │
     │                │               └──→ ACTIVE (reactivation)
     │                │
     │                └──→ DECOMMISSIONED (direct shutdown)
     │
     └──→ DECOMMISSIONED (abandoned onboarding)
```

| Status | Description | Allowed Operations |
|---|---|---|
| `ONBOARDING` | Schemas created, configuration in progress, no live trading | Admin API, config writes, test orders |
| `ACTIVE` | Fully operational, live trading permitted | All operations |
| `SUSPENDED` | Trading halted by platform admin, data preserved | Read-only queries, no new orders, settlement of existing obligations continues |
| `DECOMMISSIONED` | Tenant shut down, data archived, schemas retained read-only for audit | Read-only queries by platform-admin only |

### 2.4 Governance Tiers

| Tier | Description | Config Change Workflow |
|---|---|---|
| `FLAGSHIP` | Platform design decisions defer to this tenant's needs | 2-person approval, audit trail, rollback plan required |
| `STANDARD` | Normal tenant, follows platform defaults unless overridden | 1-person approval, audit trail |
| `SANDBOX` | Test/development tenant, no production data, relaxed controls | Self-service, no approval required |

---

## 3. Data Isolation

### 3.1 Per-Tenant Postgres Schemas

Every existing schema is renamed with a tenant prefix. The `auth` schema remains platform-level. Two new platform schemas are created.

**Schema Mapping (ace-commodities tenant):**

| Current Schema | Renamed To | Owner Role |
|---|---|---|
| `reference` | `ace_reference` | `garudax_ace_reference_svc` |
| `participants` | `ace_participants` | `garudax_ace_participants_svc` |
| `exchange` | `ace_exchange` | `garudax_ace_exchange_svc` |
| `clearing` | `ace_clearing` | `garudax_ace_clearing_svc` |
| `compliance` | `ace_compliance` | `garudax_ace_compliance_svc` |
| `warehouse` | `ace_warehouse` | `garudax_ace_warehouse_svc` |
| `market_data` | `ace_market_data` | `garudax_ace_market_data_svc` |
| `securities` | `ace_securities` | `garudax_ace_exchange_svc` |

**Schema Mapping (mse-equities tenant):**

| New Schema | Owner Role |
|---|---|
| `mse_reference` | `garudax_mse_reference_svc` |
| `mse_participants` | `garudax_mse_participants_svc` |
| `mse_exchange` | `garudax_mse_exchange_svc` |
| `mse_clearing` | `garudax_mse_clearing_svc` |
| `mse_compliance` | `garudax_mse_compliance_svc` |
| `mse_securities` | `garudax_mse_exchange_svc` |
| `mse_market_data` | `garudax_mse_market_data_svc` |

**Platform Schemas (shared):**

| Schema | Purpose | Owner Role |
|---|---|---|
| `auth` | Identity, sessions, RBAC (platform-level) | `garudax_auth_svc` |
| `platform` | Tenant registry, platform audit | `garudax_platform_svc` |

### 3.2 `platform.audit` Table

```sql
CREATE TABLE platform.audit (
    audit_id            VARCHAR(64) PRIMARY KEY,           -- UUID v7
    tenant_id           VARCHAR(64) NOT NULL,              -- 'platform' for platform-level events
    actor_id            VARCHAR(64) NOT NULL,              -- User or service account ID
    actor_type          VARCHAR(20) NOT NULL               -- 'user', 'service', 'platform-admin'
                        CHECK (actor_type IN ('user', 'service', 'platform-admin', 'system')),
    action              VARCHAR(100) NOT NULL,             -- 'tenant.created', 'tenant.suspended', 'cross-tenant-query'
    resource_type       VARCHAR(100) NOT NULL,             -- 'tenant', 'schema', 'config', 'role'
    resource_id         VARCHAR(255),                      -- Identifier of the affected resource
    details             JSONB NOT NULL DEFAULT '{}',
    ip_address          INET,
    user_agent          TEXT,
    prev_hash           VARCHAR(64),                       -- SHA-256 of previous audit entry (hash chain)
    entry_hash          VARCHAR(64) NOT NULL,              -- SHA-256 of this entry (hash chain)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_platform_audit_tenant ON platform.audit (tenant_id);
CREATE INDEX idx_platform_audit_actor ON platform.audit (actor_id);
CREATE INDEX idx_platform_audit_action ON platform.audit (action);
CREATE INDEX idx_platform_audit_created ON platform.audit (created_at);

-- Append-only protection
CREATE RULE no_update_platform_audit AS ON UPDATE TO platform.audit DO INSTEAD NOTHING;
CREATE RULE no_delete_platform_audit AS ON DELETE TO platform.audit DO INSTEAD NOTHING;
```

### 3.3 Defense-in-Depth: `tenant_id` Column

Every table within a tenant schema includes a `tenant_id` column, even though it is already within a tenant-prefixed schema. This is defense-in-depth -- it prevents bugs where a service accidentally queries the wrong schema.

```sql
-- Example: ace_exchange.orders (after migration)
ALTER TABLE ace_exchange.orders
    ADD COLUMN tenant_id VARCHAR(64) NOT NULL DEFAULT 'ace-commodities';

-- Example: mse_exchange.orders (new tenant)
CREATE TABLE mse_exchange.orders (
    id              VARCHAR(64) PRIMARY KEY,
    tenant_id       VARCHAR(64) NOT NULL DEFAULT 'mse-equities',
    -- ... all other columns ...
    CONSTRAINT chk_tenant CHECK (tenant_id = 'mse-equities')
);
```

**Rules:**
- `tenant_id` column is `NOT NULL` on every table in every tenant schema.
- A `CHECK` constraint enforces that only the correct tenant ID can be written.
- The `auth` schema does NOT have `tenant_id` columns (it is platform-level; tenant scoping is done via JWT claims).
- `platform.tenants` and `platform.audit` use `tenant_id` as a reference column, not a self-referencing constraint.

### 3.4 Cross-Tenant Query Policy

```sql
-- Platform-admin role (only role that can query across schemas)
CREATE ROLE garudax_platform_admin LOGIN;

-- Grant read-only on all tenant schemas to platform-admin
-- (applied per-schema during tenant provisioning)
GRANT USAGE ON SCHEMA ace_exchange TO garudax_platform_admin;
GRANT SELECT ON ALL TABLES IN SCHEMA ace_exchange TO garudax_platform_admin;
-- Repeat for each tenant schema...

-- Service roles are restricted to their own tenant schemas
-- garudax_ace_exchange_svc can ONLY access ace_* schemas
-- garudax_mse_exchange_svc can ONLY access mse_* schemas
```

Every cross-tenant query is logged to `platform.audit` with `action = 'cross-tenant-query'`.

### 3.5 Row-Level Security (RLS)

For additional protection, RLS policies enforce tenant isolation at the database level:

```sql
-- Enable RLS on tenant tables
ALTER TABLE ace_exchange.orders ENABLE ROW LEVEL SECURITY;

-- Policy: service roles can only see their own tenant's rows
CREATE POLICY tenant_isolation ON ace_exchange.orders
    USING (tenant_id = current_setting('app.tenant_id')::VARCHAR);

-- Platform admin bypasses RLS
ALTER TABLE ace_exchange.orders FORCE ROW LEVEL SECURITY;
-- Platform admin role is granted BYPASSRLS
ALTER ROLE garudax_platform_admin BYPASSRLS;
```

Services set the session variable before executing queries:

```sql
SET app.tenant_id = 'ace-commodities';
```

---

## 4. Tenant Context Propagation

### 4.1 HTTP Header Format

Every HTTP request to a GarudaX service must include the tenant context header:

```
X-GarudaX-Tenant: {tenant_id}:{timestamp_unix_ms}:{hmac_sha256_signature}
```

**Fields:**
- `tenant_id`: Lowercase slug, e.g., `ace-commodities`
- `timestamp_unix_ms`: Unix epoch in milliseconds, e.g., `1714000000000`
- `hmac_sha256_signature`: HMAC-SHA256 of `{tenant_id}:{timestamp_unix_ms}` using the per-environment signing key

**Example:**
```
X-GarudaX-Tenant: ace-commodities:1714000000000:a1b2c3d4e5f6...
```

**HMAC Key Management:**
- One HMAC key per environment (dev, staging, production)
- Stored in AWS Secrets Manager: `garudax/{env}/tenant-signing-key`
- Rotated quarterly with 24-hour overlap window
- Key format: 256-bit random, hex-encoded

**Validation Rules:**
- Reject if header is missing → HTTP 400 `{"error":{"code":"MISSING_TENANT_CONTEXT","message":"X-GarudaX-Tenant header is required"}}`
- Reject if timestamp is more than 5 minutes old → HTTP 400 `{"error":{"code":"TENANT_CONTEXT_EXPIRED","message":"Tenant context timestamp expired"}}`
- Reject if HMAC signature is invalid → HTTP 403 `{"error":{"code":"TENANT_CONTEXT_INVALID","message":"Tenant context signature verification failed"}}`
- Reject if tenant_id does not exist in `platform.tenants` → HTTP 404 `{"error":{"code":"TENANT_NOT_FOUND","message":"Unknown tenant"}}`
- Reject if tenant status is not `ACTIVE` (except for platform-admin endpoints) → HTTP 403 `{"error":{"code":"TENANT_INACTIVE","message":"Tenant is not active"}}`

### 4.2 Go Middleware Pseudo-Code

```go
package middleware

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    "time"
)

type tenantCtxKey struct{}

type TenantContext struct {
    TenantID  string
    Timestamp time.Time
    Verified  bool
}

// TenantFromContext extracts the tenant context from ctx.
// Panics if no tenant context is set -- this is intentional.
// A request without a tenant context is a bug.
func TenantFromContext(ctx context.Context) TenantContext {
    tc, ok := ctx.Value(tenantCtxKey{}).(TenantContext)
    if !ok {
        panic("TenantFromContext called without tenant context -- this is a bug")
    }
    return tc
}

// WithTenant injects tenant context into ctx.
func WithTenant(ctx context.Context, tc TenantContext) context.Context {
    return context.WithValue(ctx, tenantCtxKey{}, tc)
}

// TenantMiddleware validates X-GarudaX-Tenant header and injects tenant into context.
func TenantMiddleware(hmacKey []byte, tenantStore TenantStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            header := r.Header.Get("X-GarudaX-Tenant")
            if header == "" {
                http.Error(w, `{"error":{"code":"MISSING_TENANT_CONTEXT"}}`, 400)
                return
            }

            parts := strings.SplitN(header, ":", 3)
            if len(parts) != 3 {
                http.Error(w, `{"error":{"code":"TENANT_CONTEXT_INVALID"}}`, 403)
                return
            }

            tenantID := parts[0]
            tsStr := parts[1]
            sig := parts[2]

            // Verify HMAC
            mac := hmac.New(sha256.New, hmacKey)
            mac.Write([]byte(tenantID + ":" + tsStr))
            expectedSig := hex.EncodeToString(mac.Sum(nil))
            if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
                http.Error(w, `{"error":{"code":"TENANT_CONTEXT_INVALID"}}`, 403)
                return
            }

            // Verify timestamp freshness (5 min window)
            tsMs, err := strconv.ParseInt(tsStr, 10, 64)
            if err != nil {
                http.Error(w, `{"error":{"code":"TENANT_CONTEXT_INVALID"}}`, 403)
                return
            }
            ts := time.UnixMilli(tsMs)
            if time.Since(ts) > 5*time.Minute {
                http.Error(w, `{"error":{"code":"TENANT_CONTEXT_EXPIRED"}}`, 400)
                return
            }

            // Verify tenant exists and is active
            tenant, err := tenantStore.Get(r.Context(), tenantID)
            if err != nil || tenant == nil {
                http.Error(w, `{"error":{"code":"TENANT_NOT_FOUND"}}`, 404)
                return
            }
            if tenant.Status != "ACTIVE" {
                http.Error(w, `{"error":{"code":"TENANT_INACTIVE"}}`, 403)
                return
            }

            // Inject tenant into context
            ctx := WithTenant(r.Context(), TenantContext{
                TenantID:  tenantID,
                Timestamp: ts,
                Verified:  true,
            })

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// TenantStore interface for tenant registry lookups
type TenantStore interface {
    Get(ctx context.Context, tenantID string) (*Tenant, error)
}

type Tenant struct {
    TenantID  string
    Status    string
    IsFlagship bool
}
```

### 4.3 Kafka Topic Naming Convention

```
{tenant_id}.{domain}.{event-type}
```

**Current topics (pre-migration, using `ace.*` prefix):**

| Current Topic | Renamed Topic |
|---|---|
| `ace.trades.executed` | `ace-commodities.trades.executed` |
| `ace.clearing.novated` | `ace-commodities.clearing.novated` |
| `ace.margin.call-issued` | `ace-commodities.margin.call-issued` |
| `ace.settlement.completed` | `ace-commodities.settlement.completed` |
| `ace.compliance.status-changed` | `ace-commodities.compliance.status-changed` |
| `ace.market-data.trade-ingested` | `ace-commodities.market-data.trade-ingested` |
| `ace.warehouse.receipt-pledged` | `ace-commodities.warehouse.receipt-pledged` |
| `ace.warehouse.delivery-completed` | `ace-commodities.warehouse.delivery-completed` |
| `ace.auth.user-registered` | `ace-commodities.auth.user-registered` |
| `ace.securities.trade-executed` | `ace-commodities.securities.trade-executed` |
| `ace.securities.order-created` | `ace-commodities.securities.order-created` |
| `ace.securities.settlement-instructed` | `ace-commodities.securities.settlement-instructed` |
| `ace.securities.settlement-completed` | `ace-commodities.securities.settlement-completed` |
| `ace.securities.settlement-failed` | `ace-commodities.securities.settlement-failed` |
| `ace.securities.corporate-action-announced` | `ace-commodities.securities.corporate-action-announced` |
| `ace.securities.corporate-action-processed` | `ace-commodities.securities.corporate-action-processed` |
| `ace.securities.large-trader-report` | `ace-commodities.securities.large-trader-report` |
| `ace.securities.ssr-triggered` | `ace-commodities.securities.ssr-triggered` |

**MSE topics (new):**

| Topic | Partitions | Retention |
|---|---|---|
| `mse-equities.trades.executed` | 16 | 7 days |
| `mse-equities.clearing.novated` | 8 | 7 days |
| `mse-equities.margin.call-issued` | 8 | 7 days |
| `mse-equities.settlement.completed` | 8 | 7 days |
| `mse-equities.compliance.status-changed` | 4 | 7 days |
| `mse-equities.market-data.trade-ingested` | 16 | 7 days |
| `mse-equities.securities.trade-executed` | 16 | 7 days |
| `mse-equities.securities.order-created` | 16 | 7 days |
| `mse-equities.securities.settlement-instructed` | 8 | 7 days |
| `mse-equities.securities.settlement-completed` | 8 | 7 days |
| `mse-equities.securities.settlement-failed` | 4 | 7 days |
| `mse-equities.securities.corporate-action-announced` | 4 | 7 days |
| `mse-equities.securities.corporate-action-processed` | 4 | 7 days |
| `mse-equities.securities.large-trader-report` | 4 | 7 days |
| `mse-equities.securities.ssr-triggered` | 4 | 7 days |

**DLQ topics:** `{tenant_id}.dlq.{domain}.{event-type}`

**Event envelope update:** Every event envelope gains a mandatory `tenant_id` field:

```json
{
  "id": "evt-uuid-001",
  "type": "mse-equities.securities.trade-executed",
  "tenant_id": "mse-equities",
  "timestamp": "2026-04-23T09:15:00.456Z",
  "source": "matching-engine",
  "correlation_id": "corr-uuid-001",
  "schema_version": 2,
  "payload": { ... }
}
```

### 4.4 gRPC Metadata

Tenant context is propagated via gRPC metadata:

```
garudax-tenant-id: ace-commodities
```

**Go gRPC interceptor:**

```go
func TenantUnaryInterceptor(hmacKey []byte) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return nil, status.Error(codes.InvalidArgument, "missing metadata")
        }

        tenantIDs := md.Get("garudax-tenant-id")
        if len(tenantIDs) == 0 {
            return nil, status.Error(codes.InvalidArgument, "missing garudax-tenant-id metadata")
        }

        tenantID := tenantIDs[0]
        ctx = WithTenant(ctx, TenantContext{TenantID: tenantID, Verified: true})

        return handler(ctx, req)
    }
}
```

### 4.5 Observability

Every log line, metric, and trace carries `tenant_id` as a required label:

```go
// Structured logging -- tenant_id is always the first field
logger.Info("order placed",
    "tenant_id", tc.TenantID,
    "order_id", orderID,
    "instrument_id", instrumentID,
)

// Prometheus metrics
orderCounter.WithLabelValues(tc.TenantID, instrumentID, side).Inc()

// OpenTelemetry span attributes
span.SetAttributes(
    attribute.String("garudax.tenant_id", tc.TenantID),
    attribute.String("garudax.service", serviceName),
)
```

**Grafana dashboards** are tenant-scoped by default. A dashboard variable `$tenant_id` filters all panels. Platform-level dashboards (showing cross-tenant aggregates) are accessible only to platform operators.

---

## 5. Identity & Auth

### 5.1 Platform-Level Auth Schema

The `auth` schema is platform-level -- it is NOT duplicated per tenant. Users may have roles in multiple tenants. A single JWT carries claims for all tenants the user has access to.

### 5.2 JWT Structure

```json
{
  "sub": "user-uuid-001",
  "iss": "garudax-auth",
  "iat": 1714000000,
  "exp": 1714003600,
  "jti": "jwt-uuid-001",
  "name": "Bat-Erdene Dorj",
  "email": "bat@mse.mn",
  "tenant_roles": {
    "ace-commodities": ["admin", "trader"],
    "mse-equities": ["viewer"]
  },
  "platform_roles": [],
  "active_tenant": "mse-equities"
}
```

**Fields:**
- `sub`: User UUID (immutable)
- `iss`: Always `garudax-auth`
- `tenant_roles`: Map of tenant ID to role array. Empty array means no access to that tenant.
- `platform_roles`: Array of platform-level roles. Only `platform-admin` exists currently. Empty for all non-platform users.
- `active_tenant`: The tenant the user selected at login or via tenant switcher. Informational only -- actual authorization uses the `X-GarudaX-Tenant` header.

**Roles per tenant:**
| Role | Permissions |
|---|---|
| `viewer` | Read-only access to market data, instruments, own positions |
| `trader` | Submit/cancel orders, view positions, affirm settlements |
| `clearing_admin` | Manage settlement obligations, trigger netting, initiate buy-ins |
| `exchange_admin` | Manage instruments, halt/resume trading, configure circuit breakers |
| `admin` | All of the above for the tenant |

**Platform roles:**
| Role | Permissions |
|---|---|
| `platform-admin` | Tenant lifecycle, cross-tenant queries, platform config changes |

### 5.3 Token Validation Flow

```
1. Extract JWT from Authorization: Bearer header
2. Validate JWT signature (RS256, platform-level public key)
3. Check exp > now
4. Extract tenant_id from X-GarudaX-Tenant header
5. Verify HMAC signature on tenant header (see Section 4.1)
6. Look up tenant_roles[tenant_id] in JWT claims
7. If tenant_roles[tenant_id] does not exist or is empty → 403 Forbidden
8. Check if user has required role for the endpoint → 403 if not
9. Inject TenantContext + UserContext into request context
```

**Cross-tenant access:**
- A user with roles in multiple tenants can only access one tenant per request.
- The active tenant is determined by the `X-GarudaX-Tenant` header, not the JWT's `active_tenant` field.
- Platform-admin role bypasses tenant role checks but still requires a valid `X-GarudaX-Tenant` header (to scope the query).

### 5.4 IRSA (IAM Roles for Service Accounts)

**Naming convention:**
```
garudax-{tenant_id}-{service}
```

**Examples:**
| IRSA Role | Service | Tenant |
|---|---|---|
| `garudax-ace-commodities-matching-engine` | matching-engine | ace-commodities |
| `garudax-ace-commodities-clearing-engine` | clearing-engine | ace-commodities |
| `garudax-mse-equities-matching-engine` | matching-engine | mse-equities |
| `garudax-mse-equities-clearing-engine` | clearing-engine | mse-equities |
| `garudax-platform-auth-service` | auth-service | platform (shared) |
| `garudax-platform-control-plane` | control-plane | platform (shared) |

**Rules:**
- No service role spans tenants. A service instance running for `ace-commodities` cannot assume the `mse-equities` role.
- Platform services (`auth-service`, `control-plane`) use the `garudax-platform-*` prefix.
- Each IRSA role has access only to the AWS resources (S3 paths, KMS keys, Secrets Manager entries) belonging to its tenant.

### 5.5 KMS CMK Per Tenant

Each tenant has its own AWS KMS Customer Master Key (CMK):

| Tenant | CMK Alias | Purpose |
|---|---|---|
| `ace-commodities` | `alias/garudax/ace-commodities/data-key` | Encrypts ace-commodities data at rest |
| `mse-equities` | `alias/garudax/mse-equities/data-key` | Encrypts mse-equities data at rest |
| Platform | `alias/garudax/platform/data-key` | Encrypts platform.audit, auth data |

Key rotation is per tenant. Tenant A's key rotation does not affect Tenant B.

---

## 6. Platform Control Plane

### 6.1 Tenant Registry Service

The control plane is a Go service that manages tenant lifecycle. It exposes a REST API at `/platform/v1/tenants`, accessible only to users with `platform-admin` role.

**API Surface:**

| Method | Path | Description |
|---|---|---|
| `POST` | `/platform/v1/tenants` | Create a new tenant (starts ONBOARDING) |
| `GET` | `/platform/v1/tenants` | List all tenants |
| `GET` | `/platform/v1/tenants/{tenant_id}` | Get tenant details |
| `PATCH` | `/platform/v1/tenants/{tenant_id}` | Update tenant metadata |
| `POST` | `/platform/v1/tenants/{tenant_id}/activate` | Transition ONBOARDING -> ACTIVE |
| `POST` | `/platform/v1/tenants/{tenant_id}/suspend` | Transition ACTIVE -> SUSPENDED |
| `POST` | `/platform/v1/tenants/{tenant_id}/reactivate` | Transition SUSPENDED -> ACTIVE |
| `POST` | `/platform/v1/tenants/{tenant_id}/decommission` | Transition to DECOMMISSIONED |
| `GET` | `/platform/v1/tenants/{tenant_id}/health` | Tenant-level health check |
| `GET` | `/platform/v1/audit` | Query platform audit log |

### 6.2 Tenant Provisioning Workflow

When `POST /platform/v1/tenants` is called, the control plane executes the following steps in order:

```
Step 1: Validate tenant_id format and uniqueness
Step 2: INSERT into platform.tenants (status = 'ONBOARDING')
Step 3: Create DB schemas
         → CREATE SCHEMA {tenant_id}_reference
         → CREATE SCHEMA {tenant_id}_participants
         → CREATE SCHEMA {tenant_id}_exchange
         → CREATE SCHEMA {tenant_id}_clearing
         → CREATE SCHEMA {tenant_id}_compliance
         → CREATE SCHEMA {tenant_id}_market_data
         → CREATE SCHEMA {tenant_id}_securities (if asset_classes includes EQUITY/BOND/ETF)
         → CREATE SCHEMA {tenant_id}_warehouse (if asset_classes includes COMMODITY)
         → Run Flyway migrations for tenant schemas
Step 4: Create Kafka topics
         → {tenant_id}.trades.executed (16 partitions)
         → {tenant_id}.clearing.novated (8 partitions)
         → {tenant_id}.margin.call-issued (8 partitions)
         → {tenant_id}.settlement.completed (8 partitions)
         → {tenant_id}.compliance.status-changed (4 partitions)
         → {tenant_id}.market-data.trade-ingested (16 partitions)
         → (conditional) {tenant_id}.securities.* topics
         → (conditional) {tenant_id}.warehouse.* topics
         → DLQ topics for all of the above
Step 5: Create Redis keyspace
         → Reserve prefix: {tenant_id}:*
         → Create rate-limit keys: {tenant_id}:ratelimit:*
         → Create session keys: {tenant_id}:session:*
         → Create cache keys: {tenant_id}:cache:*
Step 6: Create IAM roles
         → garudax-{tenant_id}-matching-engine
         → garudax-{tenant_id}-clearing-engine
         → garudax-{tenant_id}-settlement-engine
         → garudax-{tenant_id}-margin-engine
         → garudax-{tenant_id}-compliance-service
         → garudax-{tenant_id}-market-data-service
         → garudax-{tenant_id}-gateway
         → (conditional) garudax-{tenant_id}-warehouse-service
Step 7: Create KMS CMK
         → alias/garudax/{tenant_id}/data-key
         → Grant encrypt/decrypt to tenant IRSA roles
Step 8: Update tenant config
         → Insert default trading calendar
         → Insert default circuit breaker thresholds
         → Insert default KYC policies
         → Insert default position limits
Step 9: Log to platform.audit
         → action = 'tenant.created'
Step 10: Return tenant object (status = 'ONBOARDING')
```

**Activation** (`POST /platform/v1/tenants/{tenant_id}/activate`):
- Validates all provisioning steps completed
- Runs health check on all tenant resources
- Updates status to `ACTIVE`
- Logs to platform.audit

### 6.3 Platform-Admin UI

The admin dashboard (`src/admin-ui/`) gains a new tab: **Platform Management**, accessible only to users with `platform-admin` role.

**Pages:**
- Tenant list: shows all tenants with status badges
- Tenant detail: config editor, health status, resource inventory
- Provision wizard: step-by-step tenant creation
- Audit log viewer: searchable platform audit events
- Tenant switcher: global dropdown to switch tenant context

---

## 7. Operational Isolation

### 7.1 Per-Tenant Circuit Breakers

Each tenant has independent circuit breaker configuration:

```go
type TenantCircuitBreaker struct {
    TenantID         string
    InstrumentID     string
    PriceBandPct     Decimal   // Max price deviation from reference
    VolumeThreshold  int64     // Orders per second before throttle
    HaltOnBreach     bool      // Auto-halt on circuit breaker breach
    CooldownMinutes  int       // Minutes before auto-resume
}
```

A circuit breaker triggering on `ace-commodities` does NOT affect `mse-equities` trading.

### 7.2 Per-Tenant Market Phases

Each tenant has its own market phase state machine:

```
PRE_OPEN → OPENING_AUCTION → CONTINUOUS → CLOSING_AUCTION → CLOSED → POST_CLOSE
```

MSE's market phase schedule is independent of ACE's. MSE can be in `OPENING_AUCTION` while ACE is in `CONTINUOUS`.

### 7.3 Per-Tenant Health Checks

The platform health endpoint `/platform/v1/health` returns per-tenant health:

```json
{
  "platform": "healthy",
  "tenants": {
    "ace-commodities": {
      "status": "healthy",
      "services": {
        "matching-engine": "healthy",
        "clearing-engine": "healthy",
        "settlement-engine": "degraded"
      },
      "db": "healthy",
      "kafka": "healthy",
      "redis": "healthy"
    },
    "mse-equities": {
      "status": "healthy",
      "services": { ... }
    }
  }
}
```

### 7.4 Kafka Consumer Groups

Consumer group naming includes tenant ID:

```
{tenant_id}-{service}-consumer
```

**Examples:**
| Consumer Group | Topic | Tenant |
|---|---|---|
| `ace-commodities-clearing-engine-consumer` | `ace-commodities.trades.executed` | ace-commodities |
| `ace-commodities-settlement-engine-consumer` | `ace-commodities.clearing.novated` | ace-commodities |
| `mse-equities-clearing-engine-consumer` | `mse-equities.trades.executed` | mse-equities |
| `mse-equities-settlement-engine-consumer` | `mse-equities.clearing.novated` | mse-equities |

A consumer group for one tenant is completely isolated from another tenant's consumer groups. Consumer lag on `ace-commodities-clearing-engine-consumer` does not affect `mse-equities-clearing-engine-consumer`.

### 7.5 Redis Keyspace Isolation

All Redis keys are prefixed with the tenant ID:

```
{tenant_id}:ratelimit:{user_id}:{endpoint}
{tenant_id}:session:{session_id}
{tenant_id}:cache:instrument:{instrument_id}
{tenant_id}:orderbook:{instrument_id}
{tenant_id}:market-phase
```

**Examples:**
```
ace-commodities:ratelimit:user-001:/api/v1/orders
mse-equities:ratelimit:user-002:/api/v1/securities/orders
ace-commodities:market-phase   → "CONTINUOUS"
mse-equities:market-phase      → "OPENING_AUCTION"
```

**Enforcement:** Services construct Redis keys using `fmt.Sprintf("%s:%s", tc.TenantID, key)`. There is no "bare" key without a tenant prefix.

### 7.6 Failure Isolation

| Failure Scenario | Impact on Other Tenants |
|---|---|
| `ace-commodities` matching engine crashes | `mse-equities` matching engine continues unaffected |
| `mse-equities` Kafka consumer lag spikes | `ace-commodities` consumers on different consumer groups, unaffected |
| `ace-commodities` settlement engine halted for bug fix | `mse-equities` settlement continues |
| Redis keyspace for `ace-commodities` exhausted | `mse-equities` keys in separate prefix, unaffected (but same Redis cluster -- monitor for OOM) |
| Database connection pool exhausted by `mse-equities` | Connection pooling is per-service-role; `ace` roles have separate pools |

---

## 8. Governance Isolation

### 8.1 Per-Tenant Config Store

Each tenant has its own configuration namespace in the database:

```sql
CREATE TABLE platform.tenant_config (
    tenant_id           VARCHAR(64) NOT NULL REFERENCES platform.tenants(tenant_id),
    config_key          VARCHAR(255) NOT NULL,
    config_value        JSONB NOT NULL,
    config_version      INT NOT NULL DEFAULT 1,
    updated_by          VARCHAR(64) NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, config_key)
);

CREATE INDEX idx_tenant_config_tenant ON platform.tenant_config (tenant_id);
```

### 8.2 Tenant-Specific Configuration Items

| Config Key | Description | Example (ACE) | Example (MSE) |
|---|---|---|---|
| `trading_calendar` | Holidays, trading hours, half-days | Mon-Fri 10:00-15:00 UB, MN holidays | Mon-Fri 09:00-15:00 UB, MN holidays |
| `circuit_breaker.price_band_default` | Default price band % | 5.0% | 10.0% |
| `circuit_breaker.volume_threshold` | Orders/second before throttle | 1000 | 5000 |
| `settlement.default_cycle` | Default settlement cycle | T+0 (daily MtM) | T+2 (rolling) |
| `settlement.affirmation_deadline` | Deadline for trade affirmation | N/A | T+1 16:00 local |
| `settlement.fail_penalty_rates` | Penalty rates by asset class | N/A | `{"EQUITY":0.40,"GOVT_BOND":0.25,"CORP_BOND":0.50}` bps/day |
| `settlement.buy_in_trigger_day` | Days after intended settlement | N/A | T+4 |
| `kyc.required_documents` | Documents required for KYC | Business registration | Personal ID, Bank statement |
| `kyc.auto_approve_threshold` | Risk score below which auto-approve | 30 | 20 |
| `listing_rules.min_market_cap` | Minimum market cap for listing | N/A | 1,000,000,000 MNT |
| `listing_rules.min_shareholders` | Minimum number of shareholders | N/A | 100 |
| `position_limits.default_concentration_pct` | Default max % of outstanding | N/A | 5.0% |
| `position_limits.large_trader_threshold` | Reporting threshold shares | N/A | 100,000 |
| `margin.initial_equity_pct` | Initial margin for equities | N/A | 20.0% |
| `margin.initial_bond_pct` | Initial margin for bonds | N/A | 5.0% |
| `auction.opening_duration_minutes` | Opening auction duration | N/A | 30 |
| `auction.closing_duration_minutes` | Closing auction duration | N/A | 10 |
| `short_selling.enabled` | Whether short selling is allowed | false | true |
| `short_selling.locate_required` | Whether locate is required | N/A | true |
| `short_selling.uptick_rule_enabled` | Whether SSR is enabled | N/A | true |

### 8.3 Platform Configuration

Platform-level config lives in a separate table and requires a governance workflow (2-person approval for FLAGSHIP tier changes):

```sql
CREATE TABLE platform.platform_config (
    config_key          VARCHAR(255) PRIMARY KEY,
    config_value        JSONB NOT NULL,
    config_version      INT NOT NULL DEFAULT 1,
    requires_approval   BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by          VARCHAR(64) NOT NULL,
    approved_by         VARCHAR(64),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Platform config items:**
| Config Key | Description |
|---|---|
| `auth.jwt_signing_algorithm` | RS256 |
| `auth.token_expiry_minutes` | 60 |
| `auth.refresh_token_expiry_days` | 30 |
| `tenant.max_tenants` | Maximum number of tenants (license limit) |
| `tenant.default_governance_tier` | STANDARD |
| `observability.log_retention_days` | 90 |
| `observability.metric_retention_days` | 365 |

---

## 9. Agent Fabric Tenancy

### 9.1 MCP Tool Tenant Parameter

Every MCP tool that interacts with tenant data accepts `tenant_id` as a required parameter:

```json
{
  "tool": "query_orders",
  "parameters": {
    "tenant_id": "mse-equities",
    "instrument_id": "SEC-EQ-001",
    "status": "FILLED",
    "limit": 10
  }
}
```

**Tools that require tenant_id:**
- `query_orders`
- `query_trades`
- `query_positions`
- `query_instruments`
- `query_settlements`
- `query_participants`
- `manage_circuit_breaker`
- `manage_market_phase`
- `query_compliance_events`
- `query_audit_log`
- `manage_risk_parameters`

**Tools that are platform-level (no tenant_id):**
- `list_tenants`
- `get_tenant_health`
- `query_platform_audit`
- `manage_tenant_lifecycle`

### 9.2 Bot Commands Scoped by Tenant

Admin bot commands include tenant context:

```
/instruments mse-equities                → List instruments for MSE
/instruments ace-commodities             → List instruments for ACE
/orders mse-equities --status=FILLED     → List filled orders for MSE
/halt mse-equities SEC-EQ-001            → Halt trading on APU stock at MSE
/phase ace-commodities                   → Show current market phase for ACE
/health                                  → Platform-level health (all tenants)
/health mse-equities                     → MSE-specific health
```

### 9.3 Per-Tenant Post-Mortems

Post-mortem analysis is scoped per tenant:

- An incident on `mse-equities` generates a post-mortem scoped to MSE data only.
- The agent handling the incident has read access to `mse_*` schemas only.
- Cross-tenant pattern extraction (e.g., "did the same bug affect ACE?") is a separate, explicit operation requiring `platform-admin` role.

### 9.4 Agent Context Injection

When an agent is spawned for a tenant-specific task, the Orchestrator injects tenant context:

```json
{
  "task_id": "T201",
  "tenant_id": "mse-equities",
  "agent_role": "coder",
  "constraints": {
    "schemas": ["mse_exchange", "mse_clearing", "mse_securities"],
    "kafka_topics": ["mse-equities.*"],
    "redis_prefix": "mse-equities:",
    "cross_tenant_access": false
  }
}
```

---

## 10. MSE Flagship Requirements

### 10.1 Settlement Profiles

The clearing and settlement engines support per-tenant settlement profiles:

| Profile | Tenant | Cycle | Netting | Fail Management |
|---|---|---|---|---|
| `COMMODITY_DAILY_MTM` | ace-commodities | T+0 (same day) | By instrument | Daily MtM cash settlement |
| `SECURITIES_T2_DVP` | mse-equities | T+2 | By (instrument, settlement_date, participant) | Penalty interest + buy-in at T+4 |
| `SECURITIES_T1_DVP` | mse-equities (configurable) | T+1 | Same as T+2 | Same as T+2 |

The settlement engine selects the profile based on `tenant_id` and `asset_class`:

```go
func (s *SettlementEngine) GetProfile(tenantID string, assetClass string) SettlementProfile {
    config := s.configStore.Get(tenantID, "settlement.profiles")
    return config.ProfileFor(assetClass)
}
```

### 10.2 Corporate Actions Service

A new service (`src/corporate-actions-service/`) processes corporate actions for securities tenants:

- **Dividends:** Snapshot CSD balances on record date, calculate entitlements, generate payment instructions
- **Stock splits:** Adjust instrument price, shares outstanding, all positions, all open orders, all CSD balances
- **Rights issues:** Create temporary rights instrument, credit rights to holders, process exercises
- **Mergers:** Convert old instrument positions to new instrument or cash

This service is tenant-scoped. It reads from `{tenant_id}_securities.corporate_actions` and writes entitlements to `{tenant_id}_securities.corporate_action_entitlements`.

### 10.3 Opening/Closing Auctions (Call Auction)

The matching engine supports two session types, configurable per tenant:

| Session Type | Behavior | Tenant |
|---|---|---|
| `CONTINUOUS` | Standard CLOB, price-time priority, immediate matching | Both (default for ACE) |
| `CALL_AUCTION` | Orders accumulated, single equilibrium price calculated, matched at auction end | MSE (opening/closing) |

**Call auction algorithm:**
1. Collect all orders during auction period (no matching)
2. At auction end, calculate equilibrium price that maximizes executable volume
3. Match all orders at or better than equilibrium price
4. Unmatched orders carry over to continuous session (opening) or expire (closing)

**MSE daily schedule:**
```
09:00-09:30  OPENING_AUCTION (call auction, 30 min)
09:30-13:00  CONTINUOUS (standard CLOB)
13:00-13:10  CLOSING_AUCTION (call auction, 10 min)
13:10-13:30  POST_CLOSE (no trading, settlement processing)
```

### 10.4 Short Selling with Locate

MSE requires locate confirmation before short-sell orders are accepted. The securities architecture (see `docs/securities-architecture.md` sections 4.4 and V26 schema) covers this fully. Key MSE-specific settings:

- `short_selling.enabled`: `true`
- `short_selling.locate_required`: `true`
- `short_selling.uptick_rule_enabled`: `true` (SEC Rule 201 / SSR)
- `short_selling.restricted_list_update_frequency`: daily

### 10.5 FRC Reporting

The Financial Regulatory Commission of Mongolia requires specific reports from MSE:

| Report | Frequency | Format | Contents |
|---|---|---|---|
| Daily Trading Summary | Daily at 14:00 | JSON/CSV | Volume, value, trades by instrument, top movers |
| Large Trader Report | On threshold breach | JSON | Participant ID, instrument, position size, % of outstanding |
| Settlement Fails Report | Daily at 17:00 | JSON | Failed obligations, penalty amounts, buy-in status |
| Suspicious Trading Alert | Real-time | JSON | Patterns flagged by surveillance (wash trading, spoofing, layering) |
| Quarterly Compliance Report | Quarterly | PDF/JSON | KYC status, participant count, complaint summary |

Reports are generated by the compliance service and published to:
- Kafka topic: `mse-equities.compliance.frc-report-generated`
- S3: `s3://garudax-mse-equities-reports/{report_type}/{date}/`

Reports use tenant-specific configurations stored in `platform.tenant_config` with keys prefixed `reporting.frc.*`.

### 10.6 MCSD (Mongolian Central Securities Depository) Integration

MSE settlement integrates with MCSD for securities custody and transfer:

```
GarudaX Settlement Engine → CSD Adapter → MCSD API (ISO 20022)
```

**CSD Adapter interface:**

```go
type CSDAdapter interface {
    // Account management
    CreateCustodyAccount(ctx context.Context, req CreateAccountRequest) (*CustodyAccount, error)
    GetBalance(ctx context.Context, accountID string, instrumentID string) (*Balance, error)

    // Transfers
    InstructDvP(ctx context.Context, req DvPInstruction) (*TransferResponse, error)
    InstructFoP(ctx context.Context, req FoPInstruction) (*TransferResponse, error)
    GetTransferStatus(ctx context.Context, transferID string) (*TransferStatus, error)

    // Corporate actions
    NotifyCorporateAction(ctx context.Context, action CorporateAction) error
    GetEntitlements(ctx context.Context, actionID string) ([]Entitlement, error)
}
```

**Initial implementation:** In-memory stub adapter (all operations succeed immediately). Production adapter communicates with MCSD via ISO 20022 XML messages over HTTPS.

### 10.7 MSE Priority Rule

When a design decision creates friction between MSE and ACE:

1. MSE's preferred approach is implemented as the platform default.
2. ACE receives the previous behavior as a configurable override.
3. The decision is documented in the tenant config with a comment explaining the precedent.
4. Future tenants inherit MSE's defaults unless they explicitly override.

**Examples of MSE-wins decisions:**
- Default settlement cycle: T+2 (MSE preference) -- ACE overrides to T+0
- Default market session: includes opening/closing auctions (MSE preference) -- ACE overrides to continuous-only
- Default surveillance rules: FRC-compatible (MSE preference) -- ACE overrides with MCGA-compatible rules

---

## 11. Migration Strategy

### Phase 0.5: Specifications (This Document)

**Deliverable:** `docs/platform-architecture.md` (this file)
**Status:** Complete
**Output:** Foundational multi-tenant architecture spec covering all 12 sections

### Phase 0.6: ace-commodities Retrofit

**Objective:** Rename existing schemas, topics, and roles to follow tenant-prefixed naming without downtime.

**Step 1: Schema Renames (V29-V30 migrations)**

```sql
-- V29: Platform schemas
CREATE SCHEMA IF NOT EXISTS platform;
-- Create platform.tenants table (DDL from Section 2.1)
-- Create platform.audit table (DDL from Section 3.2)
-- Create platform.tenant_config table (DDL from Section 8.1)
-- Create platform.platform_config table (DDL from Section 8.3)
-- Insert ace-commodities tenant record:
INSERT INTO platform.tenants (tenant_id, display_name, status, is_flagship, governance_tier,
    asset_classes, default_settlement_cycle, primary_currency, timezone, regulatory_body)
VALUES ('ace-commodities', 'ACE Commodity Exchange', 'ACTIVE', FALSE, 'STANDARD',
    ARRAY['COMMODITY'], 'T+0', 'MNT', 'Asia/Ulaanbaatar', 'MCGA');

-- V30: Tenant schema renames
ALTER SCHEMA reference RENAME TO ace_reference;
ALTER SCHEMA participants RENAME TO ace_participants;
ALTER SCHEMA exchange RENAME TO ace_exchange;
ALTER SCHEMA clearing RENAME TO ace_clearing;
ALTER SCHEMA compliance RENAME TO ace_compliance;
ALTER SCHEMA warehouse RENAME TO ace_warehouse;
ALTER SCHEMA market_data RENAME TO ace_market_data;
ALTER SCHEMA securities RENAME TO ace_securities;
-- auth stays as-is (platform-level)

-- Add tenant_id column to all ace_* tables
-- (executed per-table, example for ace_exchange.orders)
ALTER TABLE ace_exchange.orders
    ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(64) NOT NULL DEFAULT 'ace-commodities';
ALTER TABLE ace_exchange.orders
    ADD CONSTRAINT chk_tenant CHECK (tenant_id = 'ace-commodities');
-- Repeat for all tables in all ace_* schemas...

-- Annotate existing audit events
UPDATE ace_compliance.audit_events
    SET tenant_id = 'ace-commodities'
    WHERE tenant_id IS NULL;
```

**Step 2: Code Updates**

All Go services update schema references:
- `exchange.orders` becomes `ace_exchange.orders`
- `clearing.positions` becomes `ace_clearing.positions`
- etc.

Each service reads `GARUDAX_TENANT_ID` from environment variable and uses it to construct schema names:

```go
schemaName := fmt.Sprintf("%s_exchange", tenantID) // "ace_exchange"
```

**Step 3: Kafka Topic Renames (Dual-Write)**

```
Phase A: Create new topics (ace-commodities.trades.executed, etc.)
Phase B: Producers dual-write to old (ace.trades.executed) AND new topic
Phase C: Consumers migrate to new topic (consumer group change)
Phase D: Producers stop writing to old topic
Phase E: Old topics deleted after retention period (7 days)
```

**Step 4: IRSA Role Renames**

```
Phase A: Create new roles (garudax-ace-commodities-matching-engine, etc.)
Phase B: Service accounts assume new roles
Phase C: Old roles (garudax_exchange_svc, etc.) deleted
```

### Phase 0.7: Platform Control Plane Implementation

**Objective:** Build the tenant registry service and platform-admin API.

**Deliverables:**
- `src/control-plane/` -- Go service for tenant lifecycle
- Platform-admin API endpoints (Section 6.1)
- Tenant provisioning workflow (Section 6.2)
- Platform-admin UI tab in admin dashboard
- `TenantMiddleware` and `TenantFromContext` utilities (Section 4.2)
- Integration tests for tenant provisioning

### Phase 0.8: mse-equities Tenant Build

**Objective:** Provision and configure MSE as the flagship tenant.

**Deliverables:**
- Execute provisioning workflow for `mse-equities`
- Configure MSE-specific settings (trading calendar, settlement profiles, FRC reporting, MCSD adapter)
- Corporate actions service
- Call auction support in matching engine
- FRC reporting endpoints
- MCSD adapter stub
- MSE-specific admin dashboard pages

**Each phase is a softhouse run** -- planned by the Planner Agent, executed by Workers in isolated worktrees, reviewed by the Reviewer Agent, and post-mortem'd for learned patterns.

---

## 12. Appendix

### A.1 Schema Rename Map

| Current Schema Name | ace-commodities Schema | Notes |
|---|---|---|
| `reference` | `ace_reference` | Product catalog, units, grades |
| `participants` | `ace_participants` | Brokers, dealers, clearing members |
| `exchange` | `ace_exchange` | Orders, trades, execution reports, instruments |
| `clearing` | `ace_clearing` | Positions, obligations, netting, collateral |
| `compliance` | `ace_compliance` | KYC, AML, audit events, risk scoring |
| `warehouse` | `ace_warehouse` | Warehouse receipts, deliveries, storage |
| `market_data` | `ace_market_data` | OHLCV candles, trade ticks, market statistics |
| `securities` | `ace_securities` | Securities instruments, orders, trades, CSD, corporate actions |
| `auth` | `auth` (unchanged) | Platform-level, NOT renamed |

**mse-equities schemas (new, created during provisioning):**

| Schema | Purpose |
|---|---|
| `mse_reference` | MSE product catalog, instrument reference data |
| `mse_participants` | MSE brokers, clearing members |
| `mse_exchange` | MSE orders, trades, execution reports |
| `mse_clearing` | MSE positions, obligations, netting |
| `mse_compliance` | MSE KYC, AML, FRC reporting |
| `mse_securities` | MSE instruments, CSD accounts, corporate actions |
| `mse_market_data` | MSE OHLCV candles, trade ticks |

Note: `mse_warehouse` is NOT created (MSE does not handle physical commodities).

### A.2 Kafka Topic Rename Map

| Current Topic | New Topic (ace-commodities) | MSE Equivalent |
|---|---|---|
| `ace.trades.executed` | `ace-commodities.trades.executed` | `mse-equities.trades.executed` |
| `ace.clearing.novated` | `ace-commodities.clearing.novated` | `mse-equities.clearing.novated` |
| `ace.margin.call-issued` | `ace-commodities.margin.call-issued` | `mse-equities.margin.call-issued` |
| `ace.settlement.completed` | `ace-commodities.settlement.completed` | `mse-equities.settlement.completed` |
| `ace.compliance.status-changed` | `ace-commodities.compliance.status-changed` | `mse-equities.compliance.status-changed` |
| `ace.market-data.trade-ingested` | `ace-commodities.market-data.trade-ingested` | `mse-equities.market-data.trade-ingested` |
| `ace.warehouse.receipt-pledged` | `ace-commodities.warehouse.receipt-pledged` | N/A (no warehouse for MSE) |
| `ace.warehouse.delivery-completed` | `ace-commodities.warehouse.delivery-completed` | N/A |
| `ace.auth.user-registered` | `ace-commodities.auth.user-registered` | `mse-equities.auth.user-registered` |
| `ace.securities.trade-executed` | `ace-commodities.securities.trade-executed` | `mse-equities.securities.trade-executed` |
| `ace.securities.order-created` | `ace-commodities.securities.order-created` | `mse-equities.securities.order-created` |
| `ace.securities.settlement-instructed` | `ace-commodities.securities.settlement-instructed` | `mse-equities.securities.settlement-instructed` |
| `ace.securities.settlement-completed` | `ace-commodities.securities.settlement-completed` | `mse-equities.securities.settlement-completed` |
| `ace.securities.settlement-failed` | `ace-commodities.securities.settlement-failed` | `mse-equities.securities.settlement-failed` |
| `ace.securities.corporate-action-announced` | `ace-commodities.securities.corporate-action-announced` | `mse-equities.securities.corporate-action-announced` |
| `ace.securities.corporate-action-processed` | `ace-commodities.securities.corporate-action-processed` | `mse-equities.securities.corporate-action-processed` |
| `ace.securities.large-trader-report` | `ace-commodities.securities.large-trader-report` | `mse-equities.securities.large-trader-report` |
| `ace.securities.ssr-triggered` | `ace-commodities.securities.ssr-triggered` | `mse-equities.securities.ssr-triggered` |

**DLQ topics follow the same pattern:**
| Current | New |
|---|---|
| `ace.dlq.trades.executed` | `ace-commodities.dlq.trades.executed` |
| `ace.dlq.securities.trade-executed` | `ace-commodities.dlq.securities.trade-executed` |

### A.3 IRSA Role Naming Convention

**Pattern:** `garudax-{tenant_id}-{service}`

| Current Role | New Role (ace-commodities) | MSE Equivalent |
|---|---|---|
| `garudax_exchange_svc` | `garudax-ace-commodities-exchange` | `garudax-mse-equities-exchange` |
| `garudax_clearing_svc` | `garudax-ace-commodities-clearing` | `garudax-mse-equities-clearing` |
| `garudax_compliance_svc` | `garudax-ace-commodities-compliance` | `garudax-mse-equities-compliance` |
| `garudax_warehouse_svc` | `garudax-ace-commodities-warehouse` | N/A |
| `garudax_auth_svc` | `garudax-platform-auth` (unchanged scope) | `garudax-platform-auth` (shared) |
| `garudax_market_data_svc` | `garudax-ace-commodities-market-data` | `garudax-mse-equities-market-data` |

**Platform roles (not tenant-scoped):**
| Role | Purpose |
|---|---|
| `garudax-platform-auth` | Auth service (shared across all tenants) |
| `garudax-platform-control-plane` | Tenant registry service |
| `garudax_platform_admin` | Human platform administrator (DB role) |

### A.4 What Changes vs What Stays

**STAYS (no changes):**
- AWS dual-region topology (ADR-001) -- Tokyo / Singapore
- EKS cluster configuration (node groups, networking)
- Terraform module structure (extended with `tenant_id` input)
- Tech stack: Go, React, PostgreSQL 15, TimescaleDB, Redis, Kafka, EKS
- Agent collaboration pattern: Planner -> Orchestrator -> Workers -> Reviewer -> PostMortem
- CLOB matching algorithm (price-time priority)
- CCP novation model for clearing
- SPAN margin framework
- FIX 4.4/5.0 protocol support
- Docker Compose local development setup (extended with tenant env vars)
- Test suite structure and coverage targets

**CHANGES:**
- All schema names gain tenant prefix (V29-V30 migrations)
- All Kafka topics gain tenant prefix (dual-write migration)
- All IRSA roles gain tenant prefix (rolling migration)
- All Redis keys gain tenant prefix
- All metrics, logs, and traces gain `tenant_id` label
- JWT gains `tenant_roles` map (replaces flat `roles` array)
- New HTTP header: `X-GarudaX-Tenant` (required on every request)
- New gRPC metadata: `garudax-tenant-id` (required on every call)
- New Kafka event envelope field: `tenant_id` (required, schema_version bumped to 2)
- New Go middleware: `TenantMiddleware` on all HTTP handlers
- New Go interceptor: `TenantUnaryInterceptor` on all gRPC handlers
- New Go context utilities: `TenantFromContext`, `WithTenant`
- New service: control-plane (`src/control-plane/`)
- New service: corporate-actions-service (`src/corporate-actions-service/`)
- New platform schemas: `platform.tenants`, `platform.audit`, `platform.tenant_config`, `platform.platform_config`
- Admin UI gains: tenant switcher, platform management tab
- Bot commands gain: tenant parameter
- MCP tools gain: `tenant_id` parameter

### A.5 Migration Version Map

| Migration | Phase | Description |
|---|---|---|
| V1-V25 | Existing | Core schemas (current, pre-tenant) |
| V26-V28 | Existing | Securities module (current, pre-tenant) |
| V29 | Phase 0.6 | Create `platform` schema + tables |
| V30 | Phase 0.6 | Rename schemas to `ace_*` prefix, add `tenant_id` columns |
| V31 | Phase 0.8 | Create `mse_*` schemas (tenant provisioning) |
| V32+ | Phase 0.8+ | MSE-specific tables, indices, seed data |

---

*This document is the authoritative reference for GarudaX multi-tenant architecture. All implementation agents code directly from this spec. When in doubt, re-read Section 1: GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.*
