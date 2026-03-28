import React from 'react';
import styles from './StatusBadge.module.css';

interface StatusBadgeProps {
  status: string;
  variant?: 'default' | 'health';
}

const healthColors: Record<string, string> = {
  healthy: styles.green,
  degraded: styles.yellow,
  unhealthy: styles.red,
};

const statusColors: Record<string, string> = {
  APPROVED: styles.green,
  ACTIVE: styles.green,
  TRADING: styles.green,
  COMPLETED: styles.green,
  MET: styles.green,
  CLEAR: styles.green,
  DELIVERED: styles.green,
  RESOLVED: styles.green,
  DISMISSED: styles.gray,
  PENDING: styles.yellow,
  UNDER_REVIEW: styles.yellow,
  PRE_OPEN: styles.yellow,
  OPEN: styles.yellow,
  NETTING: styles.yellow,
  SETTLING: styles.yellow,
  IN_TRANSIT: styles.yellow,
  PENDING_REVIEW: styles.yellow,
  REJECTED: styles.red,
  HALTED: styles.red,
  FAILED: styles.red,
  BREACHED: styles.red,
  MATCH_FOUND: styles.red,
  CANCELLED: styles.gray,
  PLEDGED: styles.blue,
};

export function StatusBadge({ status, variant = 'default' }: StatusBadgeProps) {
  const colorMap = variant === 'health' ? healthColors : statusColors;
  const colorClass = colorMap[status] ?? styles.gray;

  return (
    <span className={`${styles.badge} ${colorClass}`}>
      {status}
    </span>
  );
}
