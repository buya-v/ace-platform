import React from 'react';
import styles from './ShortcutHelp.module.css';

interface ShortcutHelpProps {
  visible: boolean;
  onClose: () => void;
}

const SHORTCUTS = [
  { key: 'Ctrl + 1', description: 'Overview' },
  { key: 'Ctrl + 2', description: 'System Health' },
  { key: 'Ctrl + 3', description: 'Order Book' },
  { key: 'Ctrl + 4', description: 'Positions' },
  { key: 'Ctrl + 5', description: 'Risk Overview' },
  { key: 'Ctrl + 6', description: 'Margin Calls' },
  { key: 'Ctrl + 7', description: 'Settlement' },
  { key: 'Ctrl + 8', description: 'Circuit Breakers' },
  { key: 'Ctrl + 9', description: 'Warehouse' },
  { key: 'Ctrl + R', description: 'Refresh data' },
  { key: 'Ctrl + E', description: 'Export to CSV' },
  { key: 'Esc', description: 'Close modal' },
  { key: '?', description: 'Toggle this help' },
];

export function ShortcutHelp({ visible, onClose }: ShortcutHelpProps) {
  return (
    <div
      className={`${styles.overlay} ${visible ? styles.visible : ''}`}
      data-testid="shortcut-help-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className={styles.modal}>
        <div className={styles.header}>
          <h2 className={styles.title}>Keyboard Shortcuts</h2>
          <button className={styles.closeBtn} onClick={onClose} aria-label="Close">
            &times;
          </button>
        </div>
        <ul className={styles.list}>
          {SHORTCUTS.map((s) => (
            <li key={s.key} className={styles.row}>
              <span className={styles.description}>{s.description}</span>
              <kbd className={styles.kbd}>{s.key}</kbd>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
