import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchInstrumentList, haltInstrument, resumeInstrument } from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { useToast } from '../contexts/ToastContext';
import styles from './MarketPhase.module.css';

export interface MarketInstrument {
  instrument_id: string;
  name?: string;
  description?: string;
  phase: 'TRADING' | 'HALTED' | 'PRE_OPEN' | 'AUCTION';
  last_updated?: string;
}

export function normalizeInstruments(raw: any): MarketInstrument[] {
  const list = raw?.instruments ?? raw?.data ?? (Array.isArray(raw) ? raw : []);
  return list.map((item: any) => ({
    instrument_id: item.instrument_id ?? item.id ?? '',
    name: item.name ?? item.ticker ?? item.instrument_id ?? item.id ?? '',
    description: item.description ?? '',
    phase: item.phase ?? item.status ?? 'PRE_OPEN',
    last_updated: item.last_updated ?? item.updated_at ?? '',
  }));
}

export function getPhaseAction(phase: string): 'halt' | 'resume' | null {
  if (phase === 'TRADING') return 'halt';
  if (phase === 'HALTED') return 'resume';
  return null;
}

export function MarketPhasePage() {
  const { data, refresh, isLoading } = usePolling(
    (signal) => fetchInstrumentList(signal),
    10000,
  );
  const { showToast } = useToast();

  const instruments = normalizeInstruments(data);

  const [actionTarget, setActionTarget] = useState<MarketInstrument | null>(null);
  const [showHaltAll, setShowHaltAll] = useState(false);
  const [haltAllProgress, setHaltAllProgress] = useState<{ done: number; total: number } | null>(null);

  const handleAction = async () => {
    if (!actionTarget) return;
    const action = getPhaseAction(actionTarget.phase);
    try {
      if (action === 'halt') {
        await haltInstrument(actionTarget.instrument_id);
        showToast(`Market phase changed to HALTED for ${actionTarget.name ?? actionTarget.instrument_id}`, 'success');
      } else if (action === 'resume') {
        await resumeInstrument(actionTarget.instrument_id);
        showToast(`Market phase changed to TRADING for ${actionTarget.name ?? actionTarget.instrument_id}`, 'success');
      }
      setActionTarget(null);
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Action failed', 'error');
    }
  };

  const handleHaltAll = async () => {
    const tradingInstruments = instruments.filter(i => i.phase === 'TRADING');
    const total = tradingInstruments.length;
    if (total === 0) {
      setShowHaltAll(false);
      return;
    }

    setShowHaltAll(false);
    setHaltAllProgress({ done: 0, total });

    try {
      for (let i = 0; i < tradingInstruments.length; i++) {
        await haltInstrument(tradingInstruments[i].instrument_id);
        setHaltAllProgress({ done: i + 1, total });
      }
      showToast(`All ${total} markets halted`, 'success');
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to halt all markets', 'error');
    }

    setHaltAllProgress(null);
    refresh();
  };

  const tradingCount = instruments.filter(i => i.phase === 'TRADING').length;

  const columns: Column<MarketInstrument>[] = [
    { key: 'instrument_id', header: 'Instrument ID', sortable: true, mono: true },
    { key: 'name', header: 'Name', sortable: true },
    {
      key: 'phase',
      header: 'Current Phase',
      render: (row) => <StatusBadge status={row.phase} />,
    },
    { key: 'last_updated', header: 'Last Updated', sortable: true },
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
        return (
          <button className={styles.disabledBtn} disabled>
            —
          </button>
        );
      },
    },
  ];

  return (
    <div>
      <div className={styles.header}>
        <h1>Market Phase Control</h1>
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
        emptyMessage="No instruments found"
        loading={isLoading}
      />

      {actionTarget && (
        <ConfirmDialog
          title={getPhaseAction(actionTarget.phase) === 'halt' ? 'Halt Trading' : 'Resume Trading'}
          message={
            getPhaseAction(actionTarget.phase) === 'halt'
              ? `Are you sure you want to halt trading for ${actionTarget.name ?? actionTarget.instrument_id}?`
              : `Are you sure you want to resume trading for ${actionTarget.name ?? actionTarget.instrument_id}?`
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
