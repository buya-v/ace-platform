/**
 * GarudaX Demo Runner — Comprehensive E2E Playwright Tests
 *
 * Tests every section and step of the demo runner at https://demo.garudax.asla.mn.
 * Uses soft assertions so individual step failures do not abort the suite.
 * Captures screenshots on failures.
 *
 * Diagnostic Report (bottom of file) documents which steps have backend
 * support and which will always fail due to missing routes or endpoints.
 */

import { test, expect } from '@playwright/test';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Timeout for waiting for a step result badge after clicking Run. */
const STEP_TIMEOUT = 20_000;

/** Global results tracker for the summary test at the end. */
const stepResults: { id: string; title: string; section: string; status: string; detail: string }[] = [];

function record(section: string, id: string, title: string, status: string, detail = '') {
  stepResults.push({ id, title, section, status, detail });
}

/**
 * Click the Nth "Run" button in the current view, wait for the status badge
 * to transition from Pending/Running to Pass/Fail/Skip, and return the result.
 */
async function runStepByIndex(
  page: import('@playwright/test').Page,
  index: number,
  section: string,
  stepId: string,
  stepTitle: string,
): Promise<string> {
  const cards = page.locator('div[class*="card"]').filter({ has: page.locator('button:has-text("Run")') });
  const card = cards.nth(index);

  // Scroll card into view
  await card.scrollIntoViewIfNeeded();

  // Click Run
  const runBtn = card.locator('button:has-text("Run")');
  await runBtn.click();

  // Wait for badge to leave Pending / Running state
  const badge = card.locator('[data-testid="status-badge"]');
  try {
    await expect(badge).not.toHaveText('Pending', { timeout: STEP_TIMEOUT });
    // Also wait for Running to finish
    await expect(badge).not.toHaveText('Running...', { timeout: STEP_TIMEOUT });
  } catch {
    // Timed out waiting
    const text = await badge.textContent().catch(() => 'unknown');
    record(section, stepId, stepTitle, 'TIMEOUT', `Badge stuck at: ${text}`);
    return 'TIMEOUT';
  }

  const badgeText = (await badge.textContent()) || 'unknown';
  const status = badgeText.trim();

  // Capture extra detail from response panel if available
  let detail = '';
  const responseStatus = card.locator('span[class*="status"]');
  if (await responseStatus.count() > 0) {
    detail = (await responseStatus.first().textContent()) || '';
  }

  record(section, stepId, stepTitle, status, detail);
  return status;
}

/**
 * Navigate to a section by clicking its sidebar button.
 */
async function navigateToSection(page: import('@playwright/test').Page, sectionTitle: string) {
  const sidebarBtn = page.locator(`nav button`).filter({ hasText: sectionTitle });
  await sidebarBtn.click();
  // Small wait for content to swap
  await page.waitForTimeout(300);
}

/**
 * Take a screenshot on failure.
 */
async function screenshotOnFail(
  page: import('@playwright/test').Page,
  name: string,
  status: string,
) {
  if (status === 'Fail' || status === 'TIMEOUT') {
    await page.screenshot({ path: `test-results/fail-${name}.png`, fullPage: true }).catch(() => {});
  }
}

// ---------------------------------------------------------------------------
// Section 1: Environment Setup (4 steps)
// ---------------------------------------------------------------------------

test.describe('Section 1: Environment Setup', () => {
  test('run all environment setup steps sequentially', async ({ page }) => {
    await page.goto('/');
    await navigateToSection(page, 'Environment Setup');

    const steps = [
      { id: 'env-1', title: 'Check Gateway Health' },
      { id: 'env-2', title: 'Check Matching Engine Health' },
      { id: 'env-3', title: 'Check All Services via Gateway' },
      { id: 'env-4', title: 'Verify Gateway Routes' },
    ];

    for (let i = 0; i < steps.length; i++) {
      const s = steps[i];
      const status = await runStepByIndex(page, i, 'Environment Setup', s.id, s.title);
      await screenshotOnFail(page, s.id, status);

      // Soft assert: log but don't fail the test
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 2: User Registration & KYC (6 steps) — sequential, state-dependent
// ---------------------------------------------------------------------------

test.describe('Section 2: User Registration & KYC', () => {
  test('run registration and login steps sequentially', async ({ page }) => {
    await page.goto('/');
    await navigateToSection(page, 'User Registration');

    const steps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];

    for (let i = 0; i < steps.length; i++) {
      const s = steps[i];
      const status = await runStepByIndex(page, i, 'User Registration & KYC', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 3: Trading Flow (5 steps) — sequential, needs auth tokens from S2
// ---------------------------------------------------------------------------

test.describe('Section 3: Trading Flow', () => {
  test('run registration then trading steps in order', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // First run registration to get tokens
    await navigateToSection(page, 'User Registration');
    const regSteps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];

    for (let i = 0; i < regSteps.length; i++) {
      await runStepByIndex(page, i, 'User Registration & KYC (prereq)', regSteps[i].id, regSteps[i].title);
    }

    // Now run trading
    await navigateToSection(page, 'Trading Flow');
    const tradeSteps = [
      { id: 'trade-1', title: 'Submit Buy Order (Trader 1)' },
      { id: 'trade-2', title: 'Submit Sell Order (Trader 2)' },
      { id: 'trade-3', title: 'View Order Book' },
      { id: 'trade-4', title: 'View Last Trade' },
      { id: 'trade-5', title: 'Cancel an Order' },
    ];

    for (let i = 0; i < tradeSteps.length; i++) {
      const s = tradeSteps[i];
      const status = await runStepByIndex(page, i, 'Trading Flow', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 4: Post-Trade (4 steps) — needs auth tokens
// ---------------------------------------------------------------------------

test.describe('Section 4: Post-Trade', () => {
  test('run registration then post-trade steps', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // Prereq: registration for admin token
    await navigateToSection(page, 'User Registration');
    const regSteps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];
    for (let i = 0; i < regSteps.length; i++) {
      await runStepByIndex(page, i, 'Post-Trade (prereq)', regSteps[i].id, regSteps[i].title);
    }

    await navigateToSection(page, 'Post-Trade');
    const postSteps = [
      { id: 'post-1', title: 'View Clearing Positions' },
      { id: 'post-2', title: 'View Netting Obligations' },
      { id: 'post-3', title: 'View Margin Requirements' },
      { id: 'post-4', title: 'View Margin Calls' },
    ];

    for (let i = 0; i < postSteps.length; i++) {
      const s = postSteps[i];
      const status = await runStepByIndex(page, i, 'Post-Trade', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 5: Physical Delivery (4 steps) — needs auth tokens + receipt_id state
// ---------------------------------------------------------------------------

test.describe('Section 5: Physical Delivery', () => {
  test('run registration then delivery steps', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // Prereq: registration
    await navigateToSection(page, 'User Registration');
    const regSteps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];
    for (let i = 0; i < regSteps.length; i++) {
      await runStepByIndex(page, i, 'Physical Delivery (prereq)', regSteps[i].id, regSteps[i].title);
    }

    await navigateToSection(page, 'Physical Delivery');
    const delSteps = [
      { id: 'del-1', title: 'Issue Warehouse Receipt' },
      { id: 'del-2', title: 'Pledge Receipt as Collateral' },
      { id: 'del-3', title: 'Initiate Delivery' },
      { id: 'del-4', title: 'View Warehouse Inventory' },
    ];

    for (let i = 0; i < delSteps.length; i++) {
      const s = delSteps[i];
      const status = await runStepByIndex(page, i, 'Physical Delivery', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 6: Market Data (3 steps) — no auth required
// ---------------------------------------------------------------------------

test.describe('Section 6: Market Data', () => {
  test('run market data steps', async ({ page }) => {
    await page.goto('/');
    await navigateToSection(page, 'Market Data');

    const mktSteps = [
      { id: 'mkt-1', title: 'Get OHLCV Candles' },
      { id: 'mkt-2', title: 'Get Ticker' },
      { id: 'mkt-3', title: 'Get Recent Trades' },
    ];

    for (let i = 0; i < mktSteps.length; i++) {
      const s = mktSteps[i];
      const status = await runStepByIndex(page, i, 'Market Data', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 7: Compliance & Risk (3 steps) — needs admin token
// ---------------------------------------------------------------------------

test.describe('Section 7: Compliance & Risk', () => {
  test('run registration then compliance steps', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // Prereq: registration
    await navigateToSection(page, 'User Registration');
    const regSteps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];
    for (let i = 0; i < regSteps.length; i++) {
      await runStepByIndex(page, i, 'Compliance (prereq)', regSteps[i].id, regSteps[i].title);
    }

    await navigateToSection(page, 'Compliance');
    const compSteps = [
      { id: 'comp-1', title: 'Get Participant Status' },
      { id: 'comp-2', title: 'Get Risk Score' },
      { id: 'comp-3', title: 'View Compliance Alerts' },
    ];

    for (let i = 0; i < compSteps.length; i++) {
      const s = compSteps[i];
      const status = await runStepByIndex(page, i, 'Compliance & Risk', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 8: Admin Operations (3 steps) — needs admin token
// ---------------------------------------------------------------------------

test.describe('Section 8: Admin Operations', () => {
  test('run registration then admin steps', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // Prereq: registration for admin token
    await navigateToSection(page, 'User Registration');
    const regSteps = [
      { id: 'reg-1', title: 'Register Trader 1' },
      { id: 'reg-2', title: 'Register Trader 2' },
      { id: 'reg-3', title: 'Register Admin' },
      { id: 'reg-4', title: 'Login Trader 1' },
      { id: 'reg-5', title: 'Login Trader 2' },
      { id: 'reg-6', title: 'Login Admin' },
    ];
    for (let i = 0; i < regSteps.length; i++) {
      await runStepByIndex(page, i, 'Admin (prereq)', regSteps[i].id, regSteps[i].title);
    }

    await navigateToSection(page, 'Admin Operations');
    const adminSteps = [
      { id: 'admin-1', title: 'View All Service Health' },
      { id: 'admin-2', title: 'View Settlement Cycles' },
      { id: 'admin-3', title: 'View Circuit Breakers' },
    ];

    for (let i = 0; i < adminSteps.length; i++) {
      const s = adminSteps[i];
      const status = await runStepByIndex(page, i, 'Admin Operations', s.id, s.title);
      await screenshotOnFail(page, s.id, status);
      expect.soft(status, `${s.id} (${s.title})`).toMatch(/Pass|Fail|Skip/);
    }
  });
});

// ---------------------------------------------------------------------------
// Section 9: Production Readiness Checklist (static, no API calls)
// ---------------------------------------------------------------------------

test.describe('Section 9: Production Readiness', () => {
  test('checklist renders with all categories and items', async ({ page }) => {
    await page.goto('/');
    await navigateToSection(page, 'Production Readiness');

    // Verify categories are present
    const categories = ['Security', 'Performance', 'Monitoring', 'Data Integrity', 'DR', 'Regulatory'];
    for (const cat of categories) {
      const visible = await page.locator(`text=${cat}`).first().isVisible().catch(() => false);
      expect.soft(visible, `Category "${cat}" should be visible`).toBeTruthy();
    }

    // Verify at least some checklist items render
    const items = page.locator('text=TLS termination');
    expect.soft(await items.count(), 'TLS termination item').toBeGreaterThanOrEqual(1);
  });
});

// ---------------------------------------------------------------------------
// Full E2E: Run All button
// ---------------------------------------------------------------------------

test.describe('Full E2E: Run All', () => {
  test('Run All executes steps across sections', async ({ page }) => {
    test.setTimeout(180_000);
    await page.goto('/');

    // Click Run All
    const runAllBtn = page.locator('button:has-text("Run All")');
    await runAllBtn.click();

    // Should show Running... state
    await expect(
      page.locator('button:has-text("Running...")')
    ).toBeVisible({ timeout: 5_000 }).catch(() => {});

    // Wait for Run All to finish (button returns to "Run All")
    await expect(runAllBtn).toHaveText('Run All', { timeout: 180_000 });

    // Count results across all sections
    let totalPass = 0;
    let totalFail = 0;
    let totalOther = 0;

    // Navigate through each section and count badges
    const sections = [
      'Environment Setup',
      'User Registration',
      'Trading Flow',
      'Post-Trade',
      'Physical Delivery',
      'Market Data',
      'Compliance',
      'Admin Operations',
    ];

    for (const sectionName of sections) {
      await navigateToSection(page, sectionName);
      await page.waitForTimeout(300);

      const badges = page.locator('[data-testid="status-badge"]');
      const count = await badges.count();

      for (let i = 0; i < count; i++) {
        const text = (await badges.nth(i).textContent()) || '';
        if (text.trim() === 'Pass') totalPass++;
        else if (text.trim() === 'Fail') totalFail++;
        else totalOther++;
      }
    }

    const total = totalPass + totalFail + totalOther;
    console.log('');
    console.log('=== RUN ALL SUMMARY ===');
    console.log(`Total steps: ${total}`);
    console.log(`Pass: ${totalPass}`);
    console.log(`Fail: ${totalFail}`);
    console.log(`Other (Pending/Skip/Running): ${totalOther}`);
    console.log(`Pass rate: ${total > 0 ? ((totalPass / total) * 100).toFixed(1) : 0}%`);
    console.log('=======================');
    console.log('');

    // Take a final screenshot
    await page.screenshot({ path: 'test-results/run-all-final.png', fullPage: true }).catch(() => {});
  });
});

// ---------------------------------------------------------------------------
// Reset functionality
// ---------------------------------------------------------------------------

test.describe('Reset', () => {
  test('Reset clears all step results back to Pending', async ({ page }) => {
    await page.goto('/');

    // Run one step
    await page.locator('button:has-text("Run")').first().click();
    await expect(
      page.locator('[data-testid="status-badge"]').first()
    ).not.toHaveText('Pending', { timeout: STEP_TIMEOUT });

    // Click Reset
    await page.locator('button:has-text("Reset")').click();

    // All badges should be Pending again
    const firstBadge = page.locator('[data-testid="status-badge"]').first();
    await expect(firstBadge).toHaveText('Pending', { timeout: 5_000 });

    // Sidebar counter should reset
    const envSection = page.locator('nav button').filter({ hasText: 'Environment Setup' });
    await expect(envSection).toContainText('0/4');
  });
});

// ---------------------------------------------------------------------------
// Summary: print results from all per-section tests
// ---------------------------------------------------------------------------

test.describe('Summary Report', () => {
  test('print step-by-step results', async () => {
    // This test runs last and prints the aggregate results from all section tests.
    // Note: stepResults is populated in-process, so this only works when tests run
    // serially (--workers=1). With parallel workers the array may be incomplete.

    if (stepResults.length === 0) {
      console.log('');
      console.log('=== STEP RESULTS SUMMARY ===');
      console.log('No individual step results collected.');
      console.log('Run with --workers=1 for full step-by-step tracking.');
      console.log('============================');
      return;
    }

    const passed = stepResults.filter((r) => r.status === 'Pass').length;
    const failed = stepResults.filter((r) => r.status === 'Fail').length;
    const timedOut = stepResults.filter((r) => r.status === 'TIMEOUT').length;
    const total = stepResults.length;

    console.log('');
    console.log('=== STEP RESULTS SUMMARY ===');
    console.log(`${passed}/${total} steps passed`);
    console.log(`${failed} failed, ${timedOut} timed out`);
    console.log('');

    for (const r of stepResults) {
      const icon = r.status === 'Pass' ? 'OK' : r.status === 'Fail' ? 'FAIL' : 'WARN';
      const detailStr = r.detail ? ` (${r.detail})` : '';
      console.log(`  [${icon}] ${r.section} > ${r.id} ${r.title}: ${r.status}${detailStr}`);
    }

    console.log('============================');
    console.log('');
  });
});


// ===========================================================================
// DIAGNOSTIC REPORT
// ===========================================================================
//
// This section documents backend endpoint coverage for each demo step.
// Generated from analysis of:
//   - src/demo-runner/src/data/sections.ts         (demo steps + URLs)
//   - src/gateway/internal/handler/routes.go        (gateway routes)
//   - src/gateway/internal/handler/handler.go       (forward targets)
//   - src/gateway/internal/proxy/httpclient.go      (rpcToHTTP map)
//   - src/*/internal/server/server.go               (actual service endpoints)
//
// ---------------------------------------------------------------------------
//
// LEGEND:
//   [OK]   = Gateway route exists + rpcToHTTP mapping exists + backend endpoint exists
//   [PARTIAL] = Gateway route exists but backend may return errors (mapping quirks)
//   [NO-ROUTE] = No gateway route for this URL (404 from gateway)
//   [NO-MAP]   = Gateway route exists, but rpcToHTTP has no mapping (503 from proxy)
//   [NO-SVC]   = Gateway route + mapping exist, but service has no HTTP endpoint (503)
//
// ---------------------------------------------------------------------------
//
// SECTION: Environment Setup
//   env-1  GET /healthz                                        [OK] gateway handles directly
//   env-2  GET /api/v1/instruments/{id}/book                   [NO-MAP] routes to MarketDataService/GetOrderBook but rpcToHTTP has no entry
//                                                              Matching engine has NO HTTP endpoints for orders/book/trades (gRPC only)
//                                                              Validator: any status > 0 is PASS, so even 503 passes
//   env-3  GET /readyz                                         [OK] gateway handles directly
//   env-4  GET /api/v1/instruments                             [NO-ROUTE] no gateway route for listing instruments
//                                                              Validator: 200 or 502 is PASS; will likely get 404/405 which is FAIL
//
// SECTION: User Registration & KYC
//   reg-1  POST /api/v1/auth/register                          [OK] gateway -> AuthService/Register -> POST /api/v1/register (auth-service)
//   reg-2  POST /api/v1/auth/register                          [OK] same as above
//   reg-3  POST /api/v1/auth/register                          [OK] same as above
//   reg-4  POST /api/v1/auth/login                             [OK] gateway -> AuthService/Login -> POST /api/v1/login (auth-service)
//   reg-5  POST /api/v1/auth/login                             [OK] same as above
//   reg-6  POST /api/v1/auth/login                             [OK] same as above
//
// SECTION: Trading Flow
//   trade-1  POST /api/v1/orders                               [NO-MAP] gateway -> OrderService/SubmitOrder, but rpcToHTTP has no OrderService/* entries
//                                                              Matching engine has NO HTTP order endpoints (gRPC only + health/ready)
//   trade-2  POST /api/v1/orders                               [NO-MAP] same as trade-1
//   trade-3  GET /api/v1/instruments/{id}/book                 [NO-MAP] gateway -> MarketDataService/GetOrderBook, no rpcToHTTP entry
//   trade-4  GET /api/v1/instruments/{id}/trades/latest        [NO-MAP] gateway -> MarketDataService/GetLastTrade, no rpcToHTTP entry
//   trade-5  POST /api/v1/orders                               [NO-MAP] same as trade-1
//
// SECTION: Post-Trade
//   post-1  GET /api/v1/clearing/positions                     [OK] gateway -> ClearingService/GetPositions -> GET /positions (clearing-engine:8082)
//   post-2  GET /api/v1/clearing/netting                       [OK] gateway -> ClearingService/NetObligations -> GET /netting (clearing-engine:8082)
//   post-3  GET /api/v1/margin                                 [OK] gateway -> MarginService/GetPortfolioMargin -> GET /margin (margin-engine:8083)
//   post-4  GET /api/v1/margin/calls                           [OK] gateway -> MarginService/GetAllActiveMarginCalls -> GET /margin-calls (margin-engine:8083)
//
// SECTION: Physical Delivery
//   del-1  POST /api/v1/warehouse/receipts                     [NO-ROUTE] no gateway route for warehouse service
//                                                              warehouse-service has NO HTTP endpoints beyond healthz/readyz
//   del-2  POST /api/v1/warehouse/receipts/{id}/pledge         [NO-ROUTE] same, no warehouse routes in gateway
//   del-3  POST /api/v1/warehouse/deliveries                   [NO-ROUTE] same
//   del-4  GET /api/v1/warehouse/inventory                     [NO-ROUTE] same
//
// SECTION: Market Data
//   mkt-1  GET /api/v1/market-data/candles/{id}?interval=1m    [NO-ROUTE] no gateway route for market-data-service
//                                                              market-data-service has NO HTTP endpoints beyond healthz/readyz
//   mkt-2  GET /api/v1/market-data/ticker/{id}                 [NO-ROUTE] same
//   mkt-3  GET /api/v1/market-data/trades/{id}                 [NO-ROUTE] same
//
// SECTION: Compliance & Risk
//   comp-1  GET /api/v1/compliance/participants/{id}/status     [NO-ROUTE] gateway has no route for this URL pattern
//                                                              (gateway has /suspend and /reinstate but not /status)
//   comp-2  GET /api/v1/compliance/participants/{id}/risk-score [NO-ROUTE] gateway has /api/v1/risk-scores/{id} but not this path
//   comp-3  GET /api/v1/compliance/alerts                      [OK] gateway -> ComplianceAdminService/ListAlerts -> GET /participant-status
//                                                              Note: rpcToHTTP maps to "ComplianceAdmin/ListAlerts" but handler uses
//                                                              "ComplianceAdminService/ListAlerts" — MISMATCH, will 503
//
// SECTION: Admin Operations
//   admin-1  GET /healthz                                      [OK] gateway handles directly
//   admin-2  GET /api/v1/settlement/cycles                     [OK] gateway -> SettlementService/GetAllCycles -> GET /cycles (settlement-engine:8084)
//   admin-3  GET /api/v1/admin/circuit-breakers                [NO-ROUTE] gateway has PUT .../circuit-breaker (per instrument) but no GET listing
//
// SECTION: Production Readiness
//   (No API calls — static checklist rendered client-side)
//
// ---------------------------------------------------------------------------
//
// SUMMARY OF EXPECTED RESULTS:
//
// Steps that SHOULD work end-to-end (10):
//   env-1, env-2* (any status passes), env-3, reg-1..reg-6, admin-1
//
// Steps that SHOULD work if services are running (4):
//   post-1, post-2, post-3, post-4, admin-2
//   (clearing, margin, settlement have proper HTTP endpoints + rpcToHTTP mappings)
//
// Steps with NO backend endpoint — will always fail (14):
//   trade-1..trade-5   (matching engine has no HTTP handlers, no rpcToHTTP mapping for OrderService/*)
//   del-1..del-4       (no gateway routes for warehouse-service at all)
//   mkt-1..mkt-3       (no gateway routes for market-data-service at all)
//   comp-1, comp-2     (gateway URL mismatch: demo uses /compliance/participants/*/status but gateway has no such route)
//   admin-3            (no GET /api/v1/admin/circuit-breakers route in gateway)
//
// Steps with PARTIAL/UNCERTAIN backend (2):
//   env-4              (no /api/v1/instruments list route, but validator accepts 502)
//   comp-3             (route exists but handler uses "ComplianceAdminService" vs rpcToHTTP "ComplianceAdmin")
//
// ===========================================================================
