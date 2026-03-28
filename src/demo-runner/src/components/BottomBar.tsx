import React from 'react';
import { useDemoRunner } from '../hooks/useDemoRunner';
import styles from './BottomBar.module.css';

export function BottomBar() {
  const { runAll, reset, runAllInProgress } = useDemoRunner();

  return (
    <div className={styles.bar}>
      <button className={styles.runAll} onClick={runAll} disabled={runAllInProgress}>
        {runAllInProgress ? 'Running...' : 'Run All'}
      </button>
      <button className={styles.reset} onClick={reset} disabled={runAllInProgress}>
        Reset
      </button>
    </div>
  );
}
