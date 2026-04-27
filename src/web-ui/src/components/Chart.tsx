import React, { useEffect, useState } from 'react';
import { apiRequest } from '../services/api';
import styles from './Chart.module.css';

interface ChartProps {
  instrumentId: string | null;
}

interface InstrumentDetail {
  id: string;
  ticker: string;
  name: string;
  isin: string;
  asset_class: string;
  security_type: string;
  exchange_code: string;
  lot_size: number;
  tick_size: number;
  currency: string;
  listing_date: string;
  trading_status: string;
  outstanding_shares: number;
}

interface OrderSummary {
  id: string;
  side: string;
  price: number;
  quantity: number;
  filled_quantity: number;
  status: string;
  created_at: string;
}

export const Chart: React.FC<ChartProps> = ({ instrumentId }) => {
  const [instrument, setInstrument] = useState<InstrumentDetail | null>(null);
  const [stats, setStats] = useState<{ lastPrice: number; high: number; low: number; volume: number; trades: number } | null>(null);

  useEffect(() => {
    if (!instrumentId) { setInstrument(null); setStats(null); return; }

    apiRequest<InstrumentDetail>(`/securities/instruments/${instrumentId}`)
      .then(setInstrument)
      .catch(() => setInstrument(null));

    apiRequest<{ data: OrderSummary[] }>('/securities/orders')
      .then((res) => {
        const orders = (res.data || []).filter((o) => o.status === 'FILLED');
        const instrOrders = orders.filter((o) => String(o.id).length > 0); // all filled orders
        if (instrOrders.length === 0) { setStats(null); return; }
        const prices = instrOrders.map((o) => Number(o.price)).filter((p) => p > 0);
        const totalVol = instrOrders.reduce((s, o) => s + Number(o.filled_quantity || o.quantity), 0);
        setStats({
          lastPrice: prices[0] || 0,
          high: Math.max(...prices),
          low: Math.min(...prices),
          volume: totalVol,
          trades: instrOrders.length,
        });
      })
      .catch(() => setStats(null));
  }, [instrumentId]);

  if (!instrumentId) {
    return (
      <div className={styles.chart}>
        <div className={styles.summaryEmpty}>Select an instrument to view market data</div>
      </div>
    );
  }

  return (
    <div className={styles.chart}>
      {instrument && (
        <div className={styles.summaryPanel}>
          <div className={styles.summaryHeader}>
            <div className={styles.tickerBlock}>
              <span className={styles.tickerName}>{instrument.ticker}</span>
              <span className={styles.instrName}>{instrument.name}</span>
            </div>
            <div className={styles.priceBlock}>
              {stats ? (
                <>
                  <span className={styles.lastPrice}>{stats.lastPrice.toLocaleString()} {instrument.currency}</span>
                  <span className={styles.priceLabel}>Last Trade</span>
                </>
              ) : (
                <span className={styles.priceLabel}>No trades yet</span>
              )}
            </div>
          </div>

          <div className={styles.statsGrid}>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>ISIN</span>
              <span className={styles.statValue}>{instrument.isin || '—'}</span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Asset Class</span>
              <span className={styles.statValue}>{instrument.asset_class}</span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Type</span>
              <span className={styles.statValue}>{instrument.security_type || '—'}</span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Status</span>
              <span className={`${styles.statValue} ${instrument.trading_status === 'ACTIVE' ? styles.statusActive : ''}`}>
                {instrument.trading_status}
              </span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Lot Size</span>
              <span className={styles.statValue}>{instrument.lot_size}</span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Tick Size</span>
              <span className={styles.statValue}>{instrument.tick_size} {instrument.currency}</span>
            </div>
            {stats && (
              <>
                <div className={styles.statCard}>
                  <span className={styles.statLabel}>Day High</span>
                  <span className={`${styles.statValue} ${styles.highVal}`}>{stats.high.toLocaleString()}</span>
                </div>
                <div className={styles.statCard}>
                  <span className={styles.statLabel}>Day Low</span>
                  <span className={`${styles.statValue} ${styles.lowVal}`}>{stats.low.toLocaleString()}</span>
                </div>
                <div className={styles.statCard}>
                  <span className={styles.statLabel}>Volume</span>
                  <span className={styles.statValue}>{stats.volume.toLocaleString()}</span>
                </div>
                <div className={styles.statCard}>
                  <span className={styles.statLabel}>Trades</span>
                  <span className={styles.statValue}>{stats.trades}</span>
                </div>
              </>
            )}
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Exchange</span>
              <span className={styles.statValue}>{instrument.exchange_code?.trim()}</span>
            </div>
            <div className={styles.statCard}>
              <span className={styles.statLabel}>Listed</span>
              <span className={styles.statValue}>{instrument.listing_date}</span>
            </div>
          </div>
        </div>
      )}
      {!instrument && (
        <div className={styles.summaryEmpty}>Loading instrument data...</div>
      )}
    </div>
  );
};
