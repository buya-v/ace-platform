import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchInstruments, haltTrading, resumeTrading, setCircuitBreaker } from '../services/api';
import { InstrumentControl } from '../types';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import styles from './CircuitBreakers.module.css';

export function CircuitBreakersPage() {
  const { data, refresh } = usePolling(
    (signal) => fetchInstruments(signal),
    10000,
  );

  const [haltTarget, setHaltTarget] = useState<InstrumentControl | null>(null);
  const [editTarget, setEditTarget] = useState<InstrumentControl | null>(null);
  const [editForm, setEditForm] = useState({ upper: '', lower: '', cooldown: '', refPrice: '' });

  const instruments = data?.data ?? [];

  const handleHalt = async () => {
    if (!haltTarget) return;
    if (haltTarget.status === 'HALTED') {
      await resumeTrading(haltTarget.instrument_id);
    } else {
      await haltTrading(haltTarget.instrument_id);
    }
    setHaltTarget(null);
    refresh();
  };

  const handleEditSave = async () => {
    if (!editTarget) return;
    await setCircuitBreaker(editTarget.instrument_id, {
      upper_limit_pct: parseFloat(editForm.upper),
      lower_limit_pct: parseFloat(editForm.lower),
      cooldown_minutes: parseInt(editForm.cooldown, 10),
      reference_price: editForm.refPrice,
    });
    setEditTarget(null);
    refresh();
  };

  return (
    <div>
      <h1>Circuit Breakers</h1>

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Instrument</th>
            <th>Last Price</th>
            <th>Upper Limit</th>
            <th>Lower Limit</th>
            <th>Status</th>
            <th>Daily Volume</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {instruments.map(inst => (
            <tr key={inst.instrument_id}>
              <td className={styles.ticker}>{inst.ticker}</td>
              <td>{inst.last_price}</td>
              <td>{inst.upper_limit}</td>
              <td>{inst.lower_limit}</td>
              <td><StatusBadge status={inst.status} /></td>
              <td>{inst.daily_volume.toLocaleString()}</td>
              <td>
                <div className={styles.actionBtns}>
                  <button
                    className={inst.status === 'HALTED' ? styles.resumeBtn : styles.haltBtn}
                    onClick={() => setHaltTarget(inst)}
                  >
                    {inst.status === 'HALTED' ? 'Resume' : 'Halt'}
                  </button>
                  <button className={styles.editBtn} onClick={() => {
                    setEditTarget(inst);
                    setEditForm({
                      upper: inst.upper_limit,
                      lower: inst.lower_limit,
                      cooldown: '5',
                      refPrice: inst.last_price,
                    });
                  }}>
                    Edit Limits
                  </button>
                </div>
              </td>
            </tr>
          ))}
          {instruments.length === 0 && (
            <tr><td colSpan={7} style={{ textAlign: 'center', color: '#888', padding: 32 }}>No instruments found</td></tr>
          )}
        </tbody>
      </table>

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
        <div className={styles.overlay} onClick={() => setEditTarget(null)}>
          <div className={styles.modal} onClick={e => e.stopPropagation()} role="dialog">
            <h3>Edit Price Limits — {editTarget.ticker}</h3>
            <label className={styles.formLabel}>
              Upper Limit %
              <input type="number" value={editForm.upper} onChange={e => setEditForm(f => ({ ...f, upper: e.target.value }))} className={styles.formInput} />
            </label>
            <label className={styles.formLabel}>
              Lower Limit %
              <input type="number" value={editForm.lower} onChange={e => setEditForm(f => ({ ...f, lower: e.target.value }))} className={styles.formInput} />
            </label>
            <label className={styles.formLabel}>
              Cooldown (minutes)
              <input type="number" value={editForm.cooldown} onChange={e => setEditForm(f => ({ ...f, cooldown: e.target.value }))} className={styles.formInput} />
            </label>
            <label className={styles.formLabel}>
              Reference Price
              <input type="text" value={editForm.refPrice} onChange={e => setEditForm(f => ({ ...f, refPrice: e.target.value }))} className={styles.formInput} />
            </label>
            <div className={styles.modalActions}>
              <button onClick={() => setEditTarget(null)} className={styles.cancelBtn}>Cancel</button>
              <button onClick={handleEditSave} className={styles.saveBtn}>Save</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
