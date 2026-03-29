import React, { useState, useEffect, useCallback } from 'react';
import { CheckIcon, AlertIcon, InfoIcon, CloseIcon } from './icons';
import { useToast, ToastItem } from '../contexts/ToastContext';
import styles from './Toast.module.css';

const iconMap = {
  success: CheckIcon,
  error: AlertIcon,
  warning: AlertIcon,
  info: InfoIcon,
};

function ToastEntry({ toast, onDismiss }: { toast: ToastItem; onDismiss: (id: string) => void }) {
  const [exiting, setExiting] = useState(false);

  const dismiss = useCallback(() => {
    setExiting(true);
    setTimeout(() => onDismiss(toast.id), 200);
  }, [toast.id, onDismiss]);

  useEffect(() => {
    const timer = setTimeout(() => {
      dismiss();
    }, 4000);
    return () => clearTimeout(timer);
  }, [dismiss]);

  const Icon = iconMap[toast.type];
  const typeClass = styles[toast.type];

  return (
    <div className={`${styles.toast} ${typeClass} ${exiting ? styles.exiting : ''}`}>
      <span className={styles.icon}><Icon size={16} /></span>
      <span className={styles.message}>{toast.message}</span>
      <button className={styles.closeBtn} onClick={dismiss} aria-label="Close">
        <CloseIcon size={14} />
      </button>
    </div>
  );
}

export function ToastContainer() {
  const { toasts, dismissToast } = useToast();

  const visible = toasts.slice(-5);

  return (
    <div className={styles.container}>
      {visible.map(toast => (
        <ToastEntry key={toast.id} toast={toast} onDismiss={dismissToast} />
      ))}
    </div>
  );
}
