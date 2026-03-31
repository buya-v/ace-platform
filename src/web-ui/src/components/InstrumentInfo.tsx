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
    apiRequest<InstrumentDetail>(`/instruments/${instrumentId}`)
      .then((data) => setDetail(data))
      .catch(() => setDetail(null))
      .finally(() => setLoading(false));
  }, [instrumentId]);

  const toggle = useCallback(() => setExpanded((prev) => !prev), []);

  if (!instrumentId) return null;

  return (
    <div className={styles.instrumentInfo}>
      <button className={styles.toggleBtn} onClick={toggle}>
        {expanded ? 'Hide Contract Specs' : 'Contract Specs'}
      </button>

      {expanded && (
        <div className={styles.panel}>
          {loading && <div className={styles.loading}>Loading...</div>}
          {!loading && !detail && <div className={styles.empty}>No contract info available</div>}
          {!loading && detail && (
            <div className={styles.grid}>
              <div className={styles.item}>
                <span className={styles.label}>Symbol</span>
                <span className={styles.value}>{detail.symbol}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Commodity</span>
                <span className={styles.value}>{detail.commodityName}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Delivery Month</span>
                <span className={styles.value}>{detail.deliveryMonth}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Contract Size</span>
                <span className={styles.value}>{detail.contractSize ?? detail.lotSize}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Tick Size</span>
                <span className={styles.value}>{detail.tickSize}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Lot Size</span>
                <span className={styles.value}>{detail.lotSize}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Position Limit</span>
                <span className={styles.value}>{detail.positionLimit ?? 'N/A'}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Settlement</span>
                <span className={styles.value}>{detail.settlementType ?? 'N/A'}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Delivery Location</span>
                <span className={styles.value}>{detail.deliveryLocation}</span>
              </div>
              <div className={styles.item}>
                <span className={styles.label}>Status</span>
                <span className={`${styles.value} ${styles[detail.status]}`}>
                  {detail.status.toUpperCase()}
                </span>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
