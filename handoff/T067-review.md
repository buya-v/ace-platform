# Review — T067: Playwright Admin Dashboard Verification Tests

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The test suite correctly implements 13 serial Playwright tests that verify admin dashboard pages after demo runner setup. Key observations:

- Serial execution order is correct: Test 1 sets up demo data, Tests 2-13 verify admin views. The `test.describe.serial` usage is appropriate.
- SPA navigation via sidebar clicks (instead of `page.goto()`) correctly preserves in-memory auth tokens — this is a real constraint of the admin-ui architecture.
- The `loginAsAdmin()` helper correctly handles the login flow with API response interception for diagnostics and redirect waiting.
- Soft assertions (`expect.soft()`) are used appropriately for content checks on pages that may show empty states depending on backend data.
- Test 3 correctly re-runs registration before trading steps (fresh page per serial test), ensuring demo data exists before verifying the orderbook.
- The `navigateAdminSidebar()` helper expands collapsed sections before clicking links — handles the real sidebar UX.

Minor note: Each test after Test 1 re-logins because `test.describe.serial` provides a fresh page per test. This is documented and correct given the in-memory auth constraint, though it adds latency.

### Security: PASS

- Hardcoded credentials (`admin@ace.mn` / `Adm1n@Pass!`) are test credentials for the demo environment, not production secrets. These are appropriate for E2E test files targeting the demo deployment.
- No secrets stored in config files or environment variable leakage.
- The test only reads data from the admin dashboard — no destructive operations.
- URLs target the demo/staging environment (`demo.ace.asla.mn`, `ace.asla.mn/admin`), not production.

### Code Quality: PASS

- Follows the existing Playwright test conventions established in `demo-runbook.spec.ts` (same `STEP_TIMEOUT`, same `runStepByIndex` helper pattern, same soft assertion approach, same comment structure).
- Helper functions are well-named and focused (`loginAsAdmin`, `navigateAdminSidebar`, `takeScreenshot`, `runStepByIndex`).
- The config changes (`screenshot: 'on'`, `outputDir`) are minimal and non-breaking additions to the shared config.
- Screenshot naming convention (`admin-01-*` through `admin-13-*`) is clear and sortable.
- Test names include numeric prefixes matching the serial order, making failures easy to identify.
- No dead code or unnecessary complexity.

### Test Coverage: PASS

The suite covers the critical admin dashboard verification flow:
- **Setup path**: Demo runner registration (6 steps) and trading flow (buy/sell orders)
- **Auth flow**: Admin login with form fill, API response interception, redirect handling
- **All 12 admin pages**: Participants, Order Book, Positions, System Health, Margin Calls, Settlement, Circuit Breakers, Risk Overview, Warehouse, Compliance Alerts, Audit Log, Market Phase
- **Error states**: Each page checks for error boundaries and handles empty states
- **Data verification**: Tests 2-3 verify specific data (trader emails, instrument IDs) created by Test 1

The assertions are appropriately flexible — they verify that pages render meaningful content without being brittle against exact DOM structure changes.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Shared `runStepByIndex` helper**: The `runStepByIndex` function in this file is a simplified copy of the one in `demo-runbook.spec.ts` (missing the `record()` tracking). Consider extracting a shared helper module (`tests/playwright/helpers.ts`) to avoid duplication.

2. **Login state reuse**: Since `test.describe.serial` gives a fresh page per test, Tests 4-13 each re-login independently. Consider using `test.beforeEach` with a `storageState` approach or a shared browser context to reduce the 12 redundant login round-trips (~500ms each).

3. **Screenshot on failure only**: The config change `screenshot: 'on'` captures screenshots for ALL tests globally (including `demo-runbook.spec.ts` and `reset-runall.spec.ts`). Consider using `screenshot: 'only-on-failure'` in the global config and keeping the explicit `takeScreenshot()` calls in this file for always-capture behavior.
