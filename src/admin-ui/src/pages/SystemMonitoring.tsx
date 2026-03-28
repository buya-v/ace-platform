import React from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchHealth } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import styles from './SystemMonitoring.module.css';

export function SystemMonitoring() {
  const { data, lastUpdated } = usePolling(
    (signal) => fetchHealth(signal),
    15000,
  );

  return (
    <div>
      <h1>System Health</h1>
      {data && (
        <div className={styles.overallBanner} data-status={data.overall_status}>
          {data.overall_status === 'healthy'
            ? 'All Systems Operational'
            : data.overall_status === 'degraded'
            ? 'Degraded Performance'
            : 'System Outage Detected'}
        </div>
      )}
      {lastUpdated && (
        <p className={styles.lastCheck}>Last checked: {new Date(lastUpdated).toLocaleTimeString()}</p>
      )}
      <div className={styles.grid}>
        {data?.services.map(svc => (
          <div key={svc.name} className={styles.card}>
            <div className={styles.cardHeader}>
              <span className={styles.serviceName}>{svc.name}</span>
              <StatusBadge status={svc.status} variant="health" />
            </div>
            <div className={styles.stats}>
              <div><span className={styles.statLabel}>Latency</span> {svc.latency_ms}ms</div>
              <div><span className={styles.statLabel}>Uptime</span> {formatUptime(svc.uptime_seconds)}</div>
              <div><span className={styles.statLabel}>Version</span> {svc.version}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${mins}m`;
}

export { formatUptime };
