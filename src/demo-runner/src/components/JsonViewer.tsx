import React from 'react';
import styles from './JsonViewer.module.css';

interface JsonViewerProps {
  data: unknown;
}

export function JsonViewer({ data }: JsonViewerProps) {
  if (data === null || data === undefined) {
    return <span className={styles.null}>null</span>;
  }
  return <pre className={styles.container}>{renderValue(data, 0)}</pre>;
}

export function renderValue(value: unknown, indent: number): React.ReactNode {
  if (value === null) return <span className="json-null">null</span>;
  if (value === undefined) return <span className="json-null">undefined</span>;

  const type = typeof value;
  if (type === 'string') return <span className="json-string">&quot;{String(value)}&quot;</span>;
  if (type === 'number') return <span className="json-number">{String(value)}</span>;
  if (type === 'boolean') return <span className="json-boolean">{String(value)}</span>;

  if (Array.isArray(value)) {
    if (value.length === 0) return <span>{'[]'}</span>;
    const pad = '  '.repeat(indent + 1);
    const closePad = '  '.repeat(indent);
    return (
      <span>
        {'[\n'}
        {value.map((item, i) => (
          <span key={i}>
            {pad}
            {renderValue(item, indent + 1)}
            {i < value.length - 1 ? ',' : ''}
            {'\n'}
          </span>
        ))}
        {closePad}
        {']'}
      </span>
    );
  }

  if (type === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    if (entries.length === 0) return <span>{'{}'}</span>;
    const pad = '  '.repeat(indent + 1);
    const closePad = '  '.repeat(indent);
    return (
      <span>
        {'{\n'}
        {entries.map(([key, val], i) => (
          <span key={key}>
            {pad}
            <span className="json-key">&quot;{key}&quot;</span>
            {': '}
            {renderValue(val, indent + 1)}
            {i < entries.length - 1 ? ',' : ''}
            {'\n'}
          </span>
        ))}
        {closePad}
        {'}'}
      </span>
    );
  }

  return <span>{String(value)}</span>;
}
