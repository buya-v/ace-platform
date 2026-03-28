import { describe, it, expect } from 'vitest';
import { isAdminRole, hasAdminAccess, hasComplianceAccess } from '../types';

describe('Role helpers', () => {
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

    it('returns false for empty string', () => {
      expect(isAdminRole('')).toBe(false);
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

    it('returns false for non-admin roles', () => {
      expect(hasAdminAccess(['trader', 'viewer'])).toBe(false);
    });
  });

  describe('hasComplianceAccess', () => {
    it('returns true for admin', () => {
      expect(hasComplianceAccess(['admin'])).toBe(true);
    });

    it('returns true for exchange_admin', () => {
      expect(hasComplianceAccess(['exchange_admin'])).toBe(true);
    });

    it('returns true for compliance_officer', () => {
      expect(hasComplianceAccess(['compliance_officer'])).toBe(true);
    });

    it('returns true when both roles present', () => {
      expect(hasComplianceAccess(['admin', 'compliance_officer'])).toBe(true);
    });

    it('returns false for trader only', () => {
      expect(hasComplianceAccess(['trader'])).toBe(false);
    });

    it('returns false for empty roles', () => {
      expect(hasComplianceAccess([])).toBe(false);
    });
  });
});
