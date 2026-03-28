import React, { useState } from 'react';
import styles from './ConfirmDialog.module.css';

interface ConfirmDialogProps {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  requireTypedConfirmation?: string;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
}

export function ConfirmDialog({
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  requireTypedConfirmation,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const [typed, setTyped] = useState('');
  const [loading, setLoading] = useState(false);

  const canConfirm = requireTypedConfirmation
    ? typed === requireTypedConfirmation
    : true;

  const handleConfirm = async () => {
    setLoading(true);
    try {
      await onConfirm();
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className={styles.overlay} onClick={onCancel}>
      <div className={styles.dialog} onClick={e => e.stopPropagation()} role="dialog" aria-modal="true">
        <h3 className={styles.title}>{title}</h3>
        <p className={styles.message}>{message}</p>
        {requireTypedConfirmation && (
          <div className={styles.typedConfirm}>
            <label>
              Type <strong>{requireTypedConfirmation}</strong> to confirm:
            </label>
            <input
              type="text"
              value={typed}
              onChange={e => setTyped(e.target.value)}
              className={styles.input}
              autoFocus
            />
          </div>
        )}
        <div className={styles.actions}>
          <button onClick={onCancel} className={styles.cancelBtn} disabled={loading}>
            {cancelLabel}
          </button>
          <button
            onClick={handleConfirm}
            className={styles.confirmBtn}
            disabled={!canConfirm || loading}
          >
            {loading ? 'Processing...' : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
