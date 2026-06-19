# DB-2 — Flyway Migration: ACE Commodities Schema Retrofit

**Status:** success
**Deliverable:** `infrastructure/db/migrations/V32__ace_schema_rename.sql`

## Summary
Created the ACE-commodities schema-retrofit migration: idempotent `ace_` schema
renames (harmonised with the existing V30) plus the genuinely net-new work —
stamping every existing immutable audit/event record with
`tenant_id = 'ace-commodities'`.

## Key decisions

1. **V7 → V32 (version collision).** The task named the file
   `V7__ace_schema_rename.sql`, but `V7__kyc_aml_tables.sql` already owns version 7.
   Duplicate Flyway versions fail `flyway validate`/`migrate` in every environment
   (a known breakage class here — see the historical V8/V9 conflict and DB-1's
   identical V6→V31 fix). Authored at the next free version, **V32**.

2. **Renames are idempotent, not a re-do of V30.** `V30__ace_schema_renames.sql`
   already performs the `ALTER SCHEMA ... RENAME`. Re-running a bare rename would
   error ("schema already exists" / "does not exist"). PART 1 renames a schema only
   when the legacy name exists *and* the `ace_` target does not — a clean no-op on
   any DB that applied V30, self-healing otherwise. `auth` and `platform` stay
   platform-level and are deliberately not renamed (platform-architecture.md §3.4).

3. **Audit annotation via `ADD COLUMN ... DEFAULT`, not `UPDATE`.** The target
   tables are append-only (`no_update_*` / `no_delete_*` rules,
   `DO INSTEAD NOTHING`). A `UPDATE ... SET tenant_id` would be silently swallowed
   and backfill nothing. Adding the column with a constant `NOT NULL DEFAULT
   'ace-commodities'` backfills all pre-existing rows as a DDL metadata operation
   (rules only intercept DML), and keeps tenant_id mandatory for new inserts —
   satisfying the invariant "Tenant ID is never optional."

4. **`auth.audit_log` excluded.** It is platform-level / cross-tenant and must not
   be branded ace-commodities. `platform.audit` already carries its own tenant_id.

## Tables annotated
- `ace_compliance`: screening_results, risk_scores, sar_filings, sar_filings_v2, audit_log
- `ace_exchange`: trades, execution_reports, matching_execution_reports
- `ace_securities`: trades, execution_reports

(Each step is guarded by an `information_schema` existence check, so the migration
is robust to differing historical migration chains.)

## Verification (live PostgreSQL, scratch DB)
- Schemas renamed; `auth` untouched.
- All pre-existing rows backfilled to `ace-commodities`; column is `NOT NULL` with
  the correct default.
- `auth.audit_log` has **no** tenant_id column.
- Append-only rules still enforced after the ALTER (a deliberate malicious `UPDATE`
  returned `UPDATE 0`; data unchanged) — proving the DEFAULT-backfill approach was
  required.
- Second apply is a clean idempotent no-op.

## Suggested follow-ups
- **Application code (Phase 0.6):** services reading these tables should select/use
  the new `tenant_id` column and reference `ace_*` schema-qualified names (already
  the contract set by V30).
- **mse-equities build (Phase 0.8):** equivalent immutable event tables in the MSE
  tenant schemas should be created with a `tenant_id` column from the start (no
  retrofit needed).
