import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiFetch, setAccessToken, ApiError } from '../services/api';

describe('apiFetch', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    setAccessToken(null);
    // Set up window.__ACE_CONFIG__
    (globalThis as Record<string, unknown>).window = {
      __ACE_CONFIG__: { API_BASE_URL: 'http://test-api/api/v1' },
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('makes GET request with correct URL', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: 'test' }),
    });

    await apiFetch('/health');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/health',
      expect.objectContaining({
        headers: expect.objectContaining({
          'Content-Type': 'application/json',
        }),
      }),
    );
  });

  it('includes Authorization header when token is set', async () => {
    setAccessToken('my-token');

    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ data: 'test' }),
    });

    await apiFetch('/health');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer my-token',
        }),
      }),
    );
  });

  it('throws ApiError on non-ok response', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      json: () => Promise.resolve({ error: { code: 'FORBIDDEN', message: 'Access denied' } }),
    });

    await expect(apiFetch('/admin/health')).rejects.toThrow(ApiError);
    try {
      await apiFetch('/admin/health');
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(403);
      expect((e as ApiError).code).toBe('FORBIDDEN');
    }
  });

  it('returns undefined for 204 responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const result = await apiFetch('/some/action');
    expect(result).toBeUndefined();
  });

  it('passes abort signal to fetch', async () => {
    const controller = new AbortController();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    await apiFetch('/health', {}, controller.signal);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        signal: controller.signal,
      }),
    );
  });
});
