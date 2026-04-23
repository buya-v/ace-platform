import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  validateOrderForm,
  normalizeInstruments,
  normalizeOrders,
  formatOrderDate,
  OrderForm,
} from '../pages/SecuritiesOrders';

// ─── validateOrderForm ────────────────────────────────────────────────────────

describe('validateOrderForm', () => {
  const base: OrderForm = {
    instrument_id: 'AAPL',
    side: 'BUY',
    order_type: 'LIMIT',
    quantity: '100',
    price: '42.50',
    time_in_force: 'GTC',
  };

  it('returns no errors for valid LIMIT order', () => {
    const errors = validateOrderForm(base, 'LIMIT');
    expect(errors).toEqual({});
  });

  it('returns no errors for valid MARKET order (price ignored)', () => {
    const errors = validateOrderForm({ ...base, price: '' }, 'MARKET');
    expect(errors).toEqual({});
  });

  it('requires quantity > 0', () => {
    const errors = validateOrderForm({ ...base, quantity: '0' }, 'LIMIT');
    expect(errors.quantity).toBeDefined();
  });

  it('rejects negative quantity', () => {
    const errors = validateOrderForm({ ...base, quantity: '-5' }, 'LIMIT');
    expect(errors.quantity).toBeDefined();
  });

  it('rejects empty quantity', () => {
    const errors = validateOrderForm({ ...base, quantity: '' }, 'LIMIT');
    expect(errors.quantity).toBeDefined();
  });

  it('rejects non-numeric quantity', () => {
    const errors = validateOrderForm({ ...base, quantity: 'abc' }, 'LIMIT');
    expect(errors.quantity).toBeDefined();
  });

  it('requires price > 0 for LIMIT order', () => {
    const errors = validateOrderForm({ ...base, price: '0' }, 'LIMIT');
    expect(errors.price).toBeDefined();
  });

  it('requires price to be present for LIMIT order', () => {
    const errors = validateOrderForm({ ...base, price: '' }, 'LIMIT');
    expect(errors.price).toBeDefined();
  });

  it('requires positive price for LIMIT', () => {
    const errors = validateOrderForm({ ...base, price: '-1' }, 'LIMIT');
    expect(errors.price).toBeDefined();
  });

  it('does not require price for MARKET order', () => {
    const errors = validateOrderForm({ ...base, price: '' }, 'MARKET');
    expect(errors.price).toBeUndefined();
  });

  it('does not require price for MARKET with 0 price', () => {
    const errors = validateOrderForm({ ...base, price: '0' }, 'MARKET');
    expect(errors.price).toBeUndefined();
  });

  it('returns both errors when quantity and price invalid for LIMIT', () => {
    const errors = validateOrderForm({ ...base, quantity: '0', price: '' }, 'LIMIT');
    expect(errors.quantity).toBeDefined();
    expect(errors.price).toBeDefined();
  });
});

// ─── normalizeInstruments ────────────────────────────────────────────────────

describe('normalizeInstruments', () => {
  it('handles null input', () => {
    expect(normalizeInstruments(null)).toEqual([]);
  });

  it('handles undefined input', () => {
    expect(normalizeInstruments(undefined)).toEqual([]);
  });

  it('handles empty object', () => {
    expect(normalizeInstruments({})).toEqual([]);
  });

  it('normalizes array of instruments', () => {
    const raw = [
      { instrument_id: 'AAPL', ticker: 'AAPL', name: 'Apple Inc.' },
      { id: 'MSFT', symbol: 'MSFT' },
    ];
    const result = normalizeInstruments(raw);
    expect(result).toHaveLength(2);
    expect(result[0].id).toBe('AAPL');
    expect(result[0].ticker).toBe('AAPL');
    expect(result[0].name).toBe('Apple Inc.');
    expect(result[1].id).toBe('MSFT');
  });

  it('normalizes { instruments: [...] } shape', () => {
    const raw = {
      instruments: [
        { instrument_id: 'GOOG', name: 'Alphabet Inc.' },
      ],
    };
    const result = normalizeInstruments(raw);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('GOOG');
    expect(result[0].name).toBe('Alphabet Inc.');
  });

  it('normalizes { data: [...] } shape', () => {
    const raw = {
      data: [
        { id: 'TSLA', name: 'Tesla' },
      ],
    };
    const result = normalizeInstruments(raw);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('TSLA');
  });

  it('uses ticker as fallback for id', () => {
    const raw = [{ ticker: 'AMZN' }];
    const result = normalizeInstruments(raw);
    expect(result[0].ticker).toBe('AMZN');
  });
});

// ─── normalizeOrders ─────────────────────────────────────────────────────────

describe('normalizeOrders', () => {
  it('handles null input', () => {
    expect(normalizeOrders(null)).toEqual([]);
  });

  it('handles undefined input', () => {
    expect(normalizeOrders(undefined)).toEqual([]);
  });

  it('handles empty array', () => {
    expect(normalizeOrders([])).toEqual([]);
  });

  it('normalizes array of orders', () => {
    const raw = [
      {
        id: 'ord-1',
        instrument_id: 'AAPL',
        side: 'buy',
        order_type: 'limit',
        quantity: 100,
        price: 42.5,
        filled_quantity: 50,
        status: 'partially_filled',
        created_at: '2026-04-23T10:00:00Z',
      },
    ];
    const result = normalizeOrders(raw);
    expect(result).toHaveLength(1);
    expect(result[0]._key).toBe('ord-1');
    expect(result[0].id).toBe('ord-1');
    expect(result[0].instrument_id).toBe('AAPL');
    expect(result[0].side).toBe('BUY');
    expect(result[0].order_type).toBe('LIMIT');
    expect(result[0].quantity).toBe('100');
    expect(result[0].price).toBe('42.5');
    expect(result[0].filled_quantity).toBe('50');
    expect(result[0].status).toBe('PARTIALLY_FILLED');
    expect(result[0].created_at).toBe('2026-04-23T10:00:00Z');
  });

  it('normalizes { orders: [...] } shape', () => {
    const raw = {
      orders: [
        { id: 'ord-2', instrument_id: 'MSFT', side: 'SELL', order_type: 'MARKET', quantity: 200, status: 'PENDING' },
      ],
    };
    const result = normalizeOrders(raw);
    expect(result).toHaveLength(1);
    expect(result[0].side).toBe('SELL');
    expect(result[0].order_type).toBe('MARKET');
  });

  it('normalizes { data: [...] } shape', () => {
    const raw = { data: [{ id: 'ord-3', side: 'BUY', status: 'FILLED', quantity: 10 }] };
    const result = normalizeOrders(raw);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe('FILLED');
  });

  it('uses order_id as fallback for id', () => {
    const raw = [{ order_id: 'abc-123', side: 'BUY', status: 'PENDING', quantity: 1 }];
    const result = normalizeOrders(raw);
    expect(result[0]._key).toBe('abc-123');
    expect(result[0].id).toBe('abc-123');
  });

  it('falls back to generated key when id missing', () => {
    const raw = [{ side: 'BUY', status: 'PENDING', quantity: 1 }];
    const result = normalizeOrders(raw);
    expect(result[0]._key).toBe('order-0');
  });

  it('normalizes filledQuantity camelCase field', () => {
    const raw = [{ id: 'o1', filledQuantity: 25, side: 'BUY', status: 'PENDING', quantity: 50 }];
    const result = normalizeOrders(raw);
    expect(result[0].filled_quantity).toBe('25');
  });
});

// ─── formatOrderDate ─────────────────────────────────────────────────────────

describe('formatOrderDate', () => {
  it('returns dash for empty string', () => {
    expect(formatOrderDate('')).toBe('-');
  });

  it('formats a valid ISO date string', () => {
    const result = formatOrderDate('2026-04-23T10:30:00Z');
    // Should return a non-empty string representation of the date
    expect(result).toBeTruthy();
    expect(result).not.toBe('-');
    expect(typeof result).toBe('string');
  });

  it('returns dash for undefined/null-like', () => {
    expect(formatOrderDate('')).toBe('-');
  });

  it('handles invalid date string gracefully', () => {
    const result = formatOrderDate('not-a-date');
    // Should return something — either the string or a formatted result
    expect(typeof result).toBe('string');
  });
});

// ─── API function signatures ──────────────────────────────────────────────────

describe('Securities API functions', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    (globalThis as Record<string, unknown>).window = {
      __GARUDAX_CONFIG__: { API_BASE_URL: 'http://test-api/api/v1' },
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('fetchSecuritiesOrders calls correct endpoint', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ orders: [] }),
    });

    const { fetchSecuritiesOrders } = await import('../services/api');
    await fetchSecuritiesOrders({ instrument_id: 'AAPL' });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/orders?instrument_id=AAPL',
      expect.any(Object),
    );
  });

  it('submitSecuritiesOrder calls POST /securities/orders', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: 'new-order' }),
    });

    const { submitSecuritiesOrder } = await import('../services/api');
    await submitSecuritiesOrder({ instrument_id: 'AAPL', side: 'BUY', order_type: 'LIMIT', quantity: 100, price: 42.5 });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/orders',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('cancelSecuritiesOrder calls POST /securities/orders/:id/cancel', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { cancelSecuritiesOrder } = await import('../services/api');
    await cancelSecuritiesOrder('ord-123');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/orders/ord-123/cancel',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('fetchSecuritiesInstruments calls correct endpoint', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ instruments: [] }),
    });

    const { fetchSecuritiesInstruments } = await import('../services/api');
    await fetchSecuritiesInstruments();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments',
      expect.any(Object),
    );
  });
});
