import React, { useState, useEffect } from 'react';
import type { Instrument } from '../types/instrument';
import { apiRequest } from '../services/api';
import styles from './InstrumentSelector.module.css';

interface InstrumentSelectorProps {
  selected: Instrument | null;
  onSelect: (instrument: Instrument) => void;
}

export const InstrumentSelector: React.FC<InstrumentSelectorProps> = ({ selected, onSelect }) => {
  const [instruments, setInstruments] = useState<Instrument[]>([]);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    apiRequest<{ data: Instrument[] }>('/securities/instruments')
      .then((res) => setInstruments(res.data || []))
      .catch(() => {});
  }, []);

  return (
    <div className={styles.selector}>
      <button className={styles.trigger} onClick={() => setOpen(!open)}>
        {selected ? (
          <span className={styles.symbol}>{selected.ticker}</span>
        ) : (
          'Select Instrument'
        )}
      </button>
      {open && (
        <div className={styles.dropdown}>
          {instruments.map((inst) => (
            <button
              key={inst.id}
              className={`${styles.item} ${selected?.id === inst.id ? styles.active : ''}`}
              onClick={() => { onSelect(inst); setOpen(false); }}
            >
              <span className={styles.itemSymbol}>{inst.ticker}</span>
              <span className={styles.itemCommodity}>{inst.name}</span>
              <span className={styles.itemPrice}>{inst.trading_status}</span>
            </button>
          ))}
          {instruments.length === 0 && (
            <div className={styles.empty}>No instruments available</div>
          )}
        </div>
      )}
    </div>
  );
};
