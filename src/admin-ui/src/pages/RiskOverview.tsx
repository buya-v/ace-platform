import React, { useMemo, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchMarginCallStats, fetchMarginCalls, fetchPositions } from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import { MarginCall, MarginCallStats } from '../types';
import styles from './RiskOverview.module.css';

interface ParticipantUtilization {
  participant: string;
  required: number;
  current: number;
  pct: number;
}

interface ConcentrationBox {
  participant_id: string;
  count: number;
  value: number;
}

function parseNum(v: unknown): number {
  if (typeof v === 'number') return v;
  if (typeof v === 'string') return parseFloat(v) || 0;
  return 0;
}

function utilizationColor(pct: number): string {
  if (pct >= 90) return 'var(--accent-red)';
  if (pct >= 70) return 'var(--accent-yellow)';
  return 'var(--accent-green)';
}

function deadlineClass(deadline: string): string {
  const now = Date.now();
  const dl = new Date(deadline).getTime();
  const hoursLeft = (dl - now) / (1000 * 60 * 60);
  if (hoursLeft < 1) return styles.deadlineCritical;
  if (hoursLeft < 4) return styles.deadlineWarning;
  return '';
}

function formatDeadline(deadline: string): string {
  const now = Date.now();
  const dl = new Date(deadline).getTime();
  const diffMs = dl - now;
  if (diffMs <= 0) return 'EXPIRED';
  const hours = Math.floor(diffMs / (1000 * 60 * 60));
  const mins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60));
  if (hours > 24) return new Date(deadline).toLocaleString();
  return `${hours}h ${mins}m`;
}

export function RiskOverviewPage() {
  const stats = usePolling<MarginCallStats>(
    useCallback((signal: AbortSignal) => fetchMarginCallStats(signal), []),
    10000,
  );

  const calls = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchMarginCalls(signal), []),
    10000,
  );

  const positions = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchPositions(signal), []),
    10000,
  );

  // Parse margin calls
  const marginCalls: MarginCall[] = useMemo(() => {
    const raw = calls.data;
    if (Array.isArray(raw)) return raw;
    if (raw && typeof raw === 'object') {
      const arr = (raw as any).data ?? (raw as any).calls ?? (raw as any).items ?? [];
      return Array.isArray(arr) ? arr : [];
    }
    return [];
  }, [calls.data]);

  // Utilization by participant (from margin calls)
  const utilizations: ParticipantUtilization[] = useMemo(() => {
    const map = new Map<string, { required: number; current: number }>();
    for (const mc of marginCalls) {
      const name = mc.participant_name ?? mc.participant_id;
      const existing = map.get(name) ?? { required: 0, current: 0 };
      existing.required += parseNum(mc.required_margin);
      existing.current += parseNum(mc.current_margin);
      map.set(name, existing);
    }
    return Array.from(map.entries())
      .map(([participant, { required, current }]) => ({
        participant,
        required,
        current,
        pct: required > 0 ? Math.min((current / required) * 100, 100) : 0,
      }))
      .sort((a, b) => b.pct - a.pct);
  }, [marginCalls]);

  // Concentration from positions
  const concentrations: ConcentrationBox[] = useMemo(() => {
    const raw = positions.data;
    let arr: any[] = [];
    if (Array.isArray(raw)) {
      arr = raw;
    } else if (raw && typeof raw === 'object') {
      arr = (raw as any).data ?? (raw as any).positions ?? (raw as any).items ?? [];
      if (!Array.isArray(arr)) arr = [];
    }
    const map = new Map<string, { count: number; value: number }>();
    for (const p of arr) {
      const pid = p.participant_id ?? p.participantId ?? 'unknown';
      const existing = map.get(pid) ?? { count: 0, value: 0 };
      existing.count += 1;
      existing.value += Math.abs(parseNum(p.net_quantity ?? p.quantity ?? 0) * parseNum(p.avg_price ?? p.price ?? 0));
      map.set(pid, existing);
    }
    return Array.from(map.entries())
      .map(([participant_id, { count, value }]) => ({ participant_id, count, value }))
      .sort((a, b) => b.value - a.value);
  }, [positions.data]);

  const maxConcentration = useMemo(() => {
    return Math.max(...concentrations.map(c => c.value), 1);
  }, [concentrations]);

  // Summary stats
  const statsData = stats.data;
  const totalMarginPool = useMemo(() => {
    return marginCalls.reduce((sum, mc) => sum + parseNum(mc.current_margin), 0);
  }, [marginCalls]);

  const highestUtilParticipant = utilizations.length > 0 ? utilizations[0] : null;

  const callColumns: Column<MarginCall>[] = useMemo(() => [
    { key: 'participant_name', header: 'Participant', sortable: true },
    { key: 'instrument_id', header: 'Instrument', sortable: true },
    { key: 'required_margin', header: 'Required', align: 'right', mono: true, sortable: true },
    { key: 'current_margin', header: 'Current', align: 'right', mono: true, sortable: true },
    { key: 'shortfall', header: 'Shortfall', align: 'right', mono: true, sortable: true },
    { key: 'status', header: 'Status', sortable: true },
    {
      key: 'deadline',
      header: 'Deadline',
      sortable: true,
      render: (row) => (
        <span className={deadlineClass(row.deadline)}>
          {formatDeadline(row.deadline)}
        </span>
      ),
    },
  ], []);

  return (
    <div className={styles.page}>
      <h1 className={styles.pageTitle}>Risk Overview</h1>

      {/* Summary Cards */}
      <div className={styles.statsGrid}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Total Margin Pool</div>
          <div className={styles.statValue}>
            {totalMarginPool.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Active Margin Calls</div>
          <div className={styles.statValue}>{statsData?.total_active ?? marginCalls.length}</div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Avg Utilization</div>
          <div className={styles.statValue}>
            {statsData?.average_utilization != null
              ? `${statsData.average_utilization}%`
              : '-'}
          </div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Highest Utilization</div>
          <div className={styles.statValue}>
            {highestUtilParticipant
              ? `${highestUtilParticipant.participant} (${highestUtilParticipant.pct.toFixed(1)}%)`
              : '-'}
          </div>
        </div>
      </div>

      {/* Utilization Bars */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Margin Utilization by Participant</h2>
        {utilizations.length === 0 ? (
          <div className={styles.emptyState}>No utilization data</div>
        ) : (
          utilizations.map((u) => (
            <div key={u.participant} className={styles.barRow}>
              <span className={styles.barLabel}>{u.participant}</span>
              <div className={styles.barTrack}>
                <div
                  className={styles.barFill}
                  style={{ width: `${u.pct}%`, background: utilizationColor(u.pct) }}
                />
              </div>
              <span className={styles.barPct} style={{ color: utilizationColor(u.pct) }}>
                {u.pct.toFixed(1)}%
              </span>
            </div>
          ))
        )}
      </div>

      {/* Concentration Grid */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Position Concentration</h2>
        {concentrations.length === 0 ? (
          <div className={styles.emptyState}>No position data</div>
        ) : (
          <div className={styles.concentrationGrid}>
            {concentrations.map((c) => {
              const intensity = Math.max(0.15, c.value / maxConcentration);
              return (
                <div
                  key={c.participant_id}
                  className={styles.concentrationBox}
                  style={{
                    background: `rgba(88, 166, 255, ${intensity * 0.3})`,
                    borderColor: `rgba(88, 166, 255, ${intensity * 0.6})`,
                    flex: `${Math.max(1, Math.round((c.value / maxConcentration) * 4))} 1 0`,
                  }}
                >
                  <span className={styles.concentrationId}>{c.participant_id}</span>
                  <span className={styles.concentrationCount}>{c.count} pos</span>
                  <span className={styles.concentrationValue}>
                    {c.value.toLocaleString(undefined, { maximumFractionDigits: 0 })}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Active Margin Calls */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Active Margin Calls</h2>
        <DataGrid
          columns={callColumns}
          data={marginCalls}
          keyField="id"
          emptyMessage="No active margin calls"
          stickyHeader
          exportFilename="margin-calls"
        />
      </div>
    </div>
  );
}
