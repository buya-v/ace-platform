/**
 * GarudaX UAT Playwright Helpers
 *
 * Reusable helper functions for acceptance testing the GarudaX admin portal.
 * Admin portal: https://admin.garudax.asla.mn
 *
 * Key design decisions:
 * - All helpers handle missing elements gracefully (won't throw on timeout)
 * - SPA navigation via sidebar clicks to preserve in-memory auth tokens
 * - Screenshots saved with timestamps to uat/screenshots/
 * - Soft assertions preferred for non-blocking checks
 */

import { type Page, type APIRequestContext, expect } from '@playwright/test';

// ---------------------------------------------------------------------------
// Node.js ambient declarations (avoids @types/node dependency)
// ---------------------------------------------------------------------------

declare const __dirname: string;
declare const process: { env: Record<string, string | undefined> };
declare function require(id: string): unknown;
interface NodePathModule {
  join(...parts: string[]): string;
}
interface NodeFsModule {
  existsSync(path: string): boolean;
  mkdirSync(path: string, opts?: { recursive?: boolean }): void;
}

// Loaded lazily at runtime — won't fail type-checking
const nodePath = require('path') as NodePathModule;
const nodeFs = require('fs') as NodeFsModule;

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const _procEnv: Record<string, string | undefined> = (typeof process !== 'undefined') ? process.env : {};

export const ADMIN_BASE_URL: string = _procEnv.ADMIN_BASE_URL || 'https://admin.garudax.asla.mn';
export const ADMIN_EMAIL: string = _procEnv.ADMIN_EMAIL || 'admin@garudax.mn';
export const ADMIN_PASSWORD: string = _procEnv.ADMIN_PASSWORD || 'Adm1n@GarudaX!';

const DEFAULT_WAIT_TIMEOUT = 15_000;

function getScreenshotDir(): string {
  const base = (typeof __dirname !== 'undefined') ? __dirname : '.';
  return nodePath.join(base, 'screenshots');
}

// ---------------------------------------------------------------------------
// Auth helpers
// ---------------------------------------------------------------------------

/**
 * Login to the admin portal.
 * Fills email/password, submits the form, and waits for /dashboard URL.
 * Returns true if login succeeded, false otherwise.
 *
 * Handles: already logged in, missing form, wrong credentials.
 */
export async function loginAsAdmin(page: Page, baseURL?: string): Promise<boolean> {
  const base = baseURL || ADMIN_BASE_URL;

  try {
    await page.goto(`${base}/login`, {
      timeout: DEFAULT_WAIT_TIMEOUT,
      waitUntil: 'domcontentloaded',
    });
  } catch {
    // Portal may be unreachable — caller should use isPortalAvailable first
    return false;
  }

  // Already on dashboard?
  if (page.url().includes('/dashboard')) return true;

  // Wait for the login form
  const emailInput = page.getByLabel('Email', { exact: false });
  try {
    await emailInput.waitFor({ state: 'visible', timeout: 8_000 });
  } catch {
    // Form not found — maybe already logged in or different page structure
    return page.url().includes('/dashboard');
  }

  // Fill credentials
  await emailInput.click();
  await emailInput.fill(ADMIN_EMAIL);

  const passwordInput = page.getByLabel('Password', { exact: false });
  await passwordInput.click();
  await passwordInput.fill(ADMIN_PASSWORD);

  // Intercept login response for diagnostics
  const loginResponsePromise = page.waitForResponse(
    (resp) => resp.url().includes('/auth/login'),
    { timeout: DEFAULT_WAIT_TIMEOUT },
  ).catch(() => null);

  // Submit
  await page.locator('button[type="submit"]').click();

  const loginResponse = await loginResponsePromise;
  if (loginResponse && loginResponse.status() !== 200) {
    const body = await loginResponse.text().catch(() => '');
    console.warn(`[UAT] Login API returned ${loginResponse.status()}: ${body.substring(0, 200)}`);
  }

  // Wait for redirect to dashboard
  try {
    await page.waitForURL(/\/dashboard/, { timeout: 10_000 });
    return true;
  } catch {
    const errorEl = page.locator('[role="alert"], [class*="error"], [class*="Error"]');
    if (await errorEl.count() > 0) {
      const errorText = await errorEl.first().textContent().catch(() => '');
      console.warn(`[UAT] Login error message: ${errorText}`);
    }
    return false;
  }
}

/**
 * Fetch a JWT token directly via the login API — for use in direct API testing.
 * Returns the access_token string or null if the request fails.
 */
export async function getAdminToken(request: APIRequestContext, baseURL?: string): Promise<string | null> {
  const base = baseURL || ADMIN_BASE_URL;
  // The admin portal's API base is /api/v1
  const loginURL = `${base}/api/v1/auth/login`;

  try {
    const response = await request.post(loginURL, {
      data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD },
      headers: { 'Content-Type': 'application/json' },
    });

    if (!response.ok()) {
      const body = await response.text().catch(() => '');
      console.warn(`[UAT] getAdminToken: login returned ${response.status()}: ${body.substring(0, 200)}`);
      return null;
    }

    const body = await response.json().catch(() => null);
    // Support both snake_case and PascalCase token fields
    return body?.access_token ?? body?.AccessToken ?? null;
  } catch (err) {
    console.warn(`[UAT] getAdminToken failed: ${err}`);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Navigation helpers
// ---------------------------------------------------------------------------

/**
 * Navigate to an admin portal section by clicking the sidebar link.
 * Uses text matching so it works even if the URL path changes.
 *
 * The admin SPA stores auth tokens in React state — always navigate via
 * sidebar (SPA navigation) rather than page.goto() to preserve the session.
 *
 * @param section - Text label of the sidebar link, e.g. "Participants", "Order Book"
 */
export async function navigateTo(page: Page, section: string): Promise<void> {
  // Expand collapsed sidebar sections first
  const collapsedToggles = page.locator(
    '[data-testid="sidebar"] button[aria-expanded="false"]',
  );
  const toggleCount = await collapsedToggles.count();
  for (let i = 0; i < toggleCount; i++) {
    await collapsedToggles.nth(i).click().catch(() => {});
    await page.waitForTimeout(80);
  }

  // Try sidebar link with data-testid first, then any sidebar link
  const sidebarLink = page
    .locator('[data-testid="sidebar"] a, nav a, aside a')
    .filter({ hasText: section })
    .first();

  try {
    await sidebarLink.waitFor({ state: 'visible', timeout: 5_000 });
    await sidebarLink.click();
  } catch {
    // Fallback: any link or button with matching text
    const fallbackEl = page
      .locator(`a, button`)
      .filter({ hasText: section })
      .first();
    await fallbackEl.click().catch((err) => {
      console.warn(`[UAT] navigateTo("${section}"): could not find element — ${err}`);
    });
  }

  await page.waitForTimeout(400);
}

// ---------------------------------------------------------------------------
// Wait helpers
// ---------------------------------------------------------------------------

/**
 * Wait for loading spinners and skeleton placeholders to disappear.
 * Gracefully exits after timeout even if they remain.
 *
 * @param timeout - Maximum wait in milliseconds (default 10_000)
 */
export async function waitForData(page: Page, timeout = 10_000): Promise<void> {
  const spinnerSelectors = [
    '[class*="spinner"]',
    '[class*="Spinner"]',
    '[class*="loading"]',
    '[class*="Loading"]',
    '[class*="skeleton"]',
    '[class*="Skeleton"]',
    '[aria-busy="true"]',
    '[role="progressbar"]',
  ];

  const combined = spinnerSelectors.join(', ');

  try {
    // Wait until no spinner is visible
    await page.waitForFunction(
      (sel) => document.querySelector(sel) === null,
      combined,
      { timeout },
    );
  } catch {
    // Timeout is acceptable — data may be partially loaded
  }
}

// ---------------------------------------------------------------------------
// Screenshot helpers
// ---------------------------------------------------------------------------

/**
 * Capture a full-page screenshot with a timestamp prefix.
 * Saves to uat/screenshots/<timestamp>-<name>.png
 *
 * Never throws — screenshot failures are silently ignored.
 */
export async function captureScreenshot(page: Page, name: string): Promise<void> {
  try {
    // Ensure screenshots directory exists
    const screenshotDir = getScreenshotDir();
    if (!nodeFs.existsSync(screenshotDir)) {
      nodeFs.mkdirSync(screenshotDir, { recursive: true });
    }

    const ts = new Date().toISOString().replace(/[:.]/g, '-').replace('T', '_').slice(0, 19);
    const safeName = name.replace(/[^a-zA-Z0-9_-]/g, '-');
    const filePath = nodePath.join(screenshotDir, `${ts}-${safeName}.png`);

    await page.screenshot({ path: filePath, fullPage: true });
  } catch {
    // Screenshot failures should never break test execution
  }
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

/**
 * Assert there are no visible server errors on the page.
 * Checks for: HTTP 500 error text, INTERNAL_ERROR codes, error toast banners.
 *
 * Uses soft assertions — failures are recorded but do not stop the test.
 */
export async function assertNoErrors(page: Page): Promise<void> {
  const bodyText = await page.textContent('body').catch(() => '');

  expect.soft(bodyText, 'Page should not contain "500"').not.toContain('500 Internal Server Error');
  expect.soft(bodyText, 'Page should not contain INTERNAL_ERROR').not.toContain('INTERNAL_ERROR');
  expect.soft(bodyText, 'Page should not contain "Something went wrong"').not.toContain('Something went wrong');

  // Check for error toast notifications
  const toastErrors = page.locator(
    '[class*="toast"][class*="error"], [role="alert"][class*="error"], [class*="Toast"][class*="Error"]',
  );
  const toastCount = await toastErrors.count();
  expect.soft(toastCount, 'No error toast notifications').toBe(0);
}

/**
 * Assert a data table has at least `min` rows.
 * Checks <table>, role="grid", and DataGrid patterns.
 *
 * @param min - Minimum number of rows expected
 */
export async function assertTableHasRows(page: Page, min: number): Promise<void> {
  // Wait briefly for table to render
  await waitForData(page, 5_000);

  const rowSelectors = [
    'table tbody tr',
    '[role="grid"] [role="row"]:not([class*="header"])',
    '[class*="DataGrid"] [class*="row"]:not([class*="header"])',
    '[class*="dataGrid"] [class*="row"]',
  ];

  let rowCount = 0;
  for (const sel of rowSelectors) {
    const count = await page.locator(sel).count();
    if (count > rowCount) rowCount = count;
  }

  expect.soft(rowCount, `Table should have at least ${min} rows (found ${rowCount})`).toBeGreaterThanOrEqual(min);
}

/**
 * Assert an element is visible, optionally containing specific text.
 * Uses soft assertions.
 *
 * @param selector - CSS selector or test ID
 * @param text - Optional text content to check for
 */
export async function assertVisible(page: Page, selector: string, text?: string): Promise<void> {
  const locator = page.locator(selector).first();

  try {
    await locator.waitFor({ state: 'visible', timeout: 5_000 });
    const isVisible = await locator.isVisible();
    expect.soft(isVisible, `Element "${selector}" should be visible`).toBeTruthy();

    if (text !== undefined) {
      const content = await locator.textContent().catch(() => '');
      expect.soft(content, `Element "${selector}" should contain "${text}"`).toContain(text);
    }
  } catch {
    expect.soft(false, `Element "${selector}" was not found or not visible`).toBeTruthy();
  }
}

// ---------------------------------------------------------------------------
// Bot/chat helpers
// ---------------------------------------------------------------------------

/**
 * Open the GarudaX Bot floating chat panel.
 * Clicks the bot button (aria-label "Open GarudaX Bot") and waits for the panel.
 *
 * Returns true if the panel opened, false if the button was not found.
 */
export async function openBotChat(page: Page): Promise<boolean> {
  // If panel is already open, do nothing
  const existingPanel = page.locator('[role="dialog"][aria-label="GarudaX Bot Chat"]');
  if (await existingPanel.isVisible().catch(() => false)) return true;

  const botButton = page.locator('[aria-label="Open GarudaX Bot"]');
  try {
    await botButton.waitFor({ state: 'visible', timeout: 5_000 });
    await botButton.click();
  } catch {
    console.warn('[UAT] openBotChat: bot button not found');
    return false;
  }

  try {
    await page
      .locator('[role="dialog"][aria-label="GarudaX Bot Chat"]')
      .waitFor({ state: 'visible', timeout: 5_000 });
    return true;
  } catch {
    console.warn('[UAT] openBotChat: chat panel did not appear');
    return false;
  }
}

/**
 * Type a message into the bot chat, submit it, and wait for a response.
 * Returns the text of the last bot message, or empty string on failure.
 *
 * @param msg - Message text to send
 */
export async function sendBotMessage(page: Page, msg: string): Promise<string> {
  const chatInput = page.locator('[aria-label="Chat message input"]');
  try {
    await chatInput.waitFor({ state: 'visible', timeout: 5_000 });
  } catch {
    console.warn('[UAT] sendBotMessage: chat input not visible');
    return '';
  }

  await chatInput.fill(msg);
  await chatInput.press('Enter');

  // Wait for typing indicator to appear and then disappear
  const typingIndicator = page.locator('[class*="typingIndicator"], [class*="typing"]');
  try {
    await typingIndicator.waitFor({ state: 'visible', timeout: 3_000 });
    await typingIndicator.waitFor({ state: 'hidden', timeout: 15_000 });
  } catch {
    // Typing indicator may not appear for instant responses
    await page.waitForTimeout(1_000);
  }

  // Get the last bot message
  const botMessages = page.locator('[class*="messageBubble"][class*="botMessage"], [class*="bot-message"]');
  const count = await botMessages.count();
  if (count === 0) return '';

  const lastMessage = botMessages.last();
  return (await lastMessage.textContent().catch(() => '')) ?? '';
}

/**
 * Assert the last bot response matches a pattern (string or RegExp).
 *
 * @param pattern - String to contain or RegExp to match
 */
export async function assertBotResponse(page: Page, pattern: string | RegExp): Promise<void> {
  const botMessages = page.locator(
    '[class*="messageBubble"][class*="botMessage"], [class*="bot-message"], [class*="botMessage"]',
  );
  const count = await botMessages.count();

  if (count === 0) {
    expect.soft(false, 'No bot messages found to assert against').toBeTruthy();
    return;
  }

  const lastText = (await botMessages.last().textContent().catch(() => '')) ?? '';

  if (typeof pattern === 'string') {
    expect.soft(lastText, `Bot response should contain "${pattern}"`).toContain(pattern);
  } else {
    expect.soft(lastText, `Bot response should match ${pattern}`).toMatch(pattern);
  }
}

// ---------------------------------------------------------------------------
// Portal availability check
// ---------------------------------------------------------------------------

/**
 * Gracefully check if the admin portal is reachable.
 * Use this at the start of test files to implement the skip pattern:
 *
 *   const available = await isPortalAvailable(page);
 *   if (!available) test.skip();
 *
 * Returns true if the portal responds with a non-5xx status.
 */
export async function isPortalAvailable(page: Page, baseURL?: string): Promise<boolean> {
  const base = baseURL || ADMIN_BASE_URL;

  try {
    const response = await page.goto(base, {
      timeout: 8_000,
      waitUntil: 'domcontentloaded',
    });

    if (!response) return false;
    const status = response.status();
    // Accept 200-499 (even 401/403 means the portal is up)
    return status < 500;
  } catch {
    return false;
  }
}
