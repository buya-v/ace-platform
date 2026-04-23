import React, { useMemo, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchSecuritiesPositions } from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import styles from './SecuritiesPositions.module.css';

// ─── Types ─────────────────────────────────────────────────────────────────

export interface SecuritiesPosition {
  _key: string;
  instrument_id: string;
  quantity: string;
  avg_cost: string;
  market_value: string;
  unrealized_pnl: string;
}

// ─── Pure exported helpers ───────────────────────────────────────────────────

export function formatPnl(value: string | number): string {
  const n = typeof value === 'number' ? value : parseFloat(String(value));
  if (isNaN(n)) return '0.00';
  const formatted = Math.abs(n).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
  return (n < 0 ? '-' : '+') + formatted;
}

export function formatMoney(value: string | number): string {
  const n = typeof value === 'number' ? value : parseFloat(String(value));
  if (isNaN(n)) return '0.00';
  return n.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function normalizePositions(raw: unknown): SecuritiesPosition[] {
  if (!raw) return [];
  let arr: any[] = [];
  if (Array.isArray(raw)) {
    arr = raw;
  } else if (typeof raw === 'object') {
    const o = raw as any;
    arr = o.positions ?? o.data ?? o.items ?? [];
    if (!Array.isArray(arr)) arr = [];
  }
  return arr.map((p: any, i: number) => ({
    _key: p.id ?? p.position_id ?? `pos-${i}`,
    instrument_id: p.instrument_id ?? p.instrumentId ?? '-',
    quantity: String(p.quantity ?? p.net_quantity ?? '0'),
    avg_cost: String(p.avg_cost ?? p.average_cost ?? p.avg_price ?? '0'),
    market_value: String(p.market_value ?? p.marketValue ?? '0'),
    unrealized_pnl: String(p.unrealized_pnl ?? p.unrealizedPnl ?? p.pnl ?? '0'),
  }));
}

// ─── Component ────────────────────────────────────────────────────────────────

export function SecuritiesPositionsPage() {
  const positionsResult = usePolling<unknown>(
    useCallback((signal: AbortSignal) => fetchSecuritiesPositions(undefined, signal), []),
    30000,
  );

  const positions: SecuritiesPosition[] = useMemo(
    () => normalizePositions(positionsResult.data),
    [positionsResult.data],
  );

  const columns: Column<SecuritiesPosition>[] = useMemo(
    () => [
      {
        key: 'instrument_id',
        header: 'Instrument',
        mono: true,
        sortable: true,
        filterable: true,
      },
      {
        key: 'quantity',
        header: 'Quantity',
        align: 'right',
        mono: true,
        sortable: true,
      },
      {
        key: 'avg_cost',
        header: 'Avg Cost',
        align: 'right',
        mono: true,
        sortable: true,
        render: (row) => formatMoney(row.avg_cost),
      },
      {
        key: 'market_value',
        header: 'Market Value',
        align: 'right',
        mono: true,
        sortable: true,
        render: (row) => formatMoney(row.market_value),
      },
      {
        key: 'unrealized_pnl',
        header: 'Unrealized P&L',
        align: 'right',
        mono: true,
        sortable: true,
        render: (row) => {
          const n = parseFloat(row.unrealized_pnl);
          const colorClass = n > 0 ? styles.pnlPositive : n < 0 ? styles.pnlNegative : '';
          return (
            <span className={colorClass}>
              {formatPnl(row.unrealized_pnl)}
            </span>
          );
        },
      },
    ],
    [],
  );

  return (
    <div className={styles.page}>
      <h1 className={styles.pageTitle}>Securities Positions</h1>

      <div className={styles.section}>
        <DataGrid
          columns={columns}
          data={positions}
          keyField="_key"
          emptyMessage="No securities positions yet"
          exportFilename="securities-positions"
          stickyHeader
          loading={positionsResult.isLoading}
        />
      </div>
    </div>
  );
}
