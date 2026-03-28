import { describe, it, expect, beforeEach } from 'vitest';
import { getConfig } from '../config';

describe('getConfig', () => {
  beforeEach(() => {
    (globalThis as Record<string, unknown>).window = {};
  });

  it('returns defaults when no runtime config', () => {
    const config = getConfig();
    expect(config.API_BASE_URL).toBe('/api/v1');
    expect(config.HEALTH_POLL_INTERVAL).toBe(15000);
    expect(config.AUTH_TOKEN_REFRESH_BUFFER).toBe(60);
  });

  it('merges runtime config over defaults', () => {
    (globalThis as Record<string, unknown>).window = {
      __ACE_CONFIG__: {
        API_BASE_URL: 'https://api.ace.mn/api/v1',
        HEALTH_POLL_INTERVAL: 30000,
      },
    };

    const config = getConfig();
    expect(config.API_BASE_URL).toBe('https://api.ace.mn/api/v1');
    expect(config.HEALTH_POLL_INTERVAL).toBe(30000);
    // Defaults still apply for unset values
    expect(config.AUTH_TOKEN_REFRESH_BUFFER).toBe(60);
  });
});
