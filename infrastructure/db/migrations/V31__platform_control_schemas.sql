-- V31: GarudaX Platform Control Schemas
-- Task DB-1 — "Flyway Migration V6: Platform Control Schemas"
--
-- ─────────────────────────────────────────────
-- VERSION NOTE (V6 → V31)
-- ─────────────────────────────────────────────
-- The originating task requested file "V6__platform_schemas.sql". Version 6 is
-- already in use by V6__exchange_engine_tables.sql, and a duplicate Flyway version
-- makes `flyway validate`/`migrate` fail across every environment. This migration is
-- therefore authored at the next free version, V31. (See also the existing
-- V8__market_data_timescaledb.sql / V9__warehouse_tables.sql historical conflict —
-- duplicate versions are a known breakage class on this project.)
--
-- ─────────────────────────────────────────────
-- SCOPE & RELATIONSHIP TO V29
-- ─────────────────────────────────────────────
-- V29__platform_schemas.sql already provisions `platform.tenants` and `platform.audit`.
-- This migration is intentionally IDEMPOTENT (CREATE ... IF NOT EXISTS, ON CONFLICT
-- DO NOTHING) so it is a safe no-op for those structures on any database that has
-- already applied V29, while remaining self-contained for fresh databases. Its net-new
-- contribution is the platform-level auth control structure described below.
--
-- ─────────────────────────────────────────────
-- AUTH HANDLING (platform.auth → platform-level auth)
-- ─────────────────────────────────────────────
-- The task lists "platform.auth". Per platform-architecture.md §3.4 and the explicit
-- comments in V30, auth is a *top-level* platform-level schema (`auth`, created in V10)
-- and is deliberately NOT renamed or nested under a tenant. We honour that committed
-- decision rather than create a duplicate nested `platform.auth` schema. The platform
-- "auth control" data structure that is genuinely missing is the per-tenant role
-- assignment that realises the directive's "users may hold roles in multiple tenants"
-- design — added here as `platform.tenant_user_roles`.
--
-- Per GarudaX_Strategy_Directive.md and platform-architecture.md §2, §3.

CREATE SCHEMA IF NOT EXISTS platform;

COMMENT ON SCHEMA platform IS
    'Platform control plane — shared across all tenants, NOT tenant-scoped. '
    'Holds the tenant registry, platform-level audit trail, and cross-tenant '
    'access-control structures. Per platform-architecture.md §2/§3.';

-- ─────────────────────────────────────────────
-- platform.tenants  (§2.1) — idempotent, harmonised with V29
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS platform.tenants (
    tenant_id                   VARCHAR(64) PRIMARY KEY,           -- Lowercase slug: 'ace-commodities', 'mse-equities'
    display_name                VARCHAR(255) NOT NULL,             -- Human-readable: 'ACE Commodity Exchange'
    description                 TEXT,
    status                      VARCHAR(20) NOT NULL DEFAULT 'ONBOARDING'
                                    CHECK (status IN ('ONBOARDING', 'ACTIVE', 'SUSPENDED', 'DECOMMISSIONED')),
    is_flagship                 BOOLEAN NOT NULL DEFAULT FALSE,    -- Only one tenant may be flagship (see unique index below)
    governance_tier             VARCHAR(10) NOT NULL DEFAULT 'STANDARD'
                                    CHECK (governance_tier IN ('FLAGSHIP', 'STANDARD', 'SANDBOX')),
    asset_classes               TEXT[] NOT NULL DEFAULT '{}',      -- ARRAY['COMMODITY'] or ARRAY['EQUITY','BOND','ETF']
    default_settlement_cycle    VARCHAR(5) NOT NULL DEFAULT 'T+0' CHECK (default_settlement_cycle IN ('T+0', 'T+1', 'T+2', 'T+3')),
    primary_currency            CHAR(3) NOT NULL DEFAULT 'MNT',
    timezone                    VARCHAR(50) NOT NULL DEFAULT 'Asia/Ulaanbaatar',
    regulatory_body             VARCHAR(100),                      -- 'FRC' for MSE, 'MCGA' for ACE
    kms_cmk_arn                 VARCHAR(255),
    onboarding_metadata         JSONB NOT NULL DEFAULT '{}',
    config_version              INT NOT NULL DEFAULT 1,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at                TIMESTAMPTZ,
    suspended_at                TIMESTAMPTZ,
    decommissioned_at           TIMESTAMPTZ
);

-- Enforce: only one flagship tenant may exist at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_tenants_flagship
    ON platform.tenants (is_flagship) WHERE is_flagship = TRUE;

CREATE INDEX IF NOT EXISTS idx_platform_tenants_status
    ON platform.tenants (status);

-- Seed known tenants (no-op if V29 already inserted them)
INSERT INTO platform.tenants (
    tenant_id, display_name, status, is_flagship, governance_tier,
    asset_classes, default_settlement_cycle, primary_currency, timezone, regulatory_body,
    activated_at
) VALUES (
    'ace-commodities', 'ACE Commodity Exchange', 'ACTIVE', FALSE, 'STANDARD',
    ARRAY['COMMODITY'], 'T+0', 'MNT', 'Asia/Ulaanbaatar', 'MCGA',
    NOW()
) ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO platform.tenants (
    tenant_id, display_name, status, is_flagship, governance_tier,
    asset_classes, default_settlement_cycle, primary_currency, timezone, regulatory_body,
    description
) VALUES (
    'mse-equities', 'Mongolian Stock Exchange', 'ONBOARDING', TRUE, 'FLAGSHIP',
    ARRAY['EQUITY', 'BOND', 'ETF'], 'T+2', 'MNT', 'Asia/Ulaanbaatar', 'FRC',
    'Flagship tenant — platform design decisions defer to MSE requirements per GarudaX_Strategy_Directive.md §4.3'
) ON CONFLICT (tenant_id) DO NOTHING;

-- ─────────────────────────────────────────────
-- platform.audit  (§3.2) — idempotent, harmonised with V29
-- Platform-level, append-only audit log (hash-chained). Separate from each tenant's
-- *_compliance.audit_events. Covers tenant lifecycle, cross-tenant admin actions,
-- and platform config changes.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS platform.audit (
    audit_id        VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    tenant_id       VARCHAR(64) NOT NULL,              -- 'platform' for platform-level events; tenant slug otherwise
    actor_id        VARCHAR(64) NOT NULL,              -- User ID, service account ID, or 'system'
    actor_type      VARCHAR(20) NOT NULL
                        CHECK (actor_type IN ('user', 'service', 'platform-admin', 'system')),
    action          VARCHAR(100) NOT NULL,             -- e.g. 'tenant.created', 'tenant.suspended'
    resource_type   VARCHAR(100) NOT NULL,             -- e.g. 'tenant', 'schema', 'config', 'role'
    resource_id     VARCHAR(255),
    details         JSONB NOT NULL DEFAULT '{}',
    ip_address      INET,
    user_agent      TEXT,
    prev_hash       VARCHAR(64),                       -- SHA-256 of the previous entry — forms a hash chain
    entry_hash      VARCHAR(64) NOT NULL DEFAULT '',   -- SHA-256 of this entry; populated by the application layer
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_platform_audit_tenant_created
    ON platform.audit (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_platform_audit_actor
    ON platform.audit (actor_id);
CREATE INDEX IF NOT EXISTS idx_platform_audit_action
    ON platform.audit (action);
CREATE INDEX IF NOT EXISTS idx_platform_audit_created
    ON platform.audit (created_at DESC);

-- Append-only protection — no row may be updated or deleted after insertion
CREATE OR REPLACE RULE no_update_platform_audit
    AS ON UPDATE TO platform.audit DO INSTEAD NOTHING;
CREATE OR REPLACE RULE no_delete_platform_audit
    AS ON DELETE TO platform.audit DO INSTEAD NOTHING;

-- ─────────────────────────────────────────────
-- Platform-level auth: ensure the top-level `auth` schema exists (§3.4)
-- This is the canonical "platform.auth" — platform-level identity, NOT tenant-scoped.
-- ─────────────────────────────────────────────
CREATE SCHEMA IF NOT EXISTS auth;

COMMENT ON SCHEMA auth IS
    'Platform-level identity and auth — shared across all tenants. '
    'Users may hold roles in multiple tenants (see platform.tenant_user_roles); '
    'JWT carries per-tenant claims. NOT tenant-scoped per platform-architecture.md §3.4.';

-- platform.tenant_user_roles — per-tenant role assignments (the multi-tenant auth control structure)
-- Realises the directive's "users may hold roles in multiple tenants" model by binding
-- platform-level identities (auth.users) to tenants (platform.tenants) with a scoped role.
-- Guarded so it only references auth.users when that table is present (auth.users is created
-- in V10, which always precedes V31 in a complete chain; the guard makes the migration robust
-- if auth tables are provisioned out of band).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'auth' AND table_name = 'users'
    ) THEN
        CREATE TABLE IF NOT EXISTS platform.tenant_user_roles (
            id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id   VARCHAR(64) NOT NULL REFERENCES platform.tenants(tenant_id) ON DELETE CASCADE,
            user_id     UUID        NOT NULL REFERENCES auth.users(id)              ON DELETE CASCADE,
            role        VARCHAR(50) NOT NULL,              -- tenant-scoped role, e.g. 'tenant-admin', 'trader', 'compliance'
            granted_by  VARCHAR(64),                       -- actor that granted the role (user/service id or 'system')
            granted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            revoked     BOOLEAN     NOT NULL DEFAULT FALSE,
            revoked_at  TIMESTAMPTZ,
            CONSTRAINT uq_tenant_user_role UNIQUE (tenant_id, user_id, role)
        );
    ELSE
        -- auth.users not present yet: create the table without the cross-schema FK on users.
        CREATE TABLE IF NOT EXISTS platform.tenant_user_roles (
            id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id   VARCHAR(64) NOT NULL REFERENCES platform.tenants(tenant_id) ON DELETE CASCADE,
            user_id     UUID        NOT NULL,
            role        VARCHAR(50) NOT NULL,
            granted_by  VARCHAR(64),
            granted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            revoked     BOOLEAN     NOT NULL DEFAULT FALSE,
            revoked_at  TIMESTAMPTZ,
            CONSTRAINT uq_tenant_user_role UNIQUE (tenant_id, user_id, role)
        );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_tenant_user_roles_tenant
    ON platform.tenant_user_roles (tenant_id) WHERE revoked = FALSE;
CREATE INDEX IF NOT EXISTS idx_tenant_user_roles_user
    ON platform.tenant_user_roles (user_id) WHERE revoked = FALSE;

COMMENT ON TABLE platform.tenant_user_roles IS
    'Per-tenant role assignments binding platform-level auth.users to platform.tenants. '
    'A single user may appear once per (tenant, role). Drives per-tenant JWT claims (§3.4).';

-- ─────────────────────────────────────────────
-- Service role for the platform control-plane service (Phase 0.7)
-- ─────────────────────────────────────────────
DO $$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_platform_svc') THEN
        CREATE ROLE garudax_platform_svc LOGIN;
    END IF;
END $$;

GRANT USAGE ON SCHEMA platform TO garudax_platform_svc;
GRANT SELECT, INSERT, UPDATE ON platform.tenants            TO garudax_platform_svc;
GRANT SELECT, INSERT         ON platform.audit              TO garudax_platform_svc;  -- INSERT only; UPDATE/DELETE blocked by rules
GRANT SELECT, INSERT, UPDATE ON platform.tenant_user_roles  TO garudax_platform_svc;

-- Read access for existing service roles so they can validate tenant context and roles
DO $$
DECLARE
    svc TEXT;
BEGIN
    FOREACH svc IN ARRAY ARRAY[
        'garudax_exchange_svc', 'garudax_clearing_svc', 'garudax_compliance_svc',
        'garudax_auth_svc', 'garudax_market_data_svc', 'garudax_warehouse_svc'
    ] LOOP
        IF EXISTS (SELECT FROM pg_roles WHERE rolname = svc) THEN
            EXECUTE format('GRANT USAGE ON SCHEMA platform TO %I', svc);
            EXECUTE format('GRANT SELECT ON platform.tenants TO %I', svc);
        END IF;
    END LOOP;

    -- The auth service additionally manages per-tenant role assignments.
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_auth_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON platform.tenant_user_roles TO garudax_auth_svc;
    END IF;
END $$;
