import React from 'react';
import { Outlet, Navigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { KPIProvider, useKPI } from '../contexts/KPIContext';
import { ToastProvider } from '../contexts/ToastContext';
import { hasComplianceAccess } from '../types';
import { Sidebar } from '../components/Sidebar';
import { TopBar } from '../components/TopBar';
import { ToastContainer } from '../components/Toast';
import styles from './DashboardLayout.module.css';

function DashboardInner() {
  const { health } = useKPI();
  const status = health?.overall_status ?? 'unknown';

  return (
    <div className={styles.layout}>
      <Sidebar systemStatus={status as 'healthy' | 'degraded' | 'unhealthy' | 'unknown'} />
      <main className={styles.main}>
        <TopBar />
        <div className={styles.content}>
          <Outlet />
        </div>
      </main>
      <ToastContainer />
    </div>
  );
}

export function DashboardLayout() {
  const { state } = useAuth();

  if (!state.isAuthenticated || !state.user) {
    return <Navigate to="/login" replace />;
  }

  if (!hasComplianceAccess(state.user.roles)) {
    return <Navigate to="/login" replace />;
  }

  return (
    <ToastProvider>
      <KPIProvider>
        <DashboardInner />
      </KPIProvider>
    </ToastProvider>
  );
}
