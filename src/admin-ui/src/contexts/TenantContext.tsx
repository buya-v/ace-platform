import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { setApiTenant } from '../services/api';
import { getConfig } from '../config';

interface TenantInfo {
  id: string;
  name: string;
  status: string;
  flagship: boolean;
  governance_tier: string;
}

interface TenantContextType {
  currentTenant: TenantInfo | null;
  tenants: TenantInfo[];
  setCurrentTenant: (id: string) => void;
  isLoading: boolean;
}

const STORAGE_KEY = 'garudax-tenant';

const TenantContext = createContext<TenantContextType | null>(null);

export function TenantProvider({ children }: { children: React.ReactNode }) {
  const [tenants, setTenants] = useState<TenantInfo[]>([]);
  const [currentTenant, setCurrentTenantState] = useState<TenantInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const config = getConfig();
    const url = `${config.API_BASE_URL}/platform/v1/tenants`;

    fetch(url)
      .then(res => {
        if (!res.ok) throw new Error(`Failed to fetch tenants: ${res.status}`);
        return res.json();
      })
      .then((data: unknown) => {
        const raw = data as Record<string, unknown>;
        const list: TenantInfo[] = (
          (raw.data ?? raw.tenants ?? data) as Record<string, unknown>[]
        ).map((t: Record<string, unknown>) => ({
          id: t.id as string,
          name: (t.name as string) ?? '',
          status: (t.status as string) ?? '',
          flagship: (t.flagship as boolean) ?? false,
          governance_tier: (t.governance_tier as string) ?? '',
        }));

        setTenants(list);

        const savedId = localStorage.getItem(STORAGE_KEY);
        const saved = savedId ? list.find(t => t.id === savedId) ?? null : null;
        const defaultTenant =
          saved ??
          list.find(t => t.status === 'ACTIVE') ??
          list[0] ??
          null;

        if (defaultTenant) {
          setCurrentTenantState(defaultTenant);
          setApiTenant(defaultTenant.id);
          localStorage.setItem(STORAGE_KEY, defaultTenant.id);
        }
      })
      .catch(_err => {
        // Tenant fetch failed — leave isLoading false so the app can still render
      })
      .finally(() => {
        setIsLoading(false);
      });
  }, []);

  const setCurrentTenant = useCallback(
    (id: string) => {
      const tenant = tenants.find(t => t.id === id) ?? null;
      if (!tenant) return;
      setCurrentTenantState(tenant);
      setApiTenant(tenant.id);
      localStorage.setItem(STORAGE_KEY, tenant.id);
    },
    [tenants],
  );

  return (
    <TenantContext.Provider value={{ currentTenant, tenants, setCurrentTenant, isLoading }}>
      {children}
    </TenantContext.Provider>
  );
}

export function useTenant(): TenantContextType {
  const ctx = useContext(TenantContext);
  if (!ctx) throw new Error('useTenant must be used within TenantProvider');
  return ctx;
}
