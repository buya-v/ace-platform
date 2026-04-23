-- V29: GarudaX Platform Schemas — Multi-Tenant Foundation
-- Creates platform-level schemas for tenant registry and audit trail.
-- Per GarudaX_Strategy_Directive.md §3.2 and platform-architecture.md §2, §3.
--
-- The `platform` schema is shared across all tenants and is NOT tenant-scoped.
-- The `auth` schema (created in V10) remains platform-level per §3.4.
--
-- Phase 0.6 retrofit sequence: V29 (platform schemas) → V30 (schema renames)

CREATE SCHEMA IF NOT EXISTS platform;

-- ─────────────────────────────────────────────
-- platform.tenants  (§2.1)
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
    -- Asset classes handled by this tenant
    asset_classes               TEXT[] NOT NULL DEFAULT '{}',     -- ARRAY['COMMODITY'] or ARRAY['EQUITY','BOND','ETF']
    -- Settlement
    default_settlement_cycle    VARCHAR(5) NOT NULL DEFAULT 'T+0' CHECK (default_settlement_cycle IN ('T+0', 'T+1', 'T+2', 'T+3')),
    -- Regional / regulatory
    primary_currency            CHAR(3) NOT NULL DEFAULT 'MNT',
    timezone                    VARCHAR(50) NOT NULL DEFAULT 'Asia/Ulaanbaatar',
    regulatory_body             VARCHAR(100),                      -- 'FRC' for MSE, 'MCGA' for ACE
    -- KMS — per-tenant CMK for at-rest encryption
    kms_cmk_arn                 VARCHAR(255),
    -- Flexible onboarding state bag
    onboarding_metadata         JSONB NOT NULL DEFAULT '{}',
    config_version              INT NOT NULL DEFAULT 1,            -- Incremented on every config change
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

-- ─────────────────────────────────────────────
-- Seed known tenants
-- ─────────────────────────────────────────────
-- ace-commodities is live and operational (Phase 0 baseline)
INSERT INTO platform.tenants (
    tenant_id, display_name, status, is_flagship, governance_tier,
    asset_classes, default_settlement_cycle, primary_currency, timezone, regulatory_body,
    activated_at
) VALUES (
    'ace-commodities', 'ACE Commodity Exchange', 'ACTIVE', FALSE, 'STANDARD',
    ARRAY['COMMODITY'], 'T+0', 'MNT', 'Asia/Ulaanbaatar', 'MCGA',
    NOW()
) ON CONFLICT (tenant_id) DO NOTHING;

-- mse-equities is the flagship tenant, currently being onboarded
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
-- platform.audit  (§3.2)
-- Platform-level audit log — separate from each tenant's *_compliance.audit_events.
-- Covers: tenant lifecycle events, cross-tenant admin actions, platform config changes.
-- Append-only: updates and deletes are blocked by rules below.
-- ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS platform.audit (
    audit_id        VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,  -- UUID v4; switch to v7 when available
    tenant_id       VARCHAR(64) NOT NULL,              -- 'platform' for platform-level events; tenant slug otherwise
    actor_id        VARCHAR(64) NOT NULL,              -- User ID, service account ID, or 'system'
    actor_type      VARCHAR(20) NOT NULL
                        CHECK (actor_type IN ('user', 'service', 'platform-admin', 'system')),
    action          VARCHAR(100) NOT NULL,             -- e.g. 'tenant.created', 'tenant.suspended', 'cross-tenant-query'
    resource_type   VARCHAR(100) NOT NULL,             -- e.g. 'tenant', 'schema', 'config', 'role'
    resource_id     VARCHAR(255),                      -- Identifier of the affected resource
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
-- Service role for the platform control-plane service (Phase 0.7)
-- ─────────────────────────────────────────────
DO $$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_platform_svc') THEN
        CREATE ROLE garudax_platform_svc LOGIN;
    END IF;
END $$;

GRANT USAGE ON SCHEMA platform TO garudax_platform_svc;
GRANT SELECT, INSERT, UPDATE ON platform.tenants TO garudax_platform_svc;
GRANT SELECT, INSERT            ON platform.audit   TO garudax_platform_svc;  -- INSERT only; UPDATE/DELETE blocked by rules

-- Read access for existing service roles so they can validate tenant context
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_exchange_svc;
        GRANT SELECT ON platform.tenants TO garudax_exchange_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_clearing_svc;
        GRANT SELECT ON platform.tenants TO garudax_clearing_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_compliance_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_compliance_svc;
        GRANT SELECT ON platform.tenants TO garudax_compliance_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_auth_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_auth_svc;
        GRANT SELECT ON platform.tenants TO garudax_auth_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_market_data_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_market_data_svc;
        GRANT SELECT ON platform.tenants TO garudax_market_data_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_warehouse_svc') THEN
        GRANT USAGE ON SCHEMA platform TO garudax_warehouse_svc;
        GRANT SELECT ON platform.tenants TO garudax_warehouse_svc;
    END IF;
END $$;
