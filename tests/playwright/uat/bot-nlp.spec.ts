/**
 * GarudaX Bot NLP + Master Data UAT
 *
 * Tests that:
 * 1. Seeded master data (commodities, instruments, fee schedules) is visible
 *    through bot responses — not empty JSON arrays.
 * 2. Natural-language variations ("list of commodities", "show me the instruments",
 *    "what are the margin calls", etc.) resolve to the correct handler.
 * 3. Response quality — no raw JSON braces, no <nil>, no "undefined".
 *
 * Design decisions:
 * - All assertions use expect.soft() so a single failure does not abort the test
 * - isPortalAvailable() guard in beforeEach for graceful CI skip
 * - Screenshots captured on key data-visibility checks
 * - 12 tests: 3 master-data, 5 NL-variation, 4 response-quality
 *
 * Seeded data referenced (from gateway SeedDefaults):
 *   Commodities: WHT-HRW (wheat), CRN-YEL (corn), SBN-NO2 (soybeans),
 *                BRL-MALT (barley), CSH-RAW (cashmere), LVS-CATTLE (cattle)
 *   Instruments: WHT-HRW-2026M07-UB, CRN-YEL-2026M09-UB, etc.
 *   Fee schedule: GarudaX Default Fee Schedule — tiers: farmer(10bps),
 *                 hedger(15bps), speculator(25bps), market_maker(5bps)
 */

import { test, expect } from '@playwright/test';
import {
  loginAsAdmin,
  openBotChat,
  sendBotMessage,
  captureScreenshot,
  isPortalAvailable,
} from './helpers';

test.describe('Bot NLP + Master Data UAT', () => {
  // ---------------------------------------------------------------------------
  // beforeEach: skip if portal unreachable, then login and open bot chat
  // ---------------------------------------------------------------------------

  test.beforeEach(async ({ page }) => {
    const available = await isPortalAvailable(page);
    if (!available) {
      test.skip(true, 'Admin portal not reachable — skipping bot NLP tests');
      return;
    }

    const loggedIn = await loginAsAdmin(page);
    if (!loggedIn) {
      test.skip(true, 'Could not login to admin portal — skipping bot NLP tests');
      return;
    }

    await openBotChat(page);
  });

  // ---------------------------------------------------------------------------
  // MASTER DATA VISIBILITY
  // Tests that seeded data is present and returned in human-readable form.
  // ---------------------------------------------------------------------------

  // 1. show commodities — seeded data not empty
  test('show commodities — returns seeded data (not empty)', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show commodities');

    // Should mention at least one seeded commodity name/ID
    expect.soft(reply, 'Bot should mention at least one seeded commodity').toMatch(
      /wheat|corn|soybean|barley|cashmere|cattle|WHT|CRN|SBN|BRL|CSH|LVS|commodity/i,
    );
    // Should NOT return the raw empty-list JSON that indicates a data miss
    expect.soft(reply, 'Bot should not return raw empty JSON list').not.toMatch(
      /"\s*data"\s*:\s*\[\s*\]/,
    );
    // Should NOT start with a raw JSON object brace (response should be prose)
    expect.soft(reply, 'Bot commodity reply should not be raw JSON').not.toMatch(/^\s*\{/);
    // Should not be empty
    expect.soft(reply, 'Bot commodity reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-nlp-commodities');
  });

  // 2. show instruments — seeded instruments visible
  test('show instruments — returns seeded instruments (not empty)', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show instruments');

    // Should mention a seeded instrument ID or common instrument terms
    expect.soft(reply, 'Bot should mention a seeded instrument').toMatch(
      /WHT-HRW|CRN-YEL|SBN-NO2|BRL-MALT|CSH-RAW|LVS-CATTLE|instrument|contract|2026/i,
    );
    // Should NOT return raw empty array
    expect.soft(reply, 'Bot should not return empty instrument list JSON').not.toMatch(
      /"\s*data"\s*:\s*\[\s*\]/,
    );
    expect.soft(reply, 'Bot instrument reply should not be raw JSON').not.toMatch(/^\s*\{/);
    expect.soft(reply, 'Bot instrument reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-nlp-instruments');
  });

  // 3. show fees — fee schedule and rates visible
  test('show fees — returns seeded fee schedule', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show fees');

    // Should mention the seeded schedule name, a tier, or bps values
    expect.soft(reply, 'Bot should mention fee schedule or tier names').toMatch(
      /fee|schedule|bps|farmer|hedger|speculator|market.?maker|default|trading|clearing/i,
    );
    expect.soft(reply, 'Bot fee reply should not be raw JSON').not.toMatch(/^\s*\{/);
    expect.soft(reply, 'Bot fee reply should not return empty list').not.toMatch(
      /"\s*data"\s*:\s*\[\s*\]/,
    );

    await captureScreenshot(page, 'bot-nlp-fees');
  });

  // ---------------------------------------------------------------------------
  // NATURAL LANGUAGE VARIATIONS
  // Tests that the NLP normalizer resolves alternative phrasings correctly.
  // ---------------------------------------------------------------------------

  // 4. "list of commodities" — NL variation
  test('list of commodities — NL variation resolves to commodity list', async ({ page }) => {
    const reply = await sendBotMessage(page, 'list of commodities');

    expect.soft(reply, 'NL variation "list of commodities" should return commodity data').toMatch(
      /wheat|corn|soybean|barley|cashmere|cattle|commodity|WHT|CRN|grain|oilseed|fiber|livestock/i,
    );
    expect.soft(reply, 'NL variation reply should not crash with internal error').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'NL variation reply should not be empty').not.toBe('');
  });

  // 5. "show me the instruments" — NL variation with filler words
  test('show me the instruments — NL variation with filler words resolves correctly', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show me the instruments');

    expect.soft(reply, 'NL filler-word variation should return instrument data').toMatch(
      /WHT|CRN|SBN|BRL|instrument|contract|2026|commodity/i,
    );
    expect.soft(reply, 'NL filler-word reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'NL filler-word reply should not be empty').not.toBe('');
  });

  // 6. "what are the margin calls" — question format with "what are the"
  test('what are the margin calls — question-format NL variation', async ({ page }) => {
    const reply = await sendBotMessage(page, 'what are the margin calls');

    expect.soft(reply, 'Question-format "what are the margin calls" should resolve to margin data').toMatch(
      /margin|active|calls|shortfall|requirement|no.*active|no.*margin/i,
    );
    expect.soft(reply, 'Question-format reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
  });

  // 7. "check system health" — action verb variation
  test('check system health — action-verb NL variation', async ({ page }) => {
    const reply = await sendBotMessage(page, 'check system health');

    expect.soft(reply, 'Action-verb NL variation should return health status').toMatch(
      /healthy|ok|service|matching.?engine|gateway|status|up/i,
    );
    expect.soft(reply, 'Action-verb health reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'Action-verb health reply should not be empty').not.toBe('');

    await captureScreenshot(page, 'bot-nlp-check-health');
  });

  // 8. "any alerts?" — interrogative question mark format
  test('any alerts? — interrogative question-mark format resolves to alerts', async ({ page }) => {
    const reply = await sendBotMessage(page, 'any alerts?');

    expect.soft(reply, 'Interrogative "any alerts?" should resolve to compliance/alert data').toMatch(
      /alert|compliance|aml|kyc|surveillance|no.*alert|no.*active/i,
    );
    expect.soft(reply, 'Interrogative alerts reply should not crash').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'Interrogative alerts reply should not be empty').not.toBe('');
  });

  // ---------------------------------------------------------------------------
  // RESPONSE QUALITY
  // Tests that responses are human-readable prose, not raw API payloads.
  // ---------------------------------------------------------------------------

  // 9. commodity response is prose, not raw JSON
  test('commodity response does not start with raw JSON brace', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show commodities');

    // Bot should format data as readable text (e.g. "• WHT-HRW: Hard Red Winter Wheat")
    // not dump `{"data": [...]}` directly
    expect.soft(reply, 'Commodity response should not be a raw JSON object').not.toMatch(/^\s*\{/);
    expect.soft(reply, 'Commodity response should not be raw JSON array').not.toMatch(/^\s*\[/);
    // Confirm it actually has content
    expect.soft(reply, 'Commodity response should have some content').not.toBe('');
  });

  // 10. margin response shows no <nil> or "undefined" values
  test('margin response shows numbers not nil or undefined', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show margin calls');

    expect.soft(reply, 'Margin response should not contain Go nil pointer strings').not.toContain('<nil>');
    expect.soft(reply, 'Margin response should not contain "undefined"').not.toContain('undefined');
    expect.soft(reply, 'Margin response should not contain "null null"').not.toMatch(/null null/);
    // Should have some valid response
    expect.soft(reply, 'Margin response should not be empty').not.toBe('');
  });

  // 11. instrument response contains no raw empty JSON data
  test('instrument response does not contain raw empty data structure', async ({ page }) => {
    const reply = await sendBotMessage(page, 'show instruments');

    // Specifically guard against `{"data": []}` or `{"instruments": []}` responses
    // that would indicate the bot forwarded an empty API response without formatting
    expect.soft(reply, 'Instrument response should not contain raw empty JSON data field').not.toMatch(
      /["']data["']\s*:\s*\[\s*\]/,
    );
    expect.soft(reply, 'Instrument response should not contain raw empty instruments array').not.toMatch(
      /["']instruments["']\s*:\s*\[\s*\]/,
    );
  });

  // 12. help text is formatted, not a raw JSON dump
  test('help response is human-readable — not raw JSON', async ({ page }) => {
    const reply = await sendBotMessage(page, 'help');

    // Help should be a formatted message listing categories
    expect.soft(reply, 'Help response should mention command categories').toMatch(
      /instrument|commodity|fee|margin|settlement|order|health/i,
    );
    // Not starting with JSON
    expect.soft(reply, 'Help response should not be raw JSON').not.toMatch(/^\s*\{/);
    expect.soft(reply, 'Help response should not be raw JSON array').not.toMatch(/^\s*\[/);
    // Should not contain raw error codes
    expect.soft(reply, 'Help response should not contain internal error codes').not.toMatch(
      /INTERNAL_ERROR|500|exception/i,
    );
    expect.soft(reply, 'Help response should not be empty').not.toBe('');
  });
});
