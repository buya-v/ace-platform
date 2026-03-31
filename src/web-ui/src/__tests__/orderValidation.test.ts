import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  validateOrder,
  requiresPrice,
  requiresStopPrice,
  requiresDisplayQty,
  requiresExpiry,
} from '../types/order';

describe('validateOrder', () => {
  it('returns no errors for a valid limit order', () => {
    const errors = validateOrder({
      instrument_id: 'abc-123',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100.50',
    });
    expect(errors).toEqual([]);
  });

  it('returns no errors for a valid market order', () => {
    const errors = validateOrder({
      instrument_id: 'abc-123',
      side: 'sell',
      order_type: 'market',
      quantity: '5',
    });
    expect(errors).toEqual([]);
  });

  it('requires instrument_id', () => {
    const errors = validateOrder({
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
    });
    expect(errors).toContainEqual({
      field: 'instrument_id',
      message: 'Instrument is required',
    });
  });

  it('validates side must be buy or sell', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'invalid' as 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
    });
    expect(errors).toContainEqual({
      field: 'side',
      message: 'Side must be buy or sell',
    });
  });

  it('requires order_type', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      quantity: '10',
    });
    expect(errors).toContainEqual({
      field: 'order_type',
      message: 'Order type is required',
    });
  });

  it('requires positive quantity', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'market',
      quantity: '0',
    });
    expect(errors).toContainEqual({
      field: 'quantity',
      message: 'Quantity must be a positive number',
    });
  });

  it('rejects non-numeric quantity', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'market',
      quantity: 'abc',
    });
    expect(errors).toContainEqual({
      field: 'quantity',
      message: 'Quantity must be a positive number',
    });
  });

  it('requires price for limit orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
    });
    expect(errors).toContainEqual({
      field: 'price',
      message: 'Price is required for limit orders',
    });
  });

  it('rejects zero price for limit orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '0',
    });
    expect(errors).toContainEqual({
      field: 'price',
      message: 'Price is required for limit orders',
    });
  });

  it('does not require price for market orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'sell',
      order_type: 'market',
      quantity: '5',
    });
    const priceError = errors.find((e) => e.field === 'price');
    expect(priceError).toBeUndefined();
  });

  it('returns multiple errors at once', () => {
    const errors = validateOrder({});
    expect(errors.length).toBeGreaterThanOrEqual(3);
  });

  it('accepts negative quantity as invalid', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'market',
      quantity: '-5',
    });
    expect(errors).toContainEqual({
      field: 'quantity',
      message: 'Quantity must be a positive number',
    });
  });

  // --- Stop-Limit order validation ---

  it('requires stop_price for stop-limit orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'stop-limit',
      quantity: '10',
      price: '100',
    });
    expect(errors).toContainEqual({
      field: 'stop_price',
      message: 'Stop price is required for stop orders',
    });
  });

  it('requires price for stop-limit orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'stop-limit',
      quantity: '10',
      stop_price: '95',
    });
    expect(errors).toContainEqual({
      field: 'price',
      message: 'Price is required for limit orders',
    });
  });

  it('accepts valid stop-limit order', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'stop-limit',
      quantity: '10',
      price: '100',
      stop_price: '95',
    });
    expect(errors).toEqual([]);
  });

  // --- Stop-Market order validation ---

  it('requires stop_price for stop-market orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'sell',
      order_type: 'stop-market',
      quantity: '10',
    });
    expect(errors).toContainEqual({
      field: 'stop_price',
      message: 'Stop price is required for stop orders',
    });
  });

  it('does not require price for stop-market orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'sell',
      order_type: 'stop-market',
      quantity: '10',
      stop_price: '95',
    });
    const priceError = errors.find((e) => e.field === 'price');
    expect(priceError).toBeUndefined();
    expect(errors).toEqual([]);
  });

  // --- Iceberg order validation ---

  it('requires display_qty for iceberg orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'iceberg',
      quantity: '100',
      price: '50',
    });
    expect(errors).toContainEqual({
      field: 'display_qty',
      message: 'Display quantity is required for iceberg orders',
    });
  });

  it('requires display_qty < quantity for iceberg orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'iceberg',
      quantity: '100',
      price: '50',
      display_qty: '100',
    });
    expect(errors).toContainEqual({
      field: 'display_qty',
      message: 'Display quantity must be less than total quantity',
    });
  });

  it('accepts valid iceberg order', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'iceberg',
      quantity: '100',
      price: '50',
      display_qty: '10',
    });
    expect(errors).toEqual([]);
  });

  // --- GTD time-in-force validation ---

  it('requires expire_time for GTD orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
      time_in_force: 'gtd',
    });
    expect(errors).toContainEqual({
      field: 'expire_time',
      message: 'Expiry date is required for GTD orders',
    });
  });

  it('rejects invalid expire_time for GTD orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
      time_in_force: 'gtd',
      expire_time: 'not-a-date',
    });
    expect(errors).toContainEqual({
      field: 'expire_time',
      message: 'Invalid expiry date',
    });
  });

  it('rejects past expire_time for GTD orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
      time_in_force: 'gtd',
      expire_time: '2020-01-01T00:00:00Z',
    });
    expect(errors).toContainEqual({
      field: 'expire_time',
      message: 'Expiry date must be in the future',
    });
  });

  it('accepts valid GTD order with future expiry', () => {
    const futureDate = new Date(Date.now() + 86400000).toISOString();
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
      time_in_force: 'gtd',
      expire_time: futureDate,
    });
    // Should have no expire_time errors
    const expiryError = errors.find((e) => e.field === 'expire_time');
    expect(expiryError).toBeUndefined();
  });

  it('does not require expire_time for non-GTD orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'limit',
      quantity: '10',
      price: '100',
      time_in_force: 'gtc',
    });
    const expiryError = errors.find((e) => e.field === 'expire_time');
    expect(expiryError).toBeUndefined();
  });
});

// --- Pure helper functions ---

describe('requiresPrice', () => {
  it('returns true for limit', () => expect(requiresPrice('limit')).toBe(true));
  it('returns true for stop-limit', () => expect(requiresPrice('stop-limit')).toBe(true));
  it('returns true for iceberg', () => expect(requiresPrice('iceberg')).toBe(true));
  it('returns false for market', () => expect(requiresPrice('market')).toBe(false));
  it('returns false for stop-market', () => expect(requiresPrice('stop-market')).toBe(false));
});

describe('requiresStopPrice', () => {
  it('returns true for stop-limit', () => expect(requiresStopPrice('stop-limit')).toBe(true));
  it('returns true for stop-market', () => expect(requiresStopPrice('stop-market')).toBe(true));
  it('returns false for limit', () => expect(requiresStopPrice('limit')).toBe(false));
  it('returns false for market', () => expect(requiresStopPrice('market')).toBe(false));
  it('returns false for iceberg', () => expect(requiresStopPrice('iceberg')).toBe(false));
});

describe('requiresDisplayQty', () => {
  it('returns true for iceberg', () => expect(requiresDisplayQty('iceberg')).toBe(true));
  it('returns false for limit', () => expect(requiresDisplayQty('limit')).toBe(false));
  it('returns false for market', () => expect(requiresDisplayQty('market')).toBe(false));
});

describe('requiresExpiry', () => {
  it('returns true for gtd', () => expect(requiresExpiry('gtd')).toBe(true));
  it('returns false for day', () => expect(requiresExpiry('day')).toBe(false));
  it('returns false for gtc', () => expect(requiresExpiry('gtc')).toBe(false));
  it('returns false for ioc', () => expect(requiresExpiry('ioc')).toBe(false));
  it('returns false for fok', () => expect(requiresExpiry('fok')).toBe(false));
});
