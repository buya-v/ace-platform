import React, { useState, useEffect } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSurveillanceAlerts, resolveSurveillanceAlert, fetchSecuritiesInstruments } from '../services/api';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { useToast } from '../contexts/ToastContext';
import styles from './Surveillance.module.css';

export interface SurveillanceAlert {
  id: string;
  timestamp: string;
  participant_id: string;
  participant_name: string;
  instrument_id: string;
  rule_type: string;
  severity: 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW';
  status: 'OPEN' | 'UNDER_REVIEW' | 'RESOLVED' | 'DISMISSED';
  description: string;
}

/** Map severity to CSS class name */
export function severityClassName(severity: string): string {
  switch (severity) {
    case 'CRITICAL': return 'severityCritical';
    case 'HIGH': return 'severityHigh';
    case 'MEDIUM': return 'severityMedium';
    case 'LOW': return 'severityLow';
    default: return '';
  }
}

/** Map status to CSS class name */
export function statusClassName(status: string): string {
  switch (status) {
    case 'OPEN': return 'statusOpen';
    case 'UNDER_REVIEW': return 'statusUnderReview';
    case 'RESOLVED': return 'statusResolved';
    case 'DISMISSED': return 'statusDismissed';
    default: return '';
  }
}

/** Format ISO timestamp to locale string */
export function formatAlertTime(isoString: string): string {
  if (!isoString) return '';
  try {
    return new Date(isoString).toLocaleString();
  } catch {
    return isoString;
  }
}

/** Sort alerts by severity (CRITICAL first) then by timestamp (newest first) */
export function sortSurveillanceAlerts(alerts: SurveillanceAlert[]): SurveillanceAlert[] {
  const order: Record<string, number> = { CRITICAL: 0, HIGH: 1, MEDIUM: 2, LOW: 3 };
  return [...alerts].sort((a, b) => {
    const sevDiff = (order[a.severity] ?? 4) - (order[b.severity] ?? 4);
    if (sevDiff !== 0) return sevDiff;
    return new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime();
  });
}

/** Compute severity distribution counts */
export function computeSeverityCounts(alerts: SurveillanceAlert[]): Record<string, number> {
  const counts: Record<string, number> = { CRITICAL: 0, HIGH: 0, MEDIUM: 0, LOW: 0 };
  alerts.forEach(a => {
    if (counts[a.severity] !== undefined) {
      counts[a.severity]++;
    }
  });
  return counts;
}

/** Filter alerts by severity and status */
export function filterAlerts(
  alerts: SurveillanceAlert[],
  severityFilter: string,
  statusFilter: string,
): SurveillanceAlert[] {
  return alerts.filter(a => {
    if (severityFilter && a.severity !== severityFilter) return false;
    if (statusFilter && a.status !== statusFilter) return false;
    return true;
  });
}

export function SurveillancePage() {
  const [severityFilter, setSeverityFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [selectedAlert, setSelectedAlert] = useState<SurveillanceAlert | null>(null);
  const [resolveTarget, setResolveTarget] = useState<SurveillanceAlert | null>(null);
  const [instrumentMap, setInstrumentMap] = useState<Map<string, string>>(new Map());
  const { showToast } = useToast();

  // Load instruments for ID → ticker mapping
  useEffect(() => {
    fetchSecuritiesInstruments().then((res: any) => {
      const list = res?.data ?? res?.instruments ?? [];
      const map = new Map<string, string>();
      list.forEach((i: any) => map.set(i.id, i.ticker || i.name || i.id));
      setInstrumentMap(map);
    }).catch(() => {});
  }, []);

  const { data, refresh } = usePolling(
    (signal) => fetchSurveillanceAlerts({ severity: severityFilter || undefined, status: statusFilter || undefined }, signal),
    15000,
  );

  // Normalize alerts from securities-service format to UI format
  const rawAlerts: SurveillanceAlert[] = (data?.data ?? []).map((a: any) => ({
    id: a.id,
    timestamp: a.timestamp || a.created_at || '',
    participant_id: a.participant_id || '',
    participant_name: a.resolved_by ? a.resolved_by : (a.status === 'OPEN' ? 'Pending review' : '—'),
    instrument_id: a.instrument_id || '',
    rule_type: a.rule_type || a.alert_type || '',
    severity: a.severity || (a.alert_type === 'LARGE_TRADE' ? 'HIGH' : a.alert_type === 'PRICE_SPIKE' ? 'MEDIUM' : 'LOW'),
    status: a.status || 'OPEN',
    description: a.description || a.message || '',
  }));
  const filtered = filterAlerts(rawAlerts, severityFilter, statusFilter);
  const alerts = sortSurveillanceAlerts(filtered);
  const counts = computeSeverityCounts(rawAlerts);

  const handleResolve = async () => {
    if (!resolveTarget) return;
    try {
      await resolveSurveillanceAlert(resolveTarget.id);
      showToast('Alert resolved', 'success');
      setResolveTarget(null);
      setSelectedAlert(null);
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to resolve alert', 'error');
    }
  };

  return (
    <div>
      <h1>Surveillance Dashboard</h1>

      <div className={styles.statsRow}>
        {(['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as const).map(sev => (
          <div key={sev} className={styles.statCard}>
            <div className={styles.statValue}>{counts[sev]}</div>
            <div className={styles.statLabel}>{sev}</div>
          </div>
        ))}
      </div>

      <div className={styles.topRow}>
        <div className={styles.filters}>
          <select value={severityFilter} onChange={e => setSeverityFilter(e.target.value)} className={styles.select}>
            <option value="">All Severities</option>
            <option value="CRITICAL">Critical</option>
            <option value="HIGH">High</option>
            <option value="MEDIUM">Medium</option>
            <option value="LOW">Low</option>
          </select>
          <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className={styles.select}>
            <option value="">All Status</option>
            <option value="OPEN">Open</option>
            <option value="UNDER_REVIEW">Under Review</option>
            <option value="RESOLVED">Resolved</option>
            <option value="DISMISSED">Dismissed</option>
          </select>
        </div>
      </div>

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Time</th>
            <th>Reviewed By</th>
            <th>Instrument</th>
            <th>Rule Type</th>
            <th>Severity</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {alerts.map(alert => (
            <tr key={alert.id} onClick={() => setSelectedAlert(alert)}>
              <td>{formatAlertTime(alert.timestamp || (alert as any).created_at)}</td>
              <td>{alert.participant_name || (alert as any).resolved_by || '—'}</td>
              <td>{instrumentMap.get(alert.instrument_id) || alert.instrument_id?.slice(0, 8) || '—'}</td>
              <td>{(alert.rule_type || (alert as any).alert_type || '—').replace(/_/g, ' ')}</td>
              <td>
                <span className={styles[severityClassName(alert.severity || 'MEDIUM')]}>
                  {alert.severity || 'MEDIUM'}
                </span>
              </td>
              <td>
                <span className={`${styles.statusBadge} ${styles[statusClassName(alert.status)]}`}>
                  {(alert.status || '').replace(/_/g, ' ')}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {alerts.length === 0 && (
        <div className={styles.empty}>No surveillance alerts</div>
      )}

      {selectedAlert && (
        <div className={styles.detailPanel}>
          <div className={styles.detailHeader}>
            <span className={styles.detailTitle}>Alert Details</span>
            <button className={styles.closeBtn} onClick={() => setSelectedAlert(null)}>Close</button>
          </div>
          <div className={styles.detailGrid}>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Time</span>
              <span className={styles.detailValue}>{formatAlertTime(selectedAlert.timestamp || (selectedAlert as any).created_at)}</span>
            </div>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Reviewed By</span>
              <span className={styles.detailValue}>{selectedAlert.participant_name}</span>
            </div>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Instrument</span>
              <span className={styles.detailValue}>{instrumentMap.get(selectedAlert.instrument_id) || selectedAlert.instrument_id}</span>
            </div>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Alert Type</span>
              <span className={styles.detailValue}>{(selectedAlert.rule_type || (selectedAlert as any).alert_type || '—').replace(/_/g, ' ')}</span>
            </div>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Severity</span>
              <span className={styles[severityClassName(selectedAlert.severity)]}>
                {selectedAlert.severity || '—'}
              </span>
            </div>
            <div className={styles.detailField}>
              <span className={styles.detailLabel}>Status</span>
              <span className={`${styles.statusBadge} ${styles[statusClassName(selectedAlert.status)]}`}>
                {(selectedAlert.status || '').replace(/_/g, ' ')}
              </span>
            </div>
          </div>
          <div className={styles.detailField}>
            <span className={styles.detailLabel}>Description</span>
            <span className={styles.detailValue}>{selectedAlert.description || (selectedAlert as any).message || '—'}</span>
          </div>
          {(selectedAlert.status === 'OPEN' || selectedAlert.status === 'UNDER_REVIEW') && (
            <div style={{ marginTop: 16 }}>
              <button className={styles.resolveBtn} onClick={() => setResolveTarget(selectedAlert)}>
                Resolve Alert
              </button>
            </div>
          )}
        </div>
      )}

      {resolveTarget && (
        <ConfirmDialog
          title="Resolve Alert"
          message={`Resolve the ${resolveTarget.rule_type.replace(/_/g, ' ')} alert for ${resolveTarget.participant_name}?`}
          confirmLabel="Resolve"
          onConfirm={handleResolve}
          onCancel={() => setResolveTarget(null)}
        />
      )}
    </div>
  );
}
