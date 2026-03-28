import { describe, it, expect } from 'vitest';
import { validateOrder } from '../types/order';

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

  it('does not require price for ioc orders', () => {
    const errors = validateOrder({
      instrument_id: 'abc',
      side: 'buy',
      order_type: 'ioc',
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
});
