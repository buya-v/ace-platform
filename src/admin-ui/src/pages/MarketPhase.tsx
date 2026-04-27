import React, { useState, useEffect } from 'react';
import { apiFetch, fetchSecuritiesInstruments } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { useToast } from '../contexts/ToastContext';
import styles from './MarketPhase.module.css';

export interface MarketInstrument {
  instrument_id: string;
  name: string;
  ticker: string;
  phase: string;
  trading_status: string;
  last_updated: string;
}

export function normalizeInstruments(raw: any): MarketInstrument[] {
  const list = raw?.data ?? raw?.instruments ?? (Array.isArray(raw) ? raw : []);
  return list.map((item: any) => ({
    instrument_id: item.id ?? item.instrument_id ?? '',
    name: item.name ?? '',
    ticker: item.ticker ?? '',
    phase: 'CLOSED', // will be overridden by session data
    trading_status: item.trading_status ?? 'ACTIVE',
    last_updated: item.updated_at ?? item.last_updated ?? '',
  }));
}

export function getPhaseAction(phase: string): 'halt' | 'resume' | null {
  if (phase === 'TRADING' || phase === 'CONTINUOUS') return 'halt';
  if (phase === 'HALTED') return 'resume';
  return null;
}

export function MarketPhasePage() {
  const { showToast } = useToast();
  const [instruments, setInstruments] = useState<MarketInstrument[]>([]);
  const [dayState, setDayState] = useState<string>('UNKNOWN');
  const [isLoading, setIsLoading] = useState(true);
  const [actionTarget, setActionTarget] = useState<MarketInstrument | null>(null);
  const [showHaltAll, setShowHaltAll] = useState(false);
  const [haltAllProgress, setHaltAllProgress] = useState<{ done: number; total: number } | null>(null);

  const fetchData = async () => {
    setIsLoading(true);
    try {
      const [instrRes, bondRes, dayRes, sessRes] = await Promise.allSettled([
        fetchSecuritiesInstruments(),
        apiFetch<{ data: any[] }>('/securities/bonds'),
        apiFetch<{ state: string }>('/securities/day/status'),
        apiFetch<{ sessions: Record<string, string> }>('/securities/sessions'),
      ]);

      // Parse instruments + bonds
      let instList: MarketInstrument[] = [];
      if (instrRes.status === 'fulfilled') {
        instList = normalizeInstruments(instrRes.value);
      }
      if (bondRes.status === 'fulfilled') {
        const bonds = ((bondRes.value as any)?.data ?? []).map((b: any) => ({
          instrument_id: b.id,
          name: b.name,
          ticker: b.id,
          phase: 'CLOSED',
          trading_status: b.trading_status || 'ACTIVE',
          last_updated: b.updated_at || '',
        }));
        instList = [...instList, ...bonds];
      }

      // Parse day state
      if (dayRes.status === 'fulfilled') {
        setDayState(dayRes.value.state || 'UNKNOWN');
      }

      // Map session states to instruments
      if (sessRes.status === 'fulfilled') {
        const sessions = sessRes.value.sessions || {};
        instList = instList.map((inst) => ({
          ...inst,
          phase: sessions[inst.instrument_id] || 'CLOSED',
        }));
      }

      setInstruments(instList);
    } catch {
      // silently fail
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000);
    return () => clearInterval(interval);
  }, []);

  const handleAction = async () => {
    if (!actionTarget) return;
    const action = getPhaseAction(actionTarget.phase);
    try {
      if (action === 'halt') {
        await apiFetch<void>(`/securities/instruments/${actionTarget.instrument_id}/status`, {
          method: 'PUT',
          body: JSON.stringify({ trading_status: 'HALTED' }),
        });
        showToast(`Trading halted for ${actionTarget.ticker || actionTarget.instrument_id}`, 'success');
      } else if (action === 'resume') {
        await apiFetch<void>(`/securities/instruments/${actionTarget.instrument_id}/status`, {
          method: 'PUT',
          body: JSON.stringify({ trading_status: 'ACTIVE' }),
        });
        showToast(`Trading resumed for ${actionTarget.ticker || actionTarget.instrument_id}`, 'success');
      }
      setActionTarget(null);
      fetchData();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Action failed', 'error');
    }
  };

  const handleHaltAll = async () => {
    const tradingInstruments = instruments.filter(i => i.phase === 'TRADING' || i.phase === 'CONTINUOUS' || i.phase === 'PRE_OPEN');
    const total = tradingInstruments.length;
    if (total === 0) {
      setShowHaltAll(false);
      return;
    }

    setShowHaltAll(false);
    setHaltAllProgress({ done: 0, total });

    try {
      for (let i = 0; i < tradingInstruments.length; i++) {
        await apiFetch<void>(`/securities/instruments/${tradingInstruments[i].instrument_id}/status`, {
          method: 'PUT',
          body: JSON.stringify({ trading_status: 'HALTED' }),
        });
        setHaltAllProgress({ done: i + 1, total });
      }
      showToast(`All ${total} instruments halted`, 'success');
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to halt all', 'error');
    }

    setHaltAllProgress(null);
    fetchData();
  };

  const tradingCount = instruments.filter(i => i.phase === 'TRADING' || i.phase === 'CONTINUOUS').length;

  const columns: Column<MarketInstrument>[] = [
    { key: 'ticker', header: 'Ticker', sortable: true, mono: true },
    { key: 'name', header: 'Name', sortable: true },
    {
      key: 'phase',
      header: 'Session Phase',
      render: (row) => <StatusBadge status={row.phase} />,
    },
    {
      key: 'trading_status',
      header: 'Trading Status',
      render: (row) => <StatusBadge status={row.trading_status} />,
    },
    {
      key: 'last_updated',
      header: 'Last Updated',
      sortable: true,
      render: (row) => row.last_updated ? new Date(row.last_updated).toLocaleString() : '—',
    },
    {
      key: 'actions',
      header: 'Actions',
      render: (row) => {
        const action = getPhaseAction(row.phase);
        if (action === 'halt') {
          return (
            <button className={styles.haltBtn} onClick={() => setActionTarget(row)}>
              Halt
            </button>
          );
        }
        if (action === 'resume') {
          return (
            <button className={styles.resumeBtn} onClick={() => setActionTarget(row)}>
              Resume
            </button>
          );
        }
        return <span className={styles.disabledBtn}>—</span>;
      },
    },
  ];

  return (
    <div>
      <div className={styles.header}>
        <h1>Market Phase Control</h1>
        <div className={styles.dayState}>
          Day State: <StatusBadge status={dayState} />
        </div>
        <button
          className={styles.haltAllBtn}
          onClick={() => setShowHaltAll(true)}
          disabled={tradingCount === 0 || haltAllProgress !== null}
          data-testid="halt-all-btn"
        >
          Halt All Markets
        </button>
      </div>

      {haltAllProgress && (
        <div className={styles.progress} data-testid="halt-all-progress">
          Halting markets... {haltAllProgress.done}/{haltAllProgress.total}
          <div className={styles.progressBar}>
            <div
              className={styles.progressFill}
              style={{ width: `${(haltAllProgress.done / haltAllProgress.total) * 100}%` }}
            />
          </div>
        </div>
      )}

      <DataGrid
        columns={columns}
        data={instruments}
        keyField="instrument_id"
        emptyMessage="No instruments found. Run the demo runbook to create instruments."
        loading={isLoading}
      />

      {actionTarget && (
        <ConfirmDialog
          title={getPhaseAction(actionTarget.phase) === 'halt' ? 'Halt Trading' : 'Resume Trading'}
          message={
            getPhaseAction(actionTarget.phase) === 'halt'
              ? `Are you sure you want to halt trading for ${actionTarget.ticker}?`
              : `Are you sure you want to resume trading for ${actionTarget.ticker}?`
          }
          confirmLabel={getPhaseAction(actionTarget.phase) === 'halt' ? 'Halt' : 'Resume'}
          onConfirm={handleAction}
          onCancel={() => setActionTarget(null)}
        />
      )}

      {showHaltAll && (
        <ConfirmDialog
          title="Halt All Markets"
          message={`This will halt trading for all ${tradingCount} currently trading instrument(s). Type HALT ALL to confirm.`}
          confirmLabel="Halt All"
          requireTypedConfirmation="HALT ALL"
          onConfirm={handleHaltAll}
          onCancel={() => setShowHaltAll(false)}
        />
      )}
    </div>
  );
}
