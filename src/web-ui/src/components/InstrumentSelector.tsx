import React, { useState, useEffect } from 'react';
import type { Instrument, Ticker } from '../types/instrument';
import { apiRequest } from '../services/api';
import styles from './InstrumentSelector.module.css';

interface InstrumentSelectorProps {
  selected: Instrument | null;
  onSelect: (instrument: Instrument) => void;
}

export const InstrumentSelector: React.FC<InstrumentSelectorProps> = ({ selected, onSelect }) => {
  const [instruments, setInstruments] = useState<Instrument[]>([]);
  const [tickers, setTickers] = useState<Map<string, Ticker>>(new Map());
  const [open, setOpen] = useState(false);

  useEffect(() => {
    apiRequest<{ instruments: Instrument[] }>('/instruments?status=active')
      .then((data) => setInstruments(data.instruments || []))
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (instruments.length === 0) return;
    const interval = setInterval(() => {
      instruments.forEach((inst) => {
        apiRequest<Ticker>(`/market-data/ticker?instrument_id=${inst.instrumentId}`)
          .then((ticker) => {
            setTickers((prev) => new Map(prev).set(inst.instrumentId, ticker));
          })
          .catch(() => {});
      });
    }, 5000);
    return () => clearInterval(interval);
  }, [instruments]);

  return (
    <div className={styles.selector}>
      <button className={styles.trigger} onClick={() => setOpen(!open)}>
        {selected ? (
          <>
            <span className={styles.symbol}>{selected.symbol}</span>
            {tickers.has(selected.instrumentId) && (
              <span className={styles.price}>
                {tickers.get(selected.instrumentId)!.lastPrice}
              </span>
            )}
          </>
        ) : (
          'Select Instrument'
        )}
      </button>
      {open && (
        <div className={styles.dropdown}>
          {instruments.map((inst) => {
            const ticker = tickers.get(inst.instrumentId);
            return (
              <button
                key={inst.instrumentId}
                className={`${styles.item} ${selected?.instrumentId === inst.instrumentId ? styles.active : ''}`}
                onClick={() => { onSelect(inst); setOpen(false); }}
              >
                <span className={styles.itemSymbol}>{inst.symbol}</span>
                <span className={styles.itemCommodity}>{inst.commodityName}</span>
                {ticker && (
                  <>
                    <span className={styles.itemPrice}>{ticker.lastPrice}</span>
                    <span className={Number(ticker.change24h) >= 0 ? styles.positive : styles.negative}>
                      {ticker.changePercent24h}%
                    </span>
                  </>
                )}
              </button>
            );
          })}
          {instruments.length === 0 && (
            <div className={styles.empty}>No instruments available</div>
          )}
        </div>
      )}
    </div>
  );
};
