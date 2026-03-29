import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchComplianceAlerts, resolveAlert } from '../services/api';
import { ComplianceAlert } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { AlertIcon, InfoIcon, CheckIcon } from '../components/icons';
import styles from './ComplianceAlerts.module.css';

const SEVERITY_ORDER: Record<string, number> = { CRITICAL: 0, HIGH: 1, MEDIUM: 2, LOW: 3 };

export function sortAlertsBySeverity(alerts: ComplianceAlert[]): ComplianceAlert[] {
  return [...alerts].sort((a, b) => (SEVERITY_ORDER[a.severity] ?? 4) - (SEVERITY_ORDER[b.severity] ?? 4));
}

function SeverityIcon({ severity }: { severity: string }) {
  switch (severity) {
    case 'CRITICAL':
      return <span className={styles.severityIconCritical}><AlertIcon size={14} /></span>;
    case 'HIGH':
      return <span className={styles.severityIconHigh}><AlertIcon size={14} /></span>;
    case 'MEDIUM':
      return <span className={styles.severityIconMedium}><InfoIcon size={14} /></span>;
    case 'LOW':
      return <span className={styles.severityIconLow}><CheckIcon size={14} /></span>;
    default:
      return null;
  }
}

export { SeverityIcon };

export function ComplianceAlertsPage() {
  const [statusFilter, setStatusFilter] = useState('');
  const [resolveTarget, setResolveTarget] = useState<ComplianceAlert | null>(null);
  const [expandedNotes, setExpandedNotes] = useState<Set<string>>(new Set());
  const [notes, setNotes] = useState<Record<string, string>>({});

  const { data, refresh } = usePolling(
    (signal) => fetchComplianceAlerts({ status: statusFilter || undefined }, signal),
    30000,
  );

  const alerts = sortAlertsBySeverity(data?.data ?? []);

  // Risk distribution counts
  const distribution = { LOW: 0, MEDIUM: 0, HIGH: 0, CRITICAL: 0 };
  alerts.forEach(a => { distribution[a.severity]++; });
  const totalAlerts = alerts.length || 1;

  const handleResolve = async () => {
    if (!resolveTarget) return;
    await resolveAlert(resolveTarget.id);
    setResolveTarget(null);
    refresh();
  };

  const severityClass = (severity: string) => {
    switch (severity) {
      case 'CRITICAL': return styles.critical;
      case 'HIGH': return styles.high;
      case 'MEDIUM': return styles.medium;
      default: return '';
    }
  };

  const toggleNotes = (alertId: string) => {
    setExpandedNotes(prev => {
      const next = new Set(prev);
      if (next.has(alertId)) next.delete(alertId);
      else next.add(alertId);
      return next;
    });
  };

  return (
    <div>
      <h1>Compliance Alerts</h1>

      <div className={styles.topRow}>
        <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className={styles.select}>
          <option value="">All Status</option>
          <option value="OPEN">Open</option>
          <option value="UNDER_REVIEW">Under Review</option>
          <option value="RESOLVED">Resolved</option>
          <option value="DISMISSED">Dismissed</option>
        </select>

        {/* Risk distribution bars */}
        <div className={styles.distribution}>
          {(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as const).map(sev => {
            const count = distribution[sev];
            const pct = (count / totalAlerts) * 100;
            return (
              <div key={sev} className={styles.distItem}>
                <span className={styles.distLabel}>{sev}</span>
                <div className={styles.distBarTrack}>
                  <div
                    className={`${styles.distBarFill} ${styles[`bar${sev}`]}`}
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <span className={styles.distCount}>{count}</span>
                <span className={styles.distPct}>({pct.toFixed(0)}%)</span>
              </div>
            );
          })}
        </div>
      </div>

      <div className={styles.alertList}>
        {alerts.map(alert => (
          <div
            key={alert.id}
            className={`${styles.alertCard} ${severityClass(alert.severity)}`}
            onClick={() => toggleNotes(alert.id)}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') toggleNotes(alert.id); }}
          >
            <div className={styles.alertHeader}>
              <SeverityIcon severity={alert.severity} />
              <StatusBadge status={alert.severity} />
              <span className={styles.alertType}>{alert.alert_type.replace(/_/g, ' ')}</span>
              <StatusBadge status={alert.status} />
            </div>
            <div className={styles.alertBody}>
              <strong>{alert.participant_name}</strong>
              <p>{alert.description}</p>
              <span className={styles.alertTime}>{new Date(alert.created_at).toLocaleString()}</span>
            </div>
            {(alert.status === 'OPEN' || alert.status === 'UNDER_REVIEW') && (
              <div className={styles.alertActions}>
                <button
                  className={styles.resolveBtn}
                  onClick={(e) => { e.stopPropagation(); setResolveTarget(alert); }}
                >
                  Resolve
                </button>
              </div>
            )}
            <div className={`${styles.notesPanel} ${expandedNotes.has(alert.id) ? styles.notesPanelOpen : ''}`}>
              <div className={styles.notesContent}>
                <strong className={styles.notesLabel}>Investigation Notes</strong>
                <textarea
                  className={styles.notesTextarea}
                  placeholder="Add investigation notes..."
                  value={notes[alert.id] ?? ''}
                  onChange={(e) => setNotes(prev => ({ ...prev, [alert.id]: e.target.value }))}
                  onClick={(e) => e.stopPropagation()}
                  onKeyDown={(e) => e.stopPropagation()}
                  rows={3}
                />
              </div>
            </div>
          </div>
        ))}
        {alerts.length === 0 && (
          <div className={styles.empty}>No compliance alerts</div>
        )}
      </div>

      {resolveTarget && (
        <ConfirmDialog
          title="Resolve Alert"
          message={`Resolve the ${resolveTarget.alert_type} alert for ${resolveTarget.participant_name}?`}
          confirmLabel="Resolve"
          onConfirm={handleResolve}
          onCancel={() => setResolveTarget(null)}
        />
      )}
    </div>
  );
}
