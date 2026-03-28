import { describe, it, expect } from 'vitest';
import { allSections, getAllSteps, getTotalStepCount } from '../data/sections';
import { isChecklistSection } from '../types/section';

describe('sections data', () => {
  it('has 9 sections', () => {
    expect(allSections).toHaveLength(9);
  });

  it('last section is a checklist', () => {
    const last = allSections[allSections.length - 1];
    expect(isChecklistSection(last)).toBe(true);
  });

  it('first 8 sections have steps', () => {
    for (let i = 0; i < 8; i++) {
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
});
