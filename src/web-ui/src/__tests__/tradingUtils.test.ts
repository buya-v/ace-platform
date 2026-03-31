import { describe, it, expect } from 'vitest';
import {
  calculateUnrealizedPnl,
  calculateTotalPnl,
  formatPnl,
  pnlColorClass,
  filterTrades,
  buildTradesCsv,
  formatTradeTime,
  calculateTradeValue,
} from '../services/tradingUtils';
import type { TradeRecord, TradeHistoryFilter } from '../types/trade';
import type { Position } from '../types/trade';

// --- calculateUnrealizedPnl ---

describe('calculateUnrealizedPnl', () => {
  it('calculates positive P&L for long position when price rises', () => {
    const result = calculateUnrealizedPnl('long', '100', '110', '10');
    expect(result).toBe('100.0000');
  });

  it('calculates negative P&L for long position when price drops', () => {
    const result = calculateUnrealizedPnl('long', '100', '90', '10');
    expect(result).toBe('-100.0000');
  });

  it('calculates positive P&L for short position when price drops', () => {
    const result = calculateUnrealizedPnl('short', '100', '90', '10');
    expect(result).toBe('100.0000');
  });

  it('calculates negative P&L for short position when price rises', () => {
    const result = calculateUnrealizedPnl('short', '100', '110', '10');
    expect(result).toBe('-100.0000');
  });

  it('returns 0 for flat position', () => {
    const result = calculateUnrealizedPnl('flat', '100', '110', '10');
    expect(result).toBe('0.0000');
  });

  it('handles zero quantity', () => {
    const result = calculateUnrealizedPnl('long', '100', '110', '0');
    expect(result).toBe('0.0000');
  });

  it('returns 0 for NaN inputs', () => {
    expect(calculateUnrealizedPnl('long', 'abc', '110', '10')).toBe('0.0000');
    expect(calculateUnrealizedPnl('long', '100', 'abc', '10')).toBe('0.0000');
    expect(calculateUnrealizedPnl('long', '100', '110', 'abc')).toBe('0.0000');
  });

  it('handles negative quantity (takes absolute value)', () => {
    const result = calculateUnrealizedPnl('long', '100', '110', '-10');
    expect(result).toBe('100.0000');
  });

  it('handles fractional values', () => {
    const result = calculateUnrealizedPnl('long', '100.50', '101.75', '5');
    expect(result).toBe('6.2500');
  });
});

// --- calculateTotalPnl ---

describe('calculateTotalPnl', () => {
  it('sums realized and unrealized P&L across positions', () => {
    const positions: Position[] = [
      { instrumentId: 'a', instrumentSymbol: 'WHEAT', netQuantity: '10', avgEntryPrice: '100', unrealizedPnl: '50', realizedPnl: '20', side: 'long' },
      { instrumentId: 'b', instrumentSymbol: 'CORN', netQuantity: '5', avgEntryPrice: '200', unrealizedPnl: '-30', realizedPnl: '10', side: 'short' },
    ];
    const result = calculateTotalPnl(positions);
    expect(result.totalRealized).toBe('30.0000');
    expect(result.totalUnrealized).toBe('20.0000');
    expect(result.totalPnl).toBe('50.0000');
  });

  it('returns zeros for empty positions', () => {
    const result = calculateTotalPnl([]);
    expect(result.totalRealized).toBe('0.0000');
    expect(result.totalUnrealized).toBe('0.0000');
    expect(result.totalPnl).toBe('0.0000');
  });

  it('handles positions with zero P&L', () => {
    const positions: Position[] = [
      { instrumentId: 'a', instrumentSymbol: 'WHEAT', netQuantity: '10', avgEntryPrice: '100', unrealizedPnl: '0', realizedPnl: '0', side: 'flat' },
    ];
    const result = calculateTotalPnl(positions);
    expect(result.totalPnl).toBe('0.0000');
  });

  it('handles NaN values in positions gracefully', () => {
    const positions: Position[] = [
      { instrumentId: 'a', instrumentSymbol: 'WHEAT', netQuantity: '10', avgEntryPrice: '100', unrealizedPnl: 'abc', realizedPnl: 'xyz', side: 'long' },
    ];
    const result = calculateTotalPnl(positions);
    expect(result.totalPnl).toBe('0.0000');
  });
});

// --- formatPnl ---

describe('formatPnl', () => {
  it('adds + prefix for positive values', () => {
    expect(formatPnl('100.5')).toBe('+100.5000');
  });

  it('keeps - prefix for negative values', () => {
    expect(formatPnl('-50.25')).toBe('-50.2500');
  });

  it('formats zero without sign', () => {
    expect(formatPnl('0')).toBe('0.0000');
  });

  it('returns original string for NaN', () => {
    expect(formatPnl('abc')).toBe('abc');
  });
});

// --- pnlColorClass ---

describe('pnlColorClass', () => {
  it('returns positive for positive values', () => {
    expect(pnlColorClass('100')).toBe('positive');
  });

  it('returns negative for negative values', () => {
    expect(pnlColorClass('-50')).toBe('negative');
  });

  it('returns neutral for zero', () => {
    expect(pnlColorClass('0')).toBe('neutral');
  });

  it('returns neutral for NaN', () => {
    expect(pnlColorClass('abc')).toBe('neutral');
  });
});

// --- filterTrades ---

describe('filterTrades', () => {
  const trades: TradeRecord[] = [
    { tradeId: '1', instrumentId: 'wheat-1', instrumentSymbol: 'WHEAT', side: 'buy', quantity: '10', price: '100', totalValue: '1000', timestamp: '2026-03-15T10:00:00Z' },
    { tradeId: '2', instrumentId: 'corn-1', instrumentSymbol: 'CORN', side: 'sell', quantity: '5', price: '200', totalValue: '1000', timestamp: '2026-03-16T14:00:00Z' },
    { tradeId: '3', instrumentId: 'wheat-1', instrumentSymbol: 'WHEAT', side: 'sell', quantity: '3', price: '105', totalValue: '315', timestamp: '2026-03-17T09:00:00Z' },
    { tradeId: '4', instrumentId: 'corn-1', instrumentSymbol: 'CORN', side: 'buy', quantity: '2', price: '195', totalValue: '390', timestamp: '2026-03-18T16:00:00Z' },
  ];

  it('returns all trades with empty filter', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '', instrumentId: '', side: '' };
    expect(filterTrades(trades, filter).length).toBe(4);
  });

  it('filters by instrument', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '', instrumentId: 'wheat-1', side: '' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(2);
    expect(result.every((t) => t.instrumentId === 'wheat-1')).toBe(true);
  });

  it('filters by side', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '', instrumentId: '', side: 'buy' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(2);
    expect(result.every((t) => t.side === 'buy')).toBe(true);
  });

  it('filters by date range', () => {
    const filter: TradeHistoryFilter = { startDate: '2026-03-16', endDate: '2026-03-17', instrumentId: '', side: '' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(2);
    expect(result.map((t) => t.tradeId)).toEqual(['2', '3']);
  });

  it('filters by start date only', () => {
    const filter: TradeHistoryFilter = { startDate: '2026-03-17', endDate: '', instrumentId: '', side: '' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(2);
  });

  it('filters by end date only', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '2026-03-16', instrumentId: '', side: '' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(2);
  });

  it('combines filters', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '', instrumentId: 'wheat-1', side: 'sell' };
    const result = filterTrades(trades, filter);
    expect(result.length).toBe(1);
    expect(result[0].tradeId).toBe('3');
  });

  it('returns empty array when no trades match', () => {
    const filter: TradeHistoryFilter = { startDate: '', endDate: '', instrumentId: 'nonexistent', side: '' };
    expect(filterTrades(trades, filter).length).toBe(0);
  });
});

// --- buildTradesCsv ---

describe('buildTradesCsv', () => {
  it('builds CSV with header and rows', () => {
    const trades: TradeRecord[] = [
      { tradeId: '1', instrumentId: 'w1', instrumentSymbol: 'WHEAT', side: 'buy', quantity: '10', price: '100', totalValue: '1000', timestamp: '2026-03-15T10:00:00Z' },
    ];
    const csv = buildTradesCsv(trades);
    const lines = csv.split('\n');
    expect(lines[0]).toBe('Time,Instrument,Side,Quantity,Price,Total Value');
    expect(lines[1]).toBe('2026-03-15T10:00:00Z,WHEAT,buy,10,100,1000');
  });

  it('returns header only for empty trades', () => {
    const csv = buildTradesCsv([]);
    expect(csv).toBe('Time,Instrument,Side,Quantity,Price,Total Value');
  });

  it('handles multiple rows', () => {
    const trades: TradeRecord[] = [
      { tradeId: '1', instrumentId: 'w1', instrumentSymbol: 'WHEAT', side: 'buy', quantity: '10', price: '100', totalValue: '1000', timestamp: '2026-01-01' },
      { tradeId: '2', instrumentId: 'c1', instrumentSymbol: 'CORN', side: 'sell', quantity: '5', price: '200', totalValue: '1000', timestamp: '2026-01-02' },
    ];
    const csv = buildTradesCsv(trades);
    const lines = csv.split('\n');
    expect(lines.length).toBe(3);
  });
});

// --- formatTradeTime ---

describe('formatTradeTime', () => {
  it('formats a valid ISO timestamp', () => {
    const result = formatTradeTime('2026-03-15T10:30:45Z');
    // Should contain time parts
    expect(result).toContain('10');
    expect(result).toContain('30');
    expect(result).toContain('45');
  });

  it('returns original string for invalid timestamp', () => {
    expect(formatTradeTime('not-a-date')).toBe('not-a-date');
  });

  it('handles empty string', () => {
    expect(formatTradeTime('')).toBe('');
  });
});

// --- calculateTradeValue ---

describe('calculateTradeValue', () => {
  it('calculates price * quantity', () => {
    expect(calculateTradeValue('100', '10')).toBe('1000.0000');
  });

  it('handles fractional values', () => {
    expect(calculateTradeValue('100.50', '5')).toBe('502.5000');
  });

  it('returns 0 for NaN inputs', () => {
    expect(calculateTradeValue('abc', '10')).toBe('0.0000');
    expect(calculateTradeValue('100', 'abc')).toBe('0.0000');
  });

  it('handles zero values', () => {
    expect(calculateTradeValue('0', '10')).toBe('0.0000');
    expect(calculateTradeValue('100', '0')).toBe('0.0000');
  });
});
