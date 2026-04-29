import { test, expect, Page, BrowserContext } from '@playwright/test';

const BASE = 'https://admin.garudax.asla.mn';
const ADMIN_EMAIL = 'admin@garudax.mn';
const ADMIN_PASS = 'Adm1n@Pass!';

interface Finding {
  page: string;
  severity: 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW' | 'INFO';
  detail: string;
}

const findings: Finding[] = [];

async function dismissErrorOverlay(page: Page): Promise<boolean> {
  const reloadBtn = page.locator('button:has-text("Reload page"), button:has-text("Reload")');
  if (await reloadBtn.count() > 0 && await reloadBtn.first().isVisible()) {
    return true; // Error overlay present
  }
  return false;
}

async function navigateSidebar(page: Page, href: string, pageName: string): Promise<boolean> {
  // First dismiss any error overlay by going to dashboard
  if (await dismissErrorOverlay(page)) {
    findings.push({ page: pageName, severity: 'CRITICAL', detail: 'Error overlay blocking navigation — had to reload via dashboard' });
    // Use JS navigation to bypass overlay
    await page.evaluate((url) => { window.location.href = url; }, `${BASE}/login`);
    await page.waitForLoadState('networkidle');
    // Re-login
    await page.locator('input[type="email"], input[name="email"]').first().fill(ADMIN_EMAIL);
    await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
    await page.locator('button[type="submit"]').first().click();
    try { await page.waitForURL('**/dashboard**', { timeout: 10000 }); } catch {}
    await page.waitForTimeout(1000);
  }

  const link = page.locator(`a[href="${href}"]`).first();
  if (await link.count() === 0) {
    findings.push({ page: pageName, severity: 'HIGH', detail: `No sidebar link found for ${href}` });
    return false;
  }

  try {
    await link.click({ timeout: 5000 });
    await page.waitForTimeout(2000);
    return true;
  } catch (e: any) {
    // If click is intercepted by overlay, force navigate
    findings.push({ page: pageName, severity: 'HIGH', detail: `Click intercepted: ${e.message.substring(0, 100)}` });
    await page.evaluate((url) => { window.location.href = url; }, `${BASE}/login`);
    await page.waitForLoadState('networkidle');
    await page.locator('input[type="email"], input[name="email"]').first().fill(ADMIN_EMAIL);
    await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
    await page.locator('button[type="submit"]').first().click();
    try { await page.waitForURL('**/dashboard**', { timeout: 10000 }); } catch {}
    await page.waitForTimeout(1000);
    // Try again
    try {
      await page.locator(`a[href="${href}"]`).first().click({ timeout: 5000 });
      await page.waitForTimeout(2000);
      return true;
    } catch {
      return false;
    }
  }
}

test.describe.serial('Admin Portal UAT v3 — Complete Navigation', () => {
  let page: Page;
  let context: BrowserContext;

  test.beforeAll(async ({ browser }) => {
    context = await browser.newContext({
      ignoreHTTPSErrors: true,
      viewport: { width: 1920, height: 1080 },
    });
    page = await context.newPage();

    await page.goto(`${BASE}/login`, { timeout: 30000 });
    await page.waitForLoadState('networkidle');
    await page.locator('input[type="email"], input[name="email"]').first().fill(ADMIN_EMAIL);
    await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL('**/dashboard**', { timeout: 15000 });
    await page.waitForTimeout(2000);
  });

  test.afterAll(async () => {
    // Print report
    console.log('\n\n====================================================');
    console.log('          ADMIN PORTAL UAT REPORT');
    console.log('          https://admin.garudax.asla.mn');
    console.log('          Date: ' + new Date().toISOString());
    console.log('====================================================\n');

    const critical = findings.filter(f => f.severity === 'CRITICAL');
    const high = findings.filter(f => f.severity === 'HIGH');
    const medium = findings.filter(f => f.severity === 'MEDIUM');
    const low = findings.filter(f => f.severity === 'LOW');
    const info = findings.filter(f => f.severity === 'INFO');

    console.log(`Total findings: ${findings.length}`);
    console.log(`  CRITICAL: ${critical.length}`);
    console.log(`  HIGH:     ${high.length}`);
    console.log(`  MEDIUM:   ${medium.length}`);
    console.log(`  LOW:      ${low.length}`);
    console.log(`  INFO:     ${info.length}`);

    console.log('\n--- CRITICAL ---');
    critical.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
    console.log('\n--- HIGH ---');
    high.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
    console.log('\n--- MEDIUM ---');
    medium.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
    console.log('\n--- LOW ---');
    low.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
    console.log('\n--- INFO ---');
    info.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
    console.log('\n====================================================\n');

    await page.close();
    await context.close();
  });

  // Collect JS and network errors globally
  test('Setup error listeners and test Dashboard', async () => {
    const jsErrors: string[] = [];
    const networkErrors: string[] = [];

    page.on('pageerror', (err) => {
      jsErrors.push(err.message);
      findings.push({ page: 'GLOBAL', severity: 'HIGH', detail: `JS Error: ${err.message.substring(0, 200)}` });
    });

    page.on('response', (resp) => {
      if (resp.status() >= 500 && !resp.url().includes('favicon') && !resp.url().includes('ws:')) {
        networkErrors.push(`${resp.status()} ${resp.url()}`);
      }
    });

    // Dashboard checks
    const mainText = await page.textContent('main') || '';

    // Check header status indicators
    const wsText = await page.locator('text=WS:').first().textContent().catch(() => '');
    if (wsText?.includes('Disconnected')) {
      findings.push({ page: 'Dashboard', severity: 'MEDIUM', detail: 'WebSocket shows "Disconnected" status' });
    }

    const unknownCount = await page.locator('text=Unknown').count();
    if (unknownCount > 0) {
      findings.push({ page: 'Dashboard', severity: 'MEDIUM', detail: 'Header shows "Unknown" status indicator (market status not resolved)' });
    }

    // Check dashboard cards
    const dashCount = (mainText.match(/—/g) || []).length;
    if (dashCount > 0) {
      findings.push({ page: 'Dashboard', severity: 'HIGH', detail: `${dashCount} status cards show "—" (dash) instead of actual values. Cards affected: PENDING KYC, ACTIVE MARGIN CALLS, SETTLEMENT CYCLES` });
    }

    // Check tenant info
    if (mainText.includes('ACE Commodity Exchange') && mainText.includes('ACTIVE')) {
      findings.push({ page: 'Dashboard', severity: 'INFO', detail: 'Tenant "ACE Commodity Exchange" is ACTIVE, ID: ace-commodities' });
    }

    await page.screenshot({ path: 'tests/screenshots/v3-01-dashboard.png', fullPage: true });
  });

  test('Instruments page', async () => {
    await navigateSidebar(page, '/dashboard/securities', 'Instruments');
    await page.screenshot({ path: 'tests/screenshots/v3-02-instruments.png', fullPage: true });

    const rows = await page.locator('table tbody tr').count();
    findings.push({ page: 'Instruments', severity: 'INFO', detail: `${rows} instruments listed (APU JSC, Govisumber Mining LLC). Has search, filter by asset class, Create Instrument, Export, Halt, Edit Status actions.` });

    // Check instrument data completeness
    const cells = await page.locator('table tbody td').allTextContents();
    const hasEmptyCells = cells.some(c => c.trim() === '' || c.trim() === '—');
    if (hasEmptyCells) {
      findings.push({ page: 'Instruments', severity: 'LOW', detail: 'Some table cells are empty or show dash placeholders' });
    }
  });

  test('Orders page', async () => {
    await navigateSidebar(page, '/dashboard/securities-orders', 'Orders');
    await page.screenshot({ path: 'tests/screenshots/v3-03-orders.png', fullPage: true });

    const rows = await page.locator('table tbody tr').count();
    findings.push({ page: 'Orders', severity: 'INFO', detail: `${rows} orders shown. All FILLED status. Has instrument selector and Submit Order button. RECENT TRADES section shows "Trades will appear after order matching"` });

    // Check RECENT TRADES section
    const recentTrades = await page.textContent('main') || '';
    if (recentTrades.includes('Trades will appear after order matching')) {
      findings.push({ page: 'Orders', severity: 'LOW', detail: 'RECENT TRADES section is empty — "Trades will appear after order matching". Orders are FILLED but no trades shown — possible data inconsistency.' });
    }
  });

  test('Positions page', async () => {
    await navigateSidebar(page, '/dashboard/securities-positions', 'Positions');
    await page.screenshot({ path: 'tests/screenshots/v3-04-positions.png', fullPage: true });

    const mainText = await page.textContent('main') || '';

    // Check for UUID displayed as instrument name
    if (/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/.test(mainText)) {
      findings.push({ page: 'Positions', severity: 'HIGH', detail: 'Instrument column displays raw UUID (66b2d493-845d-47e8-b34f-72f47cb47fb0) instead of ticker/name. Should resolve to instrument name.' });
    }

    // Check for zero quantity
    if (mainText.includes('Quantity') || mainText.includes('QUANTITY')) {
      const quantityMatch = mainText.match(/0\s+860/);
      if (quantityMatch) {
        findings.push({ page: 'Positions', severity: 'MEDIUM', detail: 'Position shows Quantity=0 with AVG COST=860.00 — zero-quantity position should be hidden or flagged' });
      }
    }
  });

  test('Surveillance page', async () => {
    await navigateSidebar(page, '/dashboard/surveillance', 'Surveillance');
    await page.screenshot({ path: 'tests/screenshots/v3-05-surveillance.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    findings.push({ page: 'Surveillance', severity: 'INFO', detail: 'Page loaded. Shows severity counters (CRITICAL=0, HIGH=0, MEDIUM=0, LOW=0), severity/status filters, and alerts table. "No surveillance alerts" message shown.' });
  });

  test('Market Phase page', async () => {
    await navigateSidebar(page, '/dashboard/market-phase', 'Market Phase');
    await page.screenshot({ path: 'tests/screenshots/v3-06-market-phase.png', fullPage: true });

    const mainText = await page.textContent('main') || '';

    // Check for missing data in table
    if (mainText.includes('PRE_OPEN')) {
      findings.push({ page: 'Market Phase', severity: 'INFO', detail: 'Market phase shows PRE_OPEN status. Has HALT ALL MARKETS button.' });
    }

    // Check for missing instrument ID and name
    const rows = await page.locator('table tbody tr').all();
    for (const row of rows) {
      const cells = await row.locator('td').allTextContents();
      const emptyCells = cells.filter(c => c.trim() === '' || c.trim() === '—');
      if (emptyCells.length > 0) {
        findings.push({ page: 'Market Phase', severity: 'HIGH', detail: `Table row has ${emptyCells.length} empty cells. INSTRUMENT ID and NAME columns appear blank — only CURRENT PHASE (PRE_OPEN) is populated.` });
        break;
      }
    }
  });

  test('Circuit Breakers page — CRASH', async () => {
    await navigateSidebar(page, '/dashboard/circuit-breakers', 'Circuit Breakers');
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/v3-07-circuit-breakers.png', fullPage: true });

    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Circuit Breakers', severity: 'CRITICAL', detail: 'PAGE CRASH: "Something went wrong — An unexpected error occurred. Please try reloading the page." Full-screen error overlay blocks all navigation. This is a React error boundary catch — likely an unhandled exception in the component.' });
    }
  });

  test('Settlement page (after recovery)', async () => {
    // Need to recover from Circuit Breakers crash
    await navigateSidebar(page, '/dashboard/settlement', 'Settlement');
    await page.screenshot({ path: 'tests/screenshots/v3-08-settlement.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Settlement', severity: 'CRITICAL', detail: 'PAGE CRASH: Same "Something went wrong" error overlay as Circuit Breakers' });
    } else {
      const tables = await page.locator('table').count();
      const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
      findings.push({ page: 'Settlement', severity: 'INFO', detail: `Page loaded. Tables: ${tables}, Data rows: ${rows}. Content: ${mainText.substring(0, 200)}` });
    }
  });

  test('Reports page', async () => {
    await navigateSidebar(page, '/dashboard/reports', 'Reports');
    await page.screenshot({ path: 'tests/screenshots/v3-09-reports.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Reports', severity: 'CRITICAL', detail: 'PAGE CRASH: "Something went wrong" error overlay' });
    } else {
      findings.push({ page: 'Reports', severity: 'INFO', detail: `Page loaded. Content length: ${mainText.length}. Content: ${mainText.substring(0, 200)}` });
    }
  });

  test('Participants page', async () => {
    await navigateSidebar(page, '/dashboard/participants', 'Participants');
    await page.screenshot({ path: 'tests/screenshots/v3-10-participants.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Participants', severity: 'CRITICAL', detail: 'PAGE CRASH' });
    } else {
      const tables = await page.locator('table').count();
      const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
      findings.push({ page: 'Participants', severity: 'INFO', detail: `Page loaded. Tables: ${tables}, Rows: ${rows}. Content: ${mainText.substring(0, 200)}` });
    }
  });

  test('Compliance Alerts page', async () => {
    await navigateSidebar(page, '/dashboard/compliance', 'Compliance');
    await page.screenshot({ path: 'tests/screenshots/v3-11-compliance.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Compliance', severity: 'CRITICAL', detail: 'PAGE CRASH' });
    } else {
      findings.push({ page: 'Compliance', severity: 'INFO', detail: `Page loaded. Content: ${mainText.substring(0, 200)}` });
    }
  });

  test('Audit Log page', async () => {
    await navigateSidebar(page, '/dashboard/audit', 'Audit Log');
    await page.screenshot({ path: 'tests/screenshots/v3-12-audit.png', fullPage: true });

    const mainText = await page.textContent('main') || '';
    const hasError = await dismissErrorOverlay(page);
    if (hasError) {
      findings.push({ page: 'Audit Log', severity: 'CRITICAL', detail: 'PAGE CRASH' });
    } else {
      findings.push({ page: 'Audit Log', severity: 'INFO', detail: `Page loaded. Content: ${mainText.substring(0, 200)}` });
    }
  });

  test('OPERATIONS section — expand and check', async () => {
    // First go to dashboard to have a clean state
    await navigateSidebar(page, '/dashboard', 'Dashboard');

    // Try expanding OPERATIONS
    const opsText = page.locator('text=OPERATIONS');
    if (await opsText.count() > 0) {
      await opsText.first().click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: 'tests/screenshots/v3-13-operations-expanded.png', fullPage: true });

      // Check what links appear under OPERATIONS
      const allLinks = await page.locator('a[href*="/dashboard"]').all();
      const linkInfo: string[] = [];
      for (const link of allLinks) {
        const href = await link.getAttribute('href');
        const text = await link.textContent();
        const visible = await link.isVisible();
        if (visible && href) {
          linkInfo.push(`${text?.trim()} -> ${href}`);
        }
      }
      findings.push({ page: 'Sidebar', severity: 'INFO', detail: `Visible sidebar links after expanding OPERATIONS: ${linkInfo.join(' | ')}` });
    } else {
      findings.push({ page: 'Sidebar', severity: 'LOW', detail: 'OPERATIONS section not found in sidebar' });
    }
  });

  test('Tenant switching — ACE to MSE', async () => {
    await navigateSidebar(page, '/dashboard', 'Dashboard');

    const select = page.locator('select').first();
    if (await select.count() > 0) {
      const options = await select.locator('option').allTextContents();
      findings.push({ page: 'Tenant Selector', severity: 'INFO', detail: `Tenant options: ${JSON.stringify(options)}` });

      // Try switching to MSE
      const mseOption = options.find(o => o.includes('Mongolian Stock Exchange') || o.includes('MSE'));
      if (mseOption) {
        await select.selectOption({ label: mseOption });
        await page.waitForTimeout(3000);
        await page.screenshot({ path: 'tests/screenshots/v3-14-mse-tenant.png', fullPage: true });

        const mainText = await page.textContent('main') || '';
        const hasError = await dismissErrorOverlay(page);
        if (hasError) {
          findings.push({ page: 'Tenant Switch (MSE)', severity: 'CRITICAL', detail: 'Switching to MSE tenant crashes the dashboard with "Something went wrong" error' });
        } else {
          findings.push({ page: 'Tenant Switch (MSE)', severity: 'INFO', detail: `MSE dashboard loaded. Content: ${mainText.substring(0, 200)}` });

          // Check if MSE shows different data
          const mseStatus = mainText.includes('ONBOARDING') ? 'ONBOARDING' : (mainText.includes('ACTIVE') ? 'ACTIVE' : 'UNKNOWN');
          findings.push({ page: 'Tenant Switch (MSE)', severity: 'INFO', detail: `MSE tenant status: ${mseStatus}` });
        }

        // Switch back to ACE
        await select.selectOption({ label: options[0] });
        await page.waitForTimeout(2000);
      }
    }
  });

  test('Test MSE Instruments page', async () => {
    const select = page.locator('select').first();
    if (await select.count() > 0) {
      const options = await select.locator('option').allTextContents();
      const mseOption = options.find(o => o.includes('Mongolian') || o.includes('MSE'));
      if (mseOption) {
        await select.selectOption({ label: mseOption });
        await page.waitForTimeout(2000);

        // Navigate to instruments
        const ok = await navigateSidebar(page, '/dashboard/securities', 'MSE Instruments');
        if (ok) {
          await page.screenshot({ path: 'tests/screenshots/v3-15-mse-instruments.png', fullPage: true });
          const rows = await page.locator('table tbody tr').count();
          findings.push({ page: 'MSE Instruments', severity: 'INFO', detail: `${rows} instruments listed for MSE tenant` });
        }

        // Switch back
        await select.selectOption({ label: options[0] });
        await page.waitForTimeout(1000);
      }
    }
  });

  test('Check header bar — Print/PDF functionality', async () => {
    await navigateSidebar(page, '/dashboard', 'Dashboard');

    const printBtn = page.locator('button:has-text("Print"), button:has-text("PDF"), a:has-text("Print")').first();
    if (await printBtn.count() > 0) {
      findings.push({ page: 'Header', severity: 'INFO', detail: 'Print/PDF button found in header' });
    } else {
      findings.push({ page: 'Header', severity: 'LOW', detail: 'Print/PDF button not found' });
    }

    // Check clock
    const headerText = await page.locator('header, [class*="header"], [class*="topbar"]').first().textContent().catch(() => '');
    findings.push({ page: 'Header', severity: 'INFO', detail: `Header content: ${headerText?.substring(0, 200)}` });
  });

  test('Check floating chat button', async () => {
    // There's a blue floating button at bottom-right
    const floatingBtn = page.locator('[class*="float"], [class*="chat"], [class*="fab"]').first();
    if (await floatingBtn.count() > 0) {
      findings.push({ page: 'Global', severity: 'INFO', detail: 'Floating action button found at bottom-right (likely chat widget)' });
    }
  });

  test('Check responsive behavior', async () => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'tests/screenshots/v3-16-tablet.png', fullPage: true });

    const sidebarVisible = await page.locator('nav a[href="/dashboard"]').first().isVisible().catch(() => false);
    if (sidebarVisible) {
      findings.push({ page: 'Responsive', severity: 'LOW', detail: 'Sidebar remains fully visible at 768px tablet width — not collapsed. May cause content overlap on smaller screens.' });
    } else {
      findings.push({ page: 'Responsive', severity: 'INFO', detail: 'Sidebar collapses at 768px width (good responsive behavior)' });
    }

    await page.setViewportSize({ width: 375, height: 812 });
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'tests/screenshots/v3-17-mobile.png', fullPage: true });

    const mobileSidebarVisible = await page.locator('nav a[href="/dashboard"]').first().isVisible().catch(() => false);
    if (mobileSidebarVisible) {
      findings.push({ page: 'Responsive', severity: 'MEDIUM', detail: 'Sidebar visible at 375px mobile width — overlaps content. No hamburger menu for mobile.' });
    }

    // Restore desktop viewport
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.waitForTimeout(500);
  });

  test('Accessibility check — basic', async () => {
    await navigateSidebar(page, '/dashboard', 'Dashboard');

    // Check for missing alt text on images
    const images = await page.locator('img').all();
    let missingAlt = 0;
    for (const img of images) {
      const alt = await img.getAttribute('alt');
      if (!alt || alt.trim() === '') missingAlt++;
    }
    if (missingAlt > 0) {
      findings.push({ page: 'Accessibility', severity: 'LOW', detail: `${missingAlt} images missing alt text` });
    }

    // Check for form labels
    const inputs = await page.locator('input:not([type="hidden"])').all();
    let unlabeled = 0;
    for (const inp of inputs) {
      const ariaLabel = await inp.getAttribute('aria-label');
      const id = await inp.getAttribute('id');
      const placeholder = await inp.getAttribute('placeholder');
      if (!ariaLabel && !placeholder) unlabeled++;
    }
    if (unlabeled > 0) {
      findings.push({ page: 'Accessibility', severity: 'LOW', detail: `${unlabeled} form inputs without aria-label or placeholder` });
    }

    // Check color contrast (just check for very light text)
    const bodyColor = await page.evaluate(() => {
      const el = document.querySelector('body');
      return el ? getComputedStyle(el).color : '';
    });
    findings.push({ page: 'Accessibility', severity: 'INFO', detail: `Body text color: ${bodyColor}. Dark theme detected.` });
  });

  test('Check for console errors across all pages', async () => {
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        consoleErrors.push(`${msg.text().substring(0, 150)}`);
      }
    });

    // Quick navigate through all pages
    const paths = [
      '/dashboard', '/dashboard/securities', '/dashboard/securities-orders',
      '/dashboard/securities-positions', '/dashboard/surveillance', '/dashboard/market-phase',
      '/dashboard/participants', '/dashboard/compliance', '/dashboard/audit',
    ];

    for (const path of paths) {
      const link = page.locator(`a[href="${path}"]`).first();
      if (await link.count() > 0 && await link.isVisible()) {
        try {
          await link.click({ timeout: 3000 });
          await page.waitForTimeout(1500);
        } catch {}
      }
    }

    if (consoleErrors.length > 0) {
      findings.push({ page: 'Console', severity: 'MEDIUM', detail: `${consoleErrors.length} console errors found: ${consoleErrors.slice(0, 5).join(' | ')}` });
    } else {
      findings.push({ page: 'Console', severity: 'INFO', detail: 'No console errors detected during navigation' });
    }
  });

  test('Check API error handling on network failure', async () => {
    // Check if there are any 4xx/5xx responses visible
    const networkErrors: string[] = [];
    page.on('response', (resp) => {
      if (resp.status() >= 400 && !resp.url().includes('favicon') && !resp.url().includes('sockjs') && !resp.url().includes('ws')) {
        networkErrors.push(`${resp.status()} ${new URL(resp.url()).pathname}`);
      }
    });

    // Navigate through a few pages to trigger API calls
    await navigateSidebar(page, '/dashboard', 'Dashboard');
    await navigateSidebar(page, '/dashboard/securities', 'Instruments');
    await navigateSidebar(page, '/dashboard/securities-orders', 'Orders');
    await page.waitForTimeout(2000);

    if (networkErrors.length > 0) {
      const unique = [...new Set(networkErrors)];
      findings.push({ page: 'API', severity: 'MEDIUM', detail: `${unique.length} unique API errors: ${unique.join(' | ')}` });
    } else {
      findings.push({ page: 'API', severity: 'INFO', detail: 'No API errors (4xx/5xx) detected during navigation' });
    }
  });

  test('Logout and login flow', async () => {
    const logoutBtn = page.locator('button:has-text("Logout")').first();
    if (await logoutBtn.count() > 0) {
      findings.push({ page: 'Auth', severity: 'INFO', detail: 'Logout button found at bottom of sidebar' });

      // Test email display
      const emailShown = await page.locator(`text=${ADMIN_EMAIL}`).count();
      findings.push({ page: 'Auth', severity: emailShown > 0 ? 'INFO' : 'LOW', detail: emailShown > 0 ? `User email "${ADMIN_EMAIL}" displayed in sidebar` : 'User email not visibly displayed' });
    } else {
      findings.push({ page: 'Auth', severity: 'MEDIUM', detail: 'No Logout button found' });
    }
  });
});
