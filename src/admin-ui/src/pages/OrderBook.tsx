import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { usePolling } from '../hooks/usePolling';
import {
  fetchInstrumentList,
  fetchOrderBook,
  fetchTicker,
  fetchMarketTrades,
} from '../services/api';
import { DataGrid, Column } from '../components/DataGrid';
import styles from './OrderBook.module.css';

interface TickerData {
  last_price?: string;
  volume_24h?: string | number;
  high_24h?: string;
  low_24h?: string;
  [key: string]: unknown;
}

interface BookLevel {
  price: string;
  quantity: number | string;
  order_count?: number;
}

interface BookData {
  bids?: BookLevel[];
  asks?: BookLevel[];
  [key: string]: unknown;
}

interface TradeRow {
  _key: string;
  time: string;
  price: string;
  quantity: string;
  side: string;
}

const DEFAULT_INSTRUMENT = 'WHT-HRW-2026M07-UB';

export function OrderBookPage() {
  const [instrumentId, setInstrumentId] = useState(DEFAULT_INSTRUMENT);
  const [instruments, setInstruments] = useState<string[]>([]);

  // Fetch instrument list once
  useEffect(() => {
    const controller = new AbortController();
    fetchInstrumentList(controller.signal)
      .then((res) => {
        if (Array.isArray(res)) {
          setInstruments(res.map((i: any) => i.instrument_id ?? i.id ?? String(i)));
        } else if (res && Array.isArray(res.instruments)) {
          setInstruments(res.instruments.map((i: any) => i.instrument_id ?? i.id ?? String(i)));
        } else if (res && typeof res === 'object') {
          const arr = (res as any).data ?? (res as any).items ?? [];
          if (Array.isArray(arr)) {
            setInstruments(arr.map((i: any) => i.instrument_id ?? i.id ?? String(i)));
          }
        }
      })
      .catch(() => {
        // Use default instrument if fetch fails
        setInstruments([DEFAULT_INSTRUMENT]);
      });
    return () => controller.abort();
  }, []);

  // Poll order book every 2s
  const book = usePolling<BookData>(
    useCallback((signal: AbortSignal) => fetchOrderBook(instrumentId, signal), [instrumentId]),
    2000,
  );

  // Poll ticker every 5s
  const ticker = usePolling<TickerData>(
    useCallback((signal: AbortSignal) => fetchTicker(instrumentId, signal), [instrumentId]),
    5000,
  );

  // Poll recent trades every 5s
  const trades = usePolling<any>(
    useCallback((signal: AbortSignal) => fetchMarketTrades(instrumentId, signal), [instrumentId]),
    5000,
  );

  // Parse bids/asks
  const bids: BookLevel[] = useMemo(() => {
    const raw = book.data?.bids;
    return Array.isArray(raw) ? raw : [];
  }, [book.data]);

  const asks: BookLevel[] = useMemo(() => {
    const raw = book.data?.asks;
    return Array.isArray(raw) ? raw : [];
  }, [book.data]);

  // Cumulative volumes
  const bidsCumulative = useMemo(() => {
    let cum = 0;
    return bids.map((b) => {
      cum += Number(b.quantity) || 0;
      return cum;
    });
  }, [bids]);

  const asksCumulative = useMemo(() => {
    let cum = 0;
    return asks.map((a) => {
      cum += Number(a.quantity) || 0;
      return cum;
    });
  }, [asks]);

  const maxCum = useMemo(() => {
    const bidMax = bidsCumulative.length > 0 ? bidsCumulative[bidsCumulative.length - 1] : 0;
    const askMax = asksCumulative.length > 0 ? asksCumulative[asksCumulative.length - 1] : 0;
    return Math.max(bidMax, askMax, 1);
  }, [bidsCumulative, asksCumulative]);

  // Spread
  const spread = useMemo(() => {
    if (bids.length === 0 || asks.length === 0) return null;
    const bestBid = Number(bids[0].price) || 0;
    const bestAsk = Number(asks[0].price) || 0;
    return (bestAsk - bestBid).toFixed(4);
  }, [bids, asks]);

  // Parse trades into rows
  const tradeRows: TradeRow[] = useMemo(() => {
    const raw = trades.data;
    let arr: any[] = [];
    if (Array.isArray(raw)) {
      arr = raw;
    } else if (raw && typeof raw === 'object') {
      arr = (raw as any).trades ?? (raw as any).data ?? (raw as any).items ?? [];
      if (!Array.isArray(arr)) arr = [];
    }
    return arr.slice(0, 50).map((t: any, i: number) => ({
      _key: t.id ?? t.trade_id ?? `trade-${i}`,
      time: t.timestamp ?? t.time ?? t.executed_at ?? '',
      price: String(t.price ?? ''),
      quantity: String(t.quantity ?? t.volume ?? ''),
      side: String(t.side ?? t.taker_side ?? '').toUpperCase(),
    }));
  }, [trades.data]);

  const tradeColumns: Column<TradeRow>[] = useMemo(() => [
    {
      key: 'time',
      header: 'Time',
      width: '100px',
      render: (row) => {
        if (!row.time) return '-';
        try {
          const d = new Date(row.time);
          return d.toLocaleTimeString();
        } catch {
          return row.time;
        }
      },
    },
    {
      key: 'price',
      header: 'Price',
      align: 'right' as const,
      mono: true,
      render: (row) => (
        <span className={row.side === 'BUY' ? styles.priceGreen : row.side === 'SELL' ? styles.priceRed : ''}>
          {row.price}
        </span>
      ),
    },
    { key: 'quantity', header: 'Qty', align: 'right' as const, mono: true },
    {
      key: 'side',
      header: 'Side',
      width: '60px',
      render: (row) => (
        <span className={row.side === 'BUY' ? styles.sideBuy : row.side === 'SELL' ? styles.sideSell : ''}>
          {row.side || '-'}
        </span>
      ),
    },
  ], []);

  const tickerData = ticker.data;

  return (
    <div className={styles.page}>
      {/* Top bar */}
      <div className={styles.topBar}>
        <div className={styles.instrumentSelect}>
          <label className={styles.selectLabel}>Instrument</label>
          <select
            className={styles.select}
            value={instrumentId}
            onChange={(e) => setInstrumentId(e.target.value)}
          >
            {instruments.length === 0 && (
              <option value={instrumentId}>{instrumentId}</option>
            )}
            {instruments.map((id) => (
              <option key={id} value={id}>{id}</option>
            ))}
          </select>
        </div>
        <div className={styles.tickerStats}>
          <div className={styles.tickerItem}>
            <span className={styles.tickerLabel}>Last Price</span>
            <span className={styles.tickerValue}>{tickerData?.last_price ?? '-'}</span>
          </div>
          <div className={styles.tickerItem}>
            <span className={styles.tickerLabel}>24h Volume</span>
            <span className={styles.tickerValue}>{tickerData?.volume_24h ?? '-'}</span>
          </div>
          <div className={styles.tickerItem}>
            <span className={styles.tickerLabel}>High</span>
            <span className={styles.tickerValue}>{tickerData?.high_24h ?? '-'}</span>
          </div>
          <div className={styles.tickerItem}>
            <span className={styles.tickerLabel}>Low</span>
            <span className={styles.tickerValue}>{tickerData?.low_24h ?? '-'}</span>
          </div>
        </div>
      </div>

      {/* Main area */}
      <div className={styles.mainArea}>
        {/* Depth View */}
        <div className={styles.depthCard}>
          <h2 className={styles.cardTitle}>Depth View</h2>
          <div className={styles.depthContainer}>
            {/* Bids */}
            <div className={styles.depthColumn}>
              <div className={styles.depthHeader}>
                <span>Price</span>
                <span>Qty</span>
                <span>Total</span>
              </div>
              {bids.length === 0 ? (
                <div className={styles.emptyDepth}>No bids</div>
              ) : (
                bids.slice(0, 20).map((b, i) => {
                  const pct = (bidsCumulative[i] / maxCum) * 100;
                  return (
                    <div key={`bid-${i}`} className={styles.depthRow}>
                      <div
                        className={styles.depthBarBid}
                        style={{ width: `${pct}%` }}
                      />
                      <span className={`${styles.depthPrice} ${styles.priceGreen}`}>
                        {b.price}
                      </span>
                      <span className={styles.depthQty}>{b.quantity}</span>
                      <span className={styles.depthCum}>{bidsCumulative[i].toFixed(2)}</span>
                    </div>
                  );
                })
              )}
            </div>

            {/* Spread */}
            <div className={styles.spreadSection}>
              <span className={styles.spreadLabel}>Spread</span>
              <span className={styles.spreadValue}>{spread ?? '-'}</span>
            </div>

            {/* Asks */}
            <div className={styles.depthColumn}>
              <div className={styles.depthHeader}>
                <span>Price</span>
                <span>Qty</span>
                <span>Total</span>
              </div>
              {asks.length === 0 ? (
                <div className={styles.emptyDepth}>No asks</div>
              ) : (
                asks.slice(0, 20).map((a, i) => {
                  const pct = (asksCumulative[i] / maxCum) * 100;
                  return (
                    <div key={`ask-${i}`} className={styles.depthRow}>
                      <div
                        className={styles.depthBarAsk}
                        style={{ width: `${pct}%` }}
                      />
                      <span className={`${styles.depthPrice} ${styles.priceRed}`}>
                        {a.price}
                      </span>
                      <span className={styles.depthQty}>{a.quantity}</span>
                      <span className={styles.depthCum}>{asksCumulative[i].toFixed(2)}</span>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>

        {/* Recent Trades */}
        <div className={styles.tradesCard}>
          <h2 className={styles.cardTitle}>Recent Trades</h2>
          <DataGrid
            columns={tradeColumns}
            data={tradeRows}
            keyField="_key"
            emptyMessage="No recent trades"
            compact
            stickyHeader
          />
        </div>
      </div>
    </div>
  );
}
