import { describe, it, expect, vi, beforeEach } from 'vitest';
import { executeStep, resolveUrl } from '../services/executor';
import { StepDefinition } from '../types/step';

describe('resolveUrl', () => {
  it('returns absolute URL unchanged', () => {
    expect(resolveUrl('http://example.com/test', 'http://gateway:8080')).toBe('http://example.com/test');
  });

  it('returns https URL unchanged', () => {
    expect(resolveUrl('https://secure.com/api', 'http://gateway:8080')).toBe('https://secure.com/api');
  });

  it('prepends gateway URL to relative path', () => {
    expect(resolveUrl('/api/v1/orders', 'http://localhost:8080')).toBe('http://localhost:8080/api/v1/orders');
  });

  it('handles gateway URL with trailing slash', () => {
    expect(resolveUrl('/healthz', 'http://localhost:8080/')).toBe('http://localhost:8080/healthz');
  });

  it('adds leading slash to path without one', () => {
    expect(resolveUrl('api/v1/test', 'http://localhost:8080')).toBe('http://localhost:8080/api/v1/test');
  });
});

describe('executeStep', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('executes a GET step and returns PASS on 200', async () => {
    const mockResponse = { ok: true, status: 200, text: () => Promise.resolve('{"status":"ok"}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-1',
      title: 'Test',
      description: 'Test step',
      method: 'GET',
      url: '/healthz',
      validateResponse: (status) => (status === 200 ? 'PASS' : 'FAIL'),
    };

    const { result } = await executeStep(step, 'http://localhost:8080', {});
    expect(result.status).toBe('PASS');
    expect(result.responseStatus).toBe(200);
    expect(result.responseBody).toEqual({ status: 'ok' });
    expect(result.responseTime).toBeGreaterThanOrEqual(0);
    expect(result.stepId).toBe('test-1');
  });

  it('executes a POST step with body', async () => {
    const mockResponse = { ok: true, status: 201, text: () => Promise.resolve('{"id":"123"}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-2',
      title: 'Post Test',
      description: 'Post step',
      method: 'POST',
      url: '/api/v1/orders',
      body: () => ({ side: 'BUY' }),
      validateResponse: (status) => (status >= 200 && status < 300 ? 'PASS' : 'FAIL'),
    };

    const { result } = await executeStep(step, 'http://localhost:8080', {});
    expect(result.status).toBe('PASS');
    expect(result.requestBody).toEqual({ side: 'BUY' });
    expect(fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/orders',
      expect.objectContaining({
        method: 'POST',
        body: '{"side":"BUY"}',
      }),
    );
  });

  it('returns FAIL on network error', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('Network error'));

    const step: StepDefinition = {
      id: 'test-3',
      title: 'Error',
      description: 'Error step',
      method: 'GET',
      url: '/healthz',
      validateResponse: (status) => (status === 200 ? 'PASS' : 'FAIL'),
    };

    const { result } = await executeStep(step, 'http://localhost:8080', {});
    expect(result.status).toBe('FAIL');
    expect(result.error).toBe('Network error');
    expect(result.responseStatus).toBeNull();
  });

  it('uses dynamic URL from state', async () => {
    const mockResponse = { ok: true, status: 200, text: () => Promise.resolve('{}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-4',
      title: 'Dynamic',
      description: 'Dynamic URL',
      method: 'GET',
      url: (state) => `/api/v1/users/${state.userId}`,
      validateResponse: () => 'PASS',
    };

    await executeStep(step, 'http://localhost:8080', { userId: '42' });
    expect(fetch).toHaveBeenCalledWith('http://localhost:8080/api/v1/users/42', expect.anything());
  });

  it('extracts state from response on PASS', async () => {
    const mockResponse = { ok: true, status: 200, text: () => Promise.resolve('{"access_token":"jwt-xyz"}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-5',
      title: 'Login',
      description: 'Login step',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ username: 'trader1' }),
      validateResponse: (status) => (status === 200 ? 'PASS' : 'FAIL'),
      extractState: (body) => ({ token: (body as Record<string, unknown>).access_token }),
    };

    const { newState } = await executeStep(step, 'http://localhost:8080', {});
    expect(newState.token).toBe('jwt-xyz');
  });

  it('does not extract state on FAIL', async () => {
    const mockResponse = { ok: false, status: 401, text: () => Promise.resolve('{"error":"unauthorized"}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-6',
      title: 'Bad Login',
      description: 'Fail step',
      method: 'POST',
      url: '/api/v1/auth/login',
      body: () => ({ username: 'bad' }),
      validateResponse: (status) => (status === 200 ? 'PASS' : 'FAIL'),
      extractState: (body) => ({ token: (body as Record<string, unknown>).access_token }),
    };

    const { newState } = await executeStep(step, 'http://localhost:8080', { existing: 'value' });
    expect(newState.token).toBeUndefined();
    expect(newState.existing).toBe('value');
  });

  it('includes headers from step definition', async () => {
    const mockResponse = { ok: true, status: 200, text: () => Promise.resolve('{}') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-7',
      title: 'With Auth',
      description: 'Auth step',
      method: 'GET',
      url: '/api/v1/orders',
      headers: (state) => ({ Authorization: `Bearer ${state.token}` }),
      validateResponse: () => 'PASS',
    };

    await executeStep(step, 'http://localhost:8080', { token: 'abc' });
    expect(fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/v1/orders',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer abc' }),
      }),
    );
  });

  it('handles non-JSON response body', async () => {
    const mockResponse = { ok: true, status: 200, text: () => Promise.resolve('plain text') };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(mockResponse as Response);

    const step: StepDefinition = {
      id: 'test-8',
      title: 'Text',
      description: 'Text response',
      method: 'GET',
      url: '/healthz',
      validateResponse: () => 'PASS',
    };

    const { result } = await executeStep(step, 'http://localhost:8080', {});
    expect(result.responseBody).toBe('plain text');
  });
});
