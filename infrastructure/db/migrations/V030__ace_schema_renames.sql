-- V30: Rename single-tenant schemas to ace-commodities tenant namespace
-- Per GarudaX_Strategy_Directive.md §5 — Retrofit Tasks for ace-commodities
-- Per platform-architecture.md §11 (Phase 0.6) and Appendix A.1
--
-- All domain schemas receive the 'ace_' tenant prefix.
-- The `auth` schema is intentionally NOT renamed — it is platform-level (§3.4).
-- The `platform` schema was created in V29 and is also NOT renamed.
--
-- IMPORTANT: Application code must be updated to reference new schema names in the
-- same deployment as this migration. No search_path tricks; services use explicit
-- schema-qualified names: 'ace_exchange.orders', 'ace_clearing.positions', etc.
--
-- Postgres ALTER SCHEMA RENAME acquires an exclusive lock on the schema for the
-- duration of the DDL statement. On a live system, plan a brief maintenance window
-- or execute during off-peak hours. Tables remain accessible in the new schema name
-- immediately after the statement completes.

-- ─────────────────────────────────────────────
-- Core commodity exchange schemas → ace_* namespace
-- ─────────────────────────────────────────────
ALTER SCHEMA reference    RENAME TO ace_reference;
ALTER SCHEMA participants RENAME TO ace_participants;
ALTER SCHEMA exchange     RENAME TO ace_exchange;
ALTER SCHEMA clearing     RENAME TO ace_clearing;
ALTER SCHEMA compliance   RENAME TO ace_compliance;
ALTER SCHEMA warehouse    RENAME TO ace_warehouse;
ALTER SCHEMA market_data  RENAME TO ace_market_data;

-- Securities schema (introduced in V26, extended in V27-V28) — also gets ace_ prefix
ALTER SCHEMA securities   RENAME TO ace_securities;

-- ─────────────────────────────────────────────
-- auth stays as 'auth' — platform-level, shared across all tenants (§3.4)
-- platform stays as 'platform' — created in V29, platform-level
-- ─────────────────────────────────────────────

-- ─────────────────────────────────────────────
-- Schema documentation
-- ─────────────────────────────────────────────
COMMENT ON SCHEMA auth IS
    'Platform-level identity and auth — shared across all tenants. '
    'Users may hold roles in multiple tenants; JWT carries per-tenant claims. '
    'NOT tenant-scoped per platform-architecture.md §3.4.';

COMMENT ON SCHEMA ace_reference IS
    'ace-commodities tenant: commodity reference data — products, grades, units of measure, '
    'pricing references, delivery locations.';

COMMENT ON SCHEMA ace_participants IS
    'ace-commodities tenant: participant management — brokers, dealers, clearing members, '
    'onboarding workflows, KYC state.';

COMMENT ON SCHEMA ace_exchange IS
    'ace-commodities tenant: exchange / trading data — orders, trades, execution reports, '
    'instruments, market sessions, circuit breakers.';

COMMENT ON SCHEMA ace_clearing IS
    'ace-commodities tenant: clearing engine data — positions, obligations, netting runs, '
    'collateral, default fund.';

COMMENT ON SCHEMA ace_compliance IS
    'ace-commodities tenant: compliance data — KYC/AML records, audit event hash chain, '
    'surveillance alerts, risk scoring.';

COMMENT ON SCHEMA ace_warehouse IS
    'ace-commodities tenant: warehouse receipts — eWR issuance, transfers, delivery '
    'instructions, storage locations.';

COMMENT ON SCHEMA ace_market_data IS
    'ace-commodities tenant: market data — OHLCV candles, trade ticks, market statistics, '
    'TimescaleDB hypertables.';

COMMENT ON SCHEMA ace_securities IS
    'ace-commodities tenant: securities trading (Phase 7) — instruments, short-sell '
    'locates, position limits, SSR triggers.';

-- ─────────────────────────────────────────────
-- Update existing service role grants to cover renamed schemas
-- The roles themselves are unchanged in this migration; IRSA role renames
-- are a separate Phase 0.6 step (Terraform, not SQL).
-- ─────────────────────────────────────────────
DO $$ BEGIN
    -- garudax_exchange_svc: exchange + reference + market_data read
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT USAGE ON SCHEMA ace_exchange    TO garudax_exchange_svc;
        GRANT USAGE ON SCHEMA ace_reference   TO garudax_exchange_svc;
        GRANT USAGE ON SCHEMA ace_market_data TO garudax_exchange_svc;
        GRANT USAGE ON SCHEMA ace_securities  TO garudax_exchange_svc;
        GRANT ALL ON ALL TABLES IN SCHEMA ace_exchange    TO garudax_exchange_svc;
        GRANT ALL ON ALL TABLES IN SCHEMA ace_securities  TO garudax_exchange_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_reference   TO garudax_exchange_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_market_data TO garudax_exchange_svc;
    END IF;

    -- garudax_clearing_svc: clearing + exchange read + securities read
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT USAGE ON SCHEMA ace_clearing   TO garudax_clearing_svc;
        GRANT USAGE ON SCHEMA ace_exchange   TO garudax_clearing_svc;
        GRANT USAGE ON SCHEMA ace_securities TO garudax_clearing_svc;
        GRANT ALL    ON ALL TABLES IN SCHEMA ace_clearing   TO garudax_clearing_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_exchange   TO garudax_clearing_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_securities TO garudax_clearing_svc;
    END IF;

    -- garudax_compliance_svc: compliance + participants read
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_compliance_svc') THEN
        GRANT USAGE ON SCHEMA ace_compliance   TO garudax_compliance_svc;
        GRANT USAGE ON SCHEMA ace_participants TO garudax_compliance_svc;
        GRANT ALL    ON ALL TABLES IN SCHEMA ace_compliance   TO garudax_compliance_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_participants TO garudax_compliance_svc;
    END IF;

    -- garudax_warehouse_svc: warehouse only
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_warehouse_svc') THEN
        GRANT USAGE ON SCHEMA ace_warehouse TO garudax_warehouse_svc;
        GRANT ALL   ON ALL TABLES IN SCHEMA ace_warehouse TO garudax_warehouse_svc;
    END IF;

    -- garudax_market_data_svc: market_data + exchange read
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_market_data_svc') THEN
        GRANT USAGE ON SCHEMA ace_market_data TO garudax_market_data_svc;
        GRANT USAGE ON SCHEMA ace_exchange    TO garudax_market_data_svc;
        GRANT ALL    ON ALL TABLES IN SCHEMA ace_market_data TO garudax_market_data_svc;
        GRANT SELECT ON ALL TABLES IN SCHEMA ace_exchange    TO garudax_market_data_svc;
    END IF;
END $$;
