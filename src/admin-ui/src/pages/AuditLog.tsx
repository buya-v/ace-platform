import React, { useState, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchAuditTrail } from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import { AuditEvent } from '../types';
import styles from './AuditLog.module.css';

const ACTION_TYPES = [
  'LOGIN', 'LOGOUT', 'KYC_SUBMITTED', 'KYC_APPROVED', 'KYC_REJECTED',
  'TRADE_HALTED', 'TRADE_RESUMED', 'CIRCUIT_BREAKER_SET',
  'MARGIN_CALL_ISSUED', 'MARGIN_CALL_MET', 'MARGIN_CALL_BREACHED',
  'SAR_FILED', 'ALERT_RESOLVED', 'SCREENING_RUN',
  'RECEIPT_ISSUED', 'RECEIPT_TRANSFERRED', 'DELIVERY_REQUESTED',
];

export function AuditLogPage() {
  const [filters, setFilters] = useState({ actor: '', action: '', from: '', to: '' });
  const [page, setPage] = useState(1);

  const fetchFn = useCallback(
    (signal: AbortSignal) => fetchAuditTrail({
      actor: filters.actor || undefined,
      action: filters.action || undefined,
      from: filters.from || undefined,
      to: filters.to || undefined,
      page,
    }, signal),
    [filters, page],
  );

  const { data, refresh } = usePolling(fetchFn, 0); // No auto-polling

  const events = data?.data ?? [];
  const pagination = data?.pagination;

  const columns: Column<AuditEvent>[] = [
    { key: 'timestamp', header: 'Timestamp', sortable: true, render: (row) => new Date(row.timestamp).toLocaleString() },
    { key: 'actor', header: 'Actor', sortable: true, filterable: true },
    { key: 'action', header: 'Action', mono: true, sortable: true, filterable: true },
    { key: 'target_type', header: 'Target', render: (row) => `${row.target_type} / ${row.target_id}` },
    { key: 'ip_address', header: 'IP Address', mono: true },
  ];

  return (
    <div>
      <div className={styles.header}>
        <h1>Audit Log</h1>
        <button onClick={refresh} className={styles.refreshBtn}>Refresh</button>
      </div>

      <div className={styles.filters}>
        <input
          type="text"
          placeholder="Filter by actor..."
          value={filters.actor}
          onChange={e => setFilters(f => ({ ...f, actor: e.target.value }))}
          className={styles.input}
        />
        <select
          value={filters.action}
          onChange={e => setFilters(f => ({ ...f, action: e.target.value }))}
          className={styles.select}
        >
          <option value="">All Actions</option>
          {ACTION_TYPES.map(a => <option key={a} value={a}>{a}</option>)}
        </select>
        <input
          type="date"
          value={filters.from}
          onChange={e => setFilters(f => ({ ...f, from: e.target.value }))}
          className={styles.input}
        />
        <input
          type="date"
          value={filters.to}
          onChange={e => setFilters(f => ({ ...f, to: e.target.value }))}
          className={styles.input}
        />
      </div>

      <DataGrid
        columns={columns}
        data={events}
        keyField="id"
        emptyMessage="No audit events found"
        exportFilename="audit-log"
      />

      {pagination && pagination.total_pages > 1 && (
        <div className={styles.pagination}>
          <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Previous</button>
          <span>Page {page} of {pagination.total_pages}</span>
          <button disabled={page >= pagination.total_pages} onClick={() => setPage(p => p + 1)}>Next</button>
        </div>
      )}
    </div>
  );
}
