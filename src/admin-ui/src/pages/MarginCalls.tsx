import React, { useState, useRef } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchMarginCalls, fetchMarginCallStats, triggerMarginCalculation } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { Sparkline } from '../components/Sparkline';
import { MarginCall } from '../types';
import styles from './MarginCalls.module.css';

const MAX_UTIL_HISTORY = 20;

function formatDeadline(deadline: string): { text: string; className: string } {
  const now = Date.now();
  const deadlineMs = new Date(deadline).getTime();
  const diff = deadlineMs - now;

  if (diff <= 0) return { text: 'EXPIRED', className: styles.deadlineExpired };

  const hours = diff / (1000 * 60 * 60);
  const totalMins = Math.floor(diff / (1000 * 60));
  const h = Math.floor(totalMins / 60);
  const m = totalMins % 60;
  const text = h > 0 ? `${h}h ${m}m remaining` : `${m}m remaining`;

  if (hours < 1) return { text, className: styles.deadlineUrgent };
  if (hours < 4) return { text, className: styles.deadlineWarning };
  return { text, className: styles.deadlineSafe };
}

export { formatDeadline };

export function MarginCallsPage() {
  const [showTrigger, setShowTrigger] = useState(false);
  const [triggerParticipant, setTriggerParticipant] = useState('');
  const [triggerInstrument, setTriggerInstrument] = useState('');
  const utilizationHistory = useRef<Map<string, number[]>>(new Map());

  const calls = usePolling(
    (signal) => fetchMarginCalls(signal),
    10000,
  );

  const stats = usePolling(
    (signal) => fetchMarginCallStats(signal),
    10000,
  );

  const marginCalls = calls.data?.data ?? [];

  // Update utilization history on each poll
  for (const mc of marginCalls) {
    const required = parseFloat(mc.required_margin) || 1;
    const current = parseFloat(mc.current_margin) || 0;
    const pct = Math.min((current / required) * 100, 100);
    const key = mc.id;
    const history = utilizationHistory.current.get(key) ?? [];
    history.push(pct);
    if (history.length > MAX_UTIL_HISTORY) history.shift();
    utilizationHistory.current.set(key, history);
  }

  const columns: Column<MarginCall>[] = [
    { key: 'participant_name', header: 'Participant', sortable: true },
    { key: 'instrument_id', header: 'Instrument', sortable: true },
    { key: 'required_margin', header: 'Required', align: 'right', mono: true },
    { key: 'current_margin', header: 'Current', align: 'right', mono: true },
    { key: 'shortfall', header: 'Shortfall', align: 'right', mono: true },
    { key: 'status', header: 'Status', render: (row) => <StatusBadge status={row.status} /> },
    { key: 'issued_at', header: 'Issued', sortable: true, render: (row) => new Date(row.issued_at).toLocaleString() },
    {
      key: 'deadline',
      header: 'Deadline',
      sortable: true,
      render: (row) => {
        const { text, className } = formatDeadline(row.deadline);
        return <span className={className}>{text}</span>;
      },
    },
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
            const history = utilizationHistory.current.get(mc.id) ?? [];
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
                {history.length > 1 && (
                  <Sparkline
                    data={history}
                    color={pct < 80 ? 'var(--accent-green)' : pct < 100 ? 'var(--accent-yellow)' : 'var(--accent-red)'}
                    width={80}
                    height={20}
                  />
                )}
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
