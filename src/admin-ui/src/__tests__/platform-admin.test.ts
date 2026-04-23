import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { validateTenantForm } from '../pages/PlatformAdmin';
import { setAccessToken } from '../services/api';
import type { CreateTenantForm } from '../pages/PlatformAdmin';

// ─── validateTenantForm ────────────────────────────────────────────────────

const validForm: CreateTenantForm = {
  id: 'mse-bonds',
  name: 'Mongolian Stock Exchange',
  governance_tier: 'STANDARD',
};

// ─── TestValidateTenantForm_Valid ──────────────────────────────────────────

describe('TestValidateTenantForm_Valid', () => {
  it('returns no errors for a fully valid form', () => {
    const errors = validateTenantForm(validForm);
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts single-segment slug id', () => {
    const errors = validateTenantForm({ ...validForm, id: 'ace' });
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts numeric id', () => {
    const errors = validateTenantForm({ ...validForm, id: 'tenant1' });
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts hyphen-separated id', () => {
    const errors = validateTenantForm({ ...validForm, id: 'ace-commodities' });
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts FLAGSHIP governance_tier', () => {
    const errors = validateTenantForm({ ...validForm, governance_tier: 'FLAGSHIP' });
    expect(Object.keys(errors)).toHaveLength(0);
  });

  it('accepts SANDBOX governance_tier', () => {
    const errors = validateTenantForm({ ...validForm, governance_tier: 'SANDBOX' });
    expect(Object.keys(errors)).toHaveLength(0);
  });
});

// ─── TestValidateTenantForm_MissingId ─────────────────────────────────────

describe('TestValidateTenantForm_MissingId', () => {
  it('requires id', () => {
    const errors = validateTenantForm({ ...validForm, id: '' });
    expect(errors.id).toBeDefined();
    expect(errors.id).toMatch(/required/i);
  });

  it('rejects whitespace-only id', () => {
    const errors = validateTenantForm({ ...validForm, id: '   ' });
    expect(errors.id).toBeDefined();
  });

  it('does not produce a name error when only id is missing', () => {
    const errors = validateTenantForm({ ...validForm, id: '' });
    expect(errors.name).toBeUndefined();
  });
});

// ─── TestValidateTenantForm_MissingName ───────────────────────────────────

describe('TestValidateTenantForm_MissingName', () => {
  it('requires name', () => {
    const errors = validateTenantForm({ ...validForm, name: '' });
    expect(errors.name).toBeDefined();
    expect(errors.name).toMatch(/required/i);
  });

  it('rejects whitespace-only name', () => {
    const errors = validateTenantForm({ ...validForm, name: '   ' });
    expect(errors.name).toBeDefined();
  });

  it('does not produce an id error when only name is missing', () => {
    const errors = validateTenantForm({ ...validForm, name: '' });
    expect(errors.id).toBeUndefined();
  });
});

// ─── TestValidateTenantForm_InvalidSlug ───────────────────────────────────

describe('TestValidateTenantForm_InvalidSlug', () => {
  it('rejects uppercase letters', () => {
    const errors = validateTenantForm({ ...validForm, id: 'MSE-bonds' });
    expect(errors.id).toBeDefined();
    expect(errors.id).toMatch(/lowercase/i);
  });

  it('rejects id with spaces', () => {
    const errors = validateTenantForm({ ...validForm, id: 'mse bonds' });
    expect(errors.id).toBeDefined();
  });

  it('rejects id with underscores', () => {
    const errors = validateTenantForm({ ...validForm, id: 'mse_bonds' });
    expect(errors.id).toBeDefined();
  });

  it('rejects id with special characters', () => {
    const errors = validateTenantForm({ ...validForm, id: 'mse@bonds!' });
    expect(errors.id).toBeDefined();
  });

  it('rejects ALL_CAPS id', () => {
    const errors = validateTenantForm({ ...validForm, id: 'MYBONDS' });
    expect(errors.id).toBeDefined();
  });

  it('rejects id with dots', () => {
    const errors = validateTenantForm({ ...validForm, id: 'mse.bonds' });
    expect(errors.id).toBeDefined();
  });

  it('accepts all-lowercase with multiple hyphens', () => {
    const errors = validateTenantForm({ ...validForm, id: 'my-exchange-bonds-v2' });
    expect(errors.id).toBeUndefined();
  });
});

// ─── Both fields missing ───────────────────────────────────────────────────

describe('validateTenantForm — both fields missing', () => {
  it('reports both id and name errors for empty form', () => {
    const emptyForm: CreateTenantForm = {
      id: '',
      name: '',
      governance_tier: 'STANDARD',
    };
    const errors = validateTenantForm(emptyForm);
    expect(errors.id).toBeDefined();
    expect(errors.name).toBeDefined();
    expect(Object.keys(errors)).toHaveLength(2);
  });
});

// ─── API: fetchTenants, createTenant, updateTenantStatus ──────────────────

describe('Platform Tenant API', () => {
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

  it('fetchTenants calls GET /platform/v1/tenants', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: [] }),
    });

    const { fetchTenants } = await import('../services/api');
    await fetchTenants();

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/platform/v1/tenants',
      expect.any(Object),
    );
  });

  it('createTenant calls POST /platform/v1/tenants with body', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 'mse-bonds', name: 'MSE Bonds' }),
    });

    const { createTenant } = await import('../services/api');
    const payload = { id: 'mse-bonds', name: 'MSE Bonds', governance_tier: 'STANDARD' };
    await createTenant(payload);

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/platform/v1/tenants',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    );
  });

  it('createTenant sends request without governance_tier when omitted', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ id: 'ace-test', name: 'ACE Test' }),
    });

    const { createTenant } = await import('../services/api');
    await createTenant({ id: 'ace-test', name: 'ACE Test' });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/platform/v1/tenants',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ id: 'ace-test', name: 'ACE Test' }),
      }),
    );
  });

  it('updateTenantStatus calls PUT /platform/v1/tenants/{id}/status', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { updateTenantStatus } = await import('../services/api');
    await updateTenantStatus('mse-bonds', 'ACTIVE');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/platform/v1/tenants/mse-bonds/status',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ status: 'ACTIVE' }),
      }),
    );
  });

  it('updateTenantStatus sends SUSPENDED status', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    });

    const { updateTenantStatus } = await import('../services/api');
    await updateTenantStatus('ace-commodities', 'SUSPENDED');

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'http://test-api/api/v1/platform/v1/tenants/ace-commodities/status',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ status: 'SUSPENDED' }),
      }),
    );
  });
});
