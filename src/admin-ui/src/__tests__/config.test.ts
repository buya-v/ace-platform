import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getConfig } from '../config';

describe('getConfig', () => {
  beforeEach(() => {
    // Reset runtime config before each test
    (globalThis as Record<string, unknown>).window = {
      location: window.location,
    };
  });

  it('returns defaults when no runtime config', () => {
    const config = getConfig();
    expect(config.API_BASE_URL).toBe('/api/v1');
    // WS_BASE_URL should be dynamic based on current location
    expect(config.WS_BASE_URL).toMatch(/^wss?:\/\/.+\/api\/v1\/ws$/);
    expect(config.HEALTH_POLL_INTERVAL).toBe(15000);
    expect(config.AUTH_TOKEN_REFRESH_BUFFER).toBe(60);
  });

  it('generates wss URL for https protocol', () => {
    // Mock window.location with https protocol
    const mockLocation = {
      protocol: 'https:',
      host: 'admin.garudax.mn',
    };
    Object.defineProperty(globalThis.window, 'location', {
      value: mockLocation,
      writable: true,
      configurable: true,
    });

    const config = getConfig();
    expect(config.WS_BASE_URL).toBe('wss://admin.garudax.mn/api/v1/ws');
  });

  it('generates ws URL for http protocol', () => {
    // Mock window.location with http protocol
    const mockLocation = {
      protocol: 'http:',
      host: 'localhost:3000',
    };
    Object.defineProperty(globalThis.window, 'location', {
      value: mockLocation,
      writable: true,
      configurable: true,
    });

    const config = getConfig();
    expect(config.WS_BASE_URL).toBe('ws://localhost:3000/api/v1/ws');
  });

  it('merges runtime config over defaults', () => {
    (globalThis as Record<string, unknown>).window = {
      location: window.location,
      __GARUDAX_CONFIG__: {
        API_BASE_URL: 'https://api.garudax.mn/api/v1',
        HEALTH_POLL_INTERVAL: 30000,
      },
    };

    const config = getConfig();
    expect(config.API_BASE_URL).toBe('https://api.garudax.mn/api/v1');
    expect(config.HEALTH_POLL_INTERVAL).toBe(30000);
    // Defaults still apply for unset values
    expect(config.AUTH_TOKEN_REFRESH_BUFFER).toBe(60);
  });

  it('allows runtime config to override WS_BASE_URL', () => {
    (globalThis as Record<string, unknown>).window = {
      location: window.location,
      __GARUDAX_CONFIG__: {
        WS_BASE_URL: 'wss://custom-ws.garudax.mn/api/v1/ws',
      },
    };

    const config = getConfig();
    expect(config.WS_BASE_URL).toBe('wss://custom-ws.garudax.mn/api/v1/ws');
  });

  it('does not contain undefined or NaN values', () => {
    const config = getConfig();
    expect(config.API_BASE_URL).not.toBe('undefined');
    expect(config.WS_BASE_URL).not.toBe('undefined');
    expect(String(config.WS_BASE_URL)).not.toContain('NaN');
    expect(String(config.HEALTH_POLL_INTERVAL)).not.toContain('NaN');
    expect(String(config.AUTH_TOKEN_REFRESH_BUFFER)).not.toContain('NaN');
  });
});
