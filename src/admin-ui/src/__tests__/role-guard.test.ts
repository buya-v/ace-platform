import { describe, it, expect } from 'vitest';
import { isAdminRole, hasAdminAccess, hasComplianceAccess } from '../types';

describe('role access functions', () => {
  describe('isAdminRole', () => {
    it('returns true for admin', () => {
      expect(isAdminRole('admin')).toBe(true);
    });

    it('returns true for exchange_admin', () => {
      expect(isAdminRole('exchange_admin')).toBe(true);
    });

    it('returns false for compliance_officer', () => {
      expect(isAdminRole('compliance_officer')).toBe(false);
    });

    it('returns false for trader', () => {
      expect(isAdminRole('trader')).toBe(false);
    });
  });

  describe('hasAdminAccess', () => {
    it('returns true when roles include admin', () => {
      expect(hasAdminAccess(['admin', 'trader'])).toBe(true);
    });

    it('returns true when roles include exchange_admin', () => {
      expect(hasAdminAccess(['exchange_admin'])).toBe(true);
    });

    it('returns false for compliance_officer only', () => {
      expect(hasAdminAccess(['compliance_officer'])).toBe(false);
    });

    it('returns false for empty roles', () => {
      expect(hasAdminAccess([])).toBe(false);
    });
  });

  describe('hasComplianceAccess', () => {
    it('returns true for admin', () => {
      expect(hasComplianceAccess(['admin'])).toBe(true);
    });

    it('returns true for compliance_officer', () => {
      expect(hasComplianceAccess(['compliance_officer'])).toBe(true);
    });

    it('returns true for exchange_admin', () => {
      expect(hasComplianceAccess(['exchange_admin'])).toBe(true);
    });

    it('returns false for trader only', () => {
      expect(hasComplianceAccess(['trader'])).toBe(false);
    });

    it('returns false for empty roles', () => {
      expect(hasComplianceAccess([])).toBe(false);
    });

    it('returns true when one role matches among many', () => {
      expect(hasComplianceAccess(['trader', 'compliance_officer', 'viewer'])).toBe(true);
    });
  });
});

describe('role-based navigation filtering', () => {
  const ADMIN_ROLES = ['admin', 'exchange_admin'];

  function canAccessRoute(userRoles: string[], allowedRoles: string[]): boolean {
    return userRoles.some(role => allowedRoles.includes(role));
  }

  it('admin can access admin-only routes', () => {
    expect(canAccessRoute(['admin'], ADMIN_ROLES)).toBe(true);
  });

  it('exchange_admin can access admin-only routes', () => {
    expect(canAccessRoute(['exchange_admin'], ADMIN_ROLES)).toBe(true);
  });

  it('compliance_officer cannot access admin-only routes', () => {
    expect(canAccessRoute(['compliance_officer'], ADMIN_ROLES)).toBe(false);
  });

  it('compliance_officer can access shared routes', () => {
    const sharedRoles = ['admin', 'exchange_admin', 'compliance_officer'];
    expect(canAccessRoute(['compliance_officer'], sharedRoles)).toBe(true);
  });
});
