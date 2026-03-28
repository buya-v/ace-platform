import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchComplianceAlerts, resolveAlert } from '../services/api';
import { ComplianceAlert } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import styles from './ComplianceAlerts.module.css';

const SEVERITY_ORDER: Record<string, number> = { CRITICAL: 0, HIGH: 1, MEDIUM: 2, LOW: 3 };

export function sortAlertsBySeverity(alerts: ComplianceAlert[]): ComplianceAlert[] {
  return [...alerts].sort((a, b) => (SEVERITY_ORDER[a.severity] ?? 4) - (SEVERITY_ORDER[b.severity] ?? 4));
}

export function ComplianceAlertsPage() {
  const [statusFilter, setStatusFilter] = useState('');
  const [resolveTarget, setResolveTarget] = useState<ComplianceAlert | null>(null);

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

        {/* Simple risk distribution */}
        <div className={styles.distribution}>
          {(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as const).map(sev => (
            <div key={sev} className={styles.distItem}>
              <span className={`${styles.distDot} ${styles[`dot${sev}`]}`} />
              <span>{sev}: {distribution[sev]}</span>
              <span className={styles.distPct}>({((distribution[sev] / totalAlerts) * 100).toFixed(0)}%)</span>
            </div>
          ))}
        </div>
      </div>

      <div className={styles.alertList}>
        {alerts.map(alert => (
          <div key={alert.id} className={`${styles.alertCard} ${severityClass(alert.severity)}`}>
            <div className={styles.alertHeader}>
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
                <button className={styles.resolveBtn} onClick={() => setResolveTarget(alert)}>
                  Resolve
                </button>
              </div>
            )}
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
