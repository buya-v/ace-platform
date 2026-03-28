import { describe, it, expect } from 'vitest';
import { getReadinessPercentage } from '../components/ReadinessChecklist';

describe('getReadinessPercentage', () => {
  it('returns 0 for empty items', () => {
    expect(getReadinessPercentage([], {})).toBe(0);
  });

  it('returns 0 when nothing checked', () => {
    const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];
    expect(getReadinessPercentage(items, {})).toBe(0);
  });

  it('returns 100 when all checked', () => {
    const items = [{ id: 'a' }, { id: 'b' }];
    expect(getReadinessPercentage(items, { a: true, b: true })).toBe(100);
  });

  it('returns correct percentage for partial', () => {
    const items = [{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }];
    expect(getReadinessPercentage(items, { a: true })).toBe(25);
  });

  it('ignores unchecked items in checkedItems', () => {
    const items = [{ id: 'a' }, { id: 'b' }];
    expect(getReadinessPercentage(items, { a: true, b: false })).toBe(50);
  });
});
