import { test, expect, Page, BrowserContext } from '@playwright/test';

const BASE = 'https://admin.garudax.asla.mn';
const ADMIN_EMAIL = 'admin@garudax.mn';
const ADMIN_PASS = 'Adm1n@Pass!';

test.describe.serial('Admin Portal UAT v2 — Sidebar Navigation', () => {
  let page: Page;
  let context: BrowserContext;

  test.beforeAll(async ({ browser }) => {
    context = await browser.newContext({
      ignoreHTTPSErrors: true,
      viewport: { width: 1920, height: 1080 },
    });
    page = await context.newPage();

    // Login
    await page.goto(`${BASE}/login`, { timeout: 30000 });
    await page.waitForLoadState('networkidle');
    await page.locator('input[type="email"], input[name="email"]').first().fill(ADMIN_EMAIL);
    await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL('**/dashboard**', { timeout: 15000 });
    await page.waitForTimeout(2000);
  });

  test.afterAll(async () => {
    await page.close();
    await context.close();
  });

  test('01 — Dashboard Overview', async () => {
    // Already on dashboard after login
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'tests/screenshots/01-dashboard-overview.png', fullPage: true });

    const title = await page.textContent('h1, h2, [class*="title"]');
    console.log(`Dashboard title: "${title?.trim()}"`);

    // Check status cards
    const cards = ['SYSTEM STATUS', 'PENDING KYC', 'ACTIVE MARGIN CALLS', 'SETTLEMENT CYCLES'];
    for (const card of cards) {
      const el = await page.locator(`text=${card}`).count();
      console.log(`  Card "${card}": ${el > 0 ? 'FOUND' : 'MISSING'}`);
    }

    // Check Quick Actions
    const triggerSettlement = await page.locator('button:has-text("Trigger Settlement")').count();
    const haltTrading = await page.locator('button:has-text("Halt All Trading")').count();
    console.log(`  Quick Actions: Trigger Settlement=${triggerSettlement > 0}, Halt All Trading=${haltTrading > 0}`);

    // Check WS status
    const wsStatus = await page.locator('text=WS:').textContent().catch(() => 'not found');
    console.log(`  WebSocket status: ${wsStatus?.trim()}`);

    // Check tenant selector
    const tenantSelector = await page.locator('select, [class*="tenant"], [class*="exchange"]').first().textContent().catch(() => 'not found');
    console.log(`  Tenant selector: ${tenantSelector?.trim()}`);

    // Check for dashes (missing data indicators)
    const mainContent = await page.textContent('main') || '';
    const dashCount = (mainContent.match(/—/g) || []).length;
    console.log(`  Dash placeholders (—) in main content: ${dashCount}`);
    if (dashCount > 0) {
      console.log(`  WARNING: ${dashCount} cards showing "—" instead of real data`);
    }

    // Check OPERATIONS section collapsed
    const opsSection = await page.locator('text=OPERATIONS').count();
    console.log(`  OPERATIONS section in sidebar: ${opsSection > 0 ? 'FOUND' : 'MISSING'}`);
  });

  test('02 — Instruments page', async () => {
    await page.locator('a[href="/dashboard/securities"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/02-instruments.png', fullPage: true });

    const url = page.url();
    console.log(`Instruments URL: ${url}`);
    expect(url).toContain('/dashboard/securities');

    // Check for table or instrument list
    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);

    // Check for search/filter
    const searchInput = await page.locator('input[placeholder*="earch" i], input[type="search"]').count();
    console.log(`  Search input: ${searchInput > 0 ? 'FOUND' : 'MISSING'}`);

    // Check for add/create button
    const addBtn = await page.locator('button:has-text("Add"), button:has-text("Create"), button:has-text("New")').count();
    console.log(`  Add/Create button: ${addBtn > 0 ? 'FOUND' : 'MISSING'}`);

    // Check for any error text
    const mainText = await page.textContent('main') || '';
    if (mainText.includes('Error') || mainText.includes('error')) {
      console.log(`  ERROR TEXT FOUND: ${mainText.substring(mainText.indexOf('rror') - 5, mainText.indexOf('rror') + 50)}`);
    }
    if (mainText.includes('undefined') || mainText.includes('NaN')) {
      console.log(`  WARNING: "undefined" or "NaN" found in page content`);
    }
  });

  test('03 — Orders page', async () => {
    await page.locator('a[href="/dashboard/securities-orders"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/03-orders.png', fullPage: true });

    const url = page.url();
    console.log(`Orders URL: ${url}`);
    expect(url).toContain('/dashboard/securities-orders');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);

    const mainText = await page.textContent('main') || '';
    const hasNoData = mainText.includes('No orders') || mainText.includes('No data') || mainText.includes('empty');
    console.log(`  Empty state message: ${hasNoData}`);
  });

  test('04 — Positions page', async () => {
    await page.locator('a[href="/dashboard/securities-positions"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/04-positions.png', fullPage: true });

    const url = page.url();
    console.log(`Positions URL: ${url}`);
    expect(url).toContain('/dashboard/securities-positions');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('05 — Surveillance page', async () => {
    await page.locator('a[href="/dashboard/surveillance"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/05-surveillance.png', fullPage: true });

    const url = page.url();
    console.log(`Surveillance URL: ${url}`);
    expect(url).toContain('/dashboard/surveillance');

    const mainText = await page.textContent('main') || '';
    console.log(`  Content length: ${mainText.length}`);

    // Check for alert list or monitoring widgets
    const alerts = await page.locator('[class*="alert" i], [class*="warn" i]').count();
    console.log(`  Alert/warning elements: ${alerts}`);
  });

  test('06 — Market Phase page', async () => {
    await page.locator('a[href="/dashboard/market-phase"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/06-market-phase.png', fullPage: true });

    const url = page.url();
    console.log(`Market Phase URL: ${url}`);
    expect(url).toContain('/dashboard/market-phase');

    const mainText = await page.textContent('main') || '';
    console.log(`  Content length: ${mainText.length}`);

    // Check for phase controls
    const buttons = await page.locator('main button').count();
    console.log(`  Buttons in main area: ${buttons}`);
  });

  test('07 — Circuit Breakers page', async () => {
    await page.locator('a[href="/dashboard/circuit-breakers"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/07-circuit-breakers.png', fullPage: true });

    const url = page.url();
    console.log(`Circuit Breakers URL: ${url}`);
    expect(url).toContain('/dashboard/circuit-breakers');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('08 — Settlement page', async () => {
    await page.locator('a[href="/dashboard/settlement"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/08-settlement.png', fullPage: true });

    const url = page.url();
    console.log(`Settlement URL: ${url}`);
    expect(url).toContain('/dashboard/settlement');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('09 — Reports page', async () => {
    await page.locator('a[href="/dashboard/reports"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/09-reports.png', fullPage: true });

    const url = page.url();
    console.log(`Reports URL: ${url}`);
    expect(url).toContain('/dashboard/reports');

    const mainText = await page.textContent('main') || '';
    console.log(`  Content length: ${mainText.length}`);
  });

  test('10 — Participants page', async () => {
    await page.locator('a[href="/dashboard/participants"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/10-participants.png', fullPage: true });

    const url = page.url();
    console.log(`Participants URL: ${url}`);
    expect(url).toContain('/dashboard/participants');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('11 — Compliance Alerts page', async () => {
    await page.locator('a[href="/dashboard/compliance"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/11-compliance.png', fullPage: true });

    const url = page.url();
    console.log(`Compliance URL: ${url}`);
    expect(url).toContain('/dashboard/compliance');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('12 — Audit Log page', async () => {
    await page.locator('a[href="/dashboard/audit"]').first().click();
    await page.waitForTimeout(2000);
    await page.screenshot({ path: 'tests/screenshots/12-audit-log.png', fullPage: true });

    const url = page.url();
    console.log(`Audit Log URL: ${url}`);
    expect(url).toContain('/dashboard/audit');

    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    console.log(`  Tables: ${tables}, Data rows: ${rows}`);
  });

  test('13 — Expand OPERATIONS section and check links', async () => {
    // Try to expand OPERATIONS section if collapsed
    const opsToggle = page.locator('text=OPERATIONS').first();
    if (await opsToggle.count() > 0) {
      await opsToggle.click();
      await page.waitForTimeout(500);
    }

    // Check what links are under OPERATIONS
    const allLinks = await page.locator('nav a, aside a, [class*="sidebar"] a').all();
    console.log('\nAll sidebar navigation links:');
    for (const link of allLinks) {
      const href = await link.getAttribute('href');
      const text = await link.textContent();
      const isVisible = await link.isVisible();
      console.log(`  ${isVisible ? 'VISIBLE' : 'HIDDEN'} | "${text?.trim()}" -> ${href}`);
    }
  });

  test('14 — Check all pages for JS errors and console errors', async () => {
    const jsErrors: string[] = [];
    const consoleErrors: string[] = [];
    const networkErrors: string[] = [];

    page.on('pageerror', (err) => jsErrors.push(err.message));
    page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
    page.on('response', (resp) => {
      if (resp.status() >= 400 && !resp.url().includes('favicon')) {
        networkErrors.push(`${resp.status()} ${resp.url()}`);
      }
    });

    // Navigate through all sidebar links to collect errors
    const sidebarPaths = [
      '/dashboard', '/dashboard/securities', '/dashboard/securities-orders',
      '/dashboard/securities-positions', '/dashboard/surveillance', '/dashboard/market-phase',
      '/dashboard/circuit-breakers', '/dashboard/settlement', '/dashboard/reports',
      '/dashboard/participants', '/dashboard/compliance', '/dashboard/audit',
    ];

    for (const path of sidebarPaths) {
      const link = page.locator(`a[href="${path}"]`).first();
      if (await link.count() > 0) {
        await link.click();
        await page.waitForTimeout(1500);
      }
    }

    console.log('\n--- Error Summary ---');
    console.log(`JavaScript errors: ${jsErrors.length}`);
    jsErrors.forEach(e => console.log(`  JS: ${e.substring(0, 200)}`));
    console.log(`Console errors: ${consoleErrors.length}`);
    consoleErrors.forEach(e => console.log(`  Console: ${e.substring(0, 200)}`));
    console.log(`Network errors (4xx/5xx): ${networkErrors.length}`);
    networkErrors.forEach(e => console.log(`  Network: ${e}`));
  });

  test('15 — Check header bar elements', async () => {
    await page.locator('a[href="/dashboard"]').first().click();
    await page.waitForTimeout(1000);

    // Check Print/PDF button
    const printBtn = await page.locator('text=Print').count() + await page.locator('text=PDF').count();
    console.log(`Print/PDF button: ${printBtn > 0 ? 'FOUND' : 'MISSING'}`);

    // Check WS status indicator
    const wsEl = await page.locator('text=WS:').first().textContent().catch(() => null);
    console.log(`WS indicator: ${wsEl || 'NOT FOUND'}`);

    // Check Unknown status indicator
    const unknownEl = await page.locator('text=Unknown').count();
    console.log(`"Unknown" status indicator: ${unknownEl > 0 ? 'FOUND (potential issue)' : 'NOT FOUND'}`);

    // Check clock
    const timeEl = await page.locator('[class*="time"], [class*="clock"]').first().textContent().catch(() => null);
    console.log(`Clock element: ${timeEl || 'checking alternative...'}`);

    // Check tenant/exchange selector
    const tenantSelect = await page.locator('select').first();
    if (await tenantSelect.count() > 0) {
      const options = await tenantSelect.locator('option').allTextContents();
      console.log(`Tenant selector options: ${options.join(', ')}`);
    }

    await page.screenshot({ path: 'tests/screenshots/15-header-bar.png' });
  });

  test('16 — Check Logout button', async () => {
    const logoutBtn = page.locator('button:has-text("Logout"), a:has-text("Logout")').first();
    const count = await logoutBtn.count();
    console.log(`Logout button: ${count > 0 ? 'FOUND' : 'MISSING'}`);

    // Check user email display
    const emailDisplay = await page.locator(`text=${ADMIN_EMAIL}`).count();
    console.log(`User email displayed: ${emailDisplay > 0 ? 'YES' : 'NO'}`);
  });

  test('17 — Test Trigger Settlement button', async () => {
    await page.locator('a[href="/dashboard"]').first().click();
    await page.waitForTimeout(1000);

    const btn = page.locator('button:has-text("Trigger Settlement")');
    if (await btn.count() > 0) {
      console.log('Trigger Settlement button found, clicking...');

      // Listen for network response
      const responsePromise = page.waitForResponse(resp => resp.url().includes('settlement'), { timeout: 5000 }).catch(() => null);
      await btn.click();
      await page.waitForTimeout(2000);

      const resp = await responsePromise;
      if (resp) {
        console.log(`  Settlement API response: ${resp.status()} ${resp.url()}`);
      } else {
        console.log('  No settlement API call detected (may be client-side only)');
      }

      // Check for confirmation dialog or toast
      const dialog = await page.locator('[role="dialog"], [class*="modal"], [class*="toast"], [class*="notification"]').count();
      console.log(`  Dialog/toast after click: ${dialog > 0 ? 'YES' : 'NO'}`);

      await page.screenshot({ path: 'tests/screenshots/17-after-trigger-settlement.png', fullPage: true });
    } else {
      console.log('Trigger Settlement button NOT FOUND');
    }
  });

  test('18 — Test Halt All Trading button (observe only)', async () => {
    const btn = page.locator('button:has-text("Halt All Trading")');
    if (await btn.count() > 0) {
      const isDisabled = await btn.isDisabled();
      const classes = await btn.getAttribute('class') || '';
      console.log(`Halt All Trading button: FOUND, disabled=${isDisabled}, classes="${classes}"`);
      // DO NOT CLICK — destructive action
      console.log('  (Not clicking — destructive action)');
    } else {
      console.log('Halt All Trading button NOT FOUND');
    }
  });

  test('19 — Check responsive behavior at tablet width', async () => {
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(1000);
    await page.screenshot({ path: 'tests/screenshots/19-tablet-view.png', fullPage: true });

    // Check if sidebar collapses
    const sidebar = page.locator('nav, aside, [class*="sidebar"]').first();
    const sidebarVisible = await sidebar.isVisible().catch(() => false);
    console.log(`Sidebar visible at 768px: ${sidebarVisible}`);

    // Check for hamburger menu
    const hamburger = await page.locator('[class*="hamburger"], [class*="menu-toggle"], button[aria-label*="menu" i]').count();
    console.log(`Hamburger menu: ${hamburger > 0 ? 'FOUND' : 'MISSING'}`);

    // Restore
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.waitForTimeout(500);
  });

  test('20 — Check for broken images and missing icons', async () => {
    await page.locator('a[href="/dashboard"]').first().click();
    await page.waitForTimeout(1000);

    const images = await page.locator('img').all();
    let broken = 0;
    for (const img of images) {
      const naturalWidth = await img.evaluate((el: HTMLImageElement) => el.naturalWidth);
      const src = await img.getAttribute('src');
      if (naturalWidth === 0) {
        broken++;
        console.log(`  Broken image: ${src}`);
      }
    }
    console.log(`Total images: ${images.length}, Broken: ${broken}`);

    // Check for SVG icons that might not render
    const svgs = await page.locator('svg').count();
    console.log(`SVG icons: ${svgs}`);
  });

  test('21 — Verify all status card values', async () => {
    await page.locator('a[href="/dashboard"]').first().click();
    await page.waitForTimeout(2000);

    // Extract all card values
    const mainContent = await page.textContent('main') || '';
    console.log('Dashboard main content (first 500 chars):');
    console.log(mainContent.substring(0, 500));

    // Check for specific data issues
    const issues: string[] = [];
    if (mainContent.includes('—')) issues.push('Dash (—) placeholders found — data may not be loading');
    if (mainContent.includes('undefined')) issues.push('"undefined" text found');
    if (mainContent.includes('NaN')) issues.push('"NaN" text found');
    if (mainContent.includes('[object')) issues.push('"[object Object]" text found');
    if (mainContent.includes('null')) issues.push('"null" text found');
    if (mainContent.includes('loading') || mainContent.includes('Loading')) issues.push('Loading state may be stuck');

    if (issues.length > 0) {
      console.log('\nISSUES FOUND:');
      issues.forEach(i => console.log(`  - ${i}`));
    } else {
      console.log('\nNo obvious data issues found');
    }
  });

  test('22 — Check tenant dropdown behavior', async () => {
    const select = page.locator('select').first();
    if (await select.count() > 0) {
      const options = await select.locator('option').allTextContents();
      console.log(`Tenant dropdown options: ${JSON.stringify(options)}`);

      const currentValue = await select.inputValue();
      console.log(`Current selection: ${currentValue}`);

      // Try switching tenant if multiple options
      if (options.length > 1) {
        const otherOption = options.find(o => !o.includes(currentValue));
        if (otherOption) {
          console.log(`Switching to: ${otherOption}`);
          await select.selectOption({ label: otherOption });
          await page.waitForTimeout(2000);
          await page.screenshot({ path: 'tests/screenshots/22-tenant-switch.png', fullPage: true });

          // Check if data refreshed
          const newContent = await page.textContent('main') || '';
          console.log(`Content after switch: ${newContent.substring(0, 200)}`);

          // Switch back
          await select.selectOption({ label: options[0] });
          await page.waitForTimeout(1000);
        }
      }
    } else {
      console.log('No tenant dropdown found');
    }
  });
});
