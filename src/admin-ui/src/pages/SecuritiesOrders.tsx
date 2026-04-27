import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { usePolling } from '../hooks/usePolling';
import {
  fetchSecuritiesInstruments,
  fetchSecuritiesOrders,
  submitSecuritiesOrder,
  cancelSecuritiesOrder,
  apiFetch,
} from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import { StatusBadge } from '../components/StatusBadge';
import { useToast } from '../contexts/ToastContext';
import { useTenant } from '../contexts/TenantContext';
import styles from './SecuritiesOrders.module.css';

// ─── Types ─────────────────────────────────────────────────────────────────

export interface SecuritiesInstrument {
  id: string;
  instrument_id?: string;
  ticker?: string;
  name?: string;
}

export interface SecuritiesOrder {
  _key: string;
  id: string;
  instrument_id: string;
  side: 'BUY' | 'SELL' | string;
  order_type: string;
  quantity: string;
  price: string;
  filled_quantity: string;
  status: string;
  created_at: string;
}

export interface OrderForm {
  instrument_id: string;
  side: string;
  order_type: string;
  quantity: string;
  price: string;
  time_in_force: string;
}

export interface OrderFormErrors {
  quantity?: string;
  price?: string;
}

// ─── Pure exported helpers ───────────────────────────────────────────────────

export function validateOrderForm(
  form: OrderForm,
  orderType: string,
): OrderFormErrors {
  const errors: OrderFormErrors = {};

  const qty = Number(form.quantity);
  if (!form.quantity || isNaN(qty) || qty <= 0) {
    errors.quantity = 'Quantity must be greater than 0';
  }

  if (orderType === 'LIMIT') {
    const px = Number(form.price);
    if (!form.price || isNaN(px) || px <= 0) {
      errors.price = 'Price must be greater than 0 for LIMIT orders';
    }
  }

  return errors;
}

export function normalizeInstruments(raw: unknown): SecuritiesInstrument[] {
  if (!raw) return [];
  let arr: any[] = [];
  if (Array.isArray(raw)) {
    arr = raw;
  } else if (typeof raw === 'object') {
    const o = raw as any;
    arr = o.instruments ?? o.data ?? o.items ?? [];
    if (!Array.isArray(arr)) arr = [];
  }
  return arr.map((i: any) => ({
    id: i.instrument_id ?? i.id ?? String(i),
    instrument_id: i.instrument_id ?? i.id ?? String(i),
    ticker: i.ticker ?? i.symbol ?? i.instrument_id ?? i.id ?? String(i),
    name: i.name ?? i.description ?? '',
  }));
}

export function normalizeOrders(raw: unknown): SecuritiesOrder[] {
  if (!raw) return [];
  let arr: any[] = [];
  if (Array.isArray(raw)) {
    arr = raw;
  } else if (typeof raw === 'object') {
    const o = raw as any;
    arr = o.orders ?? o.data ?? o.items ?? [];
    if (!Array.isArray(arr)) arr = [];
  }
  return arr.map((o: any, i: number) => ({
    _key: o.id ?? o.order_id ?? `order-${i}`,
    id: o.id ?? o.order_id ?? '',
    instrument_id: o.instrument_id ?? '-',
    side: String(o.side ?? '').toUpperCase() || '-',
    order_type: String(o.order_type ?? o.type ?? '').toUpperCase() || '-',
    quantity: String(o.quantity ?? '0'),
    price: String(o.price ?? '0'),
    filled_quantity: String(o.filled_quantity ?? o.filledQuantity ?? '0'),
    status: String(o.status ?? '').toUpperCase() || 'UNKNOWN',
    created_at: o.created_at ?? o.createdAt ?? o.timestamp ?? '',
  }));
}

export function formatOrderDate(dateStr: string): string {
  if (!dateStr) return '-';
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return dateStr;
  }
}

// ─── Component ────────────────────────────────────────────────────────────────

const INITIAL_FORM: OrderForm = {
  instrument_id: '',
  side: 'BUY',
  order_type: 'LIMIT',
  quantity: '',
  price: '',
  time_in_force: 'GTC',
};

export function SecuritiesOrdersPage() {
  const { showToast } = useToast();
  const { currentTenant } = useTenant();

  // Instruments for selector
  const [instruments, setInstruments] = useState<SecuritiesInstrument[]>([]);
  const [selectedInstrumentId, setSelectedInstrumentId] = useState<string>('');

  useEffect(() => {
    const controller = new AbortController();
    fetchSecuritiesInstruments(undefined, controller.signal)
      .then((res) => {
        const normalized = normalizeInstruments(res);
        setInstruments(normalized);
        if (normalized.length > 0 && !selectedInstrumentId) {
          setSelectedInstrumentId(normalized[0].id);
        }
      })
      .catch((err) => {
        if (err?.name === 'AbortError') return;
        showToast(err instanceof Error ? err.message : 'Failed to load instruments', 'error');
      });
    return () => controller.abort();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Poll orders for selected instrument
  const ordersResult = usePolling<unknown>(
    useCallback(
      (signal: AbortSignal) =>
        fetchSecuritiesOrders(
          selectedInstrumentId ? { instrument_id: selectedInstrumentId } : undefined,
          signal,
        ),
      [selectedInstrumentId],
    ),
    10000,
  );

  const orders: SecuritiesOrder[] = useMemo(
    () => normalizeOrders(ordersResult.data),
    [ordersResult.data],
  );

  // Fetch trades from securities-service
  const tradesResult = usePolling(
    (signal) => apiFetch<{ data: any[] }>('/securities/trades', {}, signal),
    15000,
  );
  const trades: any[] = useMemo(() => {
    const raw = tradesResult.data as any;
    if (!raw) return [];
    return raw.data ?? raw.trades ?? (Array.isArray(raw) ? raw : []);
  }, [tradesResult.data]);

  // Submit Order modal
  const [showModal, setShowModal] = useState(false);
  const [form, setForm] = useState<OrderForm>(INITIAL_FORM);
  const [formErrors, setFormErrors] = useState<OrderFormErrors>({});
  const [submitting, setSubmitting] = useState(false);

  const handleOpenModal = () => {
    setForm({
      ...INITIAL_FORM,
      instrument_id: selectedInstrumentId || (instruments[0]?.id ?? ''),
    });
    setFormErrors({});
    setShowModal(true);
  };

  const handleCloseModal = () => {
    setShowModal(false);
    setFormErrors({});
  };

  const handleFormChange = (field: keyof OrderForm, value: string) => {
    setForm((prev) => ({ ...prev, [field]: value }));
    if (formErrors[field as keyof OrderFormErrors]) {
      setFormErrors((prev) => ({ ...prev, [field]: undefined }));
    }
  };

  const handleSubmit = async () => {
    const errors = validateOrderForm(form, form.order_type);
    if (Object.keys(errors).length > 0) {
      setFormErrors(errors);
      return;
    }

    setSubmitting(true);
    try {
      const payload: Record<string, unknown> = {
        instrument_id: form.instrument_id,
        side: form.side,
        order_type: form.order_type,
        quantity: Number(form.quantity),
        time_in_force: form.time_in_force,
      };
      if (form.order_type === 'LIMIT') {
        payload.price = Number(form.price);
      }
      await submitSecuritiesOrder(payload);
      showToast('Order submitted successfully', 'success');
      setShowModal(false);
      ordersResult.refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to submit order', 'error');
    } finally {
      setSubmitting(false);
    }
  };

  // Cancel order
  const handleCancel = async (order: SecuritiesOrder) => {
    try {
      await cancelSecuritiesOrder(order.id);
      showToast(`Order ${order.id} cancelled`, 'success');
      ordersResult.refresh();
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to cancel order', 'error');
    }
  };

  // Columns
  const columns: Column<SecuritiesOrder>[] = useMemo(
    () => [
      {
        key: 'side',
        header: 'Side',
        width: '70px',
        render: (row) => (
          <span className={row.side === 'BUY' ? styles.sideBuy : row.side === 'SELL' ? styles.sideSell : ''}>
            {row.side}
          </span>
        ),
      },
      { key: 'order_type', header: 'Type', width: '80px' },
      { key: 'quantity', header: 'Qty', align: 'right', mono: true },
      { key: 'price', header: 'Price', align: 'right', mono: true },
      { key: 'filled_quantity', header: 'Filled', align: 'right', mono: true },
      {
        key: 'status',
        header: 'Status',
        render: (row) => <StatusBadge status={row.status} />,
      },
      {
        key: 'created_at',
        header: 'Created',
        render: (row) => <span className={styles.dateCell}>{formatOrderDate(row.created_at)}</span>,
      },
      {
        key: 'actions',
        header: 'Actions',
        render: (row) => {
          const cancellable = row.status === 'PENDING' || row.status === 'PARTIALLY_FILLED';
          return cancellable ? (
            <button className={styles.cancelBtn} onClick={() => handleCancel(row)}>
              Cancel
            </button>
          ) : null;
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.pageTitle}>{`Securities Order Book${currentTenant ? ` — ${currentTenant.name}` : ''}`}</h1>
        <button className={styles.submitBtn} onClick={handleOpenModal}>
          Submit Order
        </button>
      </div>

      {!currentTenant && (
        <p>Select a tenant from the dropdown above to view securities data</p>
      )}

      {/* Instrument selector */}
      <div className={styles.selectorBar}>
        <label className={styles.selectorLabel}>Instrument</label>
        <select
          className={styles.selector}
          value={selectedInstrumentId}
          onChange={(e) => setSelectedInstrumentId(e.target.value)}
        >
          {instruments.length === 0 && (
            <option value="">— No instruments —</option>
          )}
          {instruments.map((inst) => (
            <option key={inst.id} value={inst.id}>
              {inst.ticker ?? inst.id}
              {inst.name ? ` — ${inst.name}` : ''}
            </option>
          ))}
        </select>
      </div>

      {/* Orders DataGrid */}
      {currentTenant && (
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>Orders</h2>
          <DataGrid
            columns={columns}
            data={orders}
            keyField="_key"
            emptyMessage={`No orders found${currentTenant ? ` for ${currentTenant.name}` : ''}`}
            stickyHeader
            loading={ordersResult.isLoading}
          />
        </div>
      )}

      {/* Recent Trades section */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Recent Trades</h2>
        {trades.length === 0 ? (
          <p className={styles.placeholder}>
            Trades will appear after order matching
          </p>
        ) : (
          <table className={styles.tradesTable}>
            <thead>
              <tr>
                <th>Time</th>
                <th className={styles.right}>Price</th>
                <th className={styles.right}>Quantity</th>
                <th>Side</th>
              </tr>
            </thead>
            <tbody>
              {trades.slice(0, 20).map((t: any, i: number) => (
                <tr key={t.id ?? `trade-${i}`}>
                  <td className={styles.dateCell}>{formatOrderDate(t.timestamp ?? t.time ?? t.executed_at ?? '')}</td>
                  <td className={`${styles.right} ${styles.mono}`}>{String(t.price ?? '-')}</td>
                  <td className={`${styles.right} ${styles.mono}`}>{String(t.quantity ?? '-')}</td>
                  <td>
                    <span className={String(t.side ?? '').toUpperCase() === 'BUY' ? styles.sideBuy : styles.sideSell}>
                      {String(t.side ?? '-').toUpperCase()}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Submit Order Modal */}
      {showModal && (
        <div className={styles.overlay} onClick={handleCloseModal}>
          <div
            className={styles.modal}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Submit Securities Order"
          >
            <h3 className={styles.modalTitle}>Submit Order</h3>

            <label className={styles.formLabel}>
              Instrument
              <select
                className={styles.formInput}
                value={form.instrument_id}
                onChange={(e) => handleFormChange('instrument_id', e.target.value)}
              >
                {instruments.map((inst) => (
                  <option key={inst.id} value={inst.id}>
                    {inst.ticker ?? inst.id}
                  </option>
                ))}
                {instruments.length === 0 && (
                  <option value="">No instruments available</option>
                )}
              </select>
            </label>

            <label className={styles.formLabel}>
              Side
              <select
                className={styles.formInput}
                value={form.side}
                onChange={(e) => handleFormChange('side', e.target.value)}
              >
                <option value="BUY">BUY</option>
                <option value="SELL">SELL</option>
              </select>
            </label>

            <label className={styles.formLabel}>
              Order Type
              <select
                className={styles.formInput}
                value={form.order_type}
                onChange={(e) => handleFormChange('order_type', e.target.value)}
              >
                <option value="LIMIT">LIMIT</option>
                <option value="MARKET">MARKET</option>
              </select>
            </label>

            <label className={styles.formLabel}>
              Quantity
              <input
                type="number"
                className={`${styles.formInput} ${formErrors.quantity ? styles.inputError : ''}`}
                value={form.quantity}
                min={0}
                step={1}
                onChange={(e) => handleFormChange('quantity', e.target.value)}
                aria-invalid={!!formErrors.quantity}
                placeholder="e.g. 100"
              />
              {formErrors.quantity && (
                <span className={styles.fieldError}>{formErrors.quantity}</span>
              )}
            </label>

            <label className={styles.formLabel}>
              Price {form.order_type === 'MARKET' && <span className={styles.hint}>(N/A for MARKET)</span>}
              <input
                type="number"
                className={`${styles.formInput} ${formErrors.price ? styles.inputError : ''}`}
                value={form.price}
                min={0}
                step={0.01}
                disabled={form.order_type === 'MARKET'}
                onChange={(e) => handleFormChange('price', e.target.value)}
                aria-invalid={!!formErrors.price}
                placeholder={form.order_type === 'MARKET' ? 'Disabled for MARKET' : 'e.g. 42.50'}
              />
              {formErrors.price && (
                <span className={styles.fieldError}>{formErrors.price}</span>
              )}
            </label>

            <label className={styles.formLabel}>
              Time in Force
              <select
                className={styles.formInput}
                value={form.time_in_force}
                onChange={(e) => handleFormChange('time_in_force', e.target.value)}
              >
                <option value="GTC">GTC — Good Till Cancelled</option>
                <option value="IOC">IOC — Immediate or Cancel</option>
                <option value="FOK">FOK — Fill or Kill</option>
                <option value="DAY">DAY — Day Order</option>
              </select>
            </label>

            <div className={styles.modalActions}>
              <button className={styles.cancelActionBtn} onClick={handleCloseModal} disabled={submitting}>
                Cancel
              </button>
              <button className={styles.submitActionBtn} onClick={handleSubmit} disabled={submitting}>
                {submitting ? 'Submitting…' : 'Submit Order'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
