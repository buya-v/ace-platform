import { describe, it, expect } from 'vitest';
import {
  severityClassName,
  statusClassName,
  formatAlertTime,
  sortSurveillanceAlerts,
  computeSeverityCounts,
  filterAlerts,
  SurveillanceAlert,
} from '../pages/Surveillance';

function makeAlert(overrides: Partial<SurveillanceAlert> = {}): SurveillanceAlert {
  return {
    id: 'alert-1',
    timestamp: '2026-03-31T10:00:00Z',
    participant_id: 'p-1',
    participant_name: 'Trader A',
    instrument_id: 'GOLD-2026',
    rule_type: 'UNUSUAL_VOLUME',
    severity: 'HIGH',
    status: 'OPEN',
    description: 'Unusual trading volume detected',
    ...overrides,
  };
}

describe('severityClassName', () => {
  it('returns severityCritical for CRITICAL', () => {
    expect(severityClassName('CRITICAL')).toBe('severityCritical');
  });
  it('returns severityHigh for HIGH', () => {
    expect(severityClassName('HIGH')).toBe('severityHigh');
  });
  it('returns severityMedium for MEDIUM', () => {
    expect(severityClassName('MEDIUM')).toBe('severityMedium');
  });
  it('returns severityLow for LOW', () => {
    expect(severityClassName('LOW')).toBe('severityLow');
  });
  it('returns empty string for unknown severity', () => {
    expect(severityClassName('UNKNOWN')).toBe('');
  });
});

describe('statusClassName', () => {
  it('returns statusOpen for OPEN', () => {
    expect(statusClassName('OPEN')).toBe('statusOpen');
  });
  it('returns statusUnderReview for UNDER_REVIEW', () => {
    expect(statusClassName('UNDER_REVIEW')).toBe('statusUnderReview');
  });
  it('returns statusResolved for RESOLVED', () => {
    expect(statusClassName('RESOLVED')).toBe('statusResolved');
  });
  it('returns statusDismissed for DISMISSED', () => {
    expect(statusClassName('DISMISSED')).toBe('statusDismissed');
  });
  it('returns empty string for unknown status', () => {
    expect(statusClassName('SOMETHING')).toBe('');
  });
});

describe('formatAlertTime', () => {
  it('formats ISO string to locale string', () => {
    const result = formatAlertTime('2026-03-31T10:00:00Z');
    expect(result).toBeTruthy();
    expect(typeof result).toBe('string');
  });
  it('returns empty string for empty input', () => {
    expect(formatAlertTime('')).toBe('');
  });
  it('returns original string for invalid date', () => {
    const result = formatAlertTime('not-a-date');
    expect(typeof result).toBe('string');
  });
});

describe('sortSurveillanceAlerts', () => {
  it('sorts by severity (CRITICAL first)', () => {
    const alerts = [
      makeAlert({ id: '1', severity: 'LOW', timestamp: '2026-03-31T10:00:00Z' }),
      makeAlert({ id: '2', severity: 'CRITICAL', timestamp: '2026-03-31T10:00:00Z' }),
      makeAlert({ id: '3', severity: 'MEDIUM', timestamp: '2026-03-31T10:00:00Z' }),
    ];
    const sorted = sortSurveillanceAlerts(alerts);
    expect(sorted[0].severity).toBe('CRITICAL');
    expect(sorted[1].severity).toBe('MEDIUM');
    expect(sorted[2].severity).toBe('LOW');
  });

  it('sorts by timestamp within same severity (newest first)', () => {
    const alerts = [
      makeAlert({ id: '1', severity: 'HIGH', timestamp: '2026-03-31T08:00:00Z' }),
      makeAlert({ id: '2', severity: 'HIGH', timestamp: '2026-03-31T12:00:00Z' }),
      makeAlert({ id: '3', severity: 'HIGH', timestamp: '2026-03-31T10:00:00Z' }),
    ];
    const sorted = sortSurveillanceAlerts(alerts);
    expect(sorted[0].id).toBe('2');
    expect(sorted[1].id).toBe('3');
    expect(sorted[2].id).toBe('1');
  });

  it('does not mutate the original array', () => {
    const alerts = [
      makeAlert({ id: '1', severity: 'LOW' }),
      makeAlert({ id: '2', severity: 'CRITICAL' }),
    ];
    const original = [...alerts];
    sortSurveillanceAlerts(alerts);
    expect(alerts[0].id).toBe(original[0].id);
  });

  it('handles empty array', () => {
    expect(sortSurveillanceAlerts([])).toEqual([]);
  });
});

describe('computeSeverityCounts', () => {
  it('counts alerts by severity', () => {
    const alerts = [
      makeAlert({ severity: 'CRITICAL' }),
      makeAlert({ severity: 'CRITICAL' }),
      makeAlert({ severity: 'HIGH' }),
      makeAlert({ severity: 'LOW' }),
    ];
    const counts = computeSeverityCounts(alerts);
    expect(counts.CRITICAL).toBe(2);
    expect(counts.HIGH).toBe(1);
    expect(counts.MEDIUM).toBe(0);
    expect(counts.LOW).toBe(1);
  });

  it('returns all zeros for empty array', () => {
    const counts = computeSeverityCounts([]);
    expect(counts).toEqual({ CRITICAL: 0, HIGH: 0, MEDIUM: 0, LOW: 0 });
  });

  it('handles unknown severity gracefully', () => {
    const alerts = [makeAlert({ severity: 'UNKNOWN' as any })];
    const counts = computeSeverityCounts(alerts);
    expect(counts.CRITICAL).toBe(0);
    expect(counts.HIGH).toBe(0);
  });
});

describe('filterAlerts', () => {
  const alerts = [
    makeAlert({ id: '1', severity: 'CRITICAL', status: 'OPEN' }),
    makeAlert({ id: '2', severity: 'HIGH', status: 'RESOLVED' }),
    makeAlert({ id: '3', severity: 'CRITICAL', status: 'RESOLVED' }),
    makeAlert({ id: '4', severity: 'LOW', status: 'OPEN' }),
  ];

  it('returns all alerts when no filters', () => {
    expect(filterAlerts(alerts, '', '')).toHaveLength(4);
  });

  it('filters by severity', () => {
    const result = filterAlerts(alerts, 'CRITICAL', '');
    expect(result).toHaveLength(2);
    expect(result.every(a => a.severity === 'CRITICAL')).toBe(true);
  });

  it('filters by status', () => {
    const result = filterAlerts(alerts, '', 'RESOLVED');
    expect(result).toHaveLength(2);
    expect(result.every(a => a.status === 'RESOLVED')).toBe(true);
  });

  it('filters by both severity and status', () => {
    const result = filterAlerts(alerts, 'CRITICAL', 'RESOLVED');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('3');
  });

  it('returns empty array when no match', () => {
    expect(filterAlerts(alerts, 'MEDIUM', 'DISMISSED')).toHaveLength(0);
  });
});
