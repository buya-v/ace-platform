-- V33: Rename margin/settlement schemas into the ace-commodities tenant namespace
-- Task RETRO-1 — "Retrofit ACE Commodities Data Access to New Schemas"
--
-- ─────────────────────────────────────────────
-- WHY THIS MIGRATION EXISTS
-- ─────────────────────────────────────────────
-- V30__ace_schema_renames.sql renamed eight domain schemas to the `ace_` namespace
-- (reference, participants, exchange, clearing, compliance, warehouse, market_data,
-- securities). Its header states the rule plainly: "All domain schemas receive the
-- 'ace_' tenant prefix." Two domain schemas were nevertheless missed by V30/V32:
--
--   * `margin`     — created by V13__margin_engine_tables.sql
--   * `settlement` — created by V14__settlement_engine_tables.sql
--
-- They were absent from the platform-architecture.md §3.1 mapping table (which folds
-- the clearing/margin/settlement pipeline under the single "clearing" domain), so the
-- original retrofit run (20260423-ace-retrofit) never listed them and its verification
-- grep never flagged them. The result is a hole in the platform invariant
-- "Postgres schemas are namespaced per tenant" (GarudaX_Strategy_Directive.md §3) —
-- margin and settlement data for ace-commodities sits in un-prefixed, would-be-shared
-- schemas. When mse-equities is built out, `mse_margin` / `mse_settlement` would have
-- nothing to disambiguate against.
--
-- This migration closes that hole, mirroring the per-domain 1:1 `ace_` prefix applied
-- to every other schema:  margin → ace_margin,  settlement → ace_settlement.
-- The margin-engine and settlement-engine stores are updated to the new
-- schema-qualified names in the same deployment (the contract set by V30's header:
-- "Application code must be updated to reference new schema names in the same
-- deployment as this migration").
--
-- auth and platform remain platform-level and are never renamed (§3.4).
--
-- ─────────────────────────────────────────────
-- IDEMPOTENCY
-- ─────────────────────────────────────────────
-- Like V32, the rename only fires when the legacy schema still exists AND its ace_
-- target does not — a clean no-op once applied, self-healing on a partial chain.
-- ALTER SCHEMA RENAME takes a brief exclusive lock; run during off-peak hours.

-- ═════════════════════════════════════════════
-- PART 1 — Idempotent schema renames
-- ═════════════════════════════════════════════
DO $$
DECLARE
    pair   TEXT[];
    pairs  TEXT[][] := ARRAY[
        ARRAY['margin',     'ace_margin'],
        ARRAY['settlement', 'ace_settlement']
    ];
BEGIN
    FOREACH pair SLICE 1 IN ARRAY pairs LOOP
        IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = pair[1])
           AND NOT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = pair[2])
        THEN
            EXECUTE format('ALTER SCHEMA %I RENAME TO %I', pair[1], pair[2]);
            RAISE NOTICE 'V33: renamed schema % -> %', pair[1], pair[2];
        END IF;
    END LOOP;
END $$;

-- ─────────────────────────────────────────────
-- Schema documentation
-- ─────────────────────────────────────────────
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'ace_margin') THEN
        COMMENT ON SCHEMA ace_margin IS
            'ace-commodities tenant: margin engine data — portfolio margin snapshots, '
            'margin calls, margin parameters. Part of the clearing/risk pipeline.';
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'ace_settlement') THEN
        COMMENT ON SCHEMA ace_settlement IS
            'ace-commodities tenant: settlement engine data — settlement cycles, '
            'instructions, settlement prices. Part of the clearing/risk pipeline.';
    END IF;
END $$;

-- ═════════════════════════════════════════════
-- PART 2 — Service role grants (guarded by role existence, as in V30)
-- ═════════════════════════════════════════════
-- margin and settlement are part of the clearing/risk pipeline; the clearing service
-- role owns them, consistent with platform-architecture.md folding margin/settlement
-- under the clearing domain. Grants are no-ops if the schema or role is absent.
DO $$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_clearing_svc')
       AND EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'ace_margin') THEN
        GRANT USAGE ON SCHEMA ace_margin TO garudax_clearing_svc;
        GRANT ALL ON ALL TABLES IN SCHEMA ace_margin TO garudax_clearing_svc;
    END IF;
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_clearing_svc')
       AND EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'ace_settlement') THEN
        GRANT USAGE ON SCHEMA ace_settlement TO garudax_clearing_svc;
        GRANT ALL ON ALL TABLES IN SCHEMA ace_settlement TO garudax_clearing_svc;
    END IF;
END $$;
