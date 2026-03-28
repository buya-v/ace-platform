import React from 'react';
import { useDemo } from '../contexts/DemoContext';
import { getTotalStepCount } from '../data/sections';
import styles from './TopBar.module.css';

export function TopBar() {
  const { state, dispatch } = useDemo();
  const total = getTotalStepCount();
  const passed = Object.values(state.results).filter((r) => r.status === 'PASS').length;
  const failed = Object.values(state.results).filter((r) => r.status === 'FAIL').length;

  return (
    <div className={styles.bar}>
      <label className={styles.urlLabel}>
        Gateway:
        <input
          className={styles.urlInput}
          value={state.gatewayUrl}
          onChange={(e) => dispatch({ type: 'SET_GATEWAY_URL', url: e.target.value })}
        />
      </label>
      <div className={styles.stats}>
        <span className={styles.passed}>{passed} passed</span>
        {failed > 0 && <span className={styles.failed}>{failed} failed</span>}
        <span className={styles.total}>/ {total} steps</span>
      </div>
    </div>
  );
}
