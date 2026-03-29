import { describe, it, expect } from 'vitest';
import { allSections, getAllSteps, getTotalStepCount } from '../data/sections';
import { isChecklistSection } from '../types/section';

describe('sections data', () => {
  it('has 14 sections', () => {
    expect(allSections).toHaveLength(14);
  });

  it('last section is a checklist', () => {
    const last = allSections[allSections.length - 1];
    expect(isChecklistSection(last)).toBe(true);
  });

  it('first 13 sections have steps', () => {
    for (let i = 0; i < 13; i++) {
      expect(isChecklistSection(allSections[i])).toBe(false);
    }
  });

  it('getAllSteps returns all step definitions', () => {
    const steps = getAllSteps();
    expect(steps.length).toBeGreaterThan(0);
    expect(steps.every((s) => s.id && s.title && s.method)).toBe(true);
  });

  it('getTotalStepCount matches getAllSteps length', () => {
    expect(getTotalStepCount()).toBe(getAllSteps().length);
  });

  it('all step IDs are unique', () => {
    const ids = getAllSteps().map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('section IDs are unique', () => {
    const ids = allSections.map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('environment setup has 4 steps', () => {
    const env = allSections.find((s) => s.id === 'env-setup');
    expect(env && 'steps' in env && env.steps).toHaveLength(4);
  });

  it('registration has 6 steps', () => {
    const reg = allSections.find((s) => s.id === 'registration');
    expect(reg && 'steps' in reg && reg.steps).toHaveLength(6);
  });

  it('trading has 5 steps', () => {
    const trading = allSections.find((s) => s.id === 'trading');
    expect(trading && 'steps' in trading && trading.steps).toHaveLength(5);
  });

  it('readiness checklist has items with valid statuses', () => {
    const readiness = allSections.find((s) => s.id === 'readiness');
    expect(readiness && isChecklistSection(readiness)).toBe(true);
    if (readiness && isChecklistSection(readiness)) {
      expect(readiness.items.length).toBeGreaterThan(0);
      readiness.items.forEach((item) => {
        expect(['Ready', 'Not Ready', 'Partial']).toContain(item.status);
      });
    }
  });

  it('login steps have extractState for token storage', () => {
    const reg = allSections.find((s) => s.id === 'registration');
    if (reg && 'steps' in reg) {
      const loginSteps = reg.steps.filter((s) => s.title.startsWith('Login'));
      expect(loginSteps).toHaveLength(3);
      loginSteps.forEach((s) => {
        expect(s.extractState).toBeDefined();
      });
    }
  });

  it('new admin sections have expected step counts', () => {
    const expected: Record<string, number> = {
      'admin-orderbook': 4,
      'admin-positions': 4,
      'admin-settlement': 3,
      'admin-circuit-breakers': 4,
      'admin-monitoring': 4,
    };
    for (const [id, count] of Object.entries(expected)) {
      const section = allSections.find((s) => s.id === id);
      expect(section).toBeDefined();
      expect(section && 'steps' in section && section.steps).toHaveLength(count);
    }
  });

  it('admin sections use auth headers where required', () => {
    const adminSectionIds = [
      'admin-positions',
      'admin-settlement',
      'admin-circuit-breakers',
      'admin-monitoring',
    ];
    const state = { admin_token: 'test-jwt-token' };

    for (const id of adminSectionIds) {
      const section = allSections.find((s) => s.id === id);
      expect(section).toBeDefined();
      if (section && 'steps' in section) {
        section.steps.forEach((step) => {
          expect(step.headers).toBeDefined();
          const headers = step.headers!(state);
          expect(headers).toHaveProperty('Authorization', 'Bearer test-jwt-token');
        });
      }
    }
  });

  it('admin-orderbook steps are public (no auth headers)', () => {
    const section = allSections.find((s) => s.id === 'admin-orderbook');
    expect(section).toBeDefined();
    if (section && 'steps' in section) {
      section.steps.forEach((step) => {
        expect(step.headers).toBeUndefined();
      });
    }
  });

  it('settlement trigger step has extractState', () => {
    const section = allSections.find((s) => s.id === 'admin-settlement');
    expect(section).toBeDefined();
    if (section && 'steps' in section) {
      const trigger = section.steps.find((s) => s.id === 'stl-2');
      expect(trigger).toBeDefined();
      expect(trigger!.extractState).toBeDefined();
      const result = trigger!.extractState!({ cycle_id: 'CYC-123' }, {});
      expect(result).toEqual({ settlement_cycle_id: 'CYC-123' });
    }
  });

  it('circuit breaker PUT step has body', () => {
    const section = allSections.find((s) => s.id === 'admin-circuit-breakers');
    expect(section).toBeDefined();
    if (section && 'steps' in section) {
      const cb = section.steps.find((s) => s.id === 'cb-2');
      expect(cb).toBeDefined();
      expect(cb!.body).toBeDefined();
      const body = cb!.body!({});
      expect(body).toEqual({
        upper_limit_pct: 10,
        lower_limit_pct: 10,
        cooldown_minutes: 5,
        reference_price: '325.50',
      });
    }
  });
});
