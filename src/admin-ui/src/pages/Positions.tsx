import React, { useMemo, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchPositions, fetchNetting, fetchPortfolioMargin } from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import styles from './Positions.module.css';

interface PositionRow {
  _key: string;
  participant_id: string;
  instrument_id: string;
  side: string;
  net_quantity: string;
  avg_price: string;
  unrealized_pnl: string;
  margin_required: string;
}

interface NettingRow {
  _key: string;
  participant_id: string;
  net_obligation: string;
  instruments: number;
}

function parseNum(v: unknown): number {
  if (typeof v === 'number') return v;
  if (typeof v === 'string') return parseFloat(v) || 0;
  return 0;
}

function formatMoney(v: unknown): string {
  const n = parseNum(v);
  return n.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 4 });
}

export function PositionsPage() {
  const positions = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchPositions(signal), []),
    10000,
  );

  const netting = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchNetting(signal), []),
    10000,
  );

  const margin = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchPortfolioMargin(signal), []),
    10000,
  );

  // Parse positions into rows
  const positionRows: PositionRow[] = useMemo(() => {
    const raw = positions.data;
    let arr: any[] = [];
    if (Array.isArray(raw)) {
      arr = raw;
    } else if (raw && typeof raw === 'object') {
      arr = (raw as any).data ?? (raw as any).positions ?? (raw as any).items ?? [];
      if (!Array.isArray(arr)) arr = [];
    }
    return arr.map((p: any, i: number) => ({
      _key: p.id ?? `pos-${i}`,
      participant_id: p.participant_id ?? p.participantId ?? '-',
      instrument_id: p.instrument_id ?? p.instrumentId ?? '-',
      side: String(p.side ?? '').toUpperCase() || '-',
      net_quantity: String(p.net_quantity ?? p.netQuantity ?? p.quantity ?? '0'),
      avg_price: String(p.avg_price ?? p.avgPrice ?? p.average_price ?? '0'),
      unrealized_pnl: String(p.unrealized_pnl ?? p.unrealizedPnl ?? p.pnl ?? '0'),
      margin_required: String(p.margin_required ?? p.marginRequired ?? p.margin ?? '0'),
    }));
  }, [positions.data]);

  // Parse netting
  const nettingRows: NettingRow[] = useMemo(() => {
    const raw = netting.data;
    let arr: any[] = [];
    if (Array.isArray(raw)) {
      arr = raw;
    } else if (raw && typeof raw === 'object') {
      arr = (raw as any).data ?? (raw as any).netting ?? (raw as any).items ?? [];
      if (!Array.isArray(arr)) arr = [];
    }
    return arr.map((n: any, i: number) => ({
      _key: n.participant_id ?? `net-${i}`,
      participant_id: n.participant_id ?? n.participantId ?? '-',
      net_obligation: String(n.net_obligation ?? n.netObligation ?? n.amount ?? '0'),
      instruments: Number(n.instruments ?? n.instrument_count ?? 0),
    }));
  }, [netting.data]);

  // Summary stats
  const totalPositions = positionRows.length;
  const totalMargin = useMemo(() => {
    return positionRows.reduce((sum, p) => sum + parseNum(p.margin_required), 0);
  }, [positionRows]);
  const totalPnl = useMemo(() => {
    return positionRows.reduce((sum, p) => sum + parseNum(p.unrealized_pnl), 0);
  }, [positionRows]);

  const positionColumns: Column<PositionRow>[] = useMemo(() => [
    { key: 'participant_id', header: 'Participant', sortable: true, filterable: true },
    { key: 'instrument_id', header: 'Instrument', sortable: true, filterable: true },
    {
      key: 'side',
      header: 'Side',
      sortable: true,
      width: '80px',
      render: (row) => (
        <span className={row.side === 'BUY' ? styles.sideBuy : row.side === 'SELL' ? styles.sideSell : ''}>
          {row.side}
        </span>
      ),
    },
    { key: 'net_quantity', header: 'Net Qty', align: 'right', mono: true, sortable: true },
    { key: 'avg_price', header: 'Avg Price', align: 'right', mono: true, sortable: true },
    {
      key: 'unrealized_pnl',
      header: 'Unrealized PnL',
      align: 'right',
      mono: true,
      sortable: true,
      render: (row) => {
        const val = parseNum(row.unrealized_pnl);
        return (
          <span className={val > 0 ? styles.pnlPositive : val < 0 ? styles.pnlNegative : ''}>
            {formatMoney(row.unrealized_pnl)}
          </span>
        );
      },
    },
    { key: 'margin_required', header: 'Margin Required', align: 'right', mono: true, sortable: true },
  ], []);

  const nettingColumns: Column<NettingRow>[] = useMemo(() => [
    { key: 'participant_id', header: 'Participant', sortable: true },
    { key: 'net_obligation', header: 'Net Obligation', align: 'right', mono: true, sortable: true },
    { key: 'instruments', header: 'Instruments', align: 'right', mono: true, sortable: true },
  ], []);

  return (
    <div className={styles.page}>
      <h1 className={styles.pageTitle}>Position Management</h1>

      {/* Summary Cards */}
      <div className={styles.statsGrid}>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Total Open Positions</div>
          <div className={styles.statValue}>{totalPositions}</div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Total Margin Used</div>
          <div className={styles.statValue}>{formatMoney(totalMargin)}</div>
        </div>
        <div className={styles.statCard}>
          <div className={styles.statLabel}>Unrealized P&amp;L</div>
          <div className={`${styles.statValue} ${totalPnl > 0 ? styles.pnlPositive : totalPnl < 0 ? styles.pnlNegative : ''}`}>
            {totalPnl >= 0 ? '+' : ''}{formatMoney(totalPnl)}
          </div>
        </div>
      </div>

      {/* Positions Grid */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Open Positions</h2>
        <DataGrid
          columns={positionColumns}
          data={positionRows}
          keyField="_key"
          emptyMessage="No open positions"
          exportFilename="positions"
          stickyHeader
          loading={positions.isLoading}
        />
      </div>

      {/* Netting Summary */}
      <div className={styles.section}>
        <h2 className={styles.sectionTitle}>Netting Summary</h2>
        <DataGrid
          columns={nettingColumns}
          data={nettingRows}
          keyField="_key"
          emptyMessage="No netting data available"
          exportFilename="netting"
          loading={netting.isLoading}
        />
      </div>
    </div>
  );
}
