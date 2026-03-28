import { describe, it, expect } from 'vitest';
import { sortAlertsBySeverity } from '../pages/ComplianceAlerts';
import { ComplianceAlert } from '../types';

function makeAlert(severity: ComplianceAlert['severity'], id: string): ComplianceAlert {
  return {
    id,
    participant_id: 'p1',
    participant_name: 'Test',
    alert_type: 'UNUSUAL_VOLUME',
    severity,
    description: 'test',
    status: 'OPEN',
    created_at: '2026-03-28T00:00:00Z',
  };
}

describe('sortAlertsBySeverity', () => {
  it('sorts CRITICAL first, then HIGH, MEDIUM, LOW', () => {
    const alerts = [
      makeAlert('LOW', '1'),
      makeAlert('CRITICAL', '2'),
      makeAlert('MEDIUM', '3'),
      makeAlert('HIGH', '4'),
    ];

    const sorted = sortAlertsBySeverity(alerts);

    expect(sorted[0].severity).toBe('CRITICAL');
    expect(sorted[1].severity).toBe('HIGH');
    expect(sorted[2].severity).toBe('MEDIUM');
    expect(sorted[3].severity).toBe('LOW');
  });

  it('preserves order for same severity', () => {
    const alerts = [
      makeAlert('HIGH', 'a'),
      makeAlert('HIGH', 'b'),
      makeAlert('HIGH', 'c'),
    ];

    const sorted = sortAlertsBySeverity(alerts);
    expect(sorted.map(a => a.id)).toEqual(['a', 'b', 'c']);
  });

  it('returns empty array for empty input', () => {
    expect(sortAlertsBySeverity([])).toEqual([]);
  });

  it('does not mutate original array', () => {
    const alerts = [
      makeAlert('LOW', '1'),
      makeAlert('CRITICAL', '2'),
    ];
    const original = [...alerts];
    sortAlertsBySeverity(alerts);
    expect(alerts).toEqual(original);
  });
});
