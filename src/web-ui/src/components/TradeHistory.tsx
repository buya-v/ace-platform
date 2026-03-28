import React from 'react';
import type { Trade } from '../types/trade';
import styles from './TradeHistory.module.css';

interface TradeHistoryProps {
  trades: Trade[];
}

function formatTime(timestamp: string): string {
  try {
    const d = new Date(timestamp);
    return d.toLocaleTimeString('en-US', { hour12: false });
  } catch {
    return timestamp;
  }
}

export const TradeHistory: React.FC<TradeHistoryProps> = ({ trades }) => {
  return (
    <div className={styles.tradeHistory}>
      <div className={styles.header}>
        <span>Time</span>
        <span>Price</span>
        <span>Qty</span>
        <span>Side</span>
      </div>
      <div className={styles.list}>
        {trades.map((trade) => (
          <div key={trade.tradeId} className={styles.row}>
            <span className={styles.time}>{formatTime(trade.timestamp)}</span>
            <span className={trade.side === 'buy' ? styles.buyPrice : styles.sellPrice}>
              {trade.price}
            </span>
            <span>{trade.quantity}</span>
            <span className={trade.side === 'buy' ? styles.buySide : styles.sellSide}>
              {trade.side.toUpperCase()}
            </span>
          </div>
        ))}
        {trades.length === 0 && (
          <div className={styles.empty}>No recent trades</div>
        )}
      </div>
    </div>
  );
};
