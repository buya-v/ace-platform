import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchParticipants, approveParticipant, rejectParticipant } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { Participant } from '../types';
import styles from './Participants.module.css';

export function ParticipantsPage() {
  const [statusFilter, setStatusFilter] = useState('');
  const [search, setSearch] = useState('');
  const [actionTarget, setActionTarget] = useState<{ participant: Participant; action: 'approve' | 'reject' } | null>(null);
  const [rejectReason, setRejectReason] = useState('');

  const { data, refresh } = usePolling(
    (signal) => fetchParticipants({ status: statusFilter || undefined }, signal),
    30000,
  );

  const participants = data?.data ?? [];
  const filtered = participants.filter(p =>
    !search || p.name.toLowerCase().includes(search.toLowerCase()) || p.email.toLowerCase().includes(search.toLowerCase()),
  );

  const handleAction = async () => {
    if (!actionTarget) return;
    if (actionTarget.action === 'approve') {
      await approveParticipant(actionTarget.participant.id);
    } else {
      await rejectParticipant(actionTarget.participant.id, rejectReason);
    }
    setActionTarget(null);
    setRejectReason('');
    refresh();
  };

  return (
    <div>
      <h1>Participant Management</h1>

      <div className={styles.filters}>
        <input
          type="text"
          placeholder="Search by name or email..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className={styles.searchInput}
        />
        <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className={styles.select}>
          <option value="">All Status</option>
          <option value="PENDING">Pending</option>
          <option value="APPROVED">Approved</option>
          <option value="REJECTED">Rejected</option>
          <option value="UNDER_REVIEW">Under Review</option>
        </select>
      </div>

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Name</th>
            <th>Email</th>
            <th>Organization</th>
            <th>KYC Status</th>
            <th>Risk Score</th>
            <th>Submitted</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {filtered.map(p => (
            <tr key={p.id}>
              <td>{p.name}</td>
              <td>{p.email}</td>
              <td>{p.organization}</td>
              <td><StatusBadge status={p.kyc_status} /></td>
              <td>{p.risk_score}</td>
              <td>{new Date(p.submitted_at).toLocaleDateString()}</td>
              <td>
                {(p.kyc_status === 'PENDING' || p.kyc_status === 'UNDER_REVIEW') && (
                  <div className={styles.actionBtns}>
                    <button className={styles.approveBtn} onClick={() => setActionTarget({ participant: p, action: 'approve' })}>
                      Approve
                    </button>
                    <button className={styles.rejectBtn} onClick={() => setActionTarget({ participant: p, action: 'reject' })}>
                      Reject
                    </button>
                  </div>
                )}
              </td>
            </tr>
          ))}
          {filtered.length === 0 && (
            <tr><td colSpan={7} className={styles.empty}>No participants found</td></tr>
          )}
        </tbody>
      </table>

      {actionTarget && (
        <ConfirmDialog
          title={actionTarget.action === 'approve' ? 'Approve KYC' : 'Reject KYC'}
          message={`Are you sure you want to ${actionTarget.action} the KYC application for ${actionTarget.participant.name}?`}
          confirmLabel={actionTarget.action === 'approve' ? 'Approve' : 'Reject'}
          onConfirm={handleAction}
          onCancel={() => { setActionTarget(null); setRejectReason(''); }}
        />
      )}
    </div>
  );
}

export { filterParticipants };

function filterParticipants(participants: Participant[], search: string): Participant[] {
  if (!search) return participants;
  const lower = search.toLowerCase();
  return participants.filter(p =>
    p.name.toLowerCase().includes(lower) || p.email.toLowerCase().includes(lower),
  );
}
