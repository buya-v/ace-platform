import React from 'react';
import type { Position } from '../types/trade';
import type { Ticker } from '../types/instrument';
import { formatPnl, pnlColorClass, calculateTotalPnl } from '../services/tradingUtils';
import styles from './Positions.module.css';

interface PositionsProps {
  positions: Position[];
  tickers?: Map<string, Ticker>;
  onClosePosition?: (instrumentId: string, side: 'long' | 'short', quantity: string) => void;
}

export const Positions: React.FC<PositionsProps> = ({ positions, tickers, onClosePosition }) => {
  const totals = calculateTotalPnl(positions);

  return (
    <div className={styles.positions}>
      <div className={styles.titleRow}>
        <h3 className={styles.title}>Positions</h3>
        {positions.length > 0 && (
          <span className={`${styles.totalPnl} ${styles[pnlColorClass(totals.totalPnl)]}`}>
            P&L: {formatPnl(totals.totalPnl)}
          </span>
        )}
      </div>
      <div className={styles.header}>
        <span>Instrument</span>
        <span>Side</span>
        <span>Qty</span>
        <span>Avg Entry</span>
        <span>Current</span>
        <span>Unrealized P&L</span>
        <span></span>
      </div>
      <div className={styles.list}>
        {positions.map((pos) => {
          const ticker = tickers?.get(pos.instrumentId);
          const currentPrice = ticker?.lastPrice ?? '-';
          const pnlClass = pnlColorClass(pos.unrealizedPnl);
          return (
            <div key={pos.instrumentId} className={styles.row}>
              <span>{pos.instrumentSymbol}</span>
              <span className={pos.side === 'long' ? styles.long : pos.side === 'short' ? styles.short : ''}>
                {pos.side.toUpperCase()}
              </span>
              <span>{pos.netQuantity}</span>
              <span>{pos.avgEntryPrice}</span>
              <span>{currentPrice}</span>
              <span className={styles[pnlClass]}>
                {formatPnl(pos.unrealizedPnl)}
              </span>
              <span>
                {pos.side !== 'flat' && onClosePosition && (
                  <button
                    className={styles.closeBtn}
                    onClick={() => onClosePosition(pos.instrumentId, pos.side as 'long' | 'short', pos.netQuantity)}
                    title="Close position with opposing market order"
                  >
                    Close
                  </button>
                )}
              </span>
            </div>
          );
        })}
        {positions.length === 0 && (
          <div className={styles.empty}>No open positions</div>
        )}
      </div>
    </div>
  );
};
