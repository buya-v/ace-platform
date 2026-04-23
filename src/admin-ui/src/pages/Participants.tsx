import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchParticipants, approveParticipant, rejectParticipant } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { Participant } from '../types';
import { useToast } from '../contexts/ToastContext';
import styles from './Participants.module.css';

export function ParticipantsPage() {
  const [statusFilter, setStatusFilter] = useState('');
  const [search, setSearch] = useState('');
  const [actionTarget, setActionTarget] = useState<{ participant: Participant; action: 'approve' | 'reject' } | null>(null);
  const [rejectReason, setRejectReason] = useState('');
  const { showToast } = useToast();

  const { data, refresh, isLoading } = usePolling(
    (signal) => fetchParticipants({ status: statusFilter || undefined }, signal),
    30000,
  );

  const participants = data?.data ?? [];
  const filtered = participants.filter(p =>
    !search || p.name.toLowerCase().includes(search.toLowerCase()) || p.email.toLowerCase().includes(search.toLowerCase()),
  );

  const handleAction = async () => {
    if (!actionTarget) return;
    try {
      if (actionTarget.action === 'approve') {
        await approveParticipant(actionTarget.participant.id);
        showToast('Participant approved', 'success');
      } else {
        await rejectParticipant(actionTarget.participant.id, rejectReason);
        showToast('Participant rejected', 'success');
      }
      setActionTarget(null);
      setRejectReason('');
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Action failed', 'error');
    }
  };

  const columns: Column<Participant>[] = [
    { key: 'name', header: 'Name', sortable: true },
    { key: 'email', header: 'Email', sortable: true },
    { key: 'organization', header: 'Organization', sortable: true },
    { key: 'kyc_status', header: 'KYC Status', render: (row) => <StatusBadge status={row.kyc_status} /> },
    { key: 'risk_score', header: 'Risk Score', align: 'right', mono: true, sortable: true },
    { key: 'submitted_at', header: 'Submitted', sortable: true, render: (row) => new Date(row.submitted_at).toLocaleDateString() },
    {
      key: 'actions', header: 'Actions', render: (row) => (
        (row.kyc_status === 'PENDING' || row.kyc_status === 'UNDER_REVIEW') ? (
          <div className={styles.actionBtns}>
            <button className={styles.approveBtn} onClick={() => setActionTarget({ participant: row, action: 'approve' })}>
              Approve
            </button>
            <button className={styles.rejectBtn} onClick={() => setActionTarget({ participant: row, action: 'reject' })}>
              Reject
            </button>
          </div>
        ) : null
      ),
    },
  ];

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

      <DataGrid
        columns={columns}
        data={filtered}
        keyField="id"
        emptyMessage="No participants found"
        exportFilename="participants"
        loading={isLoading}
      />

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
