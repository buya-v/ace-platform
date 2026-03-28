import React from 'react';
import type { PriceLevel } from '../types/trade';
import styles from './OrderBook.module.css';

interface OrderBookProps {
  bids: PriceLevel[];
  asks: PriceLevel[];
  onPriceClick?: (price: string) => void;
}

function getMaxQuantity(levels: PriceLevel[]): number {
  return levels.reduce((max, l) => Math.max(max, Number(l.quantity)), 0);
}

function Spread({ bids, asks }: { bids: PriceLevel[]; asks: PriceLevel[] }) {
  if (bids.length === 0 || asks.length === 0) return null;
  const bestBid = Number(bids[0].price);
  const bestAsk = Number(asks[0].price);
  const spread = bestAsk - bestBid;
  const spreadPct = bestBid > 0 ? ((spread / bestBid) * 100).toFixed(3) : '0';
  return (
    <div className={styles.spread}>
      Spread: {spread.toFixed(4)} ({spreadPct}%)
    </div>
  );
}

export const OrderBook: React.FC<OrderBookProps> = ({ bids, asks, onPriceClick }) => {
  const displayAsks = asks.slice(0, 15).reverse();
  const displayBids = bids.slice(0, 15);
  const maxQty = Math.max(getMaxQuantity(displayAsks), getMaxQuantity(displayBids));

  return (
    <div className={styles.orderBook}>
      <div className={styles.header}>
        <span>Price</span>
        <span>Qty</span>
        <span>Orders</span>
      </div>
      <div className={styles.asks}>
        {displayAsks.map((level) => (
          <div
            key={level.price}
            className={styles.askRow}
            onClick={() => onPriceClick?.(level.price)}
          >
            <div
              className={styles.depthBar}
              style={{ width: maxQty > 0 ? `${(Number(level.quantity) / maxQty) * 100}%` : '0%' }}
            />
            <span className={styles.askPrice}>{level.price}</span>
            <span>{level.quantity}</span>
            <span>{level.orderCount}</span>
          </div>
        ))}
      </div>
      <Spread bids={bids} asks={asks} />
      <div className={styles.bids}>
        {displayBids.map((level) => (
          <div
            key={level.price}
            className={styles.bidRow}
            onClick={() => onPriceClick?.(level.price)}
          >
            <div
              className={styles.depthBar}
              style={{ width: maxQty > 0 ? `${(Number(level.quantity) / maxQty) * 100}%` : '0%' }}
            />
            <span className={styles.bidPrice}>{level.price}</span>
            <span>{level.quantity}</span>
            <span>{level.orderCount}</span>
          </div>
        ))}
      </div>
    </div>
  );
};
