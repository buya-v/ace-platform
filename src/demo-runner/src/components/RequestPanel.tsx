import React, { useState } from 'react';
import { JsonViewer } from './JsonViewer';
import styles from './RequestPanel.module.css';

interface RequestPanelProps {
  method: string;
  url: string;
  headers: Record<string, string>;
  body: unknown;
}

export function RequestPanel({ method, url, headers, body }: RequestPanelProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={styles.panel}>
      <button className={styles.toggle} onClick={() => setExpanded(!expanded)}>
        {expanded ? '\u25BC' : '\u25B6'} Request
        <span className={styles.method}>{method}</span>
        <span className={styles.url}>{url}</span>
      </button>
      {expanded && (
        <div className={styles.details}>
          {Object.keys(headers).length > 0 && (
            <div className={styles.section}>
              <div className={styles.label}>Headers</div>
              <JsonViewer data={headers} />
            </div>
          )}
          {body !== undefined && body !== null && (
            <div className={styles.section}>
              <div className={styles.label}>Body</div>
              <JsonViewer data={body} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
