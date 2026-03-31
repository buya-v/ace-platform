/**
 * GarudaX Bot Guided Prompts UAT — Incomplete Command Tests
 *
 * Tests that the GarudaX Bot correctly guides users through CRUD operations
 * when they provide incomplete commands (missing required arguments), rather
 * than falling through to list/display handlers.
 *
 * Design decisions:
 * - All assertions use expect.soft() so one failed check does not stop the test
 * - isPortalAvailable() guard in beforeEach for graceful CI skip
 * - Each test opens a fresh bot session via beforeEach openBotChat()
 * - Screenshots captured for key guided prompt scenarios
 * - 8 tests: 5 guided prompt tests, 2 "still-executes" tests, 1 unknown command
 *
 * Guided prompt logic (executor.go lines 653-716):
 *   When user says "create X" / "new X" / "add X" without required args,
 *   the executor returns a format guide BEFORE reaching keyword list handlers.
 *   Fully-formed commands still execute; show/list commands still return data.
 */

import { test, expect } from '@playwright/test';
import {
  loginAsAdmin,
  openBotChat,
  sendBotMessage,
  captureScreenshot,
  isPortalAvailable,
} from './helpers';

test.describe('Bot Guided Prompts UAT', () => {
  // ---------------------------------------------------------------------------
  // beforeEach: skip if portal unreachable, then login and open bot chat
  // ---------------------------------------------------------------------------

  test.beforeEach(async ({ page }) => {
    test.skip(!(await isPortalAvailable(page)), 'Portal not reachable');

    const loggedIn = await loginAsAdmin(page);
    if (!loggedIn) {
      test.skip(true, 'Could not login to admin portal — skipping guided prompt tests');
      return;
    }

    await openBotChat(page);
  });

  // ---------------------------------------------------------------------------
  // 1. Incomplete "create instrument" — guided with format, NOT a list
  // ---------------------------------------------------------------------------

  test('create new instrument — guides with format', async ({ page }) => {
    const reply = await sendBotMessage(page, 'create new instrument');

    // Should guide user with format instructions
    expect.soft(reply, 'Reply should mention how to provide instrument details').toMatch(
      /provide|example|format|create instrument/i,
    );
    // Should NOT fall through to list handler (which prefixes with 📊 Active instruments)
    expect.soft(reply, 'Reply should not list instruments when format is incomplete').not.toMatch(
      /📊.*[Aa]ctive instruments/,
    );
    // Should contain the command template or example
    expect.soft(reply, 'Reply should contain an instrument ID example or command syntax').toMatch(
      /instrument\s+\S+-\d{4}|ID|commodity.*month|contract.*tick/i,
    );

    await captureScreenshot(page, 'bot-guided-instrument');
  });

  // ---------------------------------------------------------------------------
  // 2. Incomplete "create commodity" — guided with format
  // ---------------------------------------------------------------------------

  test('create commodity — guides with format', async ({ page }) => {
    const reply = await sendBotMessage(page, 'create commodity');

    // Should guide with format (category, unit)
    expect.soft(reply, 'Reply should mention commodity format requirements').toMatch(
      /provide|example|category|unit|grain|oilseed/i,
    );
    // Should not fall through to the instrument list handler
    expect.soft(reply, 'Reply should not list instruments for an incomplete commodity command').not.toMatch(
      /📊.*[Aa]ctive instruments/,
    );

    await captureScreenshot(page, 'bot-guided-commodity');
  });

  // ---------------------------------------------------------------------------
  // 3. Incomplete "add fee" — shows fee management options
  // ---------------------------------------------------------------------------

  test('add fee — shows fee management options', async ({ page }) => {
    const reply = await sendBotMessage(page, 'add fee');

    // Should show fee management guidance (schedule, rule, tier)
    expect.soft(reply, 'Reply should describe fee management commands').toMatch(
      /fee.*schedule|fee.*rule|add fee rule|tier/i,
    );
    // Should not silently fail or return an empty string
    expect.soft(reply, 'Reply should not be empty').not.toBe('');
  });

  // ---------------------------------------------------------------------------
  // 4. Incomplete "new receipt" — guides receipt creation
  // ---------------------------------------------------------------------------

  test('new receipt — guides receipt creation', async ({ page }) => {
    const reply = await sendBotMessage(page, 'new receipt');

    // Should provide issue receipt format guidance
    expect.soft(reply, 'Reply should reference how to issue a receipt').toMatch(
      /issue.*receipt|holder|quantity|holder_id|commodity/i,
    );
    // Should contain an example command
    expect.soft(reply, 'Reply should contain an example or command format').toMatch(
      /example|issue receipt\s+\S+/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 5. Incomplete "create order" — guides order placement
  // ---------------------------------------------------------------------------

  test('create order — guides order placement', async ({ page }) => {
    const reply = await sendBotMessage(page, 'create order');

    // Should provide buy/sell order guidance
    expect.soft(reply, 'Reply should explain buy or sell syntax').toMatch(
      /buy|sell|price|at\s+\d/i,
    );
    // Should not return an internal error
    expect.soft(reply, 'Reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 6. "show instruments" — still lists instruments (not guided)
  //    Ensures show/list commands are not affected by the guided prompt logic
  // ---------------------------------------------------------------------------

  test('show instruments — still lists (not guided)', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show instruments');

    // Should return instrument data, not a format guide
    expect.soft(reply, 'Reply should contain instrument data or a related term').toMatch(
      /WHT|instrument|active|commodity|symbol|contract/i,
    );
    // Should NOT respond with "provide format" guidance
    expect.soft(reply, 'Reply should not ask for format when user is listing').not.toMatch(
      /please provide|provide.*format|required.*format/i,
    );
    // Should not contain an internal error
    expect.soft(reply, 'Reply should not contain INTERNAL_ERROR').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 7. Fully-formed "create commodity" — executes (not guided)
  //    Ensures the guided prompt only fires for incomplete commands
  // ---------------------------------------------------------------------------

  test('create commodity testuat grain kg — executes (not guided)', async ({ page }) => {
    const reply = await sendBotMessage(page, 'create commodity testuat grain kg');

    // Should execute the command and return success or failure feedback
    // NOT a format guide since all three required args (id, category, unit) are present
    expect.soft(reply, 'Reply should show execution result, not a format prompt').toMatch(
      /✅|❌|created|failed|commodity|testuat/i,
    );
    // Should not respond with "please provide" guidance
    expect.soft(reply, 'Reply should not ask for format on a fully-formed command').not.toMatch(
      /please provide.*commodity|provide.*category.*unit/i,
    );
  });

  // ---------------------------------------------------------------------------
  // 8. Unknown command — shows helpful suggestions
  // ---------------------------------------------------------------------------

  test('unknown command — shows helpful suggestions', async ({ page }) => {
    const reply = await sendBotMessage(page, 'xyzzy');

    // Should show fallback guidance with help or example commands
    expect.soft(reply, 'Reply should offer help or try alternatives').toMatch(
      /didn.*understand|help|try|show instruments|margin calls/i,
    );
    // Should not expose an internal error
    expect.soft(reply, 'Reply should not contain an internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    // Should not be empty
    expect.soft(reply, 'Reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-guided-unknown');
  });
});
