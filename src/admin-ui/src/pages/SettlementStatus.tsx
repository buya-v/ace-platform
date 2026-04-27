import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSettlementCycles, apiFetch } from '../services/api';
import { SettlementCycle } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { DataGrid, Column } from '../components/DataGrid';
import styles from './SettlementStatus.module.css';

const PHASES = ['OPEN', 'NETTING', 'SETTLING', 'COMPLETED'] as const;

export function formatCurrency(value: string | number): string {
  const num = typeof value === 'string' ? parseFloat(value) : value;
  if (isNaN(num)) return '$0.00';
  return '$' + num.toFixed(2).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

interface SecuritiesSettlement {
  id: string;
  trade_id: string;
  instrument_id: string;
  buyer_id: string;
  seller_id: string;
  quantity: number;
  price: number;
  status: string;
  settlement_date: string;
  created_at: string;
}

export function SettlementStatusPage() {
  const [expandedCycleId, setExpandedCycleId] = useState<string | null>(null);
  const [secSettlements, setSecSettlements] = useState<SecuritiesSettlement[]>([]);

  const { data, isLoading } = usePolling(
    (signal) => fetchSettlementCycles({}, signal),
    15000,
  );

  // Also fetch securities settlements
  const { data: secData } = usePolling(
    (signal) => apiFetch<{ data?: SecuritiesSettlement[]; obligations?: SecuritiesSettlement[] }>('/securities/settlements', {}, signal),
    15000,
  );

  React.useEffect(() => {
    const list = (secData as any)?.data ?? (secData as any)?.obligations ?? (Array.isArray(secData) ? secData : []);
    setSecSettlements(list);
  }, [secData]);

  const cycles = data?.data ?? [];
  const activeCycle = cycles.find(c => c.phase !== 'COMPLETED' && c.phase !== 'FAILED');
  const history = cycles.filter(c => c.phase === 'COMPLETED' || c.phase === 'FAILED');

  const columns: Column<SettlementCycle>[] = [
    { key: 'id', header: 'Cycle ID', sortable: true, mono: true },
    { key: 'phase', header: 'Phase', render: (row) => <StatusBadge status={row.phase} /> },
    { key: 'started_at', header: 'Started', sortable: true, render: (row) => new Date(row.started_at).toLocaleString() },
    { key: 'completed_at', header: 'Completed', sortable: true, render: (row) => row.completed_at ? new Date(row.completed_at).toLocaleString() : '\u2014' },
    { key: 'total_settlements', header: 'Settlements', align: 'right', mono: true, sortable: true },
    { key: 'total_value', header: 'Total Value', align: 'right', mono: true, render: (row) => formatCurrency(row.total_value) },
  ];

  const toggleExpand = (cycleId: string) => {
    setExpandedCycleId(prev => prev === cycleId ? null : cycleId);
  };

  return (
    <div>
      <h1>Settlement Status</h1>

      {activeCycle && (
        <div className={styles.activePanel}>
          <h2>Current Cycle: {activeCycle.id}</h2>
          <PhaseStepper phase={activeCycle.phase} />
          <div className={styles.cycleInfo}>
            <div><strong>Started:</strong> {new Date(activeCycle.started_at).toLocaleString()}</div>
            <div><strong>Expected:</strong> {new Date(activeCycle.expected_completion).toLocaleString()}</div>
            <div><strong>Settlements:</strong> {activeCycle.total_settlements}</div>
            <div><strong>Total Value:</strong> {formatCurrency(activeCycle.total_value)}</div>
          </div>
        </div>
      )}

      {!activeCycle && (
        <div className={styles.noActive}>No active settlement cycle</div>
      )}

      <h2>Settlement History</h2>
      <div className={styles.historyList}>
        {history.length === 0 && (
          <div className={styles.noActive}>No settlement history</div>
        )}
        {history.map(cycle => (
          <div key={cycle.id}>
            <div
              className={styles.historyRow}
              onClick={() => toggleExpand(cycle.id)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleExpand(cycle.id); }}
            >
              <span className={`${styles.expandIcon} ${expandedCycleId === cycle.id ? styles.expandIconOpen : ''}`}>&#9654;</span>
              <span className={styles.historyId}>{cycle.id}</span>
              <StatusBadge status={cycle.phase} />
              <span className={styles.historyDate}>{new Date(cycle.started_at).toLocaleString()}</span>
              <span className={styles.historyDate}>{cycle.completed_at ? new Date(cycle.completed_at).toLocaleString() : '\u2014'}</span>
              <span className={styles.historyValue}>{cycle.total_settlements}</span>
              <span className={styles.historyValue}>{formatCurrency(cycle.total_value)}</span>
            </div>
            <div className={`${styles.detailPanel} ${expandedCycleId === cycle.id ? styles.detailPanelOpen : ''}`}>
              <div className={styles.detailContent}>
                <h3>Cycle Details</h3>
                <div className={styles.detailGrid}>
                  <div><strong>Cycle ID:</strong> {cycle.id}</div>
                  <div><strong>Phase:</strong> {cycle.phase}</div>
                  <div><strong>Started:</strong> {new Date(cycle.started_at).toLocaleString()}</div>
                  <div><strong>Completed:</strong> {cycle.completed_at ? new Date(cycle.completed_at).toLocaleString() : '\u2014'}</div>
                  <div><strong>Total Settlements:</strong> {cycle.total_settlements}</div>
                  <div><strong>Total Value:</strong> {formatCurrency(cycle.total_value)}</div>
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      <h2>Securities Settlement Obligations</h2>
      {secSettlements.length === 0 ? (
        <div className={styles.noActive}>No settlement obligations found</div>
      ) : (
        <div className={styles.historyList}>
          {secSettlements.map((s, i) => (
            <div key={s.id || i} className={styles.historyRow}>
              <span className={styles.historyId}>{(s.id || '').slice(0, 8)}</span>
              <StatusBadge status={s.status || 'PENDING'} />
              <span>{s.instrument_id?.slice(0, 8) || '—'}</span>
              <span>Qty: {s.quantity}</span>
              <span>Price: {s.price}</span>
              <span>Settle: {s.settlement_date || '—'}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function PhaseStepper({ phase }: { phase: SettlementCycle['phase'] }) {
  const currentIndex = PHASES.indexOf(phase as typeof PHASES[number]);

  return (
    <div className={styles.stepper}>
      {PHASES.map((p, i) => (
        <React.Fragment key={p}>
          <div className={`${styles.step} ${i <= currentIndex ? styles.stepActive : ''} ${i === currentIndex ? styles.stepCurrent : ''}`}>
            {p}
          </div>
          {i < PHASES.length - 1 && (
            <div className={`${styles.connector} ${i < currentIndex ? styles.connectorActive : ''}`} />
          )}
        </React.Fragment>
      ))}
    </div>
  );
}
