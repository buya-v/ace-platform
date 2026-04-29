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

async function login(page: Page) {
  await page.goto(`${BASE}/login`, { timeout: 30000 });
  await page.waitForLoadState('networkidle');
  await page.locator('input[type="email"], input[name="email"]').first().fill(ADMIN_EMAIL);
  await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
  await page.locator('button[type="submit"]').first().click();
  await page.waitForURL('**/dashboard**', { timeout: 15000 });
  await page.waitForTimeout(2000);
}

async function expandOperations(page: Page) {
  const ops = page.locator('text=OPERATIONS');
  if (await ops.count() > 0) {
    await ops.first().click();
    await page.waitForTimeout(500);
  }
}

async function checkForCrash(page: Page): Promise<boolean> {
  const reloadBtn = page.locator('button:has-text("Reload page"), button:has-text("Reload")');
  const errorText = page.locator('text=Something went wrong');
  return (await reloadBtn.count() > 0 && await reloadBtn.first().isVisible().catch(() => false)) ||
         (await errorText.count() > 0 && await errorText.first().isVisible().catch(() => false));
}

async function navigateAndTest(page: Page, href: string, pageName: string): Promise<void> {
  // Check if we need to recover from a crash
  if (await checkForCrash(page)) {
    findings.push({ page: pageName, severity: 'HIGH', detail: 'Had to re-login due to previous page crash' });
    await login(page);
    await expandOperations(page);
  }

  const link = page.locator(`a[href="${href}"]`).first();
  if (await link.count() === 0) {
    findings.push({ page: pageName, severity: 'HIGH', detail: `Sidebar link "${href}" not found` });
    return;
  }

  try {
    await link.click({ timeout: 5000 });
  } catch (e: any) {
    // Overlay blocking — re-login
    findings.push({ page: pageName, severity: 'HIGH', detail: `Click blocked by overlay: ${e.message.substring(0, 80)}` });
    await login(page);
    await expandOperations(page);
    try {
      await page.locator(`a[href="${href}"]`).first().click({ timeout: 5000 });
    } catch {
      findings.push({ page: pageName, severity: 'CRITICAL', detail: 'Could not navigate even after re-login' });
      return;
    }
  }

  await page.waitForTimeout(2500);
  const screenshotName = pageName.toLowerCase().replace(/[\s\/]+/g, '-');
  await page.screenshot({ path: `tests/screenshots/v4-${screenshotName}.png`, fullPage: true });

  // Check for crash
  if (await checkForCrash(page)) {
    findings.push({ page: pageName, severity: 'CRITICAL', detail: 'PAGE CRASH: "Something went wrong" error overlay displayed' });
    return;
  }

  // Check URL
  const url = page.url();
  if (url.includes('/login')) {
    findings.push({ page: pageName, severity: 'CRITICAL', detail: 'Redirected to login — session lost' });
    return;
  }

  // Analyze content
  const mainText = await page.textContent('main') || '';
  const tables = await page.locator('table').count();
  const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
  const buttons = await page.locator('main button').count();

  // Check for data issues
  if (mainText.includes('undefined') && !mainText.includes('is undefined')) {
    findings.push({ page: pageName, severity: 'HIGH', detail: '"undefined" text visible on page' });
  }
  if (/\bNaN\b/.test(mainText)) {
    findings.push({ page: pageName, severity: 'HIGH', detail: '"NaN" text visible on page' });
  }
  if (mainText.includes('[object Object]')) {
    findings.push({ page: pageName, severity: 'HIGH', detail: '"[object Object]" text visible on page' });
  }

  // Check for raw UUIDs
  if (/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/.test(mainText)) {
    findings.push({ page: pageName, severity: 'MEDIUM', detail: 'Raw UUID displayed in page content (should resolve to human-readable name)' });
  }

  // Check for "—" dash placeholders
  const dashCount = (mainText.match(/—/g) || []).length;
  if (dashCount > 2) {
    findings.push({ page: pageName, severity: 'MEDIUM', detail: `${dashCount} dash (—) placeholders — possible missing data` });
  }

  findings.push({ page: pageName, severity: 'INFO', detail: `Loaded OK. URL: ${url}. Tables: ${tables}, Rows: ${rows}, Buttons: ${buttons}. Content (100ch): ${mainText.substring(0, 100).replace(/\n/g, ' ')}` });
}

test.describe.serial('Admin Portal UAT v4 — OPERATIONS Pages', () => {
  let page: Page;
  let context: BrowserContext;

  test.beforeAll(async ({ browser }) => {
    context = await browser.newContext({
      ignoreHTTPSErrors: true,
      viewport: { width: 1920, height: 1080 },
    });
    page = await context.newPage();
    await login(page);
    await expandOperations(page);
  });

  test.afterAll(async () => {
    console.log('\n\n====================================================');
    console.log('     OPERATIONS PAGES UAT REPORT');
    console.log('====================================================\n');

    const critical = findings.filter(f => f.severity === 'CRITICAL');
    const high = findings.filter(f => f.severity === 'HIGH');
    const medium = findings.filter(f => f.severity === 'MEDIUM');
    const low = findings.filter(f => f.severity === 'LOW');
    const info = findings.filter(f => f.severity === 'INFO');

    console.log(`Total findings: ${findings.length}`);
    console.log(`  CRITICAL: ${critical.length}  HIGH: ${high.length}  MEDIUM: ${medium.length}  LOW: ${low.length}  INFO: ${info.length}`);

    for (const sev of ['CRITICAL', 'HIGH', 'MEDIUM', 'LOW', 'INFO']) {
      const items = findings.filter(f => f.severity === sev);
      if (items.length > 0) {
        console.log(`\n--- ${sev} ---`);
        items.forEach(f => console.log(`  [${f.page}] ${f.detail}`));
      }
    }

    console.log('\n====================================================\n');
    await page.close();
    await context.close();
  });

  test('Platform page', async () => {
    await navigateAndTest(page, '/dashboard/platform', 'Platform');
  });

  test('System Health page', async () => {
    await navigateAndTest(page, '/dashboard/monitoring', 'System Health');
  });

  test('Fee Management page', async () => {
    await navigateAndTest(page, '/dashboard/fees', 'Fee Management');
  });

  test('Tickets page', async () => {
    await navigateAndTest(page, '/dashboard/tickets', 'Tickets');
  });

  test('Risk Overview page', async () => {
    await navigateAndTest(page, '/dashboard/risk', 'Risk Overview');
  });

  test('Margin Calls page', async () => {
    await navigateAndTest(page, '/dashboard/margin', 'Margin Calls');
  });

  test('Commodity Book page', async () => {
    await navigateAndTest(page, '/dashboard/orderbook', 'Commodity Book');
  });

  test('Commodity Positions page', async () => {
    await navigateAndTest(page, '/dashboard/positions', 'Commodity Positions');
  });

  test('Warehouse page', async () => {
    await navigateAndTest(page, '/dashboard/warehouse', 'Warehouse');
  });

  // Now test some interactive flows on pages that loaded successfully
  test('System Health — check service cards', async () => {
    if (await checkForCrash(page)) {
      await login(page);
      await expandOperations(page);
    }

    await page.locator('a[href="/dashboard/monitoring"]').first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2000);

    if (await checkForCrash(page)) {
      findings.push({ page: 'System Health (detail)', severity: 'CRITICAL', detail: 'Cannot access System Health page' });
      return;
    }

    const mainText = await page.textContent('main') || '';

    // Check for service status indicators
    const serviceNames = ['matching', 'clearing', 'margin', 'settlement', 'auth', 'compliance', 'gateway', 'market-data', 'warehouse'];
    for (const svc of serviceNames) {
      if (mainText.toLowerCase().includes(svc)) {
        findings.push({ page: 'System Health', severity: 'INFO', detail: `Service "${svc}" found in health dashboard` });
      }
    }

    // Check for any "down" or "unhealthy" indicators
    if (mainText.toLowerCase().includes('down') || mainText.toLowerCase().includes('unhealthy') || mainText.toLowerCase().includes('error')) {
      findings.push({ page: 'System Health', severity: 'HIGH', detail: 'Services showing down/unhealthy/error status' });
    }
  });

  test('Fee Management — check fee structure', async () => {
    if (await checkForCrash(page)) {
      await login(page);
      await expandOperations(page);
    }

    await page.locator('a[href="/dashboard/fees"]').first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2000);

    if (await checkForCrash(page)) {
      findings.push({ page: 'Fee Management (detail)', severity: 'CRITICAL', detail: 'Cannot access Fee Management page' });
      return;
    }

    const tables = await page.locator('table').count();
    if (tables > 0) {
      const headers = await page.locator('table th').allTextContents();
      findings.push({ page: 'Fee Management', severity: 'INFO', detail: `Table headers: ${headers.join(', ')}` });
    }
  });

  test('Commodity Book — check order book', async () => {
    if (await checkForCrash(page)) {
      await login(page);
      await expandOperations(page);
    }

    await page.locator('a[href="/dashboard/orderbook"]').first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2000);

    if (await checkForCrash(page)) {
      findings.push({ page: 'Commodity Book (detail)', severity: 'CRITICAL', detail: 'Cannot access Commodity Book page' });
      return;
    }

    const mainText = await page.textContent('main') || '';
    // Check for bid/ask sections
    if (mainText.toLowerCase().includes('bid') || mainText.toLowerCase().includes('ask')) {
      findings.push({ page: 'Commodity Book', severity: 'INFO', detail: 'Bid/Ask sections found' });
    }

    // Check for instrument selector
    const selects = await page.locator('select').count();
    findings.push({ page: 'Commodity Book', severity: 'INFO', detail: `${selects} dropdown selectors on page` });
  });

  test('Warehouse — check receipts', async () => {
    if (await checkForCrash(page)) {
      await login(page);
      await expandOperations(page);
    }

    await page.locator('a[href="/dashboard/warehouse"]').first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(2000);

    if (await checkForCrash(page)) {
      findings.push({ page: 'Warehouse (detail)', severity: 'CRITICAL', detail: 'Cannot access Warehouse page' });
      return;
    }

    const mainText = await page.textContent('main') || '';
    const tables = await page.locator('table').count();
    const rows = tables > 0 ? await page.locator('table tbody tr').count() : 0;
    findings.push({ page: 'Warehouse', severity: 'INFO', detail: `Tables: ${tables}, Rows: ${rows}. Content includes: ${mainText.substring(0, 150).replace(/\n/g, ' ')}` });
  });
});
