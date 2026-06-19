-- V32: ACE Commodities Schema Retrofit — schema renames + audit tenant annotation
-- Task DB-2 — "Flyway Migration V7: ACE Commodities Schema Retrofit"
--
-- ─────────────────────────────────────────────
-- VERSION NOTE (V7 → V32)
-- ─────────────────────────────────────────────
-- The originating task requested file "V7__ace_schema_rename.sql". Version 7 is
-- already in use by V7__kyc_aml_tables.sql, and a duplicate Flyway version makes
-- `flyway validate`/`migrate` fail across every environment. This migration is
-- therefore authored at the next free version, V32. (Compare DB-1's V31, authored
-- for the same reason, and the historical V8__market_data_timescaledb.sql /
-- V9 conflict — duplicate versions are a known breakage class on this project.)
--
-- ─────────────────────────────────────────────
-- SCOPE & RELATIONSHIP TO V30
-- ─────────────────────────────────────────────
-- V30__ace_schema_renames.sql already renames the single-tenant domain schemas to
-- the `ace_` namespace (reference → ace_reference, exchange → ace_exchange, etc.).
-- This migration is intentionally IDEMPOTENT: the rename section only acts when an
-- old-name schema still exists and its ace_-prefixed target does not, so it is a
-- safe no-op on any database that has already applied V30 while remaining
-- self-healing on a database that reached this point without it.
--
-- Net-new contribution: the multi-tenant retrofit's second half — stamping every
-- existing IMMUTABLE audit / event record in the ace tenant's domain schemas with
-- `tenant_id = 'ace-commodities'`, satisfying the platform invariant
-- "Tenant ID is never optional" (GarudaX_Strategy_Directive.md / CLAUDE.md).
--
-- ─────────────────────────────────────────────
-- WHY ADD COLUMN ... DEFAULT (and not UPDATE)
-- ─────────────────────────────────────────────
-- The target tables are append-only: they carry `no_update_*` / `no_delete_*` rules
-- (DO INSTEAD NOTHING). A naive `UPDATE ... SET tenant_id = 'ace-commodities'` would
-- be silently swallowed by those rules and backfill nothing. Instead we ADD the
-- column WITH a constant NOT NULL DEFAULT: PostgreSQL backfills all pre-existing
-- rows as a metadata-only operation (the default is a constant, so no table rewrite
-- on PG 11+), and the rules — which only intercept DML — never apply to DDL. Future
-- inserts inherit the same default, so the column is never optional.
--
-- ─────────────────────────────────────────────
-- WHAT IS DELIBERATELY EXCLUDED
-- ─────────────────────────────────────────────
-- auth.audit_log (V10) is NOT stamped. The `auth` schema is platform-level and
-- shared across all tenants (platform-architecture.md §3.4, V30 header); its audit
-- rows are cross-tenant and must not be branded as ace-commodities. Likewise the
-- platform.audit hash-chain (V29/V31) already carries an explicit tenant_id column.

-- ═════════════════════════════════════════════
-- PART 1 — Idempotent schema renames (harmonised with V30)
-- ═════════════════════════════════════════════
-- Rename only when the legacy schema is present AND the ace_ target is absent.
DO $$
DECLARE
    pair   TEXT[];
    pairs  TEXT[][] := ARRAY[
        ARRAY['reference',    'ace_reference'],
        ARRAY['participants', 'ace_participants'],
        ARRAY['exchange',     'ace_exchange'],
        ARRAY['clearing',     'ace_clearing'],
        ARRAY['compliance',   'ace_compliance'],
        ARRAY['warehouse',    'ace_warehouse'],
        ARRAY['market_data',  'ace_market_data'],
        ARRAY['securities',   'ace_securities']
    ];
BEGIN
    FOREACH pair SLICE 1 IN ARRAY pairs LOOP
        IF EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = pair[1])
           AND NOT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = pair[2])
        THEN
            EXECUTE format('ALTER SCHEMA %I RENAME TO %I', pair[1], pair[2]);
            RAISE NOTICE 'V32: renamed schema % -> %', pair[1], pair[2];
        END IF;
    END LOOP;
END $$;

-- auth and platform are platform-level — intentionally NOT renamed (see header).

-- ═════════════════════════════════════════════
-- PART 2 — Annotate immutable audit / event records with tenant_id
-- ═════════════════════════════════════════════
-- Each entry is a (schema, table) pair holding append-only records in the
-- ace-commodities tenant. Existing rows are backfilled to 'ace-commodities' via the
-- column default; the NOT NULL default keeps the tenant id mandatory for new rows.
-- Tables absent on a partial database are skipped (information_schema guard), so the
-- migration is robust to differing historical chains.
DO $$
DECLARE
    pair   TEXT[];
    tables TEXT[][] := ARRAY[
        -- ace_compliance: KYC/AML immutable audit trail (V7) + audit log (V15)
        ARRAY['ace_compliance', 'screening_results'],
        ARRAY['ace_compliance', 'risk_scores'],
        ARRAY['ace_compliance', 'sar_filings'],
        ARRAY['ace_compliance', 'sar_filings_v2'],
        ARRAY['ace_compliance', 'audit_log'],
        -- ace_exchange: immutable trade & execution event records (V6, V11)
        ARRAY['ace_exchange',   'trades'],
        ARRAY['ace_exchange',   'execution_reports'],
        ARRAY['ace_exchange',   'matching_execution_reports'],
        -- ace_securities: immutable trade & execution event records (V27)
        ARRAY['ace_securities', 'trades'],
        ARRAY['ace_securities', 'execution_reports']
    ];
BEGIN
    FOREACH pair SLICE 1 IN ARRAY tables LOOP
        IF EXISTS (
            SELECT 1 FROM information_schema.tables
            WHERE table_schema = pair[1] AND table_name = pair[2]
        ) THEN
            -- ADD COLUMN ... DEFAULT backfills pre-existing rows (DDL, not blocked by
            -- the table's append-only DML rules). IF NOT EXISTS makes the step re-runnable.
            EXECUTE format(
                'ALTER TABLE %I.%I '
                || 'ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(64) NOT NULL DEFAULT %L',
                pair[1], pair[2], 'ace-commodities'
            );
            EXECUTE format(
                'COMMENT ON COLUMN %I.%I.tenant_id IS %L',
                pair[1], pair[2],
                'Owning tenant (always ''ace-commodities'' in this single-tenant '
                || 'schema). Backfilled by V32 retrofit; mandatory per the platform '
                || 'invariant "Tenant ID is never optional".'
            );
            RAISE NOTICE 'V32: annotated %.% with tenant_id', pair[1], pair[2];
        END IF;
    END LOOP;
END $$;
