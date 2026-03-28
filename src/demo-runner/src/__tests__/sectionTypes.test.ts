import { describe, it, expect } from 'vitest';
import { isChecklistSection } from '../types/section';

describe('isChecklistSection', () => {
  it('returns true for section with items', () => {
    expect(isChecklistSection({ id: 'x', title: 'X', items: [] })).toBe(true);
  });

  it('returns false for section with steps', () => {
    expect(isChecklistSection({ id: 'x', title: 'X', steps: [] })).toBe(false);
  });
});
