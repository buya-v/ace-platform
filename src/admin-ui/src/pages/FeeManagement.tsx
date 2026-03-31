import React from 'react';
import { usePolling } from '../hooks/usePolling';
import { fetchFeeSchedule } from '../services/api';
import styles from './FeeManagement.module.css';

export interface FeeRule {
  id: string;
  fee_type: string;
  tier: string;
  rate_bps: number;
  min_fee: string;
  max_fee: string;
  per_contract: string;
  instrument_id?: string;
  effective_from?: string;
}

/** Format basis points for display (e.g., 25 -> "25 bps" or "0.25%") */
export function formatBps(bps: number): string {
  if (bps == null || isNaN(bps)) return '-';
  return `${bps} bps`;
}

/** Format fee amount with currency symbol */
export function formatFeeAmount(amount: string): string {
  if (!amount || amount === '0' || amount === '0.00') return '-';
  const num = parseFloat(amount);
  if (isNaN(num)) return amount;
  return `$${num.toFixed(2)}`;
}

/** Format fee type for display */
export function formatFeeType(feeType: string): string {
  if (!feeType) return '';
  return feeType.replace(/_/g, ' ').toLowerCase().replace(/\b\w/g, c => c.toUpperCase());
}

/** Compute summary stats from fee rules */
export function computeFeeSummary(rules: FeeRule[]): {
  totalRules: number;
  uniqueTypes: number;
  avgRate: number;
  tiers: string[];
} {
  const types = new Set<string>();
  const tiers = new Set<string>();
  let rateSum = 0;

  rules.forEach(r => {
    types.add(r.fee_type);
    tiers.add(r.tier);
    rateSum += r.rate_bps;
  });

  return {
    totalRules: rules.length,
    uniqueTypes: types.size,
    avgRate: rules.length > 0 ? Math.round(rateSum / rules.length) : 0,
    tiers: Array.from(tiers).sort(),
  };
}

export function FeeManagementPage() {
  const { data, refresh, isLoading } = usePolling(
    (signal) => fetchFeeSchedule(signal),
    60000,
  );

  const rules: FeeRule[] = data?.data ?? [];
  const summary = computeFeeSummary(rules);

  return (
    <div>
      <div className={styles.topRow}>
        <h1>Fee Management</h1>
        <button className={styles.refreshBtn} onClick={refresh} disabled={isLoading}>
          Refresh
        </button>
      </div>

      <div className={styles.summaryCards}>
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{summary.totalRules}</div>
          <div className={styles.summaryLabel}>Fee Rules</div>
        </div>
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{summary.uniqueTypes}</div>
          <div className={styles.summaryLabel}>Fee Types</div>
        </div>
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{summary.avgRate} bps</div>
          <div className={styles.summaryLabel}>Avg Rate</div>
        </div>
        <div className={styles.summaryCard}>
          <div className={styles.summaryValue}>{summary.tiers.length}</div>
          <div className={styles.summaryLabel}>Tiers</div>
        </div>
      </div>

      <table className={styles.table}>
        <thead>
          <tr>
            <th>Fee Type</th>
            <th>Tier</th>
            <th>Rate (bps)</th>
            <th>Min Fee</th>
            <th>Max Fee</th>
            <th>Per Contract</th>
          </tr>
        </thead>
        <tbody>
          {rules.map(rule => (
            <tr key={rule.id}>
              <td className={styles.feeType}>{formatFeeType(rule.fee_type)}</td>
              <td><span className={styles.tierBadge}>{rule.tier}</span></td>
              <td className={styles.rateBps}>{formatBps(rule.rate_bps)}</td>
              <td>{formatFeeAmount(rule.min_fee)}</td>
              <td>{formatFeeAmount(rule.max_fee)}</td>
              <td>{formatFeeAmount(rule.per_contract)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {rules.length === 0 && !isLoading && (
        <div className={styles.empty}>No fee rules configured</div>
      )}
    </div>
  );
}
