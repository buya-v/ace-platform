import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchMarginCalls, fetchMarginCallStats, triggerMarginCalculation } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
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

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Participant</th>
            <th>Instrument</th>
            <th>Required</th>
            <th>Current</th>
            <th>Shortfall</th>
            <th>Status</th>
            <th>Issued</th>
            <th>Deadline</th>
          </tr>
        </thead>
        <tbody>
          {marginCalls.map(mc => (
            <tr key={mc.id}>
              <td>{mc.participant_name}</td>
              <td>{mc.instrument_id}</td>
              <td>{mc.required_margin}</td>
              <td>{mc.current_margin}</td>
              <td>{mc.shortfall}</td>
              <td><StatusBadge status={mc.status} /></td>
              <td>{new Date(mc.issued_at).toLocaleString()}</td>
              <td>{new Date(mc.deadline).toLocaleString()}</td>
            </tr>
          ))}
          {marginCalls.length === 0 && (
            <tr><td colSpan={8} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No active margin calls</td></tr>
          )}
        </tbody>
      </table>

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
                    style={{ width: `${pct}%`, background: pct < 80 ? '#28a745' : pct < 100 ? '#ffc107' : '#dc3545' }}
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
