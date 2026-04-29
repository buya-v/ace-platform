import { test, expect, Page, BrowserContext } from '@playwright/test';

const BASE = 'https://admin.garudax.asla.mn';
const ADMIN_EMAIL = 'admin@garudax.mn';
const ADMIN_PASS = 'Adm1n@Pass!';

// All pages to test - will also discover from sidebar
const KNOWN_PAGES = [
  { name: 'Dashboard/Overview', path: '/dashboard' },
  { name: 'Instruments', path: '/dashboard/securities' },
  { name: 'Orders', path: '/dashboard/securities-orders' },
  { name: 'Positions', path: '/dashboard/securities-positions' },
  { name: 'Surveillance', path: '/dashboard/surveillance' },
  { name: 'Market Phase', path: '/dashboard/market-phase' },
  { name: 'Circuit Breakers', path: '/dashboard/circuit-breakers' },
  { name: 'Settlement', path: '/dashboard/settlement' },
  { name: 'Reports', path: '/dashboard/reports' },
  { name: 'Platform', path: '/dashboard/platform' },
  { name: 'System Health', path: '/dashboard/monitoring' },
  { name: 'Fee Management', path: '/dashboard/fees' },
  { name: 'Tickets', path: '/dashboard/tickets' },
  { name: 'Participants', path: '/dashboard/participants' },
  { name: 'Compliance', path: '/dashboard/compliance' },
  { name: 'Audit Log', path: '/dashboard/audit' },
  { name: 'Margin Calls', path: '/dashboard/margin-calls' },
  { name: 'Users', path: '/dashboard/users' },
  { name: 'Settings', path: '/dashboard/settings' },
  { name: 'Notifications', path: '/dashboard/notifications' },
  { name: 'Risk Management', path: '/dashboard/risk' },
  { name: 'Market Data', path: '/dashboard/market-data' },
  { name: 'Trading', path: '/dashboard/trading' },
  { name: 'Clearing', path: '/dashboard/clearing' },
  { name: 'Custody', path: '/dashboard/custody' },
  { name: 'Corporate Actions', path: '/dashboard/corporate-actions' },
  { name: 'Indices', path: '/dashboard/indices' },
];

interface PageIssue {
  page: string;
  path: string;
  type: 'error' | 'warning' | 'info';
  detail: string;
}

const issues: PageIssue[] = [];
const pageResults: { name: string; path: string; status: string; url: string; contentLength: number; loadTime: number }[] = [];

test.describe('Admin Portal UAT - Full Navigation', () => {
  let context: BrowserContext;
  let page: Page;

  test.beforeAll(async ({ browser }) => {
    context = await browser.newContext({
      ignoreHTTPSErrors: true,
      viewport: { width: 1920, height: 1080 },
    });
    page = await context.newPage();

    // Login
    console.log('--- Logging in ---');
    await page.goto(`${BASE}/login`, { timeout: 30000 });
    await page.waitForLoadState('networkidle');

    // Screenshot login page
    await page.screenshot({ path: 'tests/screenshots/00-login-page.png', fullPage: true });

    // Try to find email/username input
    const emailInput = page.locator('input[type="email"], input[name="email"], input[name="username"], input[placeholder*="email" i], input[placeholder*="user" i]').first();
    const passwordInput = page.locator('input[type="password"]').first();

    if (await emailInput.count() === 0) {
      console.log('WARNING: No email input found on login page. Page content:');
      const bodyText = await page.textContent('body');
      console.log(bodyText?.substring(0, 500));
      // Try alternative login approaches
    }

    await emailInput.fill(ADMIN_EMAIL);
    await passwordInput.fill(ADMIN_PASS);
    await page.screenshot({ path: 'tests/screenshots/01-login-filled.png', fullPage: true });

    const submitBtn = page.locator('button[type="submit"], button:has-text("Login"), button:has-text("Sign in"), button:has-text("Log in")').first();
    await submitBtn.click();

    // Wait for navigation
    try {
      await page.waitForURL('**/dashboard**', { timeout: 15000 });
      console.log('Login successful, redirected to:', page.url());
    } catch {
      console.log('Login may have failed. Current URL:', page.url());
      const bodyText = await page.textContent('body');
      console.log('Page content:', bodyText?.substring(0, 300));
    }

    await page.screenshot({ path: 'tests/screenshots/02-after-login.png', fullPage: true });
  });

  test.afterAll(async () => {
    // Print summary report
    console.log('\n\n========== UAT SUMMARY REPORT ==========\n');
    console.log('Pages tested:', pageResults.length);
    console.log('Issues found:', issues.length);

    console.log('\n--- Page Results ---');
    for (const r of pageResults) {
      console.log(`  ${r.status === 'OK' ? 'PASS' : 'FAIL'} | ${r.name} (${r.path}) | URL: ${r.url} | Content: ${r.contentLength} chars | Load: ${r.loadTime}ms`);
    }

    console.log('\n--- Issues ---');
    for (const i of issues) {
      console.log(`  [${i.type.toUpperCase()}] ${i.page} (${i.path}): ${i.detail}`);
    }

    console.log('\n========================================\n');
    await page.close();
    await context.close();
  });

  test('Discover sidebar navigation links', async () => {
    // Discover all links in the sidebar
    await page.waitForTimeout(2000);
    const sidebarLinks = await page.locator('nav a, aside a, [class*="sidebar"] a, [class*="nav"] a, [role="navigation"] a').all();

    const discoveredPaths = new Set<string>();
    for (const link of sidebarLinks) {
      const href = await link.getAttribute('href');
      const text = await link.textContent();
      if (href && href.startsWith('/dashboard')) {
        discoveredPaths.add(href);
        console.log(`  Sidebar link: "${text?.trim()}" -> ${href}`);
      }
    }

    // Also check for any menu items that might use JavaScript navigation
    const allLinks = await page.locator('a[href*="/dashboard"]').all();
    for (const link of allLinks) {
      const href = await link.getAttribute('href');
      if (href) discoveredPaths.add(href);
    }

    console.log(`\nDiscovered ${discoveredPaths.size} unique dashboard paths from sidebar`);
    console.log('Paths:', Array.from(discoveredPaths).join(', '));
  });

  // Test each known page
  for (const p of KNOWN_PAGES) {
    test(`Page: ${p.name} (${p.path})`, async () => {
      const jsErrors: string[] = [];
      const consoleErrors: string[] = [];
      const networkErrors: string[] = [];

      // Collect JS errors
      const errorHandler = (err: Error) => jsErrors.push(err.message);
      page.on('pageerror', errorHandler);

      // Collect console errors
      const consoleHandler = (msg: any) => {
        if (msg.type() === 'error') consoleErrors.push(msg.text());
      };
      page.on('console', consoleHandler);

      // Collect failed network requests
      const responseHandler = (response: any) => {
        if (response.status() >= 400 && !response.url().includes('favicon')) {
          networkErrors.push(`${response.status()} ${response.url()}`);
        }
      };
      page.on('response', responseHandler);

      const startTime = Date.now();

      // Navigate
      try {
        await page.goto(`${BASE}${p.path}`, { timeout: 20000, waitUntil: 'networkidle' });
      } catch (navErr: any) {
        issues.push({ page: p.name, path: p.path, type: 'error', detail: `Navigation failed: ${navErr.message}` });
        pageResults.push({ name: p.name, path: p.path, status: 'FAIL', url: '', contentLength: 0, loadTime: 0 });
        return;
      }

      const loadTime = Date.now() - startTime;
      await page.waitForTimeout(1500);

      const currentUrl = page.url();
      const bodyText = await page.textContent('body') || '';

      // Take screenshot
      const screenshotName = p.name.toLowerCase().replace(/[\s\/]+/g, '-');
      await page.screenshot({ path: `tests/screenshots/page-${screenshotName}.png`, fullPage: true });

      // Check: redirected to login?
      if (currentUrl.includes('/login')) {
        issues.push({ page: p.name, path: p.path, type: 'error', detail: 'Redirected to login page (auth issue or page does not exist)' });
        pageResults.push({ name: p.name, path: p.path, status: 'FAIL', url: currentUrl, contentLength: bodyText.length, loadTime });
        // Re-login
        const emailInput = page.locator('input[type="email"], input[name="email"], input[name="username"]').first();
        if (await emailInput.count() > 0) {
          await emailInput.fill(ADMIN_EMAIL);
          await page.locator('input[type="password"]').first().fill(ADMIN_PASS);
          await page.locator('button[type="submit"]').first().click();
          try { await page.waitForURL('**/dashboard**', { timeout: 10000 }); } catch {}
        }
        page.removeListener('pageerror', errorHandler);
        page.removeListener('console', consoleHandler);
        page.removeListener('response', responseHandler);
        return;
      }

      // Check: blank page?
      if (bodyText.length < 50) {
        issues.push({ page: p.name, path: p.path, type: 'error', detail: `Page appears blank (content length: ${bodyText.length})` });
      }

      // Check: "undefined", "NaN", "[object Object]" in visible text
      const mainContent = await page.textContent('main, [class*="content"], [class*="main"], [class*="container"]') || bodyText;
      if (mainContent.includes('undefined') && !mainContent.includes('is undefined')) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: 'Page contains "undefined" text' });
      }
      if (/\bNaN\b/.test(mainContent)) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: 'Page contains "NaN" text' });
      }
      if (mainContent.includes('[object Object]')) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: 'Page contains "[object Object]" text' });
      }

      // Check: error messages visible on page
      const errorElements = await page.locator('[class*="error" i], [class*="alert-danger"], [role="alert"]').all();
      for (const el of errorElements) {
        const text = await el.textContent();
        if (text && text.trim().length > 0) {
          issues.push({ page: p.name, path: p.path, type: 'warning', detail: `Error element visible: "${text.trim().substring(0, 100)}"` });
        }
      }

      // Check: 404 or "not found" text
      if (/not found|404|page doesn't exist/i.test(mainContent)) {
        issues.push({ page: p.name, path: p.path, type: 'error', detail: 'Page shows "not found" or 404 message' });
      }

      // Check: loading spinners still visible after timeout
      const spinners = await page.locator('[class*="spinner"], [class*="loading"], [class*="skeleton"]').all();
      let visibleSpinners = 0;
      for (const sp of spinners) {
        if (await sp.isVisible()) visibleSpinners++;
      }
      if (visibleSpinners > 0) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: `${visibleSpinners} loading spinner(s) still visible after page load` });
      }

      // JS errors
      if (jsErrors.length > 0) {
        issues.push({ page: p.name, path: p.path, type: 'error', detail: `JavaScript errors: ${jsErrors.join('; ').substring(0, 300)}` });
      }

      // Console errors
      if (consoleErrors.length > 0) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: `Console errors: ${consoleErrors.join('; ').substring(0, 300)}` });
      }

      // Network errors
      if (networkErrors.length > 0) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: `Failed network requests: ${networkErrors.join('; ').substring(0, 300)}` });
      }

      // Slow load
      if (loadTime > 5000) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: `Slow page load: ${loadTime}ms` });
      }

      // Check for broken images
      const images = await page.locator('img').all();
      let brokenImages = 0;
      for (const img of images) {
        const naturalWidth = await img.evaluate((el: HTMLImageElement) => el.naturalWidth);
        if (naturalWidth === 0) brokenImages++;
      }
      if (brokenImages > 0) {
        issues.push({ page: p.name, path: p.path, type: 'warning', detail: `${brokenImages} broken image(s) found` });
      }

      const status = jsErrors.length === 0 && !currentUrl.includes('/login') && bodyText.length >= 50 ? 'OK' : 'FAIL';
      pageResults.push({ name: p.name, path: p.path, status, url: currentUrl, contentLength: bodyText.length, loadTime });

      console.log(`  ${status} | ${p.name} | ${loadTime}ms | ${bodyText.length} chars | JS errors: ${jsErrors.length} | Console errors: ${consoleErrors.length} | Network errors: ${networkErrors.length}`);

      // Remove listeners to avoid duplicates
      page.removeListener('pageerror', errorHandler);
      page.removeListener('console', consoleHandler);
      page.removeListener('response', responseHandler);
    });
  }

  test('Check interactive elements on dashboard', async () => {
    await page.goto(`${BASE}/dashboard`, { timeout: 20000, waitUntil: 'networkidle' });
    await page.waitForTimeout(2000);

    // Check all buttons are clickable (not disabled unexpectedly)
    const buttons = await page.locator('button:visible').all();
    console.log(`Dashboard has ${buttons.length} visible buttons`);

    // Check dropdowns
    const selects = await page.locator('select:visible, [role="combobox"]:visible').all();
    console.log(`Dashboard has ${selects.length} visible dropdowns/selects`);

    // Check tables have data or show empty state
    const tables = await page.locator('table:visible').all();
    for (let i = 0; i < tables.length; i++) {
      const rows = await tables[i].locator('tbody tr').count();
      console.log(`  Table ${i + 1}: ${rows} data rows`);
    }

    // Check charts/graphs loaded
    const canvases = await page.locator('canvas:visible').all();
    const svgCharts = await page.locator('svg[class*="chart" i], svg[class*="graph" i], [class*="chart"] svg, [class*="recharts"]').all();
    console.log(`Dashboard has ${canvases.length} canvas elements, ${svgCharts.length} SVG chart elements`);
  });

  test('Verify logout functionality', async () => {
    // Find logout button/link
    const logoutBtn = page.locator('a:has-text("Logout"), a:has-text("Sign out"), button:has-text("Logout"), button:has-text("Sign out"), a[href*="logout"], a[href*="signout"]').first();

    if (await logoutBtn.count() > 0) {
      console.log('Logout button found');
    } else {
      // Check in user menu/dropdown
      const userMenu = page.locator('[class*="user-menu"], [class*="avatar"], [class*="profile"], button:has-text("admin")').first();
      if (await userMenu.count() > 0) {
        await userMenu.click();
        await page.waitForTimeout(500);
        const logoutInMenu = page.locator('a:has-text("Logout"), a:has-text("Sign out"), button:has-text("Logout")').first();
        if (await logoutInMenu.count() > 0) {
          console.log('Logout found in user dropdown menu');
        } else {
          issues.push({ page: 'Global', path: '/', type: 'warning', detail: 'No logout button found in user dropdown' });
        }
      } else {
        issues.push({ page: 'Global', path: '/', type: 'warning', detail: 'No logout button or user menu found' });
      }
    }
  });
});
