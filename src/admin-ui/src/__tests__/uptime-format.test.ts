import { describe, it, expect } from 'vitest';
import { formatUptime } from '../pages/SystemMonitoring';

describe('formatUptime', () => {
  it('formats seconds to hours and minutes', () => {
    expect(formatUptime(3661)).toBe('1h 1m');
  });

  it('formats days and hours', () => {
    expect(formatUptime(86400 + 7200)).toBe('1d 2h');
  });

  it('formats multi-day uptime', () => {
    expect(formatUptime(86400 * 3 + 3600 * 5)).toBe('3d 5h');
  });

  it('formats zero seconds', () => {
    expect(formatUptime(0)).toBe('0h 0m');
  });

  it('formats less than an hour', () => {
    expect(formatUptime(1800)).toBe('0h 30m');
  });

  it('formats exactly one day', () => {
    expect(formatUptime(86400)).toBe('1d 0h');
  });
});
