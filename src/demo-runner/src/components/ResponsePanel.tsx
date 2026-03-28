import React from 'react';
import { StepResult } from '../types/step';
import { JsonViewer } from './JsonViewer';
import styles from './ResponsePanel.module.css';

interface ResponsePanelProps {
  result: StepResult;
}

export function ResponsePanel({ result }: ResponsePanelProps) {
  const statusClass =
    result.responseStatus && result.responseStatus >= 200 && result.responseStatus < 300
      ? styles.statusOk
      : styles.statusErr;

  return (
    <div className={styles.panel}>
      <div className={styles.meta}>
        {result.responseStatus !== null && (
          <span className={statusClass}>{result.responseStatus}</span>
        )}
        <span className={styles.time}>{result.responseTime}ms</span>
        {result.error && <span className={styles.error}>{result.error}</span>}
      </div>
      {result.responseBody !== null && (
        <div className={styles.body}>
          <JsonViewer data={result.responseBody} />
        </div>
      )}
    </div>
  );
}
