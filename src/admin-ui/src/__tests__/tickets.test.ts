import { describe, it, expect } from 'vitest';
import {
  priorityClassName,
  ticketStatusClassName,
  categoryLabel,
  priorityLabel,
  statusLabel,
  formatTicketTime,
  truncateId,
  filterTickets,
  sortTickets,
  computeStatusCounts,
  Ticket,
} from '../pages/Tickets';

function makeTicket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    id: 'ticket-abc12345',
    title: 'Cannot login to platform',
    category: 'bug_report',
    priority: 'high',
    status: 'open',
    reporter_name: 'Alice',
    reporter_email: 'alice@example.com',
    description: 'Login page returns 500 error',
    metadata: {},
    created_at: '2026-03-31T10:00:00Z',
    updated_at: '2026-03-31T10:00:00Z',
    ...overrides,
  };
}

describe('priorityClassName', () => {
  it('returns priorityCritical for critical', () => {
    expect(priorityClassName('critical')).toBe('priorityCritical');
  });
  it('returns priorityHigh for high', () => {
    expect(priorityClassName('high')).toBe('priorityHigh');
  });
  it('returns priorityMedium for medium', () => {
    expect(priorityClassName('medium')).toBe('priorityMedium');
  });
  it('returns priorityLow for low', () => {
    expect(priorityClassName('low')).toBe('priorityLow');
  });
  it('returns empty string for unknown priority', () => {
    expect(priorityClassName('unknown')).toBe('');
  });
});

describe('ticketStatusClassName', () => {
  it('returns statusOpen for open', () => {
    expect(ticketStatusClassName('open')).toBe('statusOpen');
  });
  it('returns statusInProgress for in_progress', () => {
    expect(ticketStatusClassName('in_progress')).toBe('statusInProgress');
  });
  it('returns statusResolved for resolved', () => {
    expect(ticketStatusClassName('resolved')).toBe('statusResolved');
  });
  it('returns statusClosed for closed', () => {
    expect(ticketStatusClassName('closed')).toBe('statusClosed');
  });
  it('returns empty string for unknown status', () => {
    expect(ticketStatusClassName('pending')).toBe('');
  });
});

describe('categoryLabel', () => {
  it('returns Bug Report for bug_report', () => {
    expect(categoryLabel('bug_report')).toBe('Bug Report');
  });
  it('returns Customization for customization', () => {
    expect(categoryLabel('customization')).toBe('Customization');
  });
  it('returns Support for support', () => {
    expect(categoryLabel('support')).toBe('Support');
  });
  it('returns Feature Request for feature_request', () => {
    expect(categoryLabel('feature_request')).toBe('Feature Request');
  });
  it('replaces underscores for unknown categories', () => {
    expect(categoryLabel('some_other_type')).toBe('some other type');
  });
});

describe('priorityLabel', () => {
  it('returns capitalized labels', () => {
    expect(priorityLabel('critical')).toBe('Critical');
    expect(priorityLabel('high')).toBe('High');
    expect(priorityLabel('medium')).toBe('Medium');
    expect(priorityLabel('low')).toBe('Low');
  });
  it('returns raw value for unknown priority', () => {
    expect(priorityLabel('urgent')).toBe('urgent');
  });
});

describe('statusLabel', () => {
  it('returns human-readable labels', () => {
    expect(statusLabel('open')).toBe('Open');
    expect(statusLabel('in_progress')).toBe('In Progress');
    expect(statusLabel('resolved')).toBe('Resolved');
    expect(statusLabel('closed')).toBe('Closed');
  });
  it('replaces underscores for unknown status', () => {
    expect(statusLabel('on_hold')).toBe('on hold');
  });
});

describe('formatTicketTime', () => {
  it('formats ISO string to locale string', () => {
    const result = formatTicketTime('2026-03-31T10:00:00Z');
    expect(result).toBeTruthy();
    expect(typeof result).toBe('string');
  });
  it('returns empty string for empty input', () => {
    expect(formatTicketTime('')).toBe('');
  });
  it('returns original string for invalid date', () => {
    const result = formatTicketTime('not-a-date');
    expect(typeof result).toBe('string');
  });
});

describe('truncateId', () => {
  it('truncates long IDs with ellipsis', () => {
    expect(truncateId('ticket-abc12345')).toBe('ticket-a...');
  });
  it('returns short IDs unchanged', () => {
    expect(truncateId('abc')).toBe('abc');
  });
  it('returns exactly maxLen IDs unchanged', () => {
    expect(truncateId('abcdefgh')).toBe('abcdefgh');
  });
  it('returns empty string for empty input', () => {
    expect(truncateId('')).toBe('');
  });
  it('respects custom maxLen', () => {
    expect(truncateId('abcdefghij', 5)).toBe('abcde...');
  });
});

describe('filterTickets', () => {
  const tickets = [
    makeTicket({ id: '1', category: 'bug_report', priority: 'critical', status: 'open', title: 'Login broken', description: 'Cannot access account' }),
    makeTicket({ id: '2', category: 'support', priority: 'low', status: 'closed', title: 'Need help', description: 'General question' }),
    makeTicket({ id: '3', category: 'bug_report', priority: 'high', status: 'resolved', title: 'API error', description: 'Endpoint fails' }),
    makeTicket({ id: '4', category: 'feature_request', priority: 'medium', status: 'in_progress', title: 'Add dark mode', reporter_name: 'Bob', description: 'Theme request' }),
  ];

  it('returns all tickets when no filters', () => {
    expect(filterTickets(tickets, '', '', '', '')).toHaveLength(4);
  });

  it('filters by category', () => {
    const result = filterTickets(tickets, 'bug_report', '', '', '');
    expect(result).toHaveLength(2);
    expect(result.every(t => t.category === 'bug_report')).toBe(true);
  });

  it('filters by priority', () => {
    const result = filterTickets(tickets, '', 'critical', '', '');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('1');
  });

  it('filters by status', () => {
    const result = filterTickets(tickets, '', '', 'closed', '');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('2');
  });

  it('filters by search query matching title', () => {
    const result = filterTickets(tickets, '', '', '', 'login');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('1');
  });

  it('filters by search query matching reporter name', () => {
    const result = filterTickets(tickets, '', '', '', 'bob');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('4');
  });

  it('combines category and status filters', () => {
    const result = filterTickets(tickets, 'bug_report', '', 'resolved', '');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('3');
  });

  it('combines all filters', () => {
    const result = filterTickets(tickets, 'bug_report', 'critical', 'open', 'login');
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe('1');
  });

  it('returns empty when no match', () => {
    expect(filterTickets(tickets, 'customization', '', '', '')).toHaveLength(0);
  });

  it('search is case-insensitive', () => {
    expect(filterTickets(tickets, '', '', '', 'LOGIN')).toHaveLength(1);
  });
});

describe('sortTickets', () => {
  it('sorts by priority (critical first)', () => {
    const tickets = [
      makeTicket({ id: '1', priority: 'low', created_at: '2026-03-31T10:00:00Z' }),
      makeTicket({ id: '2', priority: 'critical', created_at: '2026-03-31T10:00:00Z' }),
      makeTicket({ id: '3', priority: 'medium', created_at: '2026-03-31T10:00:00Z' }),
    ];
    const sorted = sortTickets(tickets);
    expect(sorted[0].priority).toBe('critical');
    expect(sorted[1].priority).toBe('medium');
    expect(sorted[2].priority).toBe('low');
  });

  it('sorts by timestamp within same priority (newest first)', () => {
    const tickets = [
      makeTicket({ id: '1', priority: 'high', created_at: '2026-03-31T08:00:00Z' }),
      makeTicket({ id: '2', priority: 'high', created_at: '2026-03-31T12:00:00Z' }),
      makeTicket({ id: '3', priority: 'high', created_at: '2026-03-31T10:00:00Z' }),
    ];
    const sorted = sortTickets(tickets);
    expect(sorted[0].id).toBe('2');
    expect(sorted[1].id).toBe('3');
    expect(sorted[2].id).toBe('1');
  });

  it('does not mutate the original array', () => {
    const tickets = [
      makeTicket({ id: '1', priority: 'low' }),
      makeTicket({ id: '2', priority: 'critical' }),
    ];
    const original = [...tickets];
    sortTickets(tickets);
    expect(tickets[0].id).toBe(original[0].id);
  });

  it('handles empty array', () => {
    expect(sortTickets([])).toEqual([]);
  });
});

describe('computeStatusCounts', () => {
  it('counts tickets by status', () => {
    const tickets = [
      makeTicket({ status: 'open' }),
      makeTicket({ status: 'open' }),
      makeTicket({ status: 'in_progress' }),
      makeTicket({ status: 'resolved' }),
      makeTicket({ status: 'closed' }),
      makeTicket({ status: 'closed' }),
    ];
    const counts = computeStatusCounts(tickets);
    expect(counts.open).toBe(2);
    expect(counts.in_progress).toBe(1);
    expect(counts.resolved).toBe(1);
    expect(counts.closed).toBe(2);
  });

  it('returns all zeros for empty array', () => {
    const counts = computeStatusCounts([]);
    expect(counts).toEqual({ open: 0, in_progress: 0, resolved: 0, closed: 0 });
  });

  it('handles unknown status gracefully', () => {
    const tickets = [makeTicket({ status: 'unknown' as any })];
    const counts = computeStatusCounts(tickets);
    expect(counts.open).toBe(0);
    expect(counts.in_progress).toBe(0);
  });
});
