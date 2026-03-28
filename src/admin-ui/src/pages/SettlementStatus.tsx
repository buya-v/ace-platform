import React from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSettlementCycles } from '../services/api';
import { SettlementCycle } from '../types';
import { StatusBadge } from '../components/StatusBadge';
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
      <table className={styles.table}>
        <thead>
          <tr>
            <th>Cycle ID</th>
            <th>Phase</th>
            <th>Started</th>
            <th>Completed</th>
            <th>Settlements</th>
            <th>Total Value</th>
          </tr>
        </thead>
        <tbody>
          {history.map(c => (
            <tr key={c.id}>
              <td>{c.id}</td>
              <td><StatusBadge status={c.phase} /></td>
              <td>{new Date(c.started_at).toLocaleString()}</td>
              <td>{c.completed_at ? new Date(c.completed_at).toLocaleString() : '—'}</td>
              <td>{c.total_settlements}</td>
              <td>{c.total_value}</td>
            </tr>
          ))}
          {history.length === 0 && (
            <tr><td colSpan={6} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No settlement history</td></tr>
          )}
        </tbody>
      </table>
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
