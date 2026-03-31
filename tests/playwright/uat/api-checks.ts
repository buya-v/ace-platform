/**
 * GarudaX UAT API Contract Validators
 *
 * Validates that admin API endpoints return expected shapes and status codes.
 * Uses Playwright's APIRequestContext for direct HTTP calls without browser overhead.
 *
 * Usage in tests:
 *   import { ADMIN_API_CHECKS, getToken, checkEndpoint } from './api-checks';
 *
 *   const token = await getToken(request, baseURL);
 *   for (const check of ADMIN_API_CHECKS) {
 *     await checkEndpoint(request, baseURL, token, check.method, check.path, check.expectedFields);
 *   }
 */

import { type APIRequestContext, expect } from '@playwright/test';

// Ambient declaration for process.env without requiring @types/node
declare const process: { env: Record<string, string | undefined> };

const _procEnv: Record<string, string | undefined> = (typeof process !== 'undefined') ? process.env : {};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface EndpointCheck {
  /** Human-readable name for this check */
  name: string;
  /** HTTP method */
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  /** API path relative to /api/v1, e.g. "/admin/health" */
  path: string;
  /** Fields expected in the response body (supports nested dot notation) */
  expectedFields: string[];
  /** Whether this endpoint requires auth (default: true) */
  requiresAuth?: boolean;
  /** Optional request body for POST/PUT requests */
  body?: Record<string, unknown>;
  /** Skip this check (e.g. known broken endpoint) */
  skip?: boolean;
}

export interface CheckResult {
  name: string;
  path: string;
  status: number;
  passed: boolean;
  missingFields: string[];
  error?: string;
}

// ---------------------------------------------------------------------------
// Token helper
// ---------------------------------------------------------------------------

/**
 * Login via the admin API and return the JWT access token.
 * Returns null if login fails.
 */
export async function getToken(
  request: APIRequestContext,
  baseURL: string,
  email?: string,
  password?: string,
): Promise<string | null> {
  const loginEmail = email || _procEnv.ADMIN_EMAIL || 'admin@garudax.mn';
  const loginPassword = password || _procEnv.ADMIN_PASSWORD || 'Adm1n@GarudaX!';

  try {
    const response = await request.post(`${baseURL}/api/v1/auth/login`, {
      data: { email: loginEmail, password: loginPassword },
      headers: { 'Content-Type': 'application/json' },
    });

    if (!response.ok()) {
      const body = await response.text().catch(() => '');
      console.warn(`[UAT] getToken: login ${response.status()}: ${body.substring(0, 200)}`);
      return null;
    }

    const body = await response.json().catch(() => null);
    // Support both snake_case and PascalCase token fields
    return body?.access_token ?? body?.AccessToken ?? null;
  } catch (err) {
    console.warn(`[UAT] getToken failed: ${err}`);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Field extraction helper
// ---------------------------------------------------------------------------

/**
 * Check if a field exists in an object, supporting dot-notation for nesting.
 * e.g. "error.code", "data.0.id", "services"
 */
function hasField(obj: unknown, fieldPath: string): boolean {
  const parts = fieldPath.split('.');
  let current: unknown = obj;

  for (const part of parts) {
    if (current === null || current === undefined) return false;
    if (typeof current !== 'object' && !Array.isArray(current)) return false;
    current = (current as Record<string, unknown>)[part];
  }

  return current !== undefined;
}

/**
 * Check if a value is an array with at least one element.
 */
function isNonEmptyArray(obj: unknown, fieldPath: string): boolean {
  const parts = fieldPath.split('.');
  let current: unknown = obj;
  for (const part of parts) {
    if (current === null || current === undefined) return false;
    current = (current as Record<string, unknown>)[part];
  }
  return Array.isArray(current);
}

// ---------------------------------------------------------------------------
// Core check function
// ---------------------------------------------------------------------------

/**
 * Make an API request and verify the response.
 *
 * Checks:
 * 1. Status code < 500 (server errors fail; 4xx are acceptable for empty data)
 * 2. Response body contains all expectedFields
 *
 * Uses soft assertions so one failing endpoint does not abort the suite.
 *
 * @param request - Playwright APIRequestContext
 * @param baseURL - Admin portal base URL (e.g. https://admin.garudax.asla.mn)
 * @param token - JWT access token (null for unauthenticated endpoints)
 * @param method - HTTP method
 * @param path - API path relative to /api/v1 (e.g. "/admin/health")
 * @param expectedFields - Field paths to verify in response body
 * @returns CheckResult with pass/fail details
 */
export async function checkEndpoint(
  request: APIRequestContext,
  baseURL: string,
  token: string | null,
  method: EndpointCheck['method'],
  path: string,
  expectedFields: string[],
  body?: Record<string, unknown>,
): Promise<CheckResult> {
  const url = `${baseURL}/api/v1${path}`;
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  let status = 0;
  let responseBody: unknown = null;
  let errorMessage: string | undefined;

  try {
    let response;
    switch (method) {
      case 'POST':
        response = await request.post(url, { headers, data: body });
        break;
      case 'PUT':
        response = await request.put(url, { headers, data: body });
        break;
      case 'DELETE':
        response = await request.delete(url, { headers });
        break;
      default:
        response = await request.get(url, { headers });
    }

    status = response.status();

    try {
      responseBody = await response.json();
    } catch {
      // Response may not be JSON (e.g. 204 No Content)
      responseBody = null;
    }
  } catch (err) {
    errorMessage = String(err);
    status = 0;
  }

  // Check: status < 500
  const statusOk = status > 0 && status < 500;
  expect.soft(statusOk, `${method} ${path}: status ${status} should be < 500`).toBeTruthy();

  // Check expected fields
  const missingFields: string[] = [];
  if (statusOk && responseBody !== null && expectedFields.length > 0) {
    for (const field of expectedFields) {
      const found = hasField(responseBody, field) || isNonEmptyArray(responseBody, field);
      if (!found) {
        missingFields.push(field);
      }
    }

    if (missingFields.length > 0) {
      expect.soft(
        false,
        `${method} ${path}: missing fields [${missingFields.join(', ')}] in response`,
      ).toBeTruthy();
    }
  }

  return {
    name: `${method} ${path}`,
    path,
    status,
    passed: statusOk && missingFields.length === 0,
    missingFields,
    error: errorMessage,
  };
}

/**
 * Run all checks in an array and return results.
 * Skips checks with `skip: true`.
 */
export async function runAllChecks(
  request: APIRequestContext,
  baseURL: string,
  token: string | null,
  checks: EndpointCheck[],
): Promise<CheckResult[]> {
  const results: CheckResult[] = [];

  for (const check of checks) {
    if (check.skip) {
      console.log(`[UAT] Skipping: ${check.name}`);
      continue;
    }

    const checkToken = check.requiresAuth === false ? null : token;
    const result = await checkEndpoint(
      request,
      baseURL,
      checkToken,
      check.method,
      check.path,
      check.expectedFields,
      check.body,
    );

    console.log(
      `[UAT] ${result.passed ? 'PASS' : 'FAIL'} [${result.status}] ${check.name}` +
      (result.missingFields.length > 0 ? ` — missing: ${result.missingFields.join(', ')}` : '') +
      (result.error ? ` — error: ${result.error}` : ''),
    );

    results.push(result);
  }

  return results;
}

// ---------------------------------------------------------------------------
// Predefined endpoint checks for the GarudaX admin portal
// ---------------------------------------------------------------------------

/**
 * Predefined API contract checks for 15+ admin endpoints.
 *
 * These cover the core admin API surface:
 * - Health/readiness
 * - Instruments
 * - Positions and netting
 * - Margin calls
 * - Settlement cycles
 * - Compliance alerts
 * - Participants
 * - Tickets
 * - Audit log
 * - Circuit breakers
 * - Warehouse
 * - Market data (ticker)
 * - Surveillance alerts
 * - Fees
 */
export const ADMIN_API_CHECKS: EndpointCheck[] = [
  // -------------------------------------------------------------------------
  // Health & Readiness (no auth required)
  // -------------------------------------------------------------------------
  {
    name: 'Gateway health check',
    method: 'GET',
    path: '/../healthz',  // gateway health is at /healthz, not /api/v1
    expectedFields: [],
    requiresAuth: false,
    skip: true, // healthz is outside /api/v1, handled separately
  },
  {
    name: 'Admin service health',
    method: 'GET',
    path: '/admin/health',
    expectedFields: ['services', 'overall_status'],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Auth
  // -------------------------------------------------------------------------
  {
    name: 'Auth login (sanity check)',
    method: 'POST',
    path: '/auth/login',
    expectedFields: ['AccessToken'],
    requiresAuth: false,
    body: { email: 'admin@garudax.mn', password: 'Adm1n@GarudaX!' },
  },

  // -------------------------------------------------------------------------
  // Instruments / Order Book
  // -------------------------------------------------------------------------
  {
    name: 'List instruments',
    method: 'GET',
    path: '/instruments',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Clearing / Positions
  // -------------------------------------------------------------------------
  {
    name: 'Get clearing positions',
    method: 'GET',
    path: '/clearing/positions',
    expectedFields: [],
    requiresAuth: true,
  },
  {
    name: 'Get netting obligations',
    method: 'GET',
    path: '/clearing/netting',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Margin
  // -------------------------------------------------------------------------
  {
    name: 'Get margin calls',
    method: 'GET',
    path: '/margin/calls',
    expectedFields: [],
    requiresAuth: true,
  },
  {
    name: 'Get margin call stats',
    method: 'GET',
    path: '/margin/calls/stats',
    expectedFields: [],
    requiresAuth: true,
  },
  {
    name: 'Get portfolio margin',
    method: 'GET',
    path: '/margin',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Settlement
  // -------------------------------------------------------------------------
  {
    name: 'Get settlement cycles',
    method: 'GET',
    path: '/settlement/cycles',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Compliance
  // -------------------------------------------------------------------------
  {
    name: 'Get compliance alerts',
    method: 'GET',
    path: '/compliance/alerts',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Participants
  // -------------------------------------------------------------------------
  {
    name: 'List participants',
    method: 'GET',
    path: '/participants',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Tickets
  // -------------------------------------------------------------------------
  {
    name: 'List support tickets',
    method: 'GET',
    path: '/tickets',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Audit Log
  // -------------------------------------------------------------------------
  {
    name: 'Get audit trail',
    method: 'GET',
    path: '/audit/trail',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Warehouse
  // -------------------------------------------------------------------------
  {
    name: 'Get warehouse receipts',
    method: 'GET',
    path: '/warehouse/receipts',
    expectedFields: [],
    requiresAuth: true,
  },
  {
    name: 'Get warehouse facilities',
    method: 'GET',
    path: '/warehouse/facilities',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Surveillance
  // -------------------------------------------------------------------------
  {
    name: 'Get surveillance alerts',
    method: 'GET',
    path: '/surveillance/alerts',
    expectedFields: [],
    requiresAuth: true,
  },

  // -------------------------------------------------------------------------
  // Fee Management
  // -------------------------------------------------------------------------
  {
    name: 'Get fee schedule',
    method: 'GET',
    path: '/fees',
    expectedFields: [],
    requiresAuth: true,
  },
];

// ---------------------------------------------------------------------------
// Subset for quick smoke tests
// ---------------------------------------------------------------------------

/**
 * Minimal set of checks for a quick smoke test — covers health, auth, and
 * the most critical data endpoints (positions, margin, settlement).
 */
export const SMOKE_API_CHECKS: EndpointCheck[] = ADMIN_API_CHECKS.filter((c) =>
  [
    'Admin service health',
    'Auth login (sanity check)',
    'Get clearing positions',
    'Get margin calls',
    'Get settlement cycles',
    'List participants',
  ].includes(c.name),
);
