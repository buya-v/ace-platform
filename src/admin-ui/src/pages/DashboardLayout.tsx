import React from 'react';
import { Outlet, Navigate, useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { KPIProvider, useKPI } from '../contexts/KPIContext';
import { ToastProvider } from '../contexts/ToastContext';
import { hasComplianceAccess } from '../types';
import { Sidebar } from '../components/Sidebar';
import { TopBar } from '../components/TopBar';
import { ToastContainer } from '../components/Toast';
import { ShortcutHelp } from '../components/ShortcutHelp';
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';
import styles from './DashboardLayout.module.css';

function DashboardInner() {
  const { health, refresh } = useKPI();
  const status = health?.overall_status ?? 'unknown';
  const navigate = useNavigate();

  const { showHelp, setShowHelp } = useKeyboardShortcuts({
    navigate,
    onRefresh: refresh,
    onEscape: () => {},
    onExport: () => {},
  });

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
      <ShortcutHelp visible={showHelp} onClose={() => setShowHelp(false)} />
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
