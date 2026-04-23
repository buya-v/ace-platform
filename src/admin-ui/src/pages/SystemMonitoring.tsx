import React, { useRef, useMemo } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchHealth } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { Sparkline } from '../components/Sparkline';
import { Skeleton } from '../components/Skeleton';
import styles from './SystemMonitoring.module.css';

const MAX_HISTORY = 20;

export function SystemMonitoring() {
  const latencyHistory = useRef<Map<string, number[]>>(new Map());

  const { data, lastUpdated, isLoading } = usePolling(
    (signal) => fetchHealth(signal),
    30000,
  );

  // Update latency history on each poll
  if (data?.services) {
    for (const svc of data.services) {
      const history = latencyHistory.current.get(svc.name) ?? [];
      history.push(svc.latency_ms);
      if (history.length > MAX_HISTORY) history.shift();
      latencyHistory.current.set(svc.name, history);
    }
  }

  const { avgLatency, maxLatency } = useMemo(() => {
    if (!data?.services || data.services.length === 0) return { avgLatency: 0, maxLatency: 0 };
    const latencies = data.services.map(s => s.latency_ms);
    const sum = latencies.reduce((a, b) => a + b, 0);
    return {
      avgLatency: Math.round(sum / latencies.length),
      maxLatency: Math.max(...latencies),
    };
  }, [data]);

  return (
    <div>
      <h1>System Health</h1>
      {isLoading && !data && (
        <Skeleton variant="card" height="60px" />
      )}
      {data && (
        <div className={styles.overallBanner} data-status={data.overall_status}>
          <div className={styles.bannerText}>
            {data.overall_status === 'healthy'
              ? 'All Systems Operational'
              : data.overall_status === 'degraded'
              ? 'Degraded Performance'
              : 'System Outage Detected'}
          </div>
          <div className={styles.bannerStats}>
            <span>Avg Latency: <strong>{avgLatency}ms</strong></span>
            <span>Max Latency: <strong>{maxLatency}ms</strong></span>
          </div>
        </div>
      )}
      {lastUpdated && (
        <p className={styles.lastCheck}>Last checked: {new Date(lastUpdated).toLocaleTimeString()}</p>
      )}
      <div className={styles.grid}>
        {data?.services.map(svc => {
          const history = latencyHistory.current.get(svc.name) ?? [];
          const uptimePct = Math.min((svc.uptime_seconds / 86400) * 100, 100);
          return (
            <div key={svc.name} className={styles.card}>
              <div className={styles.cardHeader}>
                <span className={styles.serviceName}>{svc.name}</span>
                <StatusBadge status={svc.status} variant="health" />
              </div>
              <div className={styles.stats}>
                <div className={styles.statRow}>
                  <span className={styles.statLabel}>Latency</span>
                  <span>{svc.latency_ms}ms</span>
                  {history.length > 1 && (
                    <Sparkline data={history} color="var(--accent-blue)" width={100} height={24} />
                  )}
                </div>
                <div><span className={styles.statLabel}>Uptime</span> {formatUptime(svc.uptime_seconds)}</div>
                <div className={styles.uptimeBarWrapper}>
                  <div className={styles.uptimeBarTrack}>
                    <div
                      className={styles.uptimeBarFill}
                      style={{ width: `${uptimePct}%` }}
                    />
                  </div>
                  <span className={styles.uptimeBarLabel}>{uptimePct.toFixed(1)}%</span>
                </div>
                <div><span className={styles.statLabel}>Version</span> {svc.version}</div>
              </div>
            </div>
          );
        })}
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
