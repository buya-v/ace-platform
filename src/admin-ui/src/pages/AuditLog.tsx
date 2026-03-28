import React, { useState, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchAuditTrail } from '../services/api';
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

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Timestamp</th>
            <th>Actor</th>
            <th>Action</th>
            <th>Target</th>
            <th>IP Address</th>
          </tr>
        </thead>
        <tbody>
          {events.map(evt => (
            <tr key={evt.id}>
              <td>{new Date(evt.timestamp).toLocaleString()}</td>
              <td>{evt.actor}</td>
              <td className={styles.action}>{evt.action}</td>
              <td>{evt.target_type} / {evt.target_id}</td>
              <td className={styles.mono}>{evt.ip_address}</td>
            </tr>
          ))}
          {events.length === 0 && (
            <tr><td colSpan={5} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No audit events found</td></tr>
          )}
        </tbody>
      </table>

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
