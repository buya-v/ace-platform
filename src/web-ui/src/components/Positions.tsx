import React from 'react';
import type { Position } from '../types/trade';
import styles from './Positions.module.css';

interface PositionsProps {
  positions: Position[];
}

export const Positions: React.FC<PositionsProps> = ({ positions }) => {
  return (
    <div className={styles.positions}>
      <h3 className={styles.title}>Positions</h3>
      <div className={styles.header}>
        <span>Instrument</span>
        <span>Side</span>
        <span>Qty</span>
        <span>Avg Price</span>
        <span>Unrealized P&L</span>
      </div>
      <div className={styles.list}>
        {positions.map((pos) => (
          <div key={pos.instrumentId} className={styles.row}>
            <span>{pos.instrumentSymbol}</span>
            <span className={pos.side === 'long' ? styles.long : pos.side === 'short' ? styles.short : ''}>
              {pos.side.toUpperCase()}
            </span>
            <span>{pos.netQuantity}</span>
            <span>{pos.avgEntryPrice}</span>
            <span className={Number(pos.unrealizedPnl) >= 0 ? styles.positive : styles.negative}>
              {pos.unrealizedPnl}
            </span>
          </div>
        ))}
        {positions.length === 0 && (
          <div className={styles.empty}>No open positions</div>
        )}
      </div>
    </div>
  );
};
