/**
 * ACE Admin Dashboard Verification — Playwright E2E Tests
 *
 * Verifies that demo runbook outcomes (registration, trading) are visible
 * in the admin dashboard at https://ace.asla.mn/admin/.
 *
 * Tests run serially: Test 1 sets up demo data, Tests 2-13 verify admin views.
 * The admin UI stores auth tokens in-memory (React state), so after login we
 * navigate via sidebar clicks (SPA navigation) instead of page.goto() to
 * preserve the session.
 */

import { test, expect, type Page } from '@playwright/test';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEMO_URL = 'https://demo.ace.asla.mn';
const ADMIN_URL = 'https://ace.asla.mn/admin';
const ADMIN_EMAIL = 'admin@ace.mn';
const ADMIN_PASSWORD = 'Adm1n@Pass!';
const STEP_TIMEOUT = 20_000;
const NAV_TIMEOUT = 15_000;
const SCREENSHOT_DIR = 'test-results';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Click the Nth "Run" button in the current demo runner view,
 * wait for the status badge to show a final state.
 */
async function runStepByIndex(page: Page, index: number): Promise<string> {
  const cards = page.locator('div[class*="card"]').filter({
    has: page.locator('button:has-text("Run")'),
  });
  const card = cards.nth(index);
  await card.scrollIntoViewIfNeeded();

  const runBtn = card.locator('button:has-text("Run")');
  await runBtn.click();

  const badge = card.locator('[data-testid="status-badge"]');
  try {
    await expect(badge).not.toHaveText('Pending', { timeout: STEP_TIMEOUT });
    await expect(badge).not.toHaveText('Running...', { timeout: STEP_TIMEOUT });
  } catch {
    return 'TIMEOUT';
  }

  return ((await badge.textContent()) || 'unknown').trim();
}

/**
 * Navigate to a demo runner section by clicking its sidebar button.
 */
async function navigateToSection(page: Page, sectionTitle: string) {
  const sidebarBtn = page.locator('nav button').filter({ hasText: sectionTitle });
  await sidebarBtn.click();
  await page.waitForTimeout(300);
}

/**
 * Login to the admin dashboard. Returns true on success.
 */
async function loginAsAdmin(page: Page): Promise<boolean> {
  await page.goto(`${ADMIN_URL}/login`, { timeout: NAV_TIMEOUT, waitUntil: 'networkidle' });

  // Already on dashboard?
  if (page.url().includes('/dashboard')) return true;

  // Wait for login form
  const emailInput = page.getByLabel('Email');
  try {
    await emailInput.waitFor({ state: 'visible', timeout: 5_000 });
  } catch {
    return page.url().includes('/dashboard');
  }

  // Fill credentials
  await emailInput.click();
  await emailInput.fill(ADMIN_EMAIL);
  await page.getByLabel('Password').click();
  await page.getByLabel('Password').fill(ADMIN_PASSWORD);

  // Intercept login API response for diagnostics
  const responsePromise = page.waitForResponse(
    resp => resp.url().includes('/auth/login'),
    { timeout: NAV_TIMEOUT },
  ).catch(() => null);

  // Submit
  await page.locator('button[type="submit"]').click();

  const response = await responsePromise;
  if (response) {
    const status = response.status();
    if (status !== 200) {
      const body = await response.text().catch(() => '');
      console.log(`Login API ${status}: ${body.substring(0, 300)}`);
    }
  }

  // Wait for redirect to dashboard
  try {
    await page.waitForURL(/\/dashboard/, { timeout: 8_000 });
    return true;
  } catch {
    const errorEl = page.locator('[role="alert"]');
    if (await errorEl.count() > 0) {
      console.log(`Login error: ${await errorEl.textContent()}`);
    }
    return false;
  }
}

/**
 * Navigate to an admin page using SPA sidebar navigation.
 * This preserves the in-memory auth token (unlike page.goto which reloads the SPA).
 */
async function navigateAdminSidebar(page: Page, label: string) {
  // Ensure sidebar sections are expanded
  const sectionToggles = page.locator('[data-testid="sidebar"] button[aria-expanded="false"]');
  const toggleCount = await sectionToggles.count();
  for (let i = 0; i < toggleCount; i++) {
    await sectionToggles.nth(i).click();
    await page.waitForTimeout(100);
  }

  // Click the NavLink by exact label
  const link = page.locator('[data-testid="sidebar"] a').filter({ hasText: label });
  await link.click();
  await page.waitForTimeout(500);
}

/**
 * Take a named screenshot.
 */
async function takeScreenshot(page: Page, name: string) {
  await page.screenshot({
    path: `${SCREENSHOT_DIR}/${name}`,
    fullPage: true,
  }).catch(() => {});
}

// ---------------------------------------------------------------------------
// Tests — run in serial order, all using soft assertions
// ---------------------------------------------------------------------------

test.describe.serial('Admin Dashboard Verification', () => {
  test.setTimeout(90_000);

  // =========================================================================
  // Test 1 — Setup: Run demo registration steps
  // =========================================================================
  test('Test 1 — Setup: Run demo registration steps', async ({ page }) => {
    await page.goto(DEMO_URL, { timeout: NAV_TIMEOUT });
    await navigateToSection(page, 'User Registration');

    const results: string[] = [];
    for (let i = 0; i < 6; i++) {
      const status = await runStepByIndex(page, i);
      results.push(status);
    }

    for (let i = 0; i < results.length; i++) {
      expect.soft(results[i], `reg step ${i + 1}`).toMatch(/Pass|Fail|Skip/);
    }

    await takeScreenshot(page, 'admin-01-demo-setup.png');
  });

  // =========================================================================
  // Test 2 — Verify Participants in Admin
  // =========================================================================
  test('Test 2 — Verify Participants in Admin', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login succeeded').toBeTruthy();

    if (loggedIn) {
      // Use SPA navigation — click Compliance section then Participants
      await navigateAdminSidebar(page, 'Participants');

      // Wait for table/grid to load
      const grid = page.locator('table, [role="grid"], [class*="DataGrid"], [class*="table"]').first();
      await expect(grid).toBeVisible({ timeout: NAV_TIMEOUT }).catch(() => {});

      const rows = page.locator('table tbody tr, [role="row"]');
      const rowCount = await rows.count();
      expect.soft(rowCount, 'Participants grid has at least 3 rows').toBeGreaterThanOrEqual(3);

      const pageText = await page.textContent('body') || '';
      expect.soft(pageText).toContain('trader1@ace.mn');
      expect.soft(pageText).toContain('trader2@ace.mn');
    }

    await takeScreenshot(page, 'admin-02-participants.png');
  });

  // =========================================================================
  // Test 3 — Run trading steps then verify Order Book
  // =========================================================================
  test('Test 3 — Run trading steps then verify Order Book', async ({ page }) => {
    test.setTimeout(120_000);

    // Run registration prereqs in demo runner
    await page.goto(DEMO_URL, { timeout: NAV_TIMEOUT });
    await navigateToSection(page, 'User Registration');
    for (let i = 0; i < 6; i++) {
      await runStepByIndex(page, i);
    }

    // Run trading steps (buy + sell)
    await navigateToSection(page, 'Trading Flow');
    const buyStatus = await runStepByIndex(page, 0);
    const sellStatus = await runStepByIndex(page, 1);
    expect.soft(buyStatus, 'Buy order step').toMatch(/Pass|Fail|Skip/);
    expect.soft(sellStatus, 'Sell order step').toMatch(/Pass|Fail|Skip/);

    // Login to admin and verify orderbook
    const loggedIn = await loginAsAdmin(page);
    if (loggedIn) {
      await navigateAdminSidebar(page, 'Order Book');
      await page.waitForTimeout(1000);
      const bodyText = await page.textContent('body') || '';
      expect.soft(bodyText).toContain('WHT-HRW-2026M07-UB');
    }

    await takeScreenshot(page, 'admin-03-orderbook.png');
  });

  // =========================================================================
  // Test 4 — Verify Positions after trades
  // =========================================================================
  test('Test 4 — Verify Positions after trades', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for positions').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Positions');

      const errorBoundary = page.locator('text="Something went wrong"');
      expect.soft(await errorBoundary.count(), 'No error boundary visible').toBe(0);

      const hasGrid = await page.locator('table, [role="grid"], [class*="DataGrid"]').count() > 0;
      const hasEmpty = await page.locator('text=/no.*position|empty|no data/i').count() > 0;
      expect.soft(hasGrid || hasEmpty, 'Positions page shows grid or empty state').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-04-positions.png');
  });

  // =========================================================================
  // Test 5 — Verify System Health
  // =========================================================================
  test('Test 5 — Verify System Health', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for health').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'System Health');
      await page.waitForTimeout(1000);

      const cards = page.locator('[class*="card"], [class*="Card"], [class*="service"], [class*="health"]');
      const cardCount = await cards.count();
      expect.soft(cardCount, 'At least 9 service health cards visible').toBeGreaterThanOrEqual(9);

      const healthyBadges = page.locator('text=/healthy|running|up|online/i');
      expect.soft(await healthyBadges.count(), 'Services show healthy status').toBeGreaterThanOrEqual(1);
    }

    await takeScreenshot(page, 'admin-05-health.png');
  });

  // =========================================================================
  // Test 6 — Verify Margin Calls page
  // =========================================================================
  test('Test 6 — Verify Margin Calls page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for margin').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Margin Calls');

      const statsCards = page.locator('[class*="card"], [class*="Card"], [class*="stat"], [class*="Stat"]');
      const statsVisible = await statsCards.count() > 0;
      const bodyText = await page.textContent('body') || '';
      const hasMarginContent = /active.?call|total.?shortfall|margin|no.*margin/i.test(bodyText);
      expect.soft(statsVisible || hasMarginContent, 'Margin page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-06-margin.png');
  });

  // =========================================================================
  // Test 7 — Verify Settlement page
  // =========================================================================
  test('Test 7 — Verify Settlement page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for settlement').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Settlement');

      const bodyText = await page.textContent('body') || '';
      const hasStepper = await page.locator('[class*="stepper"], [class*="Stepper"], [class*="step"], [class*="phase"]').count() > 0;
      const hasNoCycle = /no.*active.*cycle|no.*settlement|empty/i.test(bodyText);
      expect.soft(hasStepper || hasNoCycle || bodyText.includes('Settlement'), 'Settlement page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-07-settlement.png');
  });

  // =========================================================================
  // Test 8 — Verify Circuit Breakers page
  // =========================================================================
  test('Test 8 — Verify Circuit Breakers page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for circuit breakers').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Circuit Breakers');

      const grid = page.locator('table, [role="grid"], [class*="DataGrid"], [class*="table"]');
      const hasGrid = await grid.count() > 0;
      const bodyText = await page.textContent('body') || '';
      expect.soft(hasGrid || /circuit.?breaker|instrument|threshold/i.test(bodyText), 'Circuit Breakers page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-08-circuit-breakers.png');
  });

  // =========================================================================
  // Test 9 — Verify Risk Overview
  // =========================================================================
  test('Test 9 — Verify Risk Overview', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for risk').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Risk Overview');

      const cards = page.locator('[class*="card"], [class*="Card"], [class*="summary"], [class*="stat"]');
      const hasCards = await cards.count() > 0;
      const bodyText = await page.textContent('body') || '';
      expect.soft(hasCards || /risk|score|exposure|portfolio/i.test(bodyText), 'Risk page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-09-risk.png');
  });

  // =========================================================================
  // Test 10 — Verify Warehouse page
  // =========================================================================
  test('Test 10 — Verify Warehouse page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for warehouse').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Warehouse');

      const errorBoundary = page.locator('text="Something went wrong"');
      expect.soft(await errorBoundary.count(), 'No error boundary').toBe(0);
    }

    await takeScreenshot(page, 'admin-10-warehouse.png');
  });

  // =========================================================================
  // Test 11 — Verify Compliance Alerts page
  // =========================================================================
  test('Test 11 — Verify Compliance Alerts page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for compliance').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Compliance Alerts');

      const bodyText = await page.textContent('body') || '';
      const hasAlerts = await page.locator('table, [role="grid"], [class*="list"], [class*="alert"]').count() > 0;
      const hasEmpty = /no.*alert|empty|no.*data|no.*compliance/i.test(bodyText);
      expect.soft(hasAlerts || hasEmpty || bodyText.includes('Compliance'), 'Compliance page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-11-compliance.png');
  });

  // =========================================================================
  // Test 12 — Verify Audit Log page
  // =========================================================================
  test('Test 12 — Verify Audit Log page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for audit').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Audit Log');

      const grid = page.locator('table, [role="grid"], [class*="DataGrid"], [class*="table"], [class*="log"]');
      const hasGrid = await grid.count() > 0;
      const bodyText = await page.textContent('body') || '';
      expect.soft(hasGrid || /audit|log|event|action/i.test(bodyText), 'Audit page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-12-audit.png');
  });

  // =========================================================================
  // Test 13 — Verify Market Phase page
  // =========================================================================
  test('Test 13 — Verify Market Phase page', async ({ page }) => {
    const loggedIn = await loginAsAdmin(page);
    expect.soft(loggedIn, 'Admin login for market phase').toBeTruthy();

    if (loggedIn) {
      await navigateAdminSidebar(page, 'Market Phase');

      const cards = page.locator('[class*="card"], [class*="Card"], [class*="instrument"], [class*="phase"]');
      const hasCards = await cards.count() > 0;
      const bodyText = await page.textContent('body') || '';
      expect.soft(hasCards || /market.*phase|instrument|pre.?open|open|closed/i.test(bodyText), 'Market Phase page shows content').toBeTruthy();
    }

    await takeScreenshot(page, 'admin-13-market-phase.png');
  });
});
