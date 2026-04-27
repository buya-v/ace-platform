import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSecuritiesInstruments, apiFetch, haltTrading, resumeTrading, setCircuitBreaker } from '../services/api';
import { InstrumentControl } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { useToast } from '../contexts/ToastContext';
import styles from './CircuitBreakers.module.css';

export function CircuitBreakersPage() {
  const { data, refresh, isLoading } = usePolling(
    async (signal) => {
      const [instrRes, bondRes] = await Promise.allSettled([
        fetchSecuritiesInstruments(undefined, signal),
        apiFetch<{ data: any[] }>('/securities/bonds', {}, signal),
      ]);
      const equities = instrRes.status === 'fulfilled' ? ((instrRes.value as any)?.data ?? []) : [];
      const bonds = bondRes.status === 'fulfilled' ? ((bondRes.value as any)?.data ?? []) : [];
      const all = [...equities, ...bonds].map((i: any) => ({
        instrument_id: i.id,
        ticker: i.ticker || i.id,
        last_price: '0',
        upper_limit: '10',
        lower_limit: '10',
        status: (i.trading_status === 'HALTED' ? 'HALTED' : (i.trading_status === 'ACTIVE' ? 'TRADING' : 'PRE_OPEN')) as InstrumentControl['status'],
        daily_volume: 0,
      }));
      return { data: all };
    },
    10000,
  );
  const { showToast } = useToast();

  const [haltTarget, setHaltTarget] = useState<InstrumentControl | null>(null);
  const [editTarget, setEditTarget] = useState<InstrumentControl | null>(null);
  const [editForm, setEditForm] = useState({ upper: '', lower: '', cooldown: '', refPrice: '' });

  const instruments = data?.data ?? [];

  const handleHalt = async () => {
    if (!haltTarget) return;
    try {
      if (haltTarget.status === 'HALTED') {
        await resumeTrading(haltTarget.instrument_id);
        showToast(`Trading resumed for ${haltTarget.ticker}`, 'success');
      } else {
        await haltTrading(haltTarget.instrument_id);
        showToast(`Trading halted for ${haltTarget.ticker}`, 'success');
      }
      setHaltTarget(null);
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Action failed', 'error');
    }
  };

  const [formErrors, setFormErrors] = useState<Record<string, string>>({});

  const validateEditForm = (): boolean => {
    const errors: Record<string, string> = {};
    const upper = parseFloat(editForm.upper);
    const lower = parseFloat(editForm.lower);
    const cooldown = parseInt(editForm.cooldown, 10);
    if (editForm.upper.trim() === '' || isNaN(upper)) {
      errors.upper = 'Upper limit must be a number';
    } else if (upper <= 0 || upper > 100) {
      errors.upper = 'Upper limit must be between 0 and 100';
    }
    if (editForm.lower.trim() === '' || isNaN(lower)) {
      errors.lower = 'Lower limit must be a number';
    } else if (lower <= 0 || lower > 100) {
      errors.lower = 'Lower limit must be between 0 and 100';
    }
    if (editForm.cooldown.trim() === '' || isNaN(cooldown)) {
      errors.cooldown = 'Cooldown must be a whole number';
    } else if (cooldown < 1) {
      errors.cooldown = 'Cooldown must be at least 1 minute';
    }
    if (editForm.refPrice.trim() === '' || isNaN(parseFloat(editForm.refPrice))) {
      errors.refPrice = 'Reference price must be a number';
    }
    setFormErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleEditSave = async () => {
    if (!editTarget) return;
    if (!validateEditForm()) return;
    try {
      await setCircuitBreaker(editTarget.instrument_id, {
        upper_limit_pct: parseFloat(editForm.upper),
        lower_limit_pct: parseFloat(editForm.lower),
        cooldown_minutes: parseInt(editForm.cooldown, 10),
        reference_price: editForm.refPrice,
      });
      setEditTarget(null);
      setFormErrors({});
      refresh();
      showToast('Circuit breaker limits updated', 'success');
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to update limits', 'error');
    }
  };

  const columns: Column<InstrumentControl>[] = [
    { key: 'ticker', header: 'Instrument', sortable: true, mono: true },
    { key: 'last_price', header: 'Last Price', align: 'right', mono: true },
    { key: 'upper_limit', header: 'Upper Limit', align: 'right', mono: true },
    { key: 'lower_limit', header: 'Lower Limit', align: 'right', mono: true },
    { key: 'status', header: 'Status', render: (row) => <StatusBadge status={row.status} /> },
    { key: 'daily_volume', header: 'Daily Volume', align: 'right', mono: true, sortable: true, render: (row) => (row.daily_volume ?? 0).toLocaleString() },
    {
      key: 'actions', header: 'Actions', render: (row) => (
        <div className={styles.actionBtns}>
          <button
            className={row.status === 'HALTED' ? styles.resumeBtn : styles.haltBtn}
            onClick={() => setHaltTarget(row)}
          >
            {row.status === 'HALTED' ? 'Resume' : 'Halt'}
          </button>
          <button className={styles.editBtn} onClick={() => {
            setEditTarget(row);
            setEditForm({
              upper: row.upper_limit,
              lower: row.lower_limit,
              cooldown: '5',
              refPrice: row.last_price,
            });
          }}>
            Edit Limits
          </button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <h1>Circuit Breakers</h1>

      <DataGrid
        columns={columns}
        data={instruments}
        keyField="instrument_id"
        emptyMessage="No instruments found"
        loading={isLoading}
      />

      {haltTarget && (
        <ConfirmDialog
          title={haltTarget.status === 'HALTED' ? 'Resume Trading' : 'Halt Trading'}
          message={
            haltTarget.status === 'HALTED'
              ? `Resume trading for ${haltTarget.ticker}?`
              : `Halt trading for ${haltTarget.ticker}? This will cancel all open orders.`
          }
          confirmLabel={haltTarget.status === 'HALTED' ? 'Resume' : 'Halt'}
          requireTypedConfirmation={haltTarget.status !== 'HALTED' ? haltTarget.ticker : undefined}
          onConfirm={handleHalt}
          onCancel={() => setHaltTarget(null)}
        />
      )}

      {editTarget && (
        <div className={styles.overlay} onClick={() => { setEditTarget(null); setFormErrors({}); }}>
          <div className={styles.modal} onClick={e => e.stopPropagation()} role="dialog" aria-modal="true" aria-label={`Edit price limits for ${editTarget.ticker}`}>
            <h3>Edit Price Limits — {editTarget.ticker}</h3>
            <label className={styles.formLabel}>
              Upper Limit %
              <input type="number" value={editForm.upper} onChange={e => setEditForm(f => ({ ...f, upper: e.target.value }))} className={styles.formInput} aria-invalid={!!formErrors.upper} />
              {formErrors.upper && <span className={styles.fieldError}>{formErrors.upper}</span>}
            </label>
            <label className={styles.formLabel}>
              Lower Limit %
              <input type="number" value={editForm.lower} onChange={e => setEditForm(f => ({ ...f, lower: e.target.value }))} className={styles.formInput} aria-invalid={!!formErrors.lower} />
              {formErrors.lower && <span className={styles.fieldError}>{formErrors.lower}</span>}
            </label>
            <label className={styles.formLabel}>
              Cooldown (minutes)
              <input type="number" value={editForm.cooldown} onChange={e => setEditForm(f => ({ ...f, cooldown: e.target.value }))} className={styles.formInput} aria-invalid={!!formErrors.cooldown} />
              {formErrors.cooldown && <span className={styles.fieldError}>{formErrors.cooldown}</span>}
            </label>
            <label className={styles.formLabel}>
              Reference Price
              <input type="text" value={editForm.refPrice} onChange={e => setEditForm(f => ({ ...f, refPrice: e.target.value }))} className={styles.formInput} aria-invalid={!!formErrors.refPrice} />
              {formErrors.refPrice && <span className={styles.fieldError}>{formErrors.refPrice}</span>}
            </label>
            <div className={styles.modalActions}>
              <button onClick={() => { setEditTarget(null); setFormErrors({}); }} className={styles.cancelBtn}>Cancel</button>
              <button onClick={handleEditSave} className={styles.saveBtn}>Save</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
