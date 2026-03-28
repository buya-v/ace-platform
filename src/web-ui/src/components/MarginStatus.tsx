import React from 'react';
import type { MarginStatus as MarginStatusType } from '../types/trade';
import styles from './MarginStatus.module.css';

interface MarginStatusProps {
  margin: MarginStatusType | null;
}

export const MarginStatusPanel: React.FC<MarginStatusProps> = ({ margin }) => {
  if (!margin) {
    return (
      <div className={styles.margin}>
        <h3 className={styles.title}>Margin</h3>
        <div className={styles.empty}>No margin data</div>
      </div>
    );
  }

  const utilPct = (margin.marginUtilization * 100).toFixed(1);
  const isWarning = margin.marginUtilization > 0.8;

  return (
    <div className={styles.margin}>
      <h3 className={styles.title}>Margin Status</h3>
      <div className={styles.grid}>
        <div className={styles.item}>
          <span className={styles.label}>Account Balance</span>
          <span className={styles.value}>{margin.accountBalance}</span>
        </div>
        <div className={styles.item}>
          <span className={styles.label}>Used Margin</span>
          <span className={styles.value}>{margin.usedMargin}</span>
        </div>
        <div className={styles.item}>
          <span className={styles.label}>Available Margin</span>
          <span className={styles.value}>{margin.availableMargin}</span>
        </div>
        <div className={styles.item}>
          <span className={styles.label}>Utilization</span>
          <span className={`${styles.value} ${isWarning ? styles.warning : ''}`}>
            {utilPct}%
          </span>
        </div>
      </div>
      <div className={styles.barContainer}>
        <div
          className={`${styles.bar} ${isWarning ? styles.barWarning : ''}`}
          style={{ width: `${Math.min(100, Number(utilPct))}%` }}
        />
      </div>
      {margin.marginCalls.length > 0 && (
        <div className={styles.calls}>
          <h4>Margin Calls</h4>
          {margin.marginCalls.map((call) => (
            <div key={call.callId} className={styles.call}>
              <span>Amount: {call.amount}</span>
              <span>Deadline: {new Date(call.deadline).toLocaleString()}</span>
              <span className={call.status === 'breached' ? styles.warning : ''}>{call.status}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
