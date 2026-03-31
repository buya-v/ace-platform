import { describe, it, expect } from 'vitest';
import {
  formatBps,
  formatFeeAmount,
  formatFeeType,
  computeFeeSummary,
  FeeRule,
} from '../pages/FeeManagement';

function makeRule(overrides: Partial<FeeRule> = {}): FeeRule {
  return {
    id: 'fee-1',
    fee_type: 'TRADING_FEE',
    tier: 'STANDARD',
    rate_bps: 25,
    min_fee: '1.00',
    max_fee: '500.00',
    per_contract: '0.50',
    ...overrides,
  };
}

describe('formatBps', () => {
  it('formats basis points', () => {
    expect(formatBps(25)).toBe('25 bps');
  });
  it('formats zero', () => {
    expect(formatBps(0)).toBe('0 bps');
  });
  it('returns dash for NaN', () => {
    expect(formatBps(NaN)).toBe('-');
  });
  it('returns dash for null', () => {
    expect(formatBps(null as any)).toBe('-');
  });
});

describe('formatFeeAmount', () => {
  it('formats positive amount', () => {
    expect(formatFeeAmount('1.50')).toBe('$1.50');
  });
  it('formats large amount', () => {
    expect(formatFeeAmount('500')).toBe('$500.00');
  });
  it('returns dash for zero', () => {
    expect(formatFeeAmount('0')).toBe('-');
  });
  it('returns dash for 0.00', () => {
    expect(formatFeeAmount('0.00')).toBe('-');
  });
  it('returns dash for empty string', () => {
    expect(formatFeeAmount('')).toBe('-');
  });
  it('returns original for non-numeric string', () => {
    expect(formatFeeAmount('abc')).toBe('abc');
  });
});

describe('formatFeeType', () => {
  it('formats TRADING_FEE', () => {
    expect(formatFeeType('TRADING_FEE')).toBe('Trading Fee');
  });
  it('formats CLEARING_FEE', () => {
    expect(formatFeeType('CLEARING_FEE')).toBe('Clearing Fee');
  });
  it('formats SETTLEMENT_FEE', () => {
    expect(formatFeeType('SETTLEMENT_FEE')).toBe('Settlement Fee');
  });
  it('formats single word', () => {
    expect(formatFeeType('MARGIN')).toBe('Margin');
  });
  it('returns empty string for empty input', () => {
    expect(formatFeeType('')).toBe('');
  });
});

describe('computeFeeSummary', () => {
  it('computes correct summary for multiple rules', () => {
    const rules = [
      makeRule({ fee_type: 'TRADING_FEE', tier: 'STANDARD', rate_bps: 20 }),
      makeRule({ id: 'fee-2', fee_type: 'TRADING_FEE', tier: 'PREMIUM', rate_bps: 10 }),
      makeRule({ id: 'fee-3', fee_type: 'CLEARING_FEE', tier: 'STANDARD', rate_bps: 15 }),
    ];
    const summary = computeFeeSummary(rules);
    expect(summary.totalRules).toBe(3);
    expect(summary.uniqueTypes).toBe(2);
    expect(summary.avgRate).toBe(15); // (20+10+15)/3 = 15
    expect(summary.tiers).toEqual(['PREMIUM', 'STANDARD']);
  });

  it('returns zeros for empty rules', () => {
    const summary = computeFeeSummary([]);
    expect(summary.totalRules).toBe(0);
    expect(summary.uniqueTypes).toBe(0);
    expect(summary.avgRate).toBe(0);
    expect(summary.tiers).toEqual([]);
  });

  it('handles single rule', () => {
    const summary = computeFeeSummary([makeRule()]);
    expect(summary.totalRules).toBe(1);
    expect(summary.uniqueTypes).toBe(1);
    expect(summary.avgRate).toBe(25);
    expect(summary.tiers).toEqual(['STANDARD']);
  });

  it('sorts tiers alphabetically', () => {
    const rules = [
      makeRule({ tier: 'PREMIUM' }),
      makeRule({ id: 'fee-2', tier: 'BASIC' }),
      makeRule({ id: 'fee-3', tier: 'STANDARD' }),
    ];
    const summary = computeFeeSummary(rules);
    expect(summary.tiers).toEqual(['BASIC', 'PREMIUM', 'STANDARD']);
  });
});
