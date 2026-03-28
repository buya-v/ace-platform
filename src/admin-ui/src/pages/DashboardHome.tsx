import React from 'react';
import { useAuth } from '../contexts/AuthContext';
import { hasAdminAccess } from '../types';
import { usePolling } from '../hooks/usePolling';
import { fetchHealth, fetchMarginCallStats, fetchParticipants, fetchSettlementCycles } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import styles from './DashboardHome.module.css';

export function DashboardHome() {
  const { state: auth } = useAuth();
  const isAdmin = hasAdminAccess(auth.user?.roles ?? []);

  const health = usePolling(
    (signal) => fetchHealth(signal),
    15000,
    isAdmin,
  );

  const marginStats = usePolling(
    (signal) => fetchMarginCallStats(signal),
    10000,
    isAdmin,
  );

  const participants = usePolling(
    (signal) => fetchParticipants({ status: 'PENDING', limit: 1 }, signal),
    30000,
  );

  const settlements = usePolling(
    (signal) => fetchSettlementCycles({ status: 'OPEN' }, signal),
    15000,
    isAdmin,
  );

  return (
    <div>
      <h1 className={styles.title}>Dashboard Overview</h1>
      <div className={styles.grid}>
        {isAdmin && (
          <div className={styles.card}>
            <h3>System Status</h3>
            {health.data ? (
              <StatusBadge status={health.data.overall_status} variant="health" />
            ) : (
              <span>Loading...</span>
            )}
          </div>
        )}

        <div className={styles.card}>
          <h3>Pending KYC</h3>
          <div className={styles.bigNumber}>
            {participants.data?.pagination?.total ?? '—'}
          </div>
        </div>

        {isAdmin && (
          <>
            <div className={styles.card}>
              <h3>Active Margin Calls</h3>
              <div className={styles.bigNumber}>
                {marginStats.data?.total_active ?? '—'}
              </div>
            </div>

            <div className={styles.card}>
              <h3>Settlement Cycles</h3>
              <div className={styles.bigNumber}>
                {settlements.data?.data?.length ?? '—'}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
