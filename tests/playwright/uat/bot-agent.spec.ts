/**
 * GarudaX Bot Agent UAT — Conversation Workflow Tests
 *
 * Tests that the GarudaX Bot floating panel can execute admin actions
 * via conversational commands in a real browser session.
 *
 * Design decisions:
 * - All assertions use expect.soft() so one failed check does not stop the test
 * - isPortalAvailable() guard in beforeEach for graceful CI skip
 * - Each test opens a fresh bot session via beforeEach openBotChat()
 * - Screenshots captured for evidence on all visually notable tests
 * - 12 tests covering all major bot command categories
 */

import { test, expect } from '@playwright/test';
import {
  loginAsAdmin,
  openBotChat,
  sendBotMessage,
  captureScreenshot,
  isPortalAvailable,
} from './helpers';

test.describe('Bot Agent UAT — Conversation Workflows', () => {
  // ---------------------------------------------------------------------------
  // beforeEach: skip if portal unreachable, then login and open bot chat
  // ---------------------------------------------------------------------------

  test.beforeEach(async ({ page }) => {
    const available = await isPortalAvailable(page);
    if (!available) {
      test.skip(true, 'Admin portal not reachable — skipping bot agent tests');
      return;
    }

    const loggedIn = await loginAsAdmin(page);
    if (!loggedIn) {
      test.skip(true, 'Could not login to admin portal — skipping bot agent tests');
      return;
    }

    await openBotChat(page);
  });

  // ---------------------------------------------------------------------------
  // 1. System health — lists services
  // ---------------------------------------------------------------------------

  test('system health — lists services', async ({ page }) => {
    const reply = await sendBotMessage(page, 'system health');

    expect.soft(reply, 'Bot health reply should mention known services or status').toMatch(
      /matching-engine|gateway|healthy|ok|service|status/i,
    );
    expect.soft(reply, 'Bot health reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-health');
  });

  // ---------------------------------------------------------------------------
  // 2. Help — shows command categories
  // ---------------------------------------------------------------------------

  test('help — shows command categories', async ({ page }) => {
    const reply = await sendBotMessage(page, 'help');

    expect.soft(reply, 'Bot help reply should list trading-related categories').toMatch(
      /orders|trading|margin|settlement|instrument|health/i,
    );
    expect.soft(reply, 'Bot help reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-help');
  });

  // ---------------------------------------------------------------------------
  // 3. Show instruments — returns instrument list
  // ---------------------------------------------------------------------------

  test('show instruments — returns instrument list', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show instruments');

    expect.soft(reply, 'Bot instruments reply should mention an instrument or related term').toMatch(
      /WHT|instrument|commodity|symbol|contract/i,
    );
    expect.soft(reply, 'Bot instruments reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 4. Show margin calls — returns margin data
  // ---------------------------------------------------------------------------

  test('show margin calls — returns margin data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show margin calls');

    expect.soft(reply, 'Bot margin reply should not contain the word "error"').not.toContain('error');
    expect.soft(reply, 'Bot margin reply should mention margin-related content').toMatch(
      /margin|active|calls|shortfall|requirement|no.*active/i,
    );

    await captureScreenshot(page, 'bot-margin-calls');
  });

  // ---------------------------------------------------------------------------
  // 5. Show participants — returns participant data
  // ---------------------------------------------------------------------------

  test('show participants — returns participant data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show participants');

    expect.soft(reply, 'Bot participants reply should mention participants').toMatch(
      /participant|member|firm|trader|no.*participant/i,
    );
    expect.soft(reply, 'Bot participants reply should not be empty').not.toBe('');
  });

  // ---------------------------------------------------------------------------
  // 6. Show alerts — returns compliance data
  // ---------------------------------------------------------------------------

  test('show alerts — returns compliance data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show alerts');

    expect.soft(reply, 'Bot alerts reply should mention alert-related content').toMatch(
      /alert|compliance|kyc|aml|no.*alert/i,
    );
    expect.soft(reply, 'Bot alerts reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 7. Show tickets — returns ticket data
  // ---------------------------------------------------------------------------

  test('show tickets — returns ticket data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show tickets');

    expect.soft(reply, 'Bot tickets reply should mention ticket-related content').toMatch(
      /ticket|support|issue|open|closed|no.*ticket/i,
    );
    expect.soft(reply, 'Bot tickets reply should not be empty').not.toBe('');
  });

  // ---------------------------------------------------------------------------
  // 8. Who am I — returns user profile
  // ---------------------------------------------------------------------------

  test('who am I — returns user profile', async ({ page }) => {
    const reply = await sendBotMessage(page, 'who am I');

    expect.soft(reply, 'Bot who-am-I reply should mention admin, profile, or email').toMatch(
      /admin|profile|email|user|account/i,
    );
    expect.soft(reply, 'Bot who-am-I reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-whoami');
  });

  // ---------------------------------------------------------------------------
  // 9. Wheat price — returns market data
  // ---------------------------------------------------------------------------

  test('wheat price — returns market data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'wheat price');

    expect.soft(reply, 'Bot wheat price reply should not contain INTERNAL_ERROR').not.toContain(
      'INTERNAL_ERROR',
    );
    expect.soft(reply, 'Bot wheat price reply should not be empty').not.toBe('');
    // May return a price, no-data message, or instrument info — all acceptable
    expect.soft(reply, 'Bot wheat price reply should contain some relevant content').toMatch(
      /wheat|WHT|price|market|no.*data|not.*available/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 10. Unknown command — suggests alternatives
  // ---------------------------------------------------------------------------

  test('unknown command — suggests alternatives', async ({ page }) => {
    const reply = await sendBotMessage(page, 'xyzzy nonsense');

    expect.soft(reply, 'Bot unknown command reply should suggest help or alternatives').toMatch(
      /help|try|didn.*understand|unknown|not.*recogni|sorry/i,
    );
    expect.soft(reply, 'Bot unknown command reply should not crash with internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-unknown-command');
  });

  // ---------------------------------------------------------------------------
  // 11. Show settlement cycles — returns data
  // ---------------------------------------------------------------------------

  test('show settlement cycles — returns data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show settlement cycles');

    expect.soft(reply, 'Bot settlement cycles reply should mention settlement or cycle content').toMatch(
      /settlement|cycle|pending|no.*cycle|no.*settlement/i,
    );
    expect.soft(reply, 'Bot settlement cycles reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 12. Show risk limits — returns risk data
  // ---------------------------------------------------------------------------

  test('show risk limits — returns risk data', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show risk limits');

    expect.soft(reply, 'Bot risk limits reply should mention risk, limit, or order content').toMatch(
      /risk|limit|order|position|no.*limit/i,
    );
    expect.soft(reply, 'Bot risk limits reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );

    await captureScreenshot(page, 'bot-risk-limits');
  });
});
