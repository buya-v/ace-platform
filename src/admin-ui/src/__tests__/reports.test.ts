import { describe, it, expect } from 'vitest';
import {
  formatPrice,
  formatVolume,
  formatPct,
  todayDateString,
  buildMarketSummaryCSV,
  buildLargeTraderCSV,
  MARKET_SUMMARY_COLUMNS,
  LARGE_TRADER_COLUMNS,
  MarketSummaryRow,
  LargeTraderRow,
} from '../pages/Reports';

describe('formatPrice', () => {
  it('formats numeric string to 2 decimals', () => {
    expect(formatPrice('123.456')).toBe('123.46');
  });
  it('formats integer string', () => {
    expect(formatPrice('100')).toBe('100.00');
  });
  it('returns dash for empty string', () => {
    expect(formatPrice('')).toBe('-');
  });
  it('returns original for non-numeric', () => {
    expect(formatPrice('abc')).toBe('abc');
  });
});

describe('formatVolume', () => {
  it('formats volume with separators', () => {
    const result = formatVolume(1234567);
    expect(result).toContain('1');
    expect(result).toContain('234');
    expect(result).toContain('567');
  });
  it('formats zero', () => {
    expect(formatVolume(0)).toBe('0');
  });
  it('returns dash for NaN', () => {
    expect(formatVolume(NaN)).toBe('-');
  });
  it('returns dash for null', () => {
    expect(formatVolume(null as any)).toBe('-');
  });
});

describe('formatPct', () => {
  it('formats percentage to 2 decimals', () => {
    expect(formatPct(12.345)).toBe('12.35%');
  });
  it('formats zero', () => {
    expect(formatPct(0)).toBe('0.00%');
  });
  it('returns dash for NaN', () => {
    expect(formatPct(NaN)).toBe('-');
  });
  it('returns dash for null', () => {
    expect(formatPct(null as any)).toBe('-');
  });
});

describe('todayDateString', () => {
  it('returns a string in YYYY-MM-DD format', () => {
    const result = todayDateString();
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

describe('MARKET_SUMMARY_COLUMNS', () => {
  it('has 7 columns', () => {
    expect(MARKET_SUMMARY_COLUMNS).toHaveLength(7);
  });
  it('includes instrument_id, open, high, low, close, volume, vwap', () => {
    const keys = MARKET_SUMMARY_COLUMNS.map(c => c.key);
    expect(keys).toContain('instrument_id');
    expect(keys).toContain('open');
    expect(keys).toContain('high');
    expect(keys).toContain('low');
    expect(keys).toContain('close');
    expect(keys).toContain('volume');
    expect(keys).toContain('vwap');
  });
});

describe('LARGE_TRADER_COLUMNS', () => {
  it('has 6 columns', () => {
    expect(LARGE_TRADER_COLUMNS).toHaveLength(6);
  });
  it('includes participant and position fields', () => {
    const keys = LARGE_TRADER_COLUMNS.map(c => c.key);
    expect(keys).toContain('participant_id');
    expect(keys).toContain('participant_name');
    expect(keys).toContain('net_position');
    expect(keys).toContain('notional_value');
    expect(keys).toContain('pct_of_open_interest');
  });
});

describe('buildMarketSummaryCSV', () => {
  it('builds CSV with header and rows', () => {
    const rows: MarketSummaryRow[] = [
      { instrument_id: 'GOLD-2026', open: '1800.00', high: '1850.00', low: '1790.00', close: '1830.00', volume: 5000, vwap: '1820.50' },
      { instrument_id: 'SILVER-2026', open: '25.00', high: '26.00', low: '24.50', close: '25.80', volume: 3000 },
    ];
    const csv = buildMarketSummaryCSV(rows);
    const lines = csv.split('\r\n');
    expect(lines[0]).toBe('Instrument,Open,High,Low,Close,Volume,VWAP');
    expect(lines[1]).toContain('GOLD-2026');
    expect(lines[1]).toContain('1800.00');
    expect(lines[2]).toContain('SILVER-2026');
    expect(lines).toHaveLength(3);
  });

  it('returns header only for empty data', () => {
    const csv = buildMarketSummaryCSV([]);
    expect(csv).toBe('Instrument,Open,High,Low,Close,Volume,VWAP');
  });
});

describe('buildLargeTraderCSV', () => {
  it('builds CSV with header and rows', () => {
    const rows: LargeTraderRow[] = [
      { participant_id: 'p-1', participant_name: 'Trader A', instrument_id: 'GOLD-2026', net_position: 100, notional_value: '180000.00', pct_of_open_interest: 5.5 },
    ];
    const csv = buildLargeTraderCSV(rows);
    const lines = csv.split('\r\n');
    expect(lines[0]).toBe('Participant ID,Participant,Instrument,Net Position,Notional Value,% of Open Interest');
    expect(lines[1]).toContain('Trader A');
    expect(lines[1]).toContain('GOLD-2026');
    expect(lines).toHaveLength(2);
  });

  it('handles participant name with comma', () => {
    const rows: LargeTraderRow[] = [
      { participant_id: 'p-1', participant_name: 'Smith, LLC', instrument_id: 'GOLD', net_position: 50, notional_value: '90000', pct_of_open_interest: 2.1 },
    ];
    const csv = buildLargeTraderCSV(rows);
    expect(csv).toContain('"Smith, LLC"');
  });

  it('returns header only for empty data', () => {
    const csv = buildLargeTraderCSV([]);
    expect(csv).toBe('Participant ID,Participant,Instrument,Net Position,Notional Value,% of Open Interest');
  });
});
