import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { formatPnl, formatMoney, normalizePositions } from '../pages/SecuritiesPositions';

// ─── formatPnl ────────────────────────────────────────────────────────────────

describe('formatPnl', () => {
  it('formats positive number with + prefix', () => {
    expect(formatPnl(1234.56)).toBe('+1,234.56');
  });

  it('formats negative number with - prefix', () => {
    expect(formatPnl(-500.25)).toBe('-500.25');
  });

  it('formats zero as +0.00', () => {
    expect(formatPnl(0)).toBe('+0.00');
  });

  it('formats string positive number', () => {
    expect(formatPnl('250.5')).toBe('+250.50');
  });

  it('formats string negative number', () => {
    expect(formatPnl('-750.123')).toBe('-750.12');
  });

  it('returns 0.00 for NaN/empty string', () => {
    expect(formatPnl('')).toBe('0.00');
  });

  it('returns 0.00 for non-numeric string', () => {
    expect(formatPnl('abc')).toBe('0.00');
  });

  it('formats large positive value', () => {
    const result = formatPnl(1000000);
    expect(result.startsWith('+')).toBe(true);
    expect(result).toContain('1,000,000');
  });

  it('rounds to 2 decimal places', () => {
    expect(formatPnl(1.999)).toBe('+2.00');
  });

  it('handles very small negative value', () => {
    expect(formatPnl(-0.01)).toBe('-0.01');
  });
});

// ─── formatMoney ─────────────────────────────────────────────────────────────

describe('formatMoney', () => {
  it('formats a positive number with 2 decimals', () => {
    expect(formatMoney(1234.5)).toBe('1,234.50');
  });

  it('formats zero as 0.00', () => {
    expect(formatMoney(0)).toBe('0.00');
  });

  it('formats a string number', () => {
    expect(formatMoney('500')).toBe('500.00');
  });

  it('returns 0.00 for NaN', () => {
    expect(formatMoney('abc')).toBe('0.00');
  });

  it('formats negative value', () => {
    expect(formatMoney(-100)).toBe('-100.00');
  });

  it('formats string with decimal', () => {
    expect(formatMoney('99.9')).toBe('99.90');
  });
});

// ─── normalizePositions ───────────────────────────────────────────────────────

describe('normalizePositions', () => {
  it('returns empty array for null input', () => {
    expect(normalizePositions(null)).toEqual([]);
  });

  it('returns empty array for undefined input', () => {
    expect(normalizePositions(undefined)).toEqual([]);
  });

  it('returns empty array for empty object', () => {
    expect(normalizePositions({})).toEqual([]);
  });

  it('returns empty array for empty array', () => {
    expect(normalizePositions([])).toEqual([]);
  });

  it('normalizes array of positions', () => {
    const raw = [
      {
        id: 'pos-1',
        instrument_id: 'AAPL',
        quantity: 100,
        avg_cost: 145.50,
        market_value: 15000,
        unrealized_pnl: 550,
      },
    ];
    const result = normalizePositions(raw);
    expect(result).toHaveLength(1);
    expect(result[0]._key).toBe('pos-1');
    expect(result[0].instrument_id).toBe('AAPL');
    expect(result[0].quantity).toBe('100');
    expect(result[0].avg_cost).toBe('145.5');
    expect(result[0].market_value).toBe('15000');
    expect(result[0].unrealized_pnl).toBe('550');
  });

  it('normalizes { positions: [...] } shape', () => {
    const raw = {
      positions: [
        { id: 'pos-2', instrument_id: 'MSFT', quantity: 50, avg_cost: 300, market_value: 16000, unrealized_pnl: 1000 },
      ],
    };
    const result = normalizePositions(raw);
    expect(result).toHaveLength(1);
    expect(result[0].instrument_id).toBe('MSFT');
  });

  it('normalizes { data: [...] } shape', () => {
    const raw = {
      data: [
        { position_id: 'p3', instrument_id: 'GOOG', quantity: 10 },
      ],
    };
    const result = normalizePositions(raw);
    expect(result).toHaveLength(1);
    expect(result[0]._key).toBe('p3');
  });

  it('handles camelCase field names', () => {
    const raw = [
      {
        instrumentId: 'TSLA',
        net_quantity: 200,
        average_cost: 800,
        marketValue: 170000,
        unrealizedPnl: -10000,
      },
    ];
    const result = normalizePositions(raw);
    expect(result[0].instrument_id).toBe('TSLA');
    expect(result[0].quantity).toBe('200');
    expect(result[0].market_value).toBe('170000');
    expect(result[0].unrealized_pnl).toBe('-10000');
  });

  it('falls back to generated key when id missing', () => {
    const raw = [{ instrument_id: 'X', quantity: 1 }];
    const result = normalizePositions(raw);
    expect(result[0]._key).toBe('pos-0');
  });

  it('defaults instrument_id to dash when missing', () => {
    const raw = [{ id: 'p5', quantity: 1 }];
    const result = normalizePositions(raw);
    expect(result[0].instrument_id).toBe('-');
  });

  it('normalizes multiple positions', () => {
    const raw = [
      { id: 'a', instrument_id: 'AAPL', quantity: 10 },
      { id: 'b', instrument_id: 'MSFT', quantity: 20 },
      { id: 'c', instrument_id: 'GOOG', quantity: 30 },
    ];
    const result = normalizePositions(raw);
    expect(result).toHaveLength(3);
    expect(result.map(r => r.instrument_id)).toEqual(['AAPL', 'MSFT', 'GOOG']);
  });

  it('uses avg_price as fallback for avg_cost', () => {
    const raw = [{ id: 'p6', instrument_id: 'FB', avg_price: 250 }];
    const result = normalizePositions(raw);
    expect(result[0].avg_cost).toBe('250');
  });

  it('uses pnl as fallback for unrealized_pnl', () => {
    const raw = [{ id: 'p7', instrument_id: 'NFLX', pnl: -500 }];
    const result = normalizePositions(raw);
    expect(result[0].unrealized_pnl).toBe('-500');
  });
});

// ─── Securities Positions API ─────────────────────────────────────────────────

describe('fetchSecuritiesPositions API', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    (globalThis as Record<string, unknown>).window = {
      __GARUDAX_CONFIG__: { API_BASE_URL: 'http://test-api/api/v1' },
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('calls correct endpoint without filters', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ positions: [] }),
    });

    const { fetchSecuritiesPositions } = await import('../services/api');
    await fetchSecuritiesPositions();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/positions',
      expect.any(Object),
    );
  });

  it('calls correct endpoint with participant_id filter', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ positions: [] }),
    });

    const { fetchSecuritiesPositions } = await import('../services/api');
    await fetchSecuritiesPositions({ participant_id: 'participant-42' });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/positions?participant_id=participant-42',
      expect.any(Object),
    );
  });

  it('passes abort signal', async () => {
    const controller = new AbortController();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve([]),
    });

    const { fetchSecuritiesPositions } = await import('../services/api');
    await fetchSecuritiesPositions(undefined, controller.signal);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ signal: controller.signal }),
    );
  });
});
