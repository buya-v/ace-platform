import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchMarginCalls, fetchMarginCallStats, triggerMarginCalculation } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { MarginCall } from '../types';
import styles from './MarginCalls.module.css';

export function MarginCallsPage() {
  const [showTrigger, setShowTrigger] = useState(false);
  const [triggerParticipant, setTriggerParticipant] = useState('');
  const [triggerInstrument, setTriggerInstrument] = useState('');

  const calls = usePolling(
    (signal) => fetchMarginCalls(signal),
    10000,
  );

  const stats = usePolling(
    (signal) => fetchMarginCallStats(signal),
    10000,
  );

  const marginCalls = calls.data?.data ?? [];

  const columns: Column<MarginCall>[] = [
    { key: 'participant_name', header: 'Participant', sortable: true },
    { key: 'instrument_id', header: 'Instrument', sortable: true },
    { key: 'required_margin', header: 'Required', align: 'right', mono: true },
    { key: 'current_margin', header: 'Current', align: 'right', mono: true },
    { key: 'shortfall', header: 'Shortfall', align: 'right', mono: true },
    { key: 'status', header: 'Status', render: (row) => <StatusBadge status={row.status} /> },
    { key: 'issued_at', header: 'Issued', sortable: true, render: (row) => new Date(row.issued_at).toLocaleString() },
    { key: 'deadline', header: 'Deadline', sortable: true, render: (row) => new Date(row.deadline).toLocaleString() },
  ];

  return (
    <div>
      <div className={styles.header}>
        <h1>Margin Calls</h1>
        <button className={styles.triggerBtn} onClick={() => setShowTrigger(true)}>
          Trigger Calculation
        </button>
      </div>

      {stats.data && (
        <div className={styles.statsGrid}>
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Active Calls</div>
            <div className={styles.statValue}>{stats.data.total_active}</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Total Shortfall</div>
            <div className={styles.statValue}>{stats.data.total_shortfall}</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Participants in Call</div>
            <div className={styles.statValue}>{stats.data.participants_in_call}</div>
          </div>
          <div className={styles.statCard}>
            <div className={styles.statLabel}>Avg Utilization</div>
            <div className={styles.statValue}>{stats.data.average_utilization}%</div>
          </div>
        </div>
      )}

      <DataGrid
        columns={columns}
        data={marginCalls}
        keyField="id"
        emptyMessage="No active margin calls"
      />

      {/* Utilization bars */}
      {marginCalls.length > 0 && (
        <div className={styles.utilizationSection}>
          <h2>Margin Utilization</h2>
          {marginCalls.map(mc => {
            const required = parseFloat(mc.required_margin) || 1;
            const current = parseFloat(mc.current_margin) || 0;
            const pct = Math.min((current / required) * 100, 100);
            return (
              <div key={mc.id} className={styles.barRow}>
                <span className={styles.barLabel}>{mc.participant_name}</span>
                <div className={styles.barTrack}>
                  <div
                    className={styles.barFill}
                    style={{ width: `${pct}%`, background: pct < 80 ? 'var(--accent-green)' : pct < 100 ? 'var(--accent-yellow)' : 'var(--accent-red)' }}
                  />
                </div>
                <span className={styles.barPct}>{pct.toFixed(1)}%</span>
              </div>
            );
          })}
        </div>
      )}

      {showTrigger && (
        <ConfirmDialog
          title="Trigger Margin Calculation"
          message="Enter participant and instrument IDs to trigger a manual margin calculation."
          confirmLabel="Calculate"
          onConfirm={async () => {
            await triggerMarginCalculation(triggerParticipant, triggerInstrument);
            setShowTrigger(false);
            setTriggerParticipant('');
            setTriggerInstrument('');
            calls.refresh();
          }}
          onCancel={() => setShowTrigger(false)}
        />
      )}
    </div>
  );
}
