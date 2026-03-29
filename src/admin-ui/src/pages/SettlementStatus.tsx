import React from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSettlementCycles } from '../services/api';
import { SettlementCycle } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { DataGrid, Column } from '../components/DataGrid';
import styles from './SettlementStatus.module.css';

const PHASES = ['OPEN', 'NETTING', 'SETTLING', 'COMPLETED'] as const;

export function SettlementStatusPage() {
  const { data } = usePolling(
    (signal) => fetchSettlementCycles({}, signal),
    15000,
  );

  const cycles = data?.data ?? [];
  const activeCycle = cycles.find(c => c.phase !== 'COMPLETED' && c.phase !== 'FAILED');
  const history = cycles.filter(c => c.phase === 'COMPLETED' || c.phase === 'FAILED');

  const columns: Column<SettlementCycle>[] = [
    { key: 'id', header: 'Cycle ID', sortable: true, mono: true },
    { key: 'phase', header: 'Phase', render: (row) => <StatusBadge status={row.phase} /> },
    { key: 'started_at', header: 'Started', sortable: true, render: (row) => new Date(row.started_at).toLocaleString() },
    { key: 'completed_at', header: 'Completed', sortable: true, render: (row) => row.completed_at ? new Date(row.completed_at).toLocaleString() : '\u2014' },
    { key: 'total_settlements', header: 'Settlements', align: 'right', mono: true, sortable: true },
    { key: 'total_value', header: 'Total Value', align: 'right', mono: true },
  ];

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
            <div><strong>Total Value:</strong> {activeCycle.total_value}</div>
          </div>
        </div>
      )}

      {!activeCycle && (
        <div className={styles.noActive}>No active settlement cycle</div>
      )}

      <h2>Settlement History</h2>
      <DataGrid
        columns={columns}
        data={history}
        keyField="id"
        emptyMessage="No settlement history"
      />
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
