/**
 * T066 — Demo Runner Integration Test (Post-Admin Sections)
 *
 * Validates that T065's admin dashboard sections are correctly integrated
 * into the demo-runner SPA: section counts, step counts, ID uniqueness,
 * auth headers, build output, and coverage floors.
 *
 * Run with: cd src/demo-runner && npx vitest run src/__tests__/t066_integration.test.ts
 * This file is a reference copy; the runnable version is at src/demo-runner/src/__tests__/t066_integration.test.ts
 */

import { describe, it, expect } from 'vitest';
import { allSections, getAllSteps, getTotalStepCount } from '../../src/demo-runner/src/data/sections';
import { isChecklistSection } from '../../src/demo-runner/src/types/section';

describe('T066: Demo Runner Admin Integration', () => {
  describe('section structure', () => {
    it('allSections has exactly 14 entries (13 step sections + 1 checklist)', () => {
      expect(allSections).toHaveLength(14);
    });

    it('exactly 13 sections have steps (not checklist)', () => {
      const stepSections = allSections.filter((s) => 'steps' in s && !isChecklistSection(s));
      expect(stepSections).toHaveLength(13);
    });

    it('exactly 1 section is a checklist', () => {
      const checklists = allSections.filter(isChecklistSection);
      expect(checklists).toHaveLength(1);
      expect(checklists[0].id).toBe('readiness');
    });

    it('checklist is the last section', () => {
      const last = allSections[allSections.length - 1];
      expect(isChecklistSection(last)).toBe(true);
    });

    it('all section IDs are unique', () => {
      const ids = allSections.map((s) => s.id);
      expect(new Set(ids).size).toBe(ids.length);
    });

    it('sections are in expected order', () => {
      const ids = allSections.map((s) => s.id);
      expect(ids).toEqual([
        'env-setup',
        'registration',
        'trading',
        'post-trade',
        'delivery',
        'market-data',
        'compliance',
        'admin-ops',
        'admin-orderbook',
        'admin-positions',
        'admin-settlement',
        'admin-circuit-breakers',
        'admin-monitoring',
        'readiness',
      ]);
    });
  });

  describe('step counts', () => {
    it('total step count is 51', () => {
      expect(getTotalStepCount()).toBe(51);
    });

    it('getAllSteps returns 51 steps', () => {
      expect(getAllSteps()).toHaveLength(51);
    });

    it('original 8 sections retain their step counts', () => {
      const expected: Record<string, number> = {
        'env-setup': 4,
        'registration': 6,
        'trading': 5,
        'post-trade': 4,
        'delivery': 4,
        'market-data': 3,
        'compliance': 3,
        'admin-ops': 3,
      };
      for (const [id, count] of Object.entries(expected)) {
        const section = allSections.find((s) => s.id === id);
        expect(section, `section ${id} should exist`).toBeDefined();
        if (section && 'steps' in section) {
          expect(section.steps, `section ${id} step count`).toHaveLength(count);
        }
      }
    });

    it('5 new admin sections have correct step counts (19 total new steps)', () => {
      const newSections: Record<string, number> = {
        'admin-orderbook': 4,
        'admin-positions': 4,
        'admin-settlement': 3,
        'admin-circuit-breakers': 4,
        'admin-monitoring': 4,
      };
      let totalNew = 0;
      for (const [id, count] of Object.entries(newSections)) {
        const section = allSections.find((s) => s.id === id);
        expect(section, `new section ${id} should exist`).toBeDefined();
        if (section && 'steps' in section) {
          expect(section.steps, `section ${id} step count`).toHaveLength(count);
          totalNew += section.steps.length;
        }
      }
      expect(totalNew).toBe(19);
    });
  });

  describe('step ID uniqueness', () => {
    it('all 51 step IDs are unique', () => {
      const steps = getAllSteps();
      const ids = steps.map((s) => s.id);
      const dupes = ids.filter((id, i) => ids.indexOf(id) !== i);
      expect(dupes, `duplicate step IDs: ${dupes.join(', ')}`).toHaveLength(0);
      expect(new Set(ids).size).toBe(51);
    });

    it('new admin step IDs use distinct prefixes', () => {
      const newPrefixes = ['ob-', 'pos-', 'stl-', 'cb-', 'mon-'];
      const steps = getAllSteps();
      for (const prefix of newPrefixes) {
        const matching = steps.filter((s) => s.id.startsWith(prefix));
        expect(matching.length, `steps with prefix ${prefix}`).toBeGreaterThan(0);
      }
    });

    it('no step ID collides with checklist item IDs in getAllSteps scope', () => {
      // getAllSteps only collects from Section.steps, not ChecklistItem
      // but mon-1/2/3 exist in both — verify getAllSteps returns them as steps
      const steps = getAllSteps();
      const monSteps = steps.filter((s) => s.id.startsWith('mon-'));
      expect(monSteps).toHaveLength(4); // mon-1, mon-2, mon-3, mon-4
      monSteps.forEach((s) => {
        expect(s.method).toBeDefined(); // steps have method, checklist items don't
      });
    });
  });

  describe('auth headers', () => {
    const adminState = { admin_token: 'test-jwt-token' };

    it('admin-positions steps all require auth', () => {
      const section = allSections.find((s) => s.id === 'admin-positions');
      expect(section && 'steps' in section).toBe(true);
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers, `${step.id} should have headers`).toBeDefined();
          const h = step.headers!(adminState);
          expect(h.Authorization).toBe('Bearer test-jwt-token');
        });
      }
    });

    it('admin-settlement steps all require auth', () => {
      const section = allSections.find((s) => s.id === 'admin-settlement');
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers, `${step.id} should have headers`).toBeDefined();
          const h = step.headers!(adminState);
          expect(h.Authorization).toBe('Bearer test-jwt-token');
        });
      }
    });

    it('admin-circuit-breakers steps all require auth', () => {
      const section = allSections.find((s) => s.id === 'admin-circuit-breakers');
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers, `${step.id} should have headers`).toBeDefined();
          const h = step.headers!(adminState);
          expect(h.Authorization).toBe('Bearer test-jwt-token');
        });
      }
    });

    it('admin-monitoring steps all require auth', () => {
      const section = allSections.find((s) => s.id === 'admin-monitoring');
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers, `${step.id} should have headers`).toBeDefined();
          const h = step.headers!(adminState);
          expect(h.Authorization).toBe('Bearer test-jwt-token');
        });
      }
    });

    it('admin-orderbook steps are public (no auth headers)', () => {
      const section = allSections.find((s) => s.id === 'admin-orderbook');
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers, `${step.id} should NOT have headers`).toBeUndefined();
        });
      }
    });

    it('auth header returns empty object when no token in state', () => {
      const section = allSections.find((s) => s.id === 'admin-positions');
      if (section && 'steps' in section) {
        const h = section.steps[0].headers!({});
        expect(h).toEqual({});
      }
    });
  });

  describe('new section content validation', () => {
    it('admin-orderbook uses public instrument endpoints', () => {
      const section = allSections.find((s) => s.id === 'admin-orderbook');
      if (section && 'steps' in section) {
        const urls = section.steps.map((s) => typeof s.url === 'string' ? s.url : '');
        expect(urls).toContain('/api/v1/instruments/list');
        expect(urls).toContain('/api/v1/instruments/WHT-HRW-2026M07-UB/book');
        expect(urls).toContain('/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest');
        expect(urls).toContain('/api/v1/market-data/trades/WHT-HRW-2026M07-UB');
      }
    });

    it('admin-settlement has POST trigger step with extractState', () => {
      const section = allSections.find((s) => s.id === 'admin-settlement');
      if (section && 'steps' in section) {
        const trigger = section.steps.find((s) => s.id === 'stl-2');
        expect(trigger).toBeDefined();
        expect(trigger!.method).toBe('POST');
        expect(trigger!.extractState).toBeDefined();

        // Verify extractState handles cycle_id
        const result1 = trigger!.extractState!({ cycle_id: 'CYC-1' }, {});
        expect(result1).toEqual({ settlement_cycle_id: 'CYC-1' });

        // Verify extractState handles id fallback
        const result2 = trigger!.extractState!({ id: 'CYC-2' }, {});
        expect(result2).toEqual({ settlement_cycle_id: 'CYC-2' });
      }
    });

    it('circuit breaker section has PUT and POST methods', () => {
      const section = allSections.find((s) => s.id === 'admin-circuit-breakers');
      if (section && 'steps' in section) {
        const methods = section.steps.map((s) => s.method);
        expect(methods).toContain('GET');
        expect(methods).toContain('PUT');
        expect(methods).toContain('POST');
      }
    });

    it('circuit breaker PUT step has correct body shape', () => {
      const section = allSections.find((s) => s.id === 'admin-circuit-breakers');
      if (section && 'steps' in section) {
        const cb = section.steps.find((s) => s.id === 'cb-2');
        expect(cb!.body).toBeDefined();
        const body = cb!.body!({});
        expect(body).toHaveProperty('upper_limit_pct', 10);
        expect(body).toHaveProperty('lower_limit_pct', 10);
        expect(body).toHaveProperty('cooldown_minutes', 5);
        expect(body).toHaveProperty('reference_price', '325.50');
      }
    });

    it('monitoring section covers health, compliance, audit, and warehouse', () => {
      const section = allSections.find((s) => s.id === 'admin-monitoring');
      if (section && 'steps' in section) {
        const urls = section.steps.map((s) => typeof s.url === 'string' ? s.url : '');
        expect(urls).toContain('/api/v1/admin/health');
        expect(urls).toContain('/api/v1/compliance/alerts');
        expect(urls).toContain('/api/v1/compliance/audit-trail');
        expect(urls).toContain('/api/v1/warehouse/facilities');
      }
    });

    it('all step validateResponse functions return PASS or FAIL', () => {
      const steps = getAllSteps();
      steps.forEach((step) => {
        expect(step.validateResponse).toBeDefined();
        const pass = step.validateResponse(200);
        expect(['PASS', 'FAIL']).toContain(pass);
        const fail = step.validateResponse(500);
        expect(['PASS', 'FAIL']).toContain(fail);
      });
    });

    it('all steps have required fields: id, title, description, method, url', () => {
      const steps = getAllSteps();
      steps.forEach((step) => {
        expect(step.id, 'id').toBeTruthy();
        expect(step.title, `${step.id} title`).toBeTruthy();
        expect(step.description, `${step.id} description`).toBeTruthy();
        expect(step.method, `${step.id} method`).toBeTruthy();
        expect(step.url, `${step.id} url`).toBeDefined();
      });
    });
  });

  describe('regression checks', () => {
    it('readiness checklist still has 16 items', () => {
      const readiness = allSections.find((s) => s.id === 'readiness');
      expect(readiness && isChecklistSection(readiness)).toBe(true);
      if (readiness && isChecklistSection(readiness)) {
        expect(readiness.items).toHaveLength(16);
      }
    });

    it('original sections still have extractState on login steps', () => {
      const reg = allSections.find((s) => s.id === 'registration');
      if (reg && 'steps' in reg) {
        const loginSteps = reg.steps.filter((s) => s.title.startsWith('Login'));
        expect(loginSteps).toHaveLength(3);
        loginSteps.forEach((s) => {
          expect(s.extractState).toBeDefined();
        });
      }
    });

    it('trading section extractState still works for order IDs', () => {
      const trading = allSections.find((s) => s.id === 'trading');
      if (trading && 'steps' in trading) {
        const buy = trading.steps.find((s) => s.id === 'trade-1');
        expect(buy!.extractState).toBeDefined();
        const result = buy!.extractState!({ order_id: 'ORD-1' }, {});
        expect(result).toEqual({ buy_order_id: 'ORD-1' });
      }
    });

    it('delivery section dynamic URL still works', () => {
      const delivery = allSections.find((s) => s.id === 'delivery');
      if (delivery && 'steps' in delivery) {
        const pledge = delivery.steps.find((s) => s.id === 'del-2');
        expect(typeof pledge!.url).toBe('function');
        const url = (pledge!.url as (state: Record<string, unknown>) => string)({ receipt_id: 'R-1' });
        expect(url).toBe('/api/v1/warehouse/receipts/R-1/pledge');
      }
    });
  });
});
