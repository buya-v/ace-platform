import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createTokenManager } from '../services/tokenManager';

describe('tokenManager', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('stores and retrieves a token', () => {
    const tm = createTokenManager();
    tm.setToken('test-token', 3600);
    expect(tm.getToken()).toBe('test-token');
  });

  it('returns null when no token is set', () => {
    const tm = createTokenManager();
    expect(tm.getToken()).toBeNull();
  });

  it('returns null after token expires', () => {
    const tm = createTokenManager();
    tm.setToken('test-token', 10);
    vi.advanceTimersByTime(11000);
    expect(tm.getToken()).toBeNull();
  });

  it('returns token before expiry', () => {
    const tm = createTokenManager();
    tm.setToken('test-token', 10);
    vi.advanceTimersByTime(5000);
    expect(tm.getToken()).toBe('test-token');
  });

  it('clears the token', () => {
    const tm = createTokenManager();
    tm.setToken('test-token', 3600);
    tm.clear();
    expect(tm.getToken()).toBeNull();
  });

  it('schedules refresh at 80% of token lifetime', () => {
    const tm = createTokenManager();
    const refreshFn = vi.fn().mockResolvedValue(undefined);
    tm.onRefreshNeeded(refreshFn);
    tm.setToken('test-token', 100); // 100 seconds

    // At 80 seconds (80%), refresh should fire
    vi.advanceTimersByTime(79000);
    expect(refreshFn).not.toHaveBeenCalled();
    vi.advanceTimersByTime(2000);
    expect(refreshFn).toHaveBeenCalledTimes(1);
  });

  it('handles refresh callback error gracefully', () => {
    const tm = createTokenManager();
    const refreshFn = vi.fn().mockRejectedValue(new Error('refresh failed'));
    tm.onRefreshNeeded(refreshFn);
    tm.setToken('test-token', 100);

    vi.advanceTimersByTime(81000);
    expect(refreshFn).toHaveBeenCalledTimes(1);
    // Should not throw
  });

  it('clears refresh timer on clear()', () => {
    const tm = createTokenManager();
    const refreshFn = vi.fn().mockResolvedValue(undefined);
    tm.onRefreshNeeded(refreshFn);
    tm.setToken('test-token', 100);

    tm.clear();
    vi.advanceTimersByTime(100000);
    expect(refreshFn).not.toHaveBeenCalled();
  });

  it('replaces token with setToken', () => {
    const tm = createTokenManager();
    tm.setToken('token-1', 3600);
    tm.setToken('token-2', 3600);
    expect(tm.getToken()).toBe('token-2');
  });
});
