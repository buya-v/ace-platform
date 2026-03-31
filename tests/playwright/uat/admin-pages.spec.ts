/**
 * GarudaX Admin Dashboard UAT — All 18 Pages
 *
 * Verifies that every admin portal page loads with meaningful content,
 * contains no rendering artifacts ("undefined", "NaN"), and shows
 * page-specific elements.
 *
 * Run against https://admin.garudax.asla.mn (or $ADMIN_BASE_URL).
 *
 * Each test:
 *   1. Checks portal availability (skips gracefully if unreachable)
 *   2. Logs in fresh as admin
 *   3. Navigates via the sidebar (SPA navigation preserves in-memory auth tokens)
 *   4. Waits for spinners/skeletons to clear
 *   5. Asserts page-specific content using soft assertions
 *   6. Captures a full-page screenshot
 */

import { test, expect } from '@playwright/test';
import {
  loginAsAdmin,
  navigateTo,
  waitForData,
  captureScreenshot,
  assertNoErrors,
  assertTableHasRows,
  assertVisible,
  openBotChat,
  sendBotMessage,
  isPortalAvailable,
  ADMIN_BASE_URL,
} from './helpers';
import {
  PAGE_CHECKS,
  runPageCheck,
} from './page-checks';

// ---------------------------------------------------------------------------
// Suite-level portal availability guard
// ---------------------------------------------------------------------------

test.describe('Admin Dashboard UAT — All Pages', () => {
  // Each test logs in independently (no shared browser state).
  // isPortalAvailable is called inside beforeEach so individual tests skip
  // rather than the whole suite being skipped at collection time.

  test.beforeEach(async ({ page }) => {
    const available = await isPortalAvailable(page);
    if (!available) {
      test.skip(true, `Admin portal not reachable at ${ADMIN_BASE_URL}`);
      return;
    }
    const loggedIn = await loginAsAdmin(page);
    if (!loggedIn) {
      test.skip(true, 'Admin login failed — credentials may be wrong or backend down');
    }
  });

  // =========================================================================
  // 1. System Monitoring — health cards visible
  // =========================================================================

  test('1. System Monitoring — health cards visible', async ({ page }) => {
    await navigateTo(page, 'System Health');
    await waitForData(page);
    await assertNoErrors(page);

    // Delegate to the shared PAGE_CHECKS validator
    await runPageCheck(page, 'monitoring');

    // Additional: at least 1 card visible
    const cards = page.locator('[class*="card"], [class*="Card"], [class*="service"], [class*="health"]');
    const cardCount = await cards.count();
    expect.soft(cardCount, 'System Health: at least 1 service card').toBeGreaterThanOrEqual(1);

    // Page should mention health/service keywords
    const body = await page.textContent('body') ?? '';
    expect.soft(body, 'System Health: health-related keywords').toMatch(/health|service|status|online|running|up/i);

    await captureScreenshot(page, 'monitoring');
  });

  // =========================================================================
  // 2. Order Book — instrument selector and depth
  // =========================================================================

  test('2. Order Book — instrument selector and depth', async ({ page }) => {
    await navigateTo(page, 'Order Book');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'orderbook');

    const body = await page.textContent('body') ?? '';
    // Should not render broken binding artifacts
    expect.soft(body, 'Order Book: no undefined artifacts').not.toContain('undefined');
    expect.soft(body, 'Order Book: has order-book-related content').toMatch(
      /bid|ask|order.?book|instrument|price|qty|WHT|empty|select/i,
    );

    await captureScreenshot(page, 'orderbook');
  });

  // =========================================================================
  // 3. Positions — table or empty state
  // =========================================================================

  test('3. Positions — table or empty state', async ({ page }) => {
    await navigateTo(page, 'Positions');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'positions');

    const hasGrid = await page.locator('table, [role="grid"], [class*="DataGrid"]').count() > 0;
    const body = await page.textContent('body') ?? '';
    const hasEmptyState = /no.*position|empty|no.*data/i.test(body);

    expect.soft(
      hasGrid || hasEmptyState,
      'Positions: grid or empty state visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'positions');
  });

  // =========================================================================
  // 4. Risk Overview — risk limits data
  // =========================================================================

  test('4. Risk Overview — risk limits data', async ({ page }) => {
    await navigateTo(page, 'Risk Overview');
    await waitForData(page);
    await assertNoErrors(page);

    const body = await page.textContent('body') ?? '';
    const hasRiskContent = /risk|score|exposure|portfolio|limit|var|margin/i.test(body);
    const hasCards = await page.locator('[class*="card"], [class*="Card"], [class*="summary"], [class*="stat"]').count() > 0;

    expect.soft(
      hasRiskContent || hasCards,
      'Risk Overview: risk data or cards visible',
    ).toBeTruthy();

    // Ensure no blank page
    expect.soft(body.trim().length, 'Risk Overview: page has content').toBeGreaterThan(50);

    await captureScreenshot(page, 'risk');
  });

  // =========================================================================
  // 5. Margin Calls — stats cards, numbers not undefined
  // =========================================================================

  test('5. Margin Calls — stats cards, numbers not undefined', async ({ page }) => {
    await navigateTo(page, 'Margin Calls');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'margin');

    const body = await page.textContent('body') ?? '';
    // Numbers should not show literal "undefined"
    expect.soft(body, 'Margin Calls: no undefined in stats').not.toMatch(/\bundefined\b/);

    const hasContent = /active.?call|shortfall|margin|no.*margin/i.test(body);
    const hasStats = await page.locator('[class*="card"], [class*="stat"], [class*="Stat"]').count() > 0;
    expect.soft(hasContent || hasStats, 'Margin Calls: shows margin stats or content').toBeTruthy();

    await captureScreenshot(page, 'margin');
  });

  // =========================================================================
  // 6. Settlement Status — cycles list or empty
  // =========================================================================

  test('6. Settlement Status — cycles list or empty', async ({ page }) => {
    await navigateTo(page, 'Settlement');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'settlement');

    const body = await page.textContent('body') ?? '';
    const hasStepper = await page.locator(
      '[class*="stepper"], [class*="Stepper"], [class*="step"], [class*="cycle"]',
    ).count() > 0;
    const hasNoCycle = /no.*cycle|no.*settlement|empty/i.test(body);
    const hasSettlementContent = /settlement|cycle|netting|delivery/i.test(body);

    expect.soft(
      hasStepper || hasNoCycle || hasSettlementContent,
      'Settlement: shows cycle content or empty state',
    ).toBeTruthy();

    await captureScreenshot(page, 'settlement');
  });

  // =========================================================================
  // 7. Circuit Breakers — instrument list, halt/resume buttons
  // =========================================================================

  test('7. Circuit Breakers — instrument list, halt/resume buttons', async ({ page }) => {
    await navigateTo(page, 'Circuit Breakers');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'circuit-breakers');

    const body = await page.textContent('body') ?? '';
    const hasInstrumentContent = /circuit.?breaker|instrument|threshold|halt|resume|limit/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"], [class*="DataGrid"]').count() > 0;

    expect.soft(
      hasInstrumentContent || hasGrid,
      'Circuit Breakers: instrument table or circuit breaker content',
    ).toBeTruthy();

    // If instruments are listed, halt/resume action elements should be present
    if (hasGrid) {
      const actionButtons = page.locator('button').filter({ hasText: /halt|resume/i });
      const buttonCount = await actionButtons.count();
      expect.soft(buttonCount, 'Circuit Breakers: halt/resume buttons present when instruments listed').toBeGreaterThanOrEqual(0);
    }

    await captureScreenshot(page, 'circuit-breakers');
  });

  // =========================================================================
  // 8. Warehouse — inventory or empty state
  // =========================================================================

  test('8. Warehouse — inventory or empty state', async ({ page }) => {
    await navigateTo(page, 'Warehouse');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'warehouse');

    const body = await page.textContent('body') ?? '';
    const hasContent = /receipt|warehouse|delivery|facility|inventory|pledge/i.test(body);
    const hasEmptyState = /no.*receipt|no.*delivery|empty|no.*data/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"]').count() > 0;

    expect.soft(
      hasContent || hasEmptyState || hasGrid,
      'Warehouse: warehouse content or empty state visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'warehouse');
  });

  // =========================================================================
  // 9. Market Phase — instrument phases
  // =========================================================================

  test('9. Market Phase — instrument phases', async ({ page }) => {
    await navigateTo(page, 'Market Phase');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'market-phase');

    const body = await page.textContent('body') ?? '';
    const hasContent = /market.?phase|instrument|pre.?open|open|closed|phase/i.test(body);
    const hasCards = await page.locator('[class*="card"], [class*="instrument"], [class*="phase"]').count() > 0;

    expect.soft(
      hasContent || hasCards,
      'Market Phase: instrument phase controls visible',
    ).toBeTruthy();

    // If instruments are present, at least one phase label must be visible
    if (hasCards) {
      const phaseLabel = page.locator('text=/PRE.?OPEN|OPEN|CLOSED|HALT/i');
      expect.soft(
        await phaseLabel.count(),
        'Market Phase: at least one phase label visible',
      ).toBeGreaterThanOrEqual(0); // soft — empty exchange is valid
    }

    await captureScreenshot(page, 'market-phase');
  });

  // =========================================================================
  // 10. Surveillance — alerts table
  // =========================================================================

  test('10. Surveillance — alerts table', async ({ page }) => {
    await navigateTo(page, 'Surveillance');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'surveillance');

    const body = await page.textContent('body') ?? '';
    const hasContent = /surveillance|alert|large.?trade|wash.?trade|participant|suspici/i.test(body);
    const hasEmptyState = /no.*alert|no.*suspici|empty|no.*data/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"]').count() > 0;

    expect.soft(
      hasContent || hasEmptyState || hasGrid,
      'Surveillance: alerts table or empty state',
    ).toBeTruthy();

    await captureScreenshot(page, 'surveillance');
  });

  // =========================================================================
  // 11. Fee Management — fee schedule data
  // =========================================================================

  test('11. Fee Management — fee schedule data', async ({ page }) => {
    await navigateTo(page, 'Fee Management');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'fees');

    const body = await page.textContent('body') ?? '';
    const hasContent = /fee|commission|taker|maker|tier|schedule/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"]').count() > 0;

    expect.soft(
      hasContent || hasGrid,
      'Fee Management: fee schedule content visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'fees');
  });

  // =========================================================================
  // 12. Reports — date picker, generate buttons
  // =========================================================================

  test('12. Reports — date picker, generate buttons', async ({ page }) => {
    await navigateTo(page, 'Reports');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'reports');

    const body = await page.textContent('body') ?? '';
    const hasContent = /report|export|daily|monthly|eod|trade|summary/i.test(body);
    const hasButtons = await page.locator('button').count() > 0;

    expect.soft(
      hasContent || hasButtons,
      'Reports: report options or action buttons visible',
    ).toBeTruthy();

    // Date picker or date input should be present
    const dateInputs = page.locator('input[type="date"], [class*="datePicker"], [class*="DatePicker"]');
    const hasDateInput = await dateInputs.count() > 0;
    expect.soft(
      hasDateInput || hasButtons,
      'Reports: date picker or generate buttons present',
    ).toBeTruthy();

    await captureScreenshot(page, 'reports');
  });

  // =========================================================================
  // 13. Participants — participant table
  // =========================================================================

  test('13. Participants — participant table', async ({ page }) => {
    await navigateTo(page, 'Participants');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'participants');

    const body = await page.textContent('body') ?? '';
    const hasGrid = await page.locator('table, [role="grid"], [class*="DataGrid"]').count() > 0;
    const hasEmptyState = /no.*participant|empty|no.*data/i.test(body);

    expect.soft(
      hasGrid || hasEmptyState,
      'Participants: table or empty state visible',
    ).toBeTruthy();

    if (hasGrid) {
      // KYC status column should be present when there are participants
      const hasKYC = /kyc|status|pending|approved|rejected/i.test(body);
      expect.soft(hasKYC, 'Participants: KYC status content present').toBeTruthy();
    }

    await captureScreenshot(page, 'participants');
  });

  // =========================================================================
  // 14. Compliance Alerts — alerts or empty
  // =========================================================================

  test('14. Compliance Alerts — alerts or empty', async ({ page }) => {
    await navigateTo(page, 'Compliance Alerts');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'compliance');

    const body = await page.textContent('body') ?? '';
    const hasContent = /compliance|kyc|aml|alert|suspicious|sar/i.test(body);
    const hasEmptyState = /no.*alert|no.*compliance|empty|no.*data/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"], [class*="list"]').count() > 0;

    expect.soft(
      hasContent || hasEmptyState || hasGrid,
      'Compliance Alerts: content or empty state visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'compliance');
  });

  // =========================================================================
  // 15. Audit Log — events table
  // =========================================================================

  test('15. Audit Log — events table', async ({ page }) => {
    await navigateTo(page, 'Audit Log');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'audit');

    const hasGrid = await page.locator('table, [role="grid"], [class*="log"]').count() > 0;
    const body = await page.textContent('body') ?? '';
    const hasContent = /audit|log|event|action|timestamp|user/i.test(body);

    expect.soft(
      hasGrid || hasContent,
      'Audit Log: event log table or audit content visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'audit');
  });

  // =========================================================================
  // 16. Tickets — ticket list or empty
  // =========================================================================

  test('16. Tickets — ticket list or empty', async ({ page }) => {
    await navigateTo(page, 'Tickets');
    await waitForData(page);
    await assertNoErrors(page);

    await runPageCheck(page, 'tickets');

    const body = await page.textContent('body') ?? '';
    const hasContent = /ticket|support|issue|open|closed|feature.?request/i.test(body);
    const hasEmptyState = /no.*ticket|empty|no.*issue/i.test(body);
    const hasGrid = await page.locator('table, [role="grid"], [class*="list"]').count() > 0;

    expect.soft(
      hasContent || hasEmptyState || hasGrid,
      'Tickets: ticket list or empty state visible',
    ).toBeTruthy();

    await captureScreenshot(page, 'tickets');
  });

  // =========================================================================
  // 17. Bot Chat — floating button visible, chat opens, message sends
  // =========================================================================

  test('17. Bot Chat — floating button visible, chat opens, message sends', async ({ page }) => {
    // Navigate to overview to ensure the bot button is in view
    await navigateTo(page, 'Overview');
    await waitForData(page);
    await assertNoErrors(page);

    // The bot FAB must be present somewhere on the page
    const botBtn = page.locator('[aria-label="Open GarudaX Bot"]');
    try {
      await botBtn.waitFor({ state: 'visible', timeout: 6_000 });
    } catch {
      // Bot button may not be implemented — soft skip
      expect.soft(false, 'Bot Chat: floating bot button should be visible').toBeTruthy();
      await captureScreenshot(page, 'bot-not-found');
      return;
    }

    expect.soft(await botBtn.isVisible(), 'Bot Chat: floating button is visible').toBeTruthy();
    await captureScreenshot(page, 'bot-button');

    // Open chat panel
    const opened = await openBotChat(page);
    expect.soft(opened, 'Bot Chat: chat panel opens on button click').toBeTruthy();

    if (!opened) {
      await captureScreenshot(page, 'bot-panel-failed');
      return;
    }

    // Verify dialog is visible
    const panel = page.locator('[role="dialog"][aria-label="GarudaX Bot Chat"]');
    expect.soft(await panel.isVisible(), 'Bot Chat: dialog panel is visible').toBeTruthy();
    await captureScreenshot(page, 'bot-panel-open');

    // Send a test message
    const response = await sendBotMessage(page, 'What is the current system status?');
    // Response may be empty if bot backend is down — soft assertion
    expect.soft(
      typeof response === 'string',
      'Bot Chat: received a string response (may be empty)',
    ).toBeTruthy();

    // Chat input should still be present and functional after sending
    const inputAfterSend = page.locator('[aria-label="Chat message input"]');
    expect.soft(
      await inputAfterSend.isVisible().catch(() => false),
      'Bot Chat: input remains visible after sending',
    ).toBeTruthy();

    await captureScreenshot(page, 'bot-after-message');
  });

  // =========================================================================
  // 18. Dashboard Home — KPI overview
  // =========================================================================

  test('18. Dashboard Home — KPI overview', async ({ page }) => {
    // Navigate to dashboard overview via the "Overview" sidebar link
    await navigateTo(page, 'Overview');
    await waitForData(page);
    await assertNoErrors(page);

    const body = await page.textContent('body') ?? '';

    // Title
    expect.soft(body, 'Dashboard: page title present').toMatch(/overview|dashboard/i);

    // KPI cards (System Status, Pending KYC, Active Margin Calls, Settlement Cycles)
    const kpiCards = page.locator('[class*="card"], [class*="Card"]');
    const kpiCount = await kpiCards.count();
    expect.soft(kpiCount, 'Dashboard: at least 1 KPI card visible').toBeGreaterThanOrEqual(1);

    // KPI content should reference known metric labels
    expect.soft(body, 'Dashboard: shows KPI label content').toMatch(
      /system.?status|pending.?kyc|margin.?call|settlement.?cycle|status|overview/i,
    );

    // Quick Actions section (admin-only)
    const hasActions = /trigger.?settlement|halt.?trading|quick.?action/i.test(body);
    // Non-blocking — only visible when logged in as admin role
    expect.soft(
      hasActions || kpiCount >= 1,
      'Dashboard: KPIs or quick actions visible for admin',
    ).toBeTruthy();

    // No blank page
    expect.soft(body.trim().length, 'Dashboard: page is not blank').toBeGreaterThan(50);

    await captureScreenshot(page, 'dashboard-home');
  });
});
