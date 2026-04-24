import React, { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useKPI } from '../contexts/KPIContext';
import { useTenant } from '../contexts/TenantContext';
import { useToast } from '../contexts/ToastContext';
import { useWebSocket } from '../hooks/useWebSocket';
import styles from './TopBar.module.css';

const pageNames: Record<string, string> = {
  '': 'Overview',
  monitoring: 'System Health',
  margin: 'Margin Calls',
  settlement: 'Settlement',
  'circuit-breakers': 'Circuit Breakers',
  warehouse: 'Warehouse',
  participants: 'Participants',
  compliance: 'Compliance Alerts',
  audit: 'Audit Log',
};

function formatTime(d: Date): string {
  return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function TopBar() {
  const location = useLocation();
  const { health } = useKPI();
  const { currentTenant, tenants, setCurrentTenant, isLoading: tenantLoading, fetchError: tenantError } = useTenant();
  const { showToast } = useToast();
  const [time, setTime] = useState(() => formatTime(new Date()));
  const wsHealth = useWebSocket('/health', { enabled: true });

  const handleTenantChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const id = e.target.value;
    setCurrentTenant(id);
    const tenant = tenants.find(t => t.id === id);
    if (tenant) {
      showToast(`Switched to ${tenant.name}`, 'info');
    }
  };

  useEffect(() => {
    const id = setInterval(() => setTime(formatTime(new Date())), 1000);
    return () => clearInterval(id);
  }, []);

  // Derive page name from path: /dashboard/monitoring -> monitoring
  const segments = location.pathname.replace('/dashboard', '').replace(/^\//, '').split('/');
  const page = segments[0] || '';
  const pageName = pageNames[page] || page;

  const status = health?.overall_status ?? 'unknown';
  const statusLabel = status === 'healthy' ? 'Operational'
    : status === 'degraded' ? 'Degraded'
    : status === 'unhealthy' ? 'Unhealthy'
    : 'Unknown';

  const statusClass = status === 'healthy' ? styles.statusHealthy
    : status === 'degraded' ? styles.statusDegraded
    : status === 'unhealthy' ? styles.statusUnhealthy
    : styles.statusUnknown;

  return (
    <div className={styles.topbar}>
      <div className={styles.breadcrumb}>
        <span>Dashboard</span>
        {pageName && page !== '' && (
          <>
            <span className={styles.breadcrumbSep}>/</span>
            <span className={styles.breadcrumbCurrent}>{pageName}</span>
          </>
        )}
      </div>

      {/* ── Tenant selector ── */}
      <div className={styles.tenantSelector}>
        <label htmlFor="topbar-tenant-select" className={styles.srOnly}>
          Active tenant
        </label>
        <select
          id="topbar-tenant-select"
          className={styles.tenantSelect}
          value={currentTenant?.id ?? ''}
          onChange={handleTenantChange}
          disabled={tenantLoading || !!tenantError || tenants.length === 0}
          aria-label="Select active tenant"
          aria-busy={tenantLoading}
          data-testid="tenant-select"
        >
          {tenantLoading && (
            <option value="" disabled>Loading tenants…</option>
          )}
          {!tenantLoading && tenantError && (
            <option value="" disabled>Error loading tenants</option>
          )}
          {!tenantLoading && !tenantError && tenants.length === 0 && (
            <option value="" disabled>No tenants available</option>
          )}
          {!tenantLoading && !tenantError && tenants.map(t => (
            <option key={t.id} value={t.id}>{t.name}</option>
          ))}
        </select>
      </div>

      <div className={styles.right}>
        <button
          className={styles.printBtn}
          onClick={() => window.print()}
          title="Print / PDF"
          data-print-hide
        >
          &#128424; Print / PDF
        </button>
        <span
          className={
            wsHealth.status === 'connected' ? styles.wsConnected
            : wsHealth.status === 'connecting' ? styles.wsConnecting
            : styles.wsDisconnected
          }
          data-testid="ws-badge"
        >
          <span className={styles.statusDot} />
          {wsHealth.status === 'connected' ? 'WS: Connected' : 'WS: Disconnected'}
        </span>
        <span className={statusClass}>
          <span className={styles.statusDot} />
          {statusLabel}
        </span>
        <span className={styles.clock}>{time}</span>
      </div>
    </div>
  );
}
