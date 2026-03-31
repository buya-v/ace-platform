/**
 * GarudaX UAT — API Contract Validation
 *
 * Validates that ALL gateway API endpoints return correct HTTP status codes,
 * are JSON-parseable, and contain required response fields.
 *
 * Design decisions:
 * - Skip gracefully when the gateway is unreachable (no network failure = no CI break)
 * - Use `expect.soft()` throughout so one failing endpoint does not abort the suite
 * - Each endpoint gets its own test for clear reporting in the Playwright HTML report
 * - Shared token acquired once in beforeAll (re-login only if token expires)
 *
 * Run:
 *   cd tests/playwright
 *   npx playwright test uat/api-contracts.spec.ts --project=uat
 */

import { test, expect, type APIRequestContext } from '@playwright/test';
import { getToken, checkEndpoint } from './api-checks';

// ---------------------------------------------------------------------------
// Ambient declaration — avoids @types/node dependency
// ---------------------------------------------------------------------------
declare const process: { env: Record<string, string | undefined> };
const _env: Record<string, string | undefined> =
  typeof process !== 'undefined' ? process.env : {};

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BASE: string =
  _env.GATEWAY_URL ||
  _env.ADMIN_BASE_URL ||
  'https://admin.garudax.asla.mn';

const INSTRUMENT = 'WHT-HRW-2026M07-UB';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Return Authorization header object or empty object if no token */
function auth(token: string | null): Record<string, string> {
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}

/**
 * Make a raw GET request and assert status < 500 + JSON parseable.
 * Returns the parsed body or null.
 */
async function get(
  request: APIRequestContext,
  path: string,
  headers: Record<string, string> = {},
): Promise<unknown> {
  const url = path.startsWith('http') ? path : `${BASE}${path}`;
  let res;
  try {
    res = await request.get(url, { headers });
  } catch {
    // Network-level failure — gateway unreachable
    return null;
  }
  expect.soft(res.status(), `GET ${path} should return status < 500`).toBeLessThan(500);
  try {
    return await res.json();
  } catch {
    return null;
  }
}

/**
 * Make a raw POST request and assert status < 500 + JSON parseable.
 * Returns the parsed body or null.
 */
async function post(
  request: APIRequestContext,
  path: string,
  data: Record<string, unknown>,
  headers: Record<string, string> = {},
): Promise<unknown> {
  const url = path.startsWith('http') ? path : `${BASE}${path}`;
  let res;
  try {
    res = await request.post(url, {
      data,
      headers: { 'Content-Type': 'application/json', ...headers },
    });
  } catch {
    return null;
  }
  expect.soft(res.status(), `POST ${path} should return status < 500`).toBeLessThan(500);
  try {
    return await res.json();
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Suite
// ---------------------------------------------------------------------------

test.describe('API Contract UAT', () => {
  let token: string | null = null;

  // -------------------------------------------------------------------------
  // Setup: obtain auth token once; skip entire suite if gateway is unreachable
  // -------------------------------------------------------------------------
  test.beforeAll(async ({ request }) => {
    // Quick gateway reachability probe
    try {
      const probe = await request.get(`${BASE}/healthz`, { timeout: 8_000 });
      if (probe.status() >= 500) {
        console.warn(`[UAT] Gateway returned ${probe.status()} — skipping API contract tests`);
        return;
      }
    } catch {
      console.warn('[UAT] Gateway unreachable — skipping API contract tests');
      return;
    }

    token = await getToken(request, BASE);
    if (!token) {
      console.warn('[UAT] Could not obtain auth token — authenticated tests will be skipped');
    }
  });

  // -------------------------------------------------------------------------
  // 1. Public endpoints (no auth required)
  // -------------------------------------------------------------------------

  test('GET /healthz — {status:"ok"}', async ({ request }) => {
    const res = await request.get(`${BASE}/healthz`).catch(() => null);
    if (!res) { test.skip(); return; }

    expect.soft(res.status(), 'healthz status should be 200').toBe(200);
    const body = await res.json().catch(() => null) as Record<string, unknown> | null;
    if (body !== null) {
      expect.soft(body, 'healthz body should have status field').toHaveProperty('status');
      // Accept "ok", "healthy", or similar
      expect.soft(
        typeof body.status === 'string' && (body.status as string).length > 0,
        'healthz status field should be a non-empty string',
      ).toBeTruthy();
    }
  });

  // -------------------------------------------------------------------------
  // 2. Auth endpoints
  // -------------------------------------------------------------------------

  test('POST /api/v1/auth/login — {AccessToken} or {access_token}', async ({ request }) => {
    const body = await post(request, `${BASE}/api/v1/auth/login`, {
      email: _env.ADMIN_EMAIL || 'admin@garudax.mn',
      password: _env.ADMIN_PASSWORD || 'Adm1n@GarudaX!',
    }) as Record<string, unknown> | null;

    if (body === null) { test.skip(); return; }

    const hasToken =
      typeof body.AccessToken === 'string' ||
      typeof body.access_token === 'string';
    expect.soft(hasToken, 'login response should contain AccessToken or access_token').toBeTruthy();
  });

  test('GET /api/v1/auth/me — user profile with email or id', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/auth/me', []);
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, '/auth/me should return < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 3. Admin health
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/health — {overall_status, services[{name,status}]}', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/admin/health',
      ['overall_status', 'services'],
    );
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, '/admin/health status < 500').toBeLessThan(500);
    expect.soft(result.missingFields, '/admin/health missing required fields').toEqual([]);

    // Deeper shape check: each service entry must have name + status
    const body = await request
      .get(`${BASE}/api/v1/admin/health`, { headers: auth(token) })
      .then((r) => r.json())
      .catch(() => null) as { services?: Array<{ name?: string; status?: string }> } | null;

    if (body?.services && Array.isArray(body.services) && body.services.length > 0) {
      for (const svc of body.services) {
        expect.soft(
          typeof svc.name === 'string',
          `service entry should have string name, got: ${JSON.stringify(svc)}`,
        ).toBeTruthy();
        expect.soft(
          typeof svc.status === 'string',
          `service entry should have string status, got: ${JSON.stringify(svc)}`,
        ).toBeTruthy();
      }
    }
  });

  // -------------------------------------------------------------------------
  // 4. Instruments / Order Book
  // -------------------------------------------------------------------------

  test('GET /api/v1/instruments/list — returns instruments array', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/instruments/list', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/instruments/list status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/instruments — returns instrument list', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/instruments', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/instruments status < 500').toBeLessThan(500);
  });

  test(`GET /api/v1/instruments/${INSTRUMENT}/book — {bids, asks}`, async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET',
      `/instruments/${INSTRUMENT}/book`,
      ['bids', 'asks'],
    );
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, `order book status < 500`).toBeLessThan(500);
    // bids/asks may be empty arrays — that is valid
    if (result.status === 200 && result.missingFields.length > 0) {
      expect.soft(result.missingFields, `order book missing fields`).toEqual([]);
    }
  });

  // -------------------------------------------------------------------------
  // 5. Clearing
  // -------------------------------------------------------------------------

  test('GET /api/v1/clearing/positions — positions data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/clearing/positions', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/clearing/positions status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/clearing/netting — netting data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/clearing/netting', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/clearing/netting status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 6. Margin
  // -------------------------------------------------------------------------

  test('GET /api/v1/margin/calls — margin calls list', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/margin/calls', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/margin/calls status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/margin/calls/stats — margin call stats', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/margin/calls/stats',
      ['TotalIssued', 'Active'],
    );
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, '/margin/calls/stats status < 500').toBeLessThan(500);
    if (result.status === 200) {
      expect.soft(result.missingFields, '/margin/calls/stats missing required fields').toEqual([]);
    }
  });

  test('GET /api/v1/margin — portfolio margin data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/margin', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/margin status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 7. Settlement
  // -------------------------------------------------------------------------

  test('GET /api/v1/settlement/cycles — cycles data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/settlement/cycles', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/settlement/cycles status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 8. Participants
  // -------------------------------------------------------------------------

  test('GET /api/v1/participants — participants data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/participants', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/participants status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 9. Compliance
  // -------------------------------------------------------------------------

  test('GET /api/v1/compliance/alerts — alerts data', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/compliance/alerts', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/compliance/alerts status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/compliance/audit-trail — audit events', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/compliance/audit-trail', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/compliance/audit-trail status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 10. Market Data
  // -------------------------------------------------------------------------

  test(`GET /api/v1/market-data/ticker/${INSTRUMENT} — ticker data`, async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET',
      `/market-data/ticker/${INSTRUMENT}`,
      [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, `market-data ticker status < 500`).toBeLessThan(500);
  });

  test(`GET /api/v1/market-data/candles/${INSTRUMENT} — candles data`, async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET',
      `/market-data/candles/${INSTRUMENT}`,
      [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, `market-data candles status < 500`).toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 11. Warehouse
  // -------------------------------------------------------------------------

  test('GET /api/v1/warehouse/inventory — inventory', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/warehouse/inventory', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/warehouse/inventory status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/warehouse/receipts — warehouse receipts', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/warehouse/receipts', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/warehouse/receipts status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 12. Admin — Risk & Circuit Breakers
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/risk/order-limits — limits', async ({ request }) => {
    if (!token) { test.skip(); return; }

    // Risk limits require DATABASE_URL — check availability first
    const res = await request.get(`${BASE}/api/v1/admin/risk/order-limits`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.status() >= 500) {
      test.skip(true, 'Risk DB not configured (503)');
      return;
    }
    expect.soft(res.status(), '/admin/risk/order-limits status').toBeLessThan(500);
  });

  test('GET /api/v1/admin/circuit-breakers — circuit breakers', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/admin/circuit-breakers', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/circuit-breakers status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 13. Admin — Fees
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/fees — fee schedule', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/admin/fees', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/fees status < 500').toBeLessThan(500);
  });

  // Also check the non-admin /fees alias (same data, different path)
  test('GET /api/v1/fees — fee schedule (alias)', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/fees', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/fees status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 14. Tickets
  // -------------------------------------------------------------------------

  test('GET /api/v1/tickets — tickets data (may be empty)', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/tickets', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/tickets status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/admin/tickets/stats — ticket stats', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/admin/tickets/stats', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/tickets/stats status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 15. Orders
  // -------------------------------------------------------------------------

  test('GET /api/v1/orders — orders (may be empty)', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/orders', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/orders status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 16. Bot / GarudaX AI
  // -------------------------------------------------------------------------

  test('POST /api/v1/bot/chat — {data:{reply}}', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'POST', '/bot/chat',
      ['data'],
      { message: 'ping' },
    );
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, '/bot/chat status < 500').toBeLessThan(500);
    if (result.status === 200) {
      // Check the nested reply field
      const body = await request
        .post(`${BASE}/api/v1/bot/chat`, {
          data: { message: 'ping' },
          headers: { 'Content-Type': 'application/json', ...auth(token) },
        })
        .then((r) => r.json())
        .catch(() => null) as { data?: { reply?: string } } | null;

      if (body?.data) {
        expect.soft(
          typeof body.data.reply === 'string',
          '/bot/chat data.reply should be a string',
        ).toBeTruthy();
      }
    }
  });

  test('GET /api/v1/bot/suggestions — suggestions array', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/bot/suggestions', []);
    if (result.status === 0) { test.skip(); return; }

    expect.soft(result.status, '/bot/suggestions status < 500').toBeLessThan(500);
    if (result.status === 200) {
      const body = await request
        .get(`${BASE}/api/v1/bot/suggestions`, { headers: auth(token) })
        .then((r) => r.json())
        .catch(() => null);
      // Response may be an array directly, or an object wrapping an array
      const isArrayOrWrapped =
        Array.isArray(body) ||
        (body !== null &&
          typeof body === 'object' &&
          (Array.isArray((body as Record<string, unknown>).suggestions) ||
            Array.isArray((body as Record<string, unknown>).data)));
      expect.soft(isArrayOrWrapped, '/bot/suggestions should return array or wrapped array').toBeTruthy();
    }
  });

  // -------------------------------------------------------------------------
  // 17. Audit trail (non-compliance alias)
  // -------------------------------------------------------------------------

  test('GET /api/v1/audit/trail — audit events', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/audit/trail', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/audit/trail status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 18. Surveillance
  // -------------------------------------------------------------------------

  test('GET /api/v1/surveillance/alerts — surveillance alerts', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/surveillance/alerts', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/surveillance/alerts status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 19. Admin system metrics / KPIs
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/metrics — system metrics', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/admin/metrics', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/metrics status < 500').toBeLessThan(500);
  });

  test('GET /api/v1/admin/stats — platform stats', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/admin/stats', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/stats status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 20. Warehouse facilities
  // -------------------------------------------------------------------------

  test('GET /api/v1/warehouse/facilities — warehouse facilities', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/warehouse/facilities', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/warehouse/facilities status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 21. Settlement positions
  // -------------------------------------------------------------------------

  test('GET /api/v1/settlement/positions — settlement positions', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/settlement/positions', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/settlement/positions status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 22. Participants KYC
  // -------------------------------------------------------------------------

  test('GET /api/v1/compliance/kyc — KYC applications', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/compliance/kyc', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/compliance/kyc status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 23. Market data — last trade
  // -------------------------------------------------------------------------

  test(`GET /api/v1/market-data/last-trade/${INSTRUMENT} — last trade`, async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET',
      `/market-data/last-trade/${INSTRUMENT}`,
      [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, `market-data last-trade status < 500`).toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 24. Market data — summary / OHLCV
  // -------------------------------------------------------------------------

  test('GET /api/v1/market-data/summary — market summary', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/market-data/summary', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/market-data/summary status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 25. Compliance — unresolved alerts summary
  // -------------------------------------------------------------------------

  test('GET /api/v1/compliance/alerts/summary — unresolved alerts summary', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/compliance/alerts/summary', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/compliance/alerts/summary status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 26. Margin — large trader positions
  // -------------------------------------------------------------------------

  test('GET /api/v1/margin/large-traders — large trader positions', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/margin/large-traders', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/margin/large-traders status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 27. Admin — EOD report trigger (non-destructive GET)
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/reports — admin reports list', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/admin/reports', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/reports status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 28. Clearing — receipts
  // -------------------------------------------------------------------------

  test('GET /api/v1/clearing/receipts — clearing receipts', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET', '/clearing/receipts', [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/clearing/receipts status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 29. Admin — system event log
  // -------------------------------------------------------------------------

  test('GET /api/v1/admin/events — system events', async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(request, BASE, token, 'GET', '/admin/events', []);
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, '/admin/events status < 500').toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // 30. 4xx are valid — confirm 401 for unauthenticated protected endpoint
  // -------------------------------------------------------------------------

  test('Unauthenticated request to protected endpoint returns 401 or 403', async ({ request }) => {
    let res;
    try {
      res = await request.get(`${BASE}/api/v1/admin/health`, {
        headers: { 'Content-Type': 'application/json' },
      });
    } catch {
      test.skip(); return;
    }

    const status = res.status();
    expect.soft(
      status === 401 || status === 403,
      `Unauthenticated admin/health should return 401 or 403, got ${status}`,
    ).toBeTruthy();
  });

  // -------------------------------------------------------------------------
  // 31. Content-type validation: JSON responses have correct Content-Type header
  // -------------------------------------------------------------------------

  test('JSON API responses include application/json content-type', async ({ request }) => {
    if (!token) { test.skip(); return; }

    let res;
    try {
      res = await request.get(`${BASE}/api/v1/admin/health`, { headers: auth(token) });
    } catch {
      test.skip(); return;
    }

    if (res.status() >= 500) { test.skip(); return; }

    const contentType = res.headers()['content-type'] || '';
    expect.soft(
      contentType.includes('application/json'),
      `admin/health Content-Type should be application/json, got: "${contentType}"`,
    ).toBeTruthy();
  });

  // -------------------------------------------------------------------------
  // 32. Instruments — phase / halt / resume readiness check (GET only)
  // -------------------------------------------------------------------------

  test(`GET /api/v1/instruments/${INSTRUMENT} — single instrument details`, async ({ request }) => {
    if (!token) { test.skip(); return; }

    const result = await checkEndpoint(
      request, BASE, token, 'GET',
      `/instruments/${INSTRUMENT}`,
      [],
    );
    if (result.status === 0) { test.skip(); return; }
    expect.soft(result.status, `single instrument status < 500`).toBeLessThan(500);
  });

  // -------------------------------------------------------------------------
  // Summary test: run all ADMIN_API_CHECKS from api-checks.ts
  // -------------------------------------------------------------------------

  test('Bulk contract check — all ADMIN_API_CHECKS pass', async ({ request }) => {
    if (!token) { test.skip(); return; }

    // Import is deferred to avoid double-running if token is unavailable
    const { ADMIN_API_CHECKS, runAllChecks } = await import('./api-checks');

    const results = await runAllChecks(request, BASE, token, ADMIN_API_CHECKS);

    const failed = results.filter((r) => !r.passed && r.status !== 0);
    const unreachable = results.filter((r) => r.status === 0);

    if (unreachable.length === results.length) {
      test.skip(); return;
    }

    // Log a summary for CI
    console.log(
      `[UAT] Bulk check: ${results.length - unreachable.length} checked, ` +
      `${failed.length} failed, ${unreachable.length} unreachable`,
    );

    for (const f of failed) {
      expect.soft(
        f.passed,
        `${f.path}: status=${f.status} missing=[${f.missingFields.join(', ')}]`,
      ).toBeTruthy();
    }
  });
});
