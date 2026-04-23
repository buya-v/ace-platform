import React, { useRef, useState, useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import { useToast } from '../contexts/ToastContext';
import { hasAdminAccess, AuditEvent } from '../types';
import { usePolling } from '../hooks/usePolling';
import { Skeleton } from '../components/Skeleton';
import {
  fetchHealth,
  fetchMarginCallStats,
  fetchParticipants,
  fetchSettlementCycles,
  fetchAuditTrail,
  triggerSettlementCycle,
  massCancel,
} from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { Sparkline } from '../components/Sparkline';
import { ConfirmDialog } from '../components/ConfirmDialog';
import styles from './DashboardHome.module.css';

const MAX_HISTORY = 20;

function useMetricHistory(value: number | undefined): number[] {
  const historyRef = useRef<number[]>([]);
  useEffect(() => {
    if (value === undefined) return;
    const h = historyRef.current;
    h.push(value);
    if (h.length > MAX_HISTORY) {
      h.shift();
    }
  }, [value]);
  return historyRef.current;
}

export function DashboardHome() {
  const { state: auth } = useAuth();
  const { showToast } = useToast();
  const isAdmin = hasAdminAccess(auth.user?.roles ?? []);

  const [showHaltConfirm, setShowHaltConfirm] = useState(false);
  const [recentActivity, setRecentActivity] = useState<AuditEvent[]>([]);

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


  // Sparkline histories
  const serviceCountHistory = useMetricHistory(health.data?.services?.length);
  const pendingKycHistory = useMetricHistory(
    participants.data?.pagination?.total,
  );
  const marginCallHistory = useMetricHistory(marginStats.data?.total_active);
  const settlementHistory = useMetricHistory(settlements.data?.data?.length);

  // Recent activity polling
  useEffect(() => {
    if (!isAdmin) return;
    const controller = new AbortController();
    fetchAuditTrail({ page: 1 }, controller.signal)
      .then((res) => {
        setRecentActivity((res.data ?? []).slice(0, 5));
      })
      .catch(() => {
        /* ignore */
      });
    const interval = setInterval(() => {
      const c = new AbortController();
      fetchAuditTrail({ page: 1 }, c.signal)
        .then((res) => {
          setRecentActivity((res.data ?? []).slice(0, 5));
        })
        .catch(() => {
          /* ignore */
        });
    }, 30000);
    return () => {
      controller.abort();
      clearInterval(interval);
    };
  }, [isAdmin]);

  const handleTriggerSettlement = async () => {
    try {
      await triggerSettlementCycle();
      showToast('Settlement cycle triggered', 'success');
    } catch {
      showToast('Failed to trigger settlement cycle', 'error');
    }
  };

  const handleHaltTrading = async () => {
    try {
      await massCancel();
      showToast('All trading halted', 'success');
    } catch {
      showToast('Failed to halt trading', 'error');
    } finally {
      setShowHaltConfirm(false);
    }
  };

  return (
    <div>
      <h1 className={styles.title}>Dashboard Overview</h1>

      {/* KPI Cards */}
      <div className={styles.grid}>
        {isAdmin && (
          <div className={styles.card}>
            <h3>System Status</h3>
            {health.isLoading && !health.data ? (
              <Skeleton variant="text" height="24px" count={2} />
            ) : health.data ? (
              <>
                <StatusBadge status={health.data.overall_status} variant="health" />
                <Sparkline
                  data={serviceCountHistory}
                  color="var(--accent-green)"
                  fillColor="var(--accent-green)"
                />
              </>
            ) : null}
          </div>
        )}

        <div className={styles.card}>
          <h3>Pending KYC</h3>
          {participants.isLoading && !participants.data ? (
            <Skeleton variant="text" height="32px" />
          ) : (
            <>
              <div className={styles.bigNumber}>
                {participants.data?.pagination?.total ?? '\u2014'}
              </div>
              <Sparkline
                data={pendingKycHistory}
                color="var(--accent-yellow)"
                fillColor="var(--accent-yellow)"
              />
            </>
          )}
        </div>

        {isAdmin && (
          <>
            <div className={styles.card}>
              <h3>Active Margin Calls</h3>
              <div className={styles.bigNumber}>
                {marginStats.data?.total_active ?? '\u2014'}
              </div>
              <Sparkline
                data={marginCallHistory}
                color="var(--accent-red)"
                fillColor="var(--accent-red)"
              />
            </div>

            <div className={styles.card}>
              <h3>Settlement Cycles</h3>
              <div className={styles.bigNumber}>
                {settlements.data?.data?.length ?? '\u2014'}
              </div>
              <Sparkline
                data={settlementHistory}
                color="var(--accent-blue)"
                fillColor="var(--accent-blue)"
              />
            </div>
          </>
        )}
      </div>

      {/* Quick Actions */}
      {isAdmin && (
        <div className={styles.actionsSection}>
          <h2 className={styles.sectionTitle}>Quick Actions</h2>
          <div className={styles.actionsRow}>
            <button
              className={styles.actionBtn}
              onClick={handleTriggerSettlement}
            >
              Trigger Settlement
            </button>
            <button
              className={`${styles.actionBtn} ${styles.actionBtnDanger}`}
              onClick={() => setShowHaltConfirm(true)}
            >
              Halt All Trading
            </button>
          </div>
        </div>
      )}

      {/* Recent Activity */}
      {isAdmin && recentActivity.length > 0 && (
        <div className={styles.activitySection}>
          <h2 className={styles.sectionTitle}>Recent Activity</h2>
          <div className={styles.activityList}>
            {recentActivity.map((event) => (
              <div key={event.id} className={styles.activityRow}>
                <span className={styles.activityTime}>
                  {new Date(event.timestamp).toLocaleTimeString()}
                </span>
                <span className={styles.activityBadge}>{event.action}</span>
                <span className={styles.activityActor}>{event.actor}</span>
                <span className={styles.activityDesc}>
                  {event.target_type} {event.target_id}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Halt confirm dialog */}
      {showHaltConfirm && (
        <ConfirmDialog
          title="Halt All Trading"
          message="This will cancel all open orders and halt trading on every instrument. This action cannot be undone."
          confirmLabel="Halt Trading"
          requireTypedConfirmation="HALT"
          onConfirm={handleHaltTrading}
          onCancel={() => setShowHaltConfirm(false)}
        />
      )}
    </div>
  );
}
