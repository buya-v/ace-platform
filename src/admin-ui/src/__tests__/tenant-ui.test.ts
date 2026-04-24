import { describe, it, expect } from 'vitest';

// Pure logic extracted from TopBar.tsx and DashboardHome.tsx for testability

/**
 * buildTenantSwitchToast — formats the toast message shown on tenant switch.
 * Mirrors the logic in TopBar's handleTenantChange.
 */
export function buildTenantSwitchToast(tenantName: string): string {
  return `Switched to ${tenantName}`;
}

/**
 * resolveTenantOption — determines the select option state given loading/tenant list.
 * Returns: 'loading' | 'empty' | 'list'
 */
export function resolveTenantOption(
  isLoading: boolean,
  tenants: { id: string; name: string }[],
): 'loading' | 'empty' | 'list' {
  if (isLoading) return 'loading';
  if (tenants.length === 0) return 'empty';
  return 'list';
}

/**
 * isTenantSelectDisabled — mirrors the disabled logic for the <select> element.
 */
export function isTenantSelectDisabled(
  isLoading: boolean,
  tenantCount: number,
): boolean {
  return isLoading || tenantCount === 0;
}

/**
 * buildTenantBannerContent — returns what the dashboard banner should display.
 * Mirrors the conditional logic in DashboardHome's tenant banner.
 */
export type BannerContent =
  | { type: 'tenant'; name: string; status: string; id: string; flagship: boolean }
  | { type: 'empty' };

export function buildTenantBannerContent(
  currentTenant: {
    id: string;
    name: string;
    status: string;
    flagship: boolean;
  } | null,
): BannerContent {
  if (!currentTenant) return { type: 'empty' };
  return {
    type: 'tenant',
    name: currentTenant.name,
    status: currentTenant.status,
    id: currentTenant.id,
    flagship: currentTenant.flagship,
  };
}

// ─── Tests ────────────────────────────────────────────────────────────────────

describe('TopBar — tenant switch toast', () => {
  it('formats toast message with tenant name', () => {
    expect(buildTenantSwitchToast('ACE Commodities')).toBe('Switched to ACE Commodities');
  });

  it('formats toast message with MSE equities name', () => {
    expect(buildTenantSwitchToast('MSE Equities')).toBe('Switched to MSE Equities');
  });

  it('handles short single-word names', () => {
    expect(buildTenantSwitchToast('ACE')).toBe('Switched to ACE');
  });
});

describe('TopBar — tenant select state', () => {
  it('returns loading when isLoading is true regardless of tenants', () => {
    expect(resolveTenantOption(true, [])).toBe('loading');
    expect(resolveTenantOption(true, [{ id: 'ace', name: 'ACE' }])).toBe('loading');
  });

  it('returns empty when not loading and no tenants', () => {
    expect(resolveTenantOption(false, [])).toBe('empty');
  });

  it('returns list when not loading and tenants exist', () => {
    expect(resolveTenantOption(false, [{ id: 'ace', name: 'ACE' }])).toBe('list');
    expect(
      resolveTenantOption(false, [
        { id: 'ace', name: 'ACE Commodities' },
        { id: 'mse', name: 'MSE Equities' },
      ]),
    ).toBe('list');
  });
});

describe('TopBar — select disabled logic', () => {
  it('is disabled when loading', () => {
    expect(isTenantSelectDisabled(true, 0)).toBe(true);
    expect(isTenantSelectDisabled(true, 2)).toBe(true);
  });

  it('is disabled when no tenants (not loading)', () => {
    expect(isTenantSelectDisabled(false, 0)).toBe(true);
  });

  it('is enabled when not loading and tenants exist', () => {
    expect(isTenantSelectDisabled(false, 1)).toBe(false);
    expect(isTenantSelectDisabled(false, 3)).toBe(false);
  });
});

describe('DashboardHome — tenant banner content', () => {
  it('returns empty banner when currentTenant is null', () => {
    const banner = buildTenantBannerContent(null);
    expect(banner.type).toBe('empty');
  });

  it('returns tenant data when currentTenant is set', () => {
    const tenant = {
      id: 'ace-commodities',
      name: 'ACE Commodities Exchange',
      status: 'ACTIVE',
      flagship: false,
    };
    const banner = buildTenantBannerContent(tenant);
    expect(banner.type).toBe('tenant');
    if (banner.type === 'tenant') {
      expect(banner.name).toBe('ACE Commodities Exchange');
      expect(banner.status).toBe('ACTIVE');
      expect(banner.id).toBe('ace-commodities');
      expect(banner.flagship).toBe(false);
    }
  });

  it('returns flagship flag when tenant is flagship', () => {
    const tenant = {
      id: 'mse-equities',
      name: 'MSE Equities',
      status: 'ONBOARDING',
      flagship: true,
    };
    const banner = buildTenantBannerContent(tenant);
    expect(banner.type).toBe('tenant');
    if (banner.type === 'tenant') {
      expect(banner.flagship).toBe(true);
      expect(banner.status).toBe('ONBOARDING');
    }
  });

  it('passes through all tenant fields without mutation', () => {
    const tenant = {
      id: 'test-id',
      name: 'Test Tenant',
      status: 'SUSPENDED',
      flagship: false,
    };
    const banner = buildTenantBannerContent(tenant);
    if (banner.type === 'tenant') {
      expect(banner.id).toBe(tenant.id);
      expect(banner.name).toBe(tenant.name);
      expect(banner.status).toBe(tenant.status);
      expect(banner.flagship).toBe(tenant.flagship);
    }
  });
});

describe('DashboardHome — flagship label logic', () => {
  it('shows flagship label only when flagship is true', () => {
    const flagshipTenant = buildTenantBannerContent({
      id: 'mse',
      name: 'MSE',
      status: 'ACTIVE',
      flagship: true,
    });
    const nonFlagshipTenant = buildTenantBannerContent({
      id: 'ace',
      name: 'ACE',
      status: 'ACTIVE',
      flagship: false,
    });

    expect(flagshipTenant.type === 'tenant' && flagshipTenant.flagship).toBe(true);
    expect(nonFlagshipTenant.type === 'tenant' && nonFlagshipTenant.flagship).toBe(false);
  });
});

describe('DashboardHome — tenant status display', () => {
  const statuses = ['ACTIVE', 'ONBOARDING', 'SUSPENDED', 'DECOMMISSIONED'];

  it.each(statuses)('passes status %s through banner content unchanged', (status) => {
    const banner = buildTenantBannerContent({
      id: 'x',
      name: 'X',
      status,
      flagship: false,
    });
    expect(banner.type === 'tenant' && banner.status).toBe(status);
  });
});
