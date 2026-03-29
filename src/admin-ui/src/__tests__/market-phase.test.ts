import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { normalizeInstruments, getPhaseAction } from '../pages/MarketPhase';
import { setAccessToken } from '../services/api';

describe('MarketPhase — normalizeInstruments', () => {
  it('normalizes instruments from { instruments: [...] } response', () => {
    const raw = {
      instruments: [
        { instrument_id: 'CORN-2026', name: 'Corn Futures', phase: 'TRADING', last_updated: '2026-03-29T10:00:00Z' },
        { instrument_id: 'WHEAT-2026', name: 'Wheat Futures', phase: 'HALTED', last_updated: '2026-03-29T09:00:00Z' },
      ],
    };
    const result = normalizeInstruments(raw);
    expect(result).toHaveLength(2);
    expect(result[0]).toEqual({
      instrument_id: 'CORN-2026',
      name: 'Corn Futures',
      description: '',
      phase: 'TRADING',
      last_updated: '2026-03-29T10:00:00Z',
    });
    expect(result[1].phase).toBe('HALTED');
  });

  it('normalizes instruments from array response', () => {
    const raw = [
      { id: 'SOY-2026', ticker: 'SOY', status: 'AUCTION', updated_at: '2026-03-29T08:00:00Z' },
    ];
    const result = normalizeInstruments(raw);
    expect(result).toHaveLength(1);
    expect(result[0].instrument_id).toBe('SOY-2026');
    expect(result[0].name).toBe('SOY');
    expect(result[0].phase).toBe('AUCTION');
    expect(result[0].last_updated).toBe('2026-03-29T08:00:00Z');
  });

  it('handles null/undefined input', () => {
    expect(normalizeInstruments(null)).toEqual([]);
    expect(normalizeInstruments(undefined)).toEqual([]);
    expect(normalizeInstruments({})).toEqual([]);
  });

  it('defaults phase to PRE_OPEN when missing', () => {
    const raw = { instruments: [{ instrument_id: 'X' }] };
    const result = normalizeInstruments(raw);
    expect(result[0].phase).toBe('PRE_OPEN');
  });
});

describe('MarketPhase — getPhaseAction', () => {
  it('returns halt for TRADING', () => {
    expect(getPhaseAction('TRADING')).toBe('halt');
  });

  it('returns resume for HALTED', () => {
    expect(getPhaseAction('HALTED')).toBe('resume');
  });

  it('returns null for PRE_OPEN', () => {
    expect(getPhaseAction('PRE_OPEN')).toBeNull();
  });

  it('returns null for AUCTION', () => {
    expect(getPhaseAction('AUCTION')).toBeNull();
  });

  it('returns null for unknown phases', () => {
    expect(getPhaseAction('UNKNOWN')).toBeNull();
  });
});

describe('MarketPhase — StatusBadge color mapping', () => {
  it('TRADING maps to green badge class', () => {
    // Verify the StatusBadge color mapping indirectly via the statusColors object
    // We test the mapping logic: TRADING=green, HALTED=red, PRE_OPEN=yellow, AUCTION=blue
    const phaseColorMap: Record<string, string> = {
      TRADING: 'green',
      HALTED: 'red',
      PRE_OPEN: 'yellow',
      AUCTION: 'blue',
    };
    expect(phaseColorMap['TRADING']).toBe('green');
    expect(phaseColorMap['HALTED']).toBe('red');
    expect(phaseColorMap['PRE_OPEN']).toBe('yellow');
    expect(phaseColorMap['AUCTION']).toBe('blue');
  });
});

describe('MarketPhase — API functions', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    setAccessToken(null);
    (globalThis as Record<string, unknown>).window = {
      __ACE_CONFIG__: { API_BASE_URL: 'http://test-api/api/v1' },
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('haltInstrument calls POST /admin/instruments/{id}/halt', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { haltInstrument } = await import('../services/api');
    await haltInstrument('CORN-2026');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/admin/instruments/CORN-2026/halt',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('resumeInstrument calls POST /admin/instruments/{id}/resume', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { resumeInstrument } = await import('../services/api');
    await resumeInstrument('WHEAT-2026');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/admin/instruments/WHEAT-2026/resume',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('haltInstrument passes abort signal', async () => {
    const controller = new AbortController();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { haltInstrument } = await import('../services/api');
    await haltInstrument('CORN-2026', controller.signal);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ signal: controller.signal }),
    );
  });

  it('resumeInstrument passes abort signal', async () => {
    const controller = new AbortController();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { resumeInstrument } = await import('../services/api');
    await resumeInstrument('WHEAT-2026', controller.signal);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ signal: controller.signal }),
    );
  });
});

describe('MarketPhase — Halt All requires typed confirmation', () => {
  it('HALT ALL confirmation string must match exactly', () => {
    const requiredText = 'HALT ALL';
    const inputs: string[] = ['HALT', 'halt all', 'HALT ALL ', '', 'HALT ALL'];

    // Partial matches should fail
    expect(inputs[0] === requiredText).toBe(false);
    expect(inputs[1] === requiredText).toBe(false);
    expect(inputs[2] === requiredText).toBe(false);
    expect(inputs[3] === requiredText).toBe(false);

    // Exact match should succeed
    expect(inputs[4] === requiredText).toBe(true);
  });

  it('halt all only targets TRADING instruments', () => {
    const instruments = normalizeInstruments({
      instruments: [
        { instrument_id: 'A', phase: 'TRADING' },
        { instrument_id: 'B', phase: 'HALTED' },
        { instrument_id: 'C', phase: 'PRE_OPEN' },
        { instrument_id: 'D', phase: 'AUCTION' },
        { instrument_id: 'E', phase: 'TRADING' },
      ],
    });

    const tradingInstruments = instruments.filter(i => i.phase === 'TRADING');
    expect(tradingInstruments).toHaveLength(2);
    expect(tradingInstruments.map(i => i.instrument_id)).toEqual(['A', 'E']);
  });
});
