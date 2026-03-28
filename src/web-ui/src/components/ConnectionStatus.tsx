import React from 'react';
import type { WSStatus } from '../services/ws';
import styles from './ConnectionStatus.module.css';

interface ConnectionStatusProps {
  status: WSStatus;
}

const STATUS_LABELS: Record<WSStatus, string> = {
  connecting: 'Connecting...',
  connected: 'Connected',
  disconnected: 'Disconnected',
  reconnecting: 'Reconnecting...',
};

export const ConnectionStatus: React.FC<ConnectionStatusProps> = ({ status }) => {
  return (
    <div className={`${styles.status} ${styles[status]}`}>
      <span className={styles.dot} />
      <span className={styles.label}>{STATUS_LABELS[status]}</span>
    </div>
  );
};
