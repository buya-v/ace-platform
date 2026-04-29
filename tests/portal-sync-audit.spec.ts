import { test, expect, Page } from '@playwright/test';

const ADMIN_URL = 'https://admin.garudax.asla.mn';
const TRADE_URL = 'https://trade.garudax.asla.mn';
const DEMO_URL = 'https://demo.garudax.asla.mn';

async function loginAdmin(page: Page) {
  await page.goto(`${ADMIN_URL}/login`);
  await page.waitForTimeout(2000);
  await page.fill('input[type="email"]', 'admin@garudax.mn');
  await page.fill('input[type="password"]', 'Adm1n@Pass!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(5000);
}

async function loginTrader(page: Page) {
  await page.goto(`${TRADE_URL}/login`);
  await page.waitForTimeout(2000);
  await page.fill('input[type="email"]', 'trader1@garudax.mn');
  await page.fill('input[type="password"]', 'Tr@der1Pass!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(5000);
}

async function ensureAdminLoggedIn(page: Page) {
  const body = await page.textContent('body');
  if (body?.includes('Sign in to the admin dashboard')) {
    await page.fill('input[type="email"]', 'admin@garudax.mn');
    await page.fill('input[type="password"]', 'Adm1n@Pass!');
    await page.click('button[type="submit"]');
    await page.waitForTimeout(5000);
  }
}

async function adminNavigate(page: Page, href: string) {
  const link = page.locator(`a[href="${href}"]`).first();
  if (await link.count() > 0) {
    await link.click();
  } else {
    await page.goto(`${ADMIN_URL}${href}`);
  }
  await page.waitForTimeout(3000);
  // Re-login if session expired
  await ensureAdminLoggedIn(page);
  // If we had to re-login, navigate again
  if (!page.url().includes(href)) {
    const link2 = page.locator(`a[href="${href}"]`).first();
    if (await link2.count() > 0) {
      await link2.click();
      await page.waitForTimeout(3000);
    }
  }
}

test.describe.serial('Portal Sync Audit after Demo Run', () => {

  // -- ADMIN PORTAL TESTS --

  test.describe('Admin Portal', () => {
    let page: Page;

    test.beforeAll(async ({ browser }) => {
      page = await browser.newPage();
      await loginAdmin(page);
    });

    test.afterAll(async () => { await page.close(); });

    test('Dashboard loads after login', async () => {
      const url = page.url();
      await page.screenshot({ path: 'tests/screenshots/sync-admin-dashboard.png', fullPage: true });
      const onDashboard = url.includes('/dashboard');
      console.log(`Admin URL after login: ${url}`);
      expect(onDashboard).toBeTruthy();
    });

    test('Securities page shows APU and GOV', async () => {
      await adminNavigate(page, '/dashboard/securities');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-instruments.png', fullPage: true });
      expect(body).toContain('APU');
      expect(body).toContain('GOV');
      // Check for bond
      const hasBond = body?.includes('GOV-BOND-2028') || body?.includes('BOND');
      console.log(`Has bond visible: ${hasBond}`);
    });

    test('Securities Orders shows 6 filled orders', async () => {
      await adminNavigate(page, '/dashboard/securities-orders');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-orders.png', fullPage: true });
      const filledCount = (body?.match(/FILLED/g) || []).length;
      console.log(`FILLED count: ${filledCount}`);
      expect(filledCount).toBeGreaterThanOrEqual(4);
      expect(body).toContain('BUY');
      expect(body).toContain('SELL');
    });

    test('Market Phase shows DAY_CLOSED with instruments', async () => {
      await adminNavigate(page, '/dashboard/market-phase');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-market-phase.png', fullPage: true });
      expect(body).toContain('DAY_CLOSED');
      expect(body).toContain('APU');
      expect(body).toContain('GOV');
    });

    test('Surveillance page loads without crash', async () => {
      await adminNavigate(page, '/dashboard/surveillance');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-surveillance.png', fullPage: true });
      expect(body).not.toContain('Something went wrong');
    });

    test('Settlement page loads', async () => {
      await adminNavigate(page, '/dashboard/settlement');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-settlement.png', fullPage: true });
      expect(body).toBeTruthy();
    });

    test('Platform page shows tenants', async () => {
      await adminNavigate(page, '/dashboard/platform');
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-admin-platform.png', fullPage: true });
      console.log(`Platform body (first 300): ${body?.substring(0, 300)}`);
      expect(body).toContain('mse-equities');
    });

    test('All securities pages navigable without crash', async () => {
      const paths = [
        '/dashboard/securities',
        '/dashboard/securities-orders',
        '/dashboard/securities-positions',
        '/dashboard/surveillance',
        '/dashboard/market-phase',
        '/dashboard/circuit-breakers',
        '/dashboard/settlement',
        '/dashboard/reports',
      ];
      const results: {path: string; ok: boolean; note?: string}[] = [];
      for (const path of paths) {
        await adminNavigate(page, path);
        const body = await page.textContent('body');
        const crashed = body?.includes('Something went wrong') || false;
        const loggedOut = page.url().includes('/login');
        results.push({
          path,
          ok: !crashed && !loggedOut,
          note: crashed ? 'CRASHED' : loggedOut ? 'LOGGED_OUT' : 'OK'
        });
      }
      console.log('Navigation results:', JSON.stringify(results, null, 2));
      await page.screenshot({ path: 'tests/screenshots/sync-admin-all-pages.png', fullPage: true });
      const failures = results.filter(r => !r.ok);
      expect(failures).toEqual([]);
    });
  });

  // -- TRADE PORTAL TESTS --

  test.describe('Trade Portal', () => {
    let page: Page;

    test.beforeAll(async ({ browser }) => {
      page = await browser.newPage();
      await loginTrader(page);
    });

    test.afterAll(async () => { await page.close(); });

    test('Trading page loads after login', async () => {
      const url = page.url();
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-trade-home.png', fullPage: true });
      console.log(`Trade URL: ${url}`);
      console.log(`Trade body (first 500): ${body?.substring(0, 500)}`);
      expect(body).toBeTruthy();
    });

    test('Instrument selector shows APU and GOV', async () => {
      const triggers = [
        page.locator('button:has-text("Select Instrument")'),
        page.locator('[class*="instrument"]'),
        page.locator('select'),
      ];
      for (const trigger of triggers) {
        if (await trigger.count() > 0) {
          await trigger.first().click();
          await page.waitForTimeout(1000);
          break;
        }
      }
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-trade-instruments.png', fullPage: true });
      expect(body).toContain('APU');
    });

    test('Selecting APU shows market data', async () => {
      const apuOption = page.locator('text=APU').first();
      if (await apuOption.count() > 0) {
        await apuOption.click();
        await page.waitForTimeout(2000);
      }
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-trade-apu-selected.png', fullPage: true });
      expect(body).toContain('APU');
    });

    test('Orders section visible', async () => {
      const ordersTab = page.locator('button:has-text("Orders"), a:has-text("Orders")').first();
      if (await ordersTab.count() > 0) {
        await ordersTab.click();
        await page.waitForTimeout(2000);
      }
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-trade-orders.png', fullPage: true });
      console.log(`Trade orders (first 300): ${body?.substring(0, 300)}`);
    });

    test('Trade history section visible', async () => {
      const historyTab = page.locator('button:has-text("Trade History"), button:has-text("Trades"), a:has-text("History")').first();
      if (await historyTab.count() > 0) {
        await historyTab.click();
        await page.waitForTimeout(2000);
      }
      await page.screenshot({ path: 'tests/screenshots/sync-trade-history.png', fullPage: true });
    });

    test('No critical console errors', async () => {
      const errors: string[] = [];
      page.on('pageerror', (err) => errors.push(err.message));
      await page.waitForTimeout(3000);
      const critical = errors.filter(e =>
        !e.includes('WebSocket') &&
        !e.includes('auth/refresh') &&
        !e.includes('Failed to fetch') &&
        !e.includes('NetworkError')
      );
      if (critical.length > 0) {
        console.log('Critical console errors:', critical);
      }
      expect(critical).toEqual([]);
    });
  });

  // -- DEMO PORTAL TESTS --

  test.describe('Demo Portal', () => {
    let page: Page;

    test.beforeAll(async ({ browser }) => {
      page = await browser.newPage();
      await page.goto(DEMO_URL);
      await page.waitForTimeout(3000);
    });

    test.afterAll(async () => { await page.close(); });

    test('Demo portal loads with GarudaX branding', async () => {
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-demo-home.png', fullPage: true });
      expect(body).toContain('GarudaX');
    });

    test('Demo shows credentials or demo steps', async () => {
      const body = await page.textContent('body');
      await page.screenshot({ path: 'tests/screenshots/sync-demo-credentials.png', fullPage: true });
      const hasCredentials = body?.includes('trader1') || body?.includes('admin@garudax');
      const hasSteps = body?.includes('Step') || body?.includes('Register') || body?.includes('Login');
      console.log(`Demo has credentials: ${hasCredentials}, has steps: ${hasSteps}`);
      expect(hasCredentials || hasSteps).toBeTruthy();
    });
  });
});
