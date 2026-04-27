import React, { useState, useEffect, useCallback } from 'react';
import type { InstrumentDetail } from '../types/instrument';
import { apiRequest } from '../services/api';
import styles from './InstrumentInfo.module.css';

interface InstrumentInfoProps {
  instrumentId: string | null;
}

export const InstrumentInfo: React.FC<InstrumentInfoProps> = ({ instrumentId }) => {
  const [detail, setDetail] = useState<InstrumentDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    if (!instrumentId) {
      setDetail(null);
      return;
    }

    setLoading(true);
    apiRequest<InstrumentDetail>(`/securities/instruments/${instrumentId}`)
      .then((data) => setDetail(data))
      .catch(() => setDetail(null))
      .finally(() => setLoading(false));
  }, [instrumentId]);

  const toggle = useCallback(() => setExpanded((prev) => !prev), []);

  if (!instrumentId) return null;

  return (
    <div className={styles.instrumentInfo}>
      <button className={styles.toggleBtn} onClick={toggle}>
        {expanded ? 'Hide Instrument Details' : 'Instrument Details'}
      </button>

      {expanded && (
        <div className={styles.panel}>
          {loading && <div className={styles.loading}>Loading...</div>}
          {!loading && !detail && <div className={styles.empty}>No instrument info available</div>}
          {!loading && detail && (
            <div className={styles.grid}>
              <div className={styles.item}>
                <span className={styles.label}>Ticker</span>
                <span className={styles.value}>{detail.ticker}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Name</span>
                <span className={styles.value}>{detail.name}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>ISIN</span>
                <span className={styles.value}>{detail.isin}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Asset Class</span>
                <span className={styles.value}>{detail.asset_class}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Security Type</span>
                <span className={styles.value}>{detail.security_type}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Tick Size</span>
                <span className={styles.value}>{detail.tick_size}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Lot Size</span>
                <span className={styles.value}>{detail.lot_size}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Exchange</span>
                <span className={styles.value}>{detail.exchange_code}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Currency</span>
                <span className={styles.value}>{detail.currency}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Trading Status</span>
                <span className={`${styles.value}`}>
                  {detail.trading_status.toUpperCase()}
                </span>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
