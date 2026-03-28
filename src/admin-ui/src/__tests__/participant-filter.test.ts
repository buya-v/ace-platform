import { describe, it, expect } from 'vitest';
import { filterParticipants } from '../pages/Participants';
import { Participant } from '../types';

const participants: Participant[] = [
  {
    id: '1', name: 'Alice Johnson', email: 'alice@farm.co',
    organization: 'Farm Co', kyc_status: 'PENDING', risk_score: 25,
    submitted_at: '2026-03-01', updated_at: '2026-03-01',
  },
  {
    id: '2', name: 'Bob Smith', email: 'bob@grain.ltd',
    organization: 'Grain Ltd', kyc_status: 'APPROVED', risk_score: 10,
    submitted_at: '2026-03-02', updated_at: '2026-03-02',
  },
  {
    id: '3', name: 'Charlie Brown', email: 'charlie@wheat.org',
    organization: 'Wheat Org', kyc_status: 'REJECTED', risk_score: 80,
    submitted_at: '2026-03-03', updated_at: '2026-03-03',
  },
];

describe('filterParticipants', () => {
  it('returns all participants when search is empty', () => {
    expect(filterParticipants(participants, '')).toEqual(participants);
  });

  it('filters by name (case-insensitive)', () => {
    const result = filterParticipants(participants, 'alice');
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe('Alice Johnson');
  });

  it('filters by email', () => {
    const result = filterParticipants(participants, 'grain');
    expect(result).toHaveLength(1);
    expect(result[0].email).toBe('bob@grain.ltd');
  });

  it('matches partial strings across name and email', () => {
    const result = filterParticipants(participants, 'ar');
    // Alice (farm.co has 'ar'), Charlie (charlie has 'ar')
    expect(result).toHaveLength(2);
    expect(result.map(r => r.name)).toContain('Alice Johnson');
    expect(result.map(r => r.name)).toContain('Charlie Brown');
  });

  it('returns empty for no matches', () => {
    expect(filterParticipants(participants, 'xyz')).toEqual([]);
  });

  it('handles empty participant list', () => {
    expect(filterParticipants([], 'test')).toEqual([]);
  });
});
