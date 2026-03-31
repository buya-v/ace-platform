import React, { useState, useMemo, useCallback } from 'react';
import type { TradeRecord, TradeHistoryFilter } from '../types/trade';
import type { Instrument } from '../types/instrument';
import { filterTrades, buildTradesCsv, formatTradeTime } from '../services/tradingUtils';
import styles from './TradeHistory.module.css';

interface TradeHistoryProps {
  trades: TradeRecord[];
  instruments?: Instrument[];
}

const EMPTY_FILTER: TradeHistoryFilter = {
  startDate: '',
  endDate: '',
  instrumentId: '',
  side: '',
};

export const TradeHistory: React.FC<TradeHistoryProps> = ({ trades, instruments }) => {
  const [filter, setFilter] = useState<TradeHistoryFilter>(EMPTY_FILTER);
  const [showFilters, setShowFilters] = useState(false);

  const filteredTrades = useMemo(
    () => filterTrades(trades, filter),
    [trades, filter],
  );

  const handleExportCsv = useCallback(() => {
    const csv = buildTradesCsv(filteredTrades);
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `trades-${new Date().toISOString().slice(0, 10)}.csv`;
    link.click();
    URL.revokeObjectURL(url);
  }, [filteredTrades]);

  return (
    <div className={styles.tradeHistory}>
      <div className={styles.toolbar}>
        <span className={styles.label}>Trade History</span>
        <div className={styles.actions}>
          <button
            className={styles.filterBtn}
            onClick={() => setShowFilters(!showFilters)}
          >
            {showFilters ? 'Hide Filters' : 'Filters'}
          </button>
          <button
            className={styles.exportBtn}
            onClick={handleExportCsv}
            disabled={filteredTrades.length === 0}
          >
            CSV Export
          </button>
        </div>
      </div>

      {showFilters && (
        <div className={styles.filterPanel}>
          <div className={styles.filterRow}>
            <label>
              From
              <input
                type="date"
                value={filter.startDate}
                onChange={(e) => setFilter({ ...filter, startDate: e.target.value })}
              />
            </label>
            <label>
              To
              <input
                type="date"
                value={filter.endDate}
                onChange={(e) => setFilter({ ...filter, endDate: e.target.value })}
              />
            </label>
          </div>
          <div className={styles.filterRow}>
            <label>
              Instrument
              <select
                value={filter.instrumentId}
                onChange={(e) => setFilter({ ...filter, instrumentId: e.target.value })}
              >
                <option value="">All</option>
                {instruments?.map((inst) => (
                  <option key={inst.instrumentId} value={inst.instrumentId}>
                    {inst.symbol}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Side
              <select
                value={filter.side}
                onChange={(e) => setFilter({ ...filter, side: e.target.value as '' | 'buy' | 'sell' })}
              >
                <option value="">All</option>
                <option value="buy">Buy</option>
                <option value="sell">Sell</option>
              </select>
            </label>
            <button
              className={styles.clearBtn}
              onClick={() => setFilter(EMPTY_FILTER)}
            >
              Clear
            </button>
          </div>
        </div>
      )}

      <div className={styles.header}>
        <span>Time</span>
        <span>Instrument</span>
        <span>Side</span>
        <span>Qty</span>
        <span>Price</span>
        <span>Total</span>
      </div>
      <div className={styles.list}>
        {filteredTrades.map((trade) => (
          <div key={trade.tradeId} className={styles.row}>
            <span className={styles.time}>{formatTradeTime(trade.timestamp)}</span>
            <span>{trade.instrumentSymbol}</span>
            <span className={trade.side === 'buy' ? styles.buySide : styles.sellSide}>
              {trade.side.toUpperCase()}
            </span>
            <span>{trade.quantity}</span>
            <span className={trade.side === 'buy' ? styles.buyPrice : styles.sellPrice}>
              {trade.price}
            </span>
            <span>{trade.totalValue}</span>
          </div>
        ))}
        {filteredTrades.length === 0 && (
          <div className={styles.empty}>No trades found</div>
        )}
      </div>
      <div className={styles.footer}>
        {filteredTrades.length} trade{filteredTrades.length !== 1 ? 's' : ''}
      </div>
    </div>
  );
};
