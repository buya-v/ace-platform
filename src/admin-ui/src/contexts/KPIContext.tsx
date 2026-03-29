import React, { createContext, useContext } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchHealth, fetchMarginCallStats } from '../services/api';
import { useAuth } from './AuthContext';
import { hasAdminAccess, HealthResponse, MarginCallStats } from '../types';

interface KPIContextValue {
  health: HealthResponse | null;
  marginStats: MarginCallStats | null;
  isLoading: boolean;
}

const KPIContext = createContext<KPIContextValue | null>(null);

export function KPIProvider({ children }: { children: React.ReactNode }) {
  const { state: auth } = useAuth();
  const isAdmin = hasAdminAccess(auth.user?.roles ?? []);

  const healthPoll = usePolling(
    (signal) => fetchHealth(signal),
    15000,
    isAdmin,
  );

  const marginPoll = usePolling(
    (signal) => fetchMarginCallStats(signal),
    15000,
    isAdmin,
  );

  const value: KPIContextValue = {
    health: healthPoll.data as HealthResponse | null,
    marginStats: marginPoll.data,
    isLoading: healthPoll.isLoading || marginPoll.isLoading,
  };

  return (
    <KPIContext.Provider value={value}>
      {children}
    </KPIContext.Provider>
  );
}

export function useKPI(): KPIContextValue {
  const ctx = useContext(KPIContext);
  if (!ctx) throw new Error('useKPI must be used within KPIProvider');
  return ctx;
}
