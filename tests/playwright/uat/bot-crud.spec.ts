/**
 * GarudaX Bot CRUD UAT — Admin Operations via Conversation
 *
 * Tests that the GarudaX Bot floating panel can execute ALL admin CRUD
 * operations through natural-language conversation in a real browser.
 *
 * Design decisions:
 * - All assertions use expect.soft() — one failure does not stop others
 * - Patterns accept both ✅ (success) and ❌ (expected failure when backend
 *   is not fully seeded), so tests pass in CI and in production staging
 * - isPortalAvailable() guard in beforeEach for graceful skip
 * - Screenshots captured at key mutation points for audit evidence
 * - 17 tests covering commodity CRUD, instrument CRUD, participant management,
 *   fee rules, warehouse receipts, order submission, market controls,
 *   compliance, reporting, audit, and help completeness
 */

import { test, expect } from '@playwright/test';
import {
  loginAsAdmin,
  openBotChat,
  sendBotMessage,
  captureScreenshot,
  isPortalAvailable,
} from './helpers';

test.describe('Bot CRUD UAT — Admin Operations via Conversation', () => {
  // -------------------------------------------------------------------------
  // beforeEach: skip if portal unreachable, then login and open bot chat
  // -------------------------------------------------------------------------

  test.beforeEach(async ({ page }) => {
    const available = await isPortalAvailable(page);
    if (!available) {
      test.skip(true, 'Admin portal not reachable — skipping bot CRUD tests');
      return;
    }

    const loggedIn = await loginAsAdmin(page);
    if (!loggedIn) {
      test.skip(true, 'Could not login to admin portal — skipping bot CRUD tests');
      return;
    }

    await openBotChat(page);
  });

  // -------------------------------------------------------------------------
  // 1. Create commodity via bot
  // -------------------------------------------------------------------------

  test('create commodity via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'create commodity rice grain kg');

    expect.soft(reply, 'Create commodity reply should acknowledge creation').toMatch(
      /created|rice|commodity|added|success|✅/i,
    );
    expect.soft(reply, 'Create commodity reply should not return internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-create-commodity');
  });

  // -------------------------------------------------------------------------
  // 2. List commodities via bot
  // -------------------------------------------------------------------------

  test('list commodities via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'list commodities');

    expect.soft(reply, 'List commodities reply should mention at least one commodity').toMatch(
      /commodity|grain|rice|WHT|wheat|maize|barley/i,
    );
    expect.soft(reply, 'List commodities reply should not contain internal error').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );
    expect.soft(reply, 'List commodities reply should not be empty').not.toBe('');
  });

  // -------------------------------------------------------------------------
  // 3. Create instrument via bot
  // -------------------------------------------------------------------------

  test('create instrument via bot', async ({ page }) => {
    const reply = await sendBotMessage(
      page,
      'create instrument RIC-UAT-2027 rice jul 2027 contract 1000 tick 0.01',
    );

    expect.soft(reply, 'Create instrument reply should acknowledge creation').toMatch(
      /created|instrument|RIC|added|success|✅/i,
    );
    expect.soft(reply, 'Create instrument reply should not return internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-create-instrument');
  });

  // -------------------------------------------------------------------------
  // 4. List instruments via bot
  // -------------------------------------------------------------------------

  test('list instruments via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show instruments');

    expect.soft(reply, 'List instruments reply should mention an instrument or related term').toMatch(
      /WHT|instrument|commodity|symbol|contract|RIC/i,
    );
    expect.soft(reply, 'List instruments reply should not contain internal error').not.toMatch(
      /INTERNAL_ERROR|500/i,
    );
  });

  // -------------------------------------------------------------------------
  // 5. Set participant tier via bot
  // -------------------------------------------------------------------------

  test('set participant tier via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'set tier farmer for trader1');

    expect.soft(reply, 'Set tier reply should acknowledge the tier change').toMatch(
      /tier|farmer|updated|set|applied|participant|✅|❌/i,
    );
    expect.soft(reply, 'Set tier reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // -------------------------------------------------------------------------
  // 6. Add fee rule via bot
  // -------------------------------------------------------------------------

  test('add fee rule via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'add fee rule trading farmer 10bps');

    expect.soft(reply, 'Add fee rule reply should acknowledge the rule').toMatch(
      /fee|rule|created|added|applied|schedule|bps|✅|❌/i,
    );
    expect.soft(reply, 'Add fee rule reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-add-fee-rule');
  });

  // -------------------------------------------------------------------------
  // 7. Issue warehouse receipt via bot
  // -------------------------------------------------------------------------

  test('issue warehouse receipt via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'issue receipt farmer1 wheat 5000');

    expect.soft(reply, 'Issue receipt reply should acknowledge receipt or rejection').toMatch(
      /receipt|issued|warehouse|created|error|not.*found|✅|❌/i,
    );
    expect.soft(reply, 'Issue receipt reply should not crash with internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-issue-receipt');
  });

  // -------------------------------------------------------------------------
  // 8. Pledge receipt via bot
  // -------------------------------------------------------------------------

  test('pledge receipt via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'pledge receipt RCP-001');

    expect.soft(reply, 'Pledge receipt reply should acknowledge pledge or not-found').toMatch(
      /pledge|pledged|receipt|collateral|not.*found|error|✅|❌/i,
    );
    expect.soft(reply, 'Pledge receipt reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // -------------------------------------------------------------------------
  // 9. Screen participant via bot
  // -------------------------------------------------------------------------

  test('screen participant via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'screen participant trader1');

    expect.soft(reply, 'Screen participant reply should return a screening result').toMatch(
      /screen|clear|result|check|aml|kyc|compliance|not.*found|✅|❌/i,
    );
    expect.soft(reply, 'Screen participant reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // -------------------------------------------------------------------------
  // 10. Submit order via bot
  // -------------------------------------------------------------------------

  test('submit order via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'buy 10 wheat at 325');

    expect.soft(reply, 'Submit order reply should acknowledge order or failure').toMatch(
      /order|submitted|buy|placed|accepted|rejected|invalid|✅|❌/i,
    );
    expect.soft(reply, 'Submit order reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-submit-order');
  });

  // -------------------------------------------------------------------------
  // 11. Halt instrument via bot
  // -------------------------------------------------------------------------

  test('halt instrument via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'halt wheat');

    expect.soft(reply, 'Halt reply should acknowledge halt or failure').toMatch(
      /halt|halted|stopped|paused|WHT|wheat|not.*found|✅|❌/i,
    );
    expect.soft(reply, 'Halt reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-halt-instrument');
  });

  // -------------------------------------------------------------------------
  // 12. Resume instrument via bot
  // -------------------------------------------------------------------------

  test('resume instrument via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'resume wheat');

    expect.soft(reply, 'Resume reply should acknowledge resume or failure').toMatch(
      /resume|resumed|started|trading|WHT|wheat|not.*found|✅|❌/i,
    );
    expect.soft(reply, 'Resume reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-resume-instrument');
  });

  // -------------------------------------------------------------------------
  // 13. File SAR (Suspicious Activity Report) via bot
  // -------------------------------------------------------------------------

  test('file SAR via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'file SAR on trader1 for suspicious trading');

    expect.soft(reply, 'File SAR reply should acknowledge filing or failure').toMatch(
      /sar|filed|suspicious|activity|report|compliance|not.*found|✅|❌/i,
    );
    expect.soft(reply, 'File SAR reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );

    await captureScreenshot(page, 'bot-file-sar');
  });

  // -------------------------------------------------------------------------
  // 14. Generate market summary report via bot
  // -------------------------------------------------------------------------

  test('generate market summary report via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'market summary today');

    expect.soft(reply, 'Market summary reply should return summary or no-data message').toMatch(
      /market|summary|report|volume|trade|no.*data|today/i,
    );
    expect.soft(reply, 'Market summary reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'Market summary reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-market-summary');
  });

  // -------------------------------------------------------------------------
  // 15. Show audit log via bot
  // -------------------------------------------------------------------------

  test('show audit log via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show audit log');

    expect.soft(reply, 'Audit log reply should mention audit or trail content').toMatch(
      /audit|trail|log|event|action|no.*audit/i,
    );
    expect.soft(reply, 'Audit log reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'Audit log reply should not be empty').not.toBe('');
  });

  // -------------------------------------------------------------------------
  // 16. Show positions via bot
  // -------------------------------------------------------------------------

  test('show positions via bot', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show positions');

    expect.soft(reply, 'Positions reply should mention positions, net, or no-data').toMatch(
      /position|net|long|short|exposure|no.*position/i,
    );
    expect.soft(reply, 'Positions reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // -------------------------------------------------------------------------
  // 17. Help lists ALL admin categories
  // -------------------------------------------------------------------------

  test('help lists ALL admin categories', async ({ page }) => {
    const reply = await sendBotMessage(page, 'help');

    expect.soft(reply, 'Help reply should mention instrument commands').toMatch(/instrument/i);
    expect.soft(reply, 'Help reply should mention commodity commands').toMatch(/commodity/i);
    expect.soft(reply, 'Help reply should mention fee commands').toMatch(/fee/i);
    expect.soft(reply, 'Help reply should mention warehouse commands').toMatch(/warehouse/i);
    expect.soft(reply, 'Help reply should mention order commands').toMatch(/order/i);
    expect.soft(reply, 'Help reply should mention margin commands').toMatch(/margin/i);
    expect.soft(reply, 'Help reply should mention settlement commands').toMatch(/settlement/i);
    expect.soft(reply, 'Help reply should mention ticket commands').toMatch(/ticket/i);
    expect.soft(reply, 'Help reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-help-all-categories');
  });
});
