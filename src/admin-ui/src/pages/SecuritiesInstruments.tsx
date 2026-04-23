import React, { useState } from 'react';
import { usePolling } from '../hooks/usePolling';
import {
  fetchSecuritiesInstruments,
  createSecuritiesInstrument,
  updateInstrumentStatus,
} from '../services/api';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { DataGrid, Column } from '../components/DataGrid';
import { useToast } from '../contexts/ToastContext';
import styles from './SecuritiesInstruments.module.css';

// ─── Types ───────────────────────────────────────────────────────────────────

export interface SecuritiesInstrument {
  id: string;
  ticker: string;
  name: string;
  asset_class: string;
  lot_size: number;
  tick_size: number;
  currency: string;
  exchange_code: string;
  trading_status: string;
}

export interface CreateInstrumentForm {
  ticker: string;
  name: string;
  asset_class: string;
  lot_size: string;
  tick_size: string;
  currency: string;
  exchange_code: string;
}

export interface FormErrors {
  ticker?: string;
  name?: string;
  asset_class?: string;
  lot_size?: string;
  tick_size?: string;
}

// ─── Pure validation (exported for testability) ──────────────────────────────

export function validateInstrumentForm(form: CreateInstrumentForm): FormErrors {
  const errors: FormErrors = {};

  if (!form.ticker.trim()) {
    errors.ticker = 'Ticker is required';
  }

  if (!form.name.trim()) {
    errors.name = 'Name is required';
  }

  if (!form.asset_class) {
    errors.asset_class = 'Asset class is required';
  }

  const lotSize = parseFloat(form.lot_size);
  if (!form.lot_size.trim() || isNaN(lotSize)) {
    errors.lot_size = 'Lot size must be a number';
  } else if (lotSize <= 0) {
    errors.lot_size = 'Lot size must be greater than 0';
  }

  const tickSize = parseFloat(form.tick_size);
  if (!form.tick_size.trim() || isNaN(tickSize)) {
    errors.tick_size = 'Tick size must be a number';
  } else if (tickSize <= 0) {
    errors.tick_size = 'Tick size must be greater than 0';
  }

  return errors;
}

// ─── Component ────────────────────────────────────────────────────────────────

const EMPTY_FORM: CreateInstrumentForm = {
  ticker: '',
  name: '',
  asset_class: '',
  lot_size: '',
  tick_size: '',
  currency: 'MNT',
  exchange_code: '',
};

export function SecuritiesInstrumentsPage() {
  const [search, setSearch] = useState('');
  const [assetClassFilter, setAssetClassFilter] = useState('');
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createForm, setCreateForm] = useState<CreateInstrumentForm>(EMPTY_FORM);
  const [formErrors, setFormErrors] = useState<FormErrors>({});
  const [statusTarget, setStatusTarget] = useState<{ instrument: SecuritiesInstrument; action: 'HALTED' | 'TRADING' } | null>(null);

  const { showToast } = useToast();

  const { data, refresh, isLoading } = usePolling(
    (signal) => fetchSecuritiesInstruments(
      assetClassFilter ? { asset_class: assetClassFilter } : undefined,
      signal,
    ),
    15000,
  );

  const rawInstruments: SecuritiesInstrument[] = (data as any)?.data ?? (data as any)?.instruments ?? (Array.isArray(data) ? data : []);

  const filtered = rawInstruments.filter(inst => {
    if (!search) return true;
    const lower = search.toLowerCase();
    return (
      inst.ticker.toLowerCase().includes(lower) ||
      inst.name.toLowerCase().includes(lower)
    );
  });

  // ─── Create modal handlers ─────────────────────────────────────────────────

  const openCreateModal = () => {
    setCreateForm(EMPTY_FORM);
    setFormErrors({});
    setCreateModalOpen(true);
  };

  const closeCreateModal = () => {
    setCreateModalOpen(false);
    setFormErrors({});
  };

  const handleCreateSubmit = async () => {
    const errors = validateInstrumentForm(createForm);
    if (Object.keys(errors).length > 0) {
      setFormErrors(errors);
      return;
    }

    try {
      await createSecuritiesInstrument({
        ticker: createForm.ticker.trim().toUpperCase(),
        name: createForm.name.trim(),
        asset_class: createForm.asset_class,
        lot_size: parseFloat(createForm.lot_size),
        tick_size: parseFloat(createForm.tick_size),
        currency: createForm.currency.trim() || 'MNT',
        exchange_code: createForm.exchange_code.trim(),
      });
      showToast(`Instrument ${createForm.ticker.toUpperCase()} created`, 'success');
      closeCreateModal();
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to create instrument', 'error');
    }
  };

  // ─── Status change handler ─────────────────────────────────────────────────

  const handleStatusConfirm = async () => {
    if (!statusTarget) return;
    try {
      await updateInstrumentStatus(statusTarget.instrument.id, statusTarget.action);
      const label = statusTarget.action === 'HALTED' ? 'halted' : 'resumed';
      showToast(`Trading ${label} for ${statusTarget.instrument.ticker}`, 'success');
      setStatusTarget(null);
      refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Status update failed', 'error');
    }
  };

  // ─── Columns ───────────────────────────────────────────────────────────────

  const columns: Column<SecuritiesInstrument>[] = [
    { key: 'ticker', header: 'Ticker', sortable: true, mono: true },
    { key: 'name', header: 'Name', sortable: true },
    {
      key: 'asset_class',
      header: 'Asset Class',
      render: (row) => <StatusBadge status={row.asset_class} />,
    },
    { key: 'lot_size', header: 'Lot Size', align: 'right', mono: true },
    { key: 'tick_size', header: 'Tick Size', align: 'right', mono: true },
    {
      key: 'trading_status',
      header: 'Status',
      render: (row) => <StatusBadge status={row.trading_status} />,
    },
    {
      key: 'actions',
      header: 'Actions',
      render: (row) => (
        <div className={styles.actionBtns}>
          {row.trading_status === 'HALTED' ? (
            <button
              className={styles.resumeBtn}
              onClick={() => setStatusTarget({ instrument: row, action: 'TRADING' })}
            >
              Resume
            </button>
          ) : (
            <button
              className={styles.haltBtn}
              onClick={() => setStatusTarget({ instrument: row, action: 'HALTED' })}
            >
              Halt
            </button>
          )}
          <button
            className={styles.editBtn}
            onClick={() => setStatusTarget({ instrument: row, action: row.trading_status === 'HALTED' ? 'TRADING' : 'HALTED' })}
          >
            Edit Status
          </button>
        </div>
      ),
    },
  ];

  // ─── Render ────────────────────────────────────────────────────────────────

  return (
    <div>
      <h1>Securities Instruments</h1>

      <div className={styles.toolbar}>
        <label className={styles.srOnly} htmlFor="inst-search">Search instruments</label>
        <input
          id="inst-search"
          type="text"
          placeholder="Search by ticker or name..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className={styles.searchInput}
        />
        <label className={styles.srOnly} htmlFor="inst-asset-class">Asset class filter</label>
        <select
          id="inst-asset-class"
          value={assetClassFilter}
          onChange={e => setAssetClassFilter(e.target.value)}
          className={styles.select}
          aria-label="Filter by asset class"
        >
          <option value="">All Asset Classes</option>
          <option value="EQUITY">EQUITY</option>
          <option value="BOND">BOND</option>
          <option value="ETF">ETF</option>
        </select>
        <button className={styles.createBtn} onClick={openCreateModal}>
          Create Instrument
        </button>
      </div>

      <DataGrid
        columns={columns}
        data={filtered}
        keyField="id"
        emptyMessage="No instruments found"
        exportFilename="securities-instruments"
        loading={isLoading}
      />

      {/* ─── Create Instrument Modal ─────────────────────────────────────── */}
      {createModalOpen && (
        <div className={styles.overlay} onClick={closeCreateModal}>
          <div
            className={styles.modal}
            onClick={e => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Create securities instrument"
          >
            <h3>Create Instrument</h3>

            <label className={styles.formLabel}>
              Ticker *
              <input
                type="text"
                value={createForm.ticker}
                onChange={e => setCreateForm(f => ({ ...f, ticker: e.target.value }))}
                className={styles.formInput}
                aria-invalid={!!formErrors.ticker}
                placeholder="e.g. AAPL"
              />
              {formErrors.ticker && <span className={styles.fieldError}>{formErrors.ticker}</span>}
            </label>

            <label className={styles.formLabel}>
              Name *
              <input
                type="text"
                value={createForm.name}
                onChange={e => setCreateForm(f => ({ ...f, name: e.target.value }))}
                className={styles.formInput}
                aria-invalid={!!formErrors.name}
                placeholder="e.g. Apple Inc."
              />
              {formErrors.name && <span className={styles.fieldError}>{formErrors.name}</span>}
            </label>

            <label className={styles.formLabel}>
              Asset Class *
              <select
                value={createForm.asset_class}
                onChange={e => setCreateForm(f => ({ ...f, asset_class: e.target.value }))}
                className={styles.formInput}
                aria-invalid={!!formErrors.asset_class}
              >
                <option value="">Select asset class...</option>
                <option value="EQUITY">EQUITY</option>
                <option value="BOND">BOND</option>
                <option value="ETF">ETF</option>
              </select>
              {formErrors.asset_class && <span className={styles.fieldError}>{formErrors.asset_class}</span>}
            </label>

            <label className={styles.formLabel}>
              Lot Size *
              <input
                type="number"
                value={createForm.lot_size}
                onChange={e => setCreateForm(f => ({ ...f, lot_size: e.target.value }))}
                className={styles.formInput}
                aria-invalid={!!formErrors.lot_size}
                placeholder="e.g. 100"
                min="0"
                step="any"
              />
              {formErrors.lot_size && <span className={styles.fieldError}>{formErrors.lot_size}</span>}
            </label>

            <label className={styles.formLabel}>
              Tick Size *
              <input
                type="number"
                value={createForm.tick_size}
                onChange={e => setCreateForm(f => ({ ...f, tick_size: e.target.value }))}
                className={styles.formInput}
                aria-invalid={!!formErrors.tick_size}
                placeholder="e.g. 0.01"
                min="0"
                step="any"
              />
              {formErrors.tick_size && <span className={styles.fieldError}>{formErrors.tick_size}</span>}
            </label>

            <label className={styles.formLabel}>
              Currency
              <input
                type="text"
                value={createForm.currency}
                onChange={e => setCreateForm(f => ({ ...f, currency: e.target.value }))}
                className={styles.formInput}
                placeholder="MNT"
              />
            </label>

            <label className={styles.formLabel}>
              Exchange Code
              <input
                type="text"
                value={createForm.exchange_code}
                onChange={e => setCreateForm(f => ({ ...f, exchange_code: e.target.value }))}
                className={styles.formInput}
                placeholder="e.g. MSE"
              />
            </label>

            <div className={styles.modalActions}>
              <button onClick={closeCreateModal} className={styles.cancelBtn}>
                Cancel
              </button>
              <button onClick={handleCreateSubmit} className={styles.saveBtn}>
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ─── Halt / Resume Confirm Dialog ───────────────────────────────── */}
      {statusTarget && (
        <ConfirmDialog
          title={statusTarget.action === 'HALTED' ? 'Halt Trading' : 'Resume Trading'}
          message={
            statusTarget.action === 'HALTED'
              ? `Halt trading for ${statusTarget.instrument.ticker}? This will cancel all open orders.`
              : `Resume trading for ${statusTarget.instrument.ticker}?`
          }
          confirmLabel={statusTarget.action === 'HALTED' ? 'Halt' : 'Resume'}
          onConfirm={handleStatusConfirm}
          onCancel={() => setStatusTarget(null)}
        />
      )}
    </div>
  );
}
