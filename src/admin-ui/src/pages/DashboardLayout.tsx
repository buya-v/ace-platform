import React from 'react';
import { Outlet, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import { KPIProvider, useKPI } from '../contexts/KPIContext';
import { ToastProvider } from '../contexts/ToastContext';
import { BotProvider } from '../contexts/BotContext';
import { hasComplianceAccess } from '../types';
import { Sidebar } from '../components/Sidebar';
import { TopBar } from '../components/TopBar';
import { ToastContainer } from '../components/Toast';
import { BotButton } from '../components/BotButton';
import { BotChatPanel } from '../components/BotChatPanel';
import { ShortcutHelp } from '../components/ShortcutHelp';
import { ErrorBoundary } from '../components/ErrorBoundary';
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
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </div>
      </main>
      <ToastContainer />
      <BotButton />
      <BotChatPanel />
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

  const location = useLocation();

  return (
    <ToastProvider>
      <KPIProvider>
        <BotProvider currentPage={location.pathname}>
          <DashboardInner />
        </BotProvider>
      </KPIProvider>
    </ToastProvider>
  );
}
