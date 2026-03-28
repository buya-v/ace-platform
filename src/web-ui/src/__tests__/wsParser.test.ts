import { describe, it, expect } from 'vitest';
import { parseTradeMessage, WebSocketManager } from '../services/ws';

describe('parseTradeMessage', () => {
  it('parses a valid trade message', () => {
    const result = parseTradeMessage({
      tradeId: 't1',
      price: '100.50',
      quantity: '10',
      side: 'buy',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 42,
    });

    expect(result).toEqual({
      tradeId: 't1',
      price: '100.50',
      quantity: '10',
      side: 'buy',
      timestamp: '2026-01-01T00:00:00Z',
      sequence: 42,
    });
  });

  it('returns null for null input', () => {
    expect(parseTradeMessage(null)).toBeNull();
  });

  it('returns null for non-object input', () => {
    expect(parseTradeMessage('string')).toBeNull();
    expect(parseTradeMessage(123)).toBeNull();
  });

  it('returns null for missing tradeId', () => {
    expect(parseTradeMessage({
      price: '100',
      quantity: '10',
      side: 'buy',
      timestamp: '2026-01-01',
    })).toBeNull();
  });

  it('returns null for invalid side', () => {
    expect(parseTradeMessage({
      tradeId: 't1',
      price: '100',
      quantity: '10',
      side: 'invalid',
      timestamp: '2026-01-01',
    })).toBeNull();
  });

  it('returns null for missing price', () => {
    expect(parseTradeMessage({
      tradeId: 't1',
      quantity: '10',
      side: 'buy',
      timestamp: '2026-01-01',
    })).toBeNull();
  });

  it('defaults sequence to 0 if not a number', () => {
    const result = parseTradeMessage({
      tradeId: 't1',
      price: '100',
      quantity: '10',
      side: 'sell',
      timestamp: '2026-01-01',
    });

    expect(result?.sequence).toBe(0);
  });
});

describe('WebSocketManager', () => {
  it('calculates exponential backoff with jitter', () => {
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: () => {},
      onStatusChange: () => {},
      maxDelay: 30000,
    });

    // First retry: base = 1000 * 2^0 = 1000, + jitter up to 500
    const delay0 = manager.getBackoffDelay();
    expect(delay0).toBeGreaterThanOrEqual(1000);
    expect(delay0).toBeLessThanOrEqual(1500);
  });
});
