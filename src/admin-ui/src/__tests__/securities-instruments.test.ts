import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { validateInstrumentForm } from '../pages/SecuritiesInstruments';
import { setAccessToken } from '../services/api';
import type { CreateInstrumentForm } from '../pages/SecuritiesInstruments';

// ─── validateInstrumentForm ────────────────────────────────────────────────

const validForm: CreateInstrumentForm = {
  ticker: 'AAPL',
  name: 'Apple Inc.',
  asset_class: 'EQUITY',
  lot_size: '100',
  tick_size: '0.01',
  currency: 'MNT',
  exchange_code: 'MSE',
};

describe('validateInstrumentForm — valid input', () => {
  it('returns no errors for a fully valid form', () => {
    const errors = validateInstrumentForm(validForm);
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts optional fields as empty strings', () => {
    const form = { ...validForm, currency: '', exchange_code: '' };
    const errors = validateInstrumentForm(form);
    expect(Object.keys(errors)).toHaveLength(0);
  });
});

describe('validateInstrumentForm — ticker', () => {
  it('requires ticker', () => {
    const errors = validateInstrumentForm({ ...validForm, ticker: '' });
    expect(errors.ticker).toBeDefined();
  });

  it('rejects whitespace-only ticker', () => {
    const errors = validateInstrumentForm({ ...validForm, ticker: '   ' });
    expect(errors.ticker).toBeDefined();
  });

  it('accepts non-empty ticker', () => {
    const errors = validateInstrumentForm({ ...validForm, ticker: 'X' });
    expect(errors.ticker).toBeUndefined();
  });
});

describe('validateInstrumentForm — name', () => {
  it('requires name', () => {
    const errors = validateInstrumentForm({ ...validForm, name: '' });
    expect(errors.name).toBeDefined();
  });

  it('rejects whitespace-only name', () => {
    const errors = validateInstrumentForm({ ...validForm, name: '   ' });
    expect(errors.name).toBeDefined();
  });

  it('accepts non-empty name', () => {
    const errors = validateInstrumentForm({ ...validForm, name: 'A' });
    expect(errors.name).toBeUndefined();
  });
});

describe('validateInstrumentForm — asset_class', () => {
  it('requires asset_class', () => {
    const errors = validateInstrumentForm({ ...validForm, asset_class: '' });
    expect(errors.asset_class).toBeDefined();
  });

  it('accepts EQUITY', () => {
    const errors = validateInstrumentForm({ ...validForm, asset_class: 'EQUITY' });
    expect(errors.asset_class).toBeUndefined();
  });

  it('accepts BOND', () => {
    const errors = validateInstrumentForm({ ...validForm, asset_class: 'BOND' });
    expect(errors.asset_class).toBeUndefined();
  });

  it('accepts ETF', () => {
    const errors = validateInstrumentForm({ ...validForm, asset_class: 'ETF' });
    expect(errors.asset_class).toBeUndefined();
  });
});

describe('validateInstrumentForm — lot_size', () => {
  it('requires lot_size', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: '' });
    expect(errors.lot_size).toBeDefined();
  });

  it('rejects non-numeric lot_size', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: 'abc' });
    expect(errors.lot_size).toBeDefined();
  });

  it('rejects lot_size of zero', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: '0' });
    expect(errors.lot_size).toBeDefined();
  });

  it('rejects negative lot_size', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: '-1' });
    expect(errors.lot_size).toBeDefined();
  });

  it('accepts positive lot_size', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: '100' });
    expect(errors.lot_size).toBeUndefined();
  });

  it('accepts fractional lot_size', () => {
    const errors = validateInstrumentForm({ ...validForm, lot_size: '0.5' });
    expect(errors.lot_size).toBeUndefined();
  });
});

describe('validateInstrumentForm — tick_size', () => {
  it('requires tick_size', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: '' });
    expect(errors.tick_size).toBeDefined();
  });

  it('rejects non-numeric tick_size', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: 'xyz' });
    expect(errors.tick_size).toBeDefined();
  });

  it('rejects tick_size of zero', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: '0' });
    expect(errors.tick_size).toBeDefined();
  });

  it('rejects negative tick_size', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: '-0.01' });
    expect(errors.tick_size).toBeDefined();
  });

  it('accepts positive tick_size', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: '0.01' });
    expect(errors.tick_size).toBeUndefined();
  });

  it('accepts small tick_size', () => {
    const errors = validateInstrumentForm({ ...validForm, tick_size: '0.0001' });
    expect(errors.tick_size).toBeUndefined();
  });
});

describe('validateInstrumentForm — multiple errors', () => {
  it('reports all errors at once for an empty form', () => {
    const emptyForm: CreateInstrumentForm = {
      ticker: '',
      name: '',
      asset_class: '',
      lot_size: '',
      tick_size: '',
      currency: '',
      exchange_code: '',
    };
    const errors = validateInstrumentForm(emptyForm);
    expect(errors.ticker).toBeDefined();
    expect(errors.name).toBeDefined();
    expect(errors.asset_class).toBeDefined();
    expect(errors.lot_size).toBeDefined();
    expect(errors.tick_size).toBeDefined();
    expect(Object.keys(errors)).toHaveLength(5);
  });

  it('reports only failing fields for a partially valid form', () => {
    const partialForm: CreateInstrumentForm = {
      ticker: 'TSLA',
      name: 'Tesla Inc.',
      asset_class: 'EQUITY',
      lot_size: '-5',
      tick_size: '0',
      currency: 'USD',
      exchange_code: '',
    };
    const errors = validateInstrumentForm(partialForm);
    expect(errors.ticker).toBeUndefined();
    expect(errors.name).toBeUndefined();
    expect(errors.asset_class).toBeUndefined();
    expect(errors.lot_size).toBeDefined();
    expect(errors.tick_size).toBeDefined();
  });
});

// ─── API integration: fetchSecuritiesInstruments, createSecuritiesInstrument, updateInstrumentStatus ─

describe('Securities Instruments API', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    setAccessToken(null);
    (globalThis as Record<string, unknown>).window = {
      __GARUDAX_CONFIG__: { API_BASE_URL: 'http://test-api/api/v1' },
    };
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('fetchSecuritiesInstruments calls GET /securities/instruments', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: [] }),
    });

    const { fetchSecuritiesInstruments } = await import('../services/api');
    await fetchSecuritiesInstruments();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments',
      expect.any(Object),
    );
  });

  it('fetchSecuritiesInstruments passes asset_class filter', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: [] }),
    });

    const { fetchSecuritiesInstruments } = await import('../services/api');
    await fetchSecuritiesInstruments({ asset_class: 'EQUITY' });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments?asset_class=EQUITY',
      expect.any(Object),
    );
  });

  it('fetchSecuritiesInstruments passes trading_status filter', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: [] }),
    });

    const { fetchSecuritiesInstruments } = await import('../services/api');
    await fetchSecuritiesInstruments({ trading_status: 'HALTED' });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments?trading_status=HALTED',
      expect.any(Object),
    );
  });

  it('createSecuritiesInstrument calls POST /securities/instruments with body', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 'instr-1', ticker: 'NEWCO' }),
    });

    const { createSecuritiesInstrument } = await import('../services/api');
    const payload = {
      ticker: 'NEWCO',
      name: 'New Company',
      asset_class: 'EQUITY',
      lot_size: 100,
      tick_size: 0.01,
      currency: 'MNT',
      exchange_code: 'MSE',
    };
    await createSecuritiesInstrument(payload);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    );
  });

  it('updateInstrumentStatus calls PUT /securities/instruments/{id}/status', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { updateInstrumentStatus } = await import('../services/api');
    await updateInstrumentStatus('instr-123', 'HALTED');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments/instr-123/status',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ status: 'HALTED' }),
      }),
    );
  });

  it('updateInstrumentStatus includes reason when provided', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { updateInstrumentStatus } = await import('../services/api');
    await updateInstrumentStatus('instr-456', 'HALTED', 'Circuit breaker triggered');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/securities/instruments/instr-456/status',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ status: 'HALTED', reason: 'Circuit breaker triggered' }),
      }),
    );
  });
});
