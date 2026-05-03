import React from 'react';
import { useBot } from '../contexts/BotContext';
import styles from './BotButton.module.css';

export function BotButton() {
  const { state, togglePanel } = useBot();
  const { unreadCount, alertCount, isOpen } = state;
  const badgeCount = unreadCount + alertCount;

  if (isOpen) return null;

  return (
    <button
      className={`${styles.button}${badgeCount > 0 ? ` ${styles.pulsing}` : ''}`}
      onClick={togglePanel}
      aria-label="Open GarudaX Bot"
      type="button"
    >
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
        {/* Antenna */}
        <line x1="12" y1="2" x2="12" y2="6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        <circle cx="12" cy="2" r="1.5" fill="currentColor" />
        {/* Head / chat bubble */}
        <rect x="4" y="6" width="16" height="12" rx="3" fill="currentColor" />
        {/* Eyes */}
        <circle cx="9" cy="12" r="1.5" fill="var(--bg-primary, #0d1117)" />
        <circle cx="15" cy="12" r="1.5" fill="var(--bg-primary, #0d1117)" />
        {/* Chat tail */}
        <path d="M8 18 L6 22 L12 18" fill="currentColor" />
      </svg>
      {badgeCount > 0 && (
        <span className={`${styles.badge}${alertCount > 0 ? ` ${styles.alertBadge}` : ''}`} data-testid="bot-badge">
          {badgeCount > 9 ? '9+' : badgeCount}
        </span>
      )}
      <span className={styles.tooltip}>GarudaX Bot</span>
    </button>
  );
}
