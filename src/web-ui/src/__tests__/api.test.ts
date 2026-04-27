import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mock tokenManager before importing api
vi.mock('../services/tokenManager', () => {
  let token: string | null = null;
  return {
    tokenManager: {
      getToken: vi.fn(() => token),
      setToken: vi.fn((t: string) => { token = t; }),
      clear: vi.fn(() => { token = null; }),
      onRefreshNeeded: vi.fn(),
      __setToken: (t: string | null) => { token = t; },
    },
  };
});

import { apiRequest, login, logout, silentRefresh, AuthError, ApiError } from '../services/api';
import { tokenManager } from '../services/tokenManager';

const mockFetch = vi.fn();
globalThis.fetch = mockFetch as unknown as typeof fetch;

function jsonResponse(data: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken(null);
});

describe('AuthError', () => {
  it('has correct name', () => {
    const err = new AuthError('test');
    expect(err.name).toBe('AuthError');
    expect(err.message).toBe('test');
  });
});

describe('ApiError', () => {
  it('has status and optional code', () => {
    const err = new ApiError(400, 'bad', 'INVALID');
    expect(err.name).toBe('ApiError');
    expect(err.status).toBe(400);
    expect(err.message).toBe('bad');
    expect(err.code).toBe('INVALID');
  });
});

describe('login', () => {
  it('sends credentials and stores token', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({
      access_token: 'tok123',
      expires_in: 3600,
      user: { id: 'u1', email: 'a@b.com', displayName: 'A', roles: ['trader'], participantId: 'p1' },
    }));

    const result = await login('a@b.com', 'pass');
    expect(result.user.id).toBe('u1');
    expect(tokenManager.setToken).toHaveBeenCalledWith('tok123', 3600);
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/login', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ email: 'a@b.com', password: 'pass' }),
    }));
  });

  it('throws ApiError on failure', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({ error: 'Invalid credentials' }, 401));
    await expect(login('a@b.com', 'wrong')).rejects.toThrow('Invalid credentials');
  });

  it('throws generic message when response has no error field', async () => {
    mockFetch.mockReturnValueOnce(Promise.resolve({
      ok: false,
      status: 500,
      json: () => Promise.reject(new Error('no json')),
    }));
    await expect(login('a@b.com', 'pass')).rejects.toThrow('Login failed');
  });
});

describe('logout', () => {
  it('calls logout endpoint and clears token', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({}));
    await logout();
    expect(tokenManager.clear).toHaveBeenCalled();
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/logout', expect.objectContaining({ method: 'POST' }));
  });

  it('clears token even if request fails', async () => {
    mockFetch.mockReturnValueOnce(Promise.reject(new Error('network error')));
    // logout uses try/finally — the error from fetch is swallowed by the finally block
    // but the function still throws since there's no catch
    try {
      await logout();
    } catch {
      // expected
    }
    expect(tokenManager.clear).toHaveBeenCalled();
  });
});

describe('silentRefresh', () => {
  it('returns user on success', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({
      access_token: 'new-tok',
      expires_in: 1800,
      user: { id: 'u2', email: 'b@c.com', displayName: 'B', roles: ['admin'], participantId: 'p2' },
    }));

    const result = await silentRefresh();
    expect(result?.user.id).toBe('u2');
    expect(tokenManager.setToken).toHaveBeenCalledWith('new-tok', 1800);
  });

  it('returns null on failure', async () => {
    mockFetch.mockReturnValueOnce(jsonResponse({}, 401));
    const result = await silentRefresh();
    expect(result).toBeNull();
  });

  it('returns null on network error', async () => {
    mockFetch.mockReturnValueOnce(Promise.reject(new Error('offline')));
    const result = await silentRefresh();
    expect(result).toBeNull();
  });
});

describe('apiRequest', () => {
  it('throws AuthError when no token', async () => {
    await expect(apiRequest('/test')).rejects.toThrow(AuthError);
  });

  it('sends authenticated request', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ data: 'ok' }));

    const result = await apiRequest<{ data: string }>('/test');
    expect(result.data).toBe('ok');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/test', expect.objectContaining({
      headers: expect.objectContaining({
        Authorization: 'Bearer valid-token',
      }),
    }));
  });

  it('includes X-GarudaX-Tenant header in every authenticated request', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ data: 'ok' }));

    await apiRequest('/securities/instruments');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/securities/instruments', expect.objectContaining({
      headers: expect.objectContaining({
        'X-GarudaX-Tenant': 'mse-equities',
      }),
    }));
  });

  it('uses /securities/instruments endpoint for instrument list', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ data: [{ id: 'inst-1', ticker: 'MNE', name: 'Mongolian Energy', trading_status: 'active' }] }));

    const result = await apiRequest<{ data: { id: string; ticker: string; name: string; trading_status: string }[] }>('/securities/instruments');
    expect(result.data[0].id).toBe('inst-1');
    expect(result.data[0].ticker).toBe('MNE');
    expect(result.data[0].name).toBe('Mongolian Energy');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/securities/instruments', expect.anything());
  });

  it('uses /securities/orders endpoint for order submission', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ order_id: 'ord-1', status: 'pending' }));

    const result = await apiRequest<{ order_id: string; status: string }>('/securities/orders', {
      method: 'POST',
      body: JSON.stringify({ instrument_id: 'inst-1', side: 'buy', order_type: 'limit', quantity: '10', price: '100' }),
    });
    expect(result.order_id).toBe('ord-1');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/securities/orders', expect.objectContaining({ method: 'POST' }));
  });

  it('uses /securities/trades endpoint for trade history', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ data: [{ tradeId: 'tr-1', price: '100', quantity: '5' }] }));

    const result = await apiRequest<{ data: { tradeId: string; price: string; quantity: string }[] }>('/securities/trades');
    expect(result.data[0].tradeId).toBe('tr-1');
    expect(mockFetch).toHaveBeenCalledWith('/api/v1/securities/trades', expect.anything());
  });

  it('retries on 401 after successful refresh', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('expired-token');

    // First call: 401
    mockFetch.mockReturnValueOnce(jsonResponse({}, 401));
    // Refresh call: success
    mockFetch.mockReturnValueOnce(jsonResponse({ access_token: 'new-tok', expires_in: 3600 }));
    // Retry: success
    mockFetch.mockReturnValueOnce(jsonResponse({ result: 'retry-ok' }));

    const result = await apiRequest<{ result: string }>('/test');
    expect(result.result).toBe('retry-ok');
  });

  it('throws AuthError on 401 when refresh fails', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('expired-token');

    mockFetch.mockReturnValueOnce(jsonResponse({}, 401));
    mockFetch.mockReturnValueOnce(jsonResponse({}, 401)); // refresh fails

    await expect(apiRequest('/test')).rejects.toThrow('Session expired');
  });

  it('throws ApiError on non-401 error', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(jsonResponse({ error: 'Not found', code: 'NOT_FOUND' }, 404));

    try {
      await apiRequest('/missing');
      expect.unreachable();
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).status).toBe(404);
      expect((err as ApiError).code).toBe('NOT_FOUND');
    }
  });

  it('handles non-JSON error response body', async () => {
    (tokenManager as unknown as { __setToken: (t: string | null) => void }).__setToken('valid-token');
    mockFetch.mockReturnValueOnce(Promise.resolve({
      ok: false,
      status: 500,
      json: () => Promise.reject(new Error('not json')),
    }));

    try {
      await apiRequest('/broken');
      expect.unreachable();
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError);
      expect((err as ApiError).message).toBe('Unknown error');
    }
  });
});
