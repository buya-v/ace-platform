# Review — R025: Fix DB init (zero-pad migration filenames + reconcile duplicate market_data.trades)

**Verdict:** REJECTED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: FAIL

The core DB-init fix is genuinely correct and well-diagnosed — but the change ships a
test-breaking regression (see Test Coverage), so this area cannot pass as delivered.

What is right:
- **Zero-padding fixes the ordering bug.** After renaming to `V001__`…`V033__`, the
  `LC_ALL=C` lexicographic order that `docker-entrypoint-initdb.d` uses equals the version
  order (`V001 < V006 < V007 < V008 < V009 < V010 < … < V033`). Previously `V10__`…`V33__`
  sorted before `V1__`, so `V011__matching_engine_tables.sql` ran before the `exchange`
  schema existed — exactly the failure mode R015 reported. Confirmed the version gaps
  (no V2–V5) are intentional R017 stub removals and are tolerated by initdb ordering.
- **The duplicate `market_data.trades` is correctly resolved.** Verified against source:
  - `V016__market_data_tables.sql` defines `trades(... traded_at ..., PRIMARY KEY (traded_at, id))`
    plus `candles`/`tickers`.
  - `src/market-data-service/internal/store/postgres.go` uses exactly that shape:
    `INSERT INTO ace_market_data.trades (... traded_at)`, `ON CONFLICT (traded_at, id)`,
    and `ace_market_data.candles`/`tickers`. The old V8 `executed_at`/`trade_id` shape and
    its `ohlcv_*` continuous aggregates are unused (`grep ohlcv src/` → nothing).
  - On a fresh DB in version order, V8 won the `CREATE TABLE` and V16's
    `CREATE INDEX … (traded_at …)` then aborted init with `column "traded_at" does not exist`.
    Neutralizing V008 to an idempotent extension+schema bootstrap lets V016 (authoritative)
    create the table. Diagnosis and fix are accurate.
- **The dropped role is genuinely stale.** `garudax_marketdata_svc` (no underscore) appears
  only in the old V8; the canonical `garudax_market_data_svc` is created/granted in
  `V001`/`V030` (`GRANT … ON ALL TABLES IN SCHEMA ace_market_data`), which covers the V016
  tables. Removing the misspelled role + its grants loses nothing.
- The worker performed a real fresh-volume Docker bring-up and reports 0 init errors with the
  correct hypertable on `traded_at` — credible and consistent with the code I inspected.

### Security: PASS

No security impact. No injection surface (DDL only), no secrets, no auth/authz changes.
Dropping the dead misspelled service role is a net hygiene improvement, not a privilege change.

### Code Quality: PASS

- `git mv` preserves history (`similarity index 100%` on all pure renames).
- 3-digit padding is future-proofed past the current max (V033) and unambiguous.
- The V008 neutralization keeps an explicit, accurate header documenting exactly what was
  removed and why, rather than silently deleting — good for the next reader.
- The Go edits are correctly scoped to comment-only doc references (V29→V029, V9→V009);
  no application logic touched.

### Test Coverage: FAIL

The rename breaks an existing test that opens a migration file by **exact hardcoded path**:

- `tests/compliance/test_kyc_aml_spec.py:20-22`
  ```python
  MIGRATION_PATH = os.path.join(
      REPO_ROOT, "infrastructure", "db", "migrations", "V7__kyc_aml_tables.sql"
  )
  ```
  The `migration_content` fixture (`tests/compliance/test_kyc_aml_spec.py:37-40`) does
  `open(MIGRATION_PATH)`. After this change the file is `V007__kyc_aml_tables.sql`, so the
  fixture raises `FileNotFoundError` and every test depending on it ERRORs.

The handoff states "No Flyway config, deploy manifest, or k8s manifest references migration
filenames (verified by grep)" — but the grep missed this Python test (it references the file
by name and *reads* it, not just a comment). The worker verified DB init in isolation via
Docker but did not run the repo's existing test suites against the renamed branch, so this
regression went undetected. A rename task must update every hardcoded reference to the renamed
artifact and re-run affected tests.

## Required Fixes (REJECTED)

1. **Update the hardcoded migration path in the compliance spec test.**
   `tests/compliance/test_kyc_aml_spec.py:21` — change `"V7__kyc_aml_tables.sql"` to
   `"V007__kyc_aml_tables.sql"`. Then run `pytest tests/compliance/test_kyc_aml_spec.py`
   and confirm green.
2. **Re-run the existing test suites against the renamed branch** (at minimum the Python
   tests under `tests/`) to confirm no other hardcoded migration-filename references break.
   I found only this one in executable code, but the worker's own verification step skipped
   the test suites, so this must be done before re-submission.

## Suggestions (non-blocking)

- Stale comments inside the migrations now name un-padded files, e.g.
  `V031__platform_control_schemas.sql:11,17` references `V8__…`/`V9__…`/`V29__…`. These are
  SQL comments (non-breaking) but are now inaccurate; update for consistency with the new
  naming if convenient.
- The two spec docs the worker already flagged (`docs/fix-gateway-spec.md` `V31__fix_gateway.sql`,
  `docs/T037_warehouse_spec.md` `V8__warehouse_tables.sql`) reference migration files that
  never existed as real artifacts — reconcile if those specs are revived. Correctly left out
  of scope here.
- The worker's own suggestion to add a CI check (under R014) asserting zero-padded filenames
  and no duplicate version numbers is worth adopting — this defect class is cheap to gate.
- Once the test is fixed this is a strong change; the DB-init verification was thorough and
  the rejection is solely the missed test reference.
