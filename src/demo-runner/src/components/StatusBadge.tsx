import React from 'react';
import { StepStatus } from '../types/step';
import styles from './StatusBadge.module.css';

interface StatusBadgeProps {
  status: StepStatus;
}

const labels: Record<StepStatus, string> = {
  PENDING: 'Pending',
  RUNNING: 'Running...',
  PASS: 'Pass',
  FAIL: 'Fail',
  SKIP: 'Skip',
};

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <span className={`${styles.badge} ${styles[status.toLowerCase()]}`} data-testid="status-badge">
      {status === 'RUNNING' && <span className={styles.spinner} />}
      {labels[status]}
    </span>
  );
}
