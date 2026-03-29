import { useState, useEffect, useCallback } from 'react';

export interface KeyboardShortcutConfig {
  navigate: (path: string) => void;
  onRefresh?: () => void;
  onExport?: () => void;
  onEscape?: () => void;
}

const NAV_SHORTCUTS: Record<string, string> = {
  '1': '/dashboard',
  '2': '/dashboard/monitoring',
  '3': '/dashboard/orderbook',
  '4': '/dashboard/positions',
  '5': '/dashboard/risk',
  '6': '/dashboard/margin',
  '7': '/dashboard/settlement',
  '8': '/dashboard/circuit-breakers',
  '9': '/dashboard/warehouse',
};

function isInputElement(target: EventTarget | null): boolean {
  if (!target || !(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
}

export function useKeyboardShortcuts(config: KeyboardShortcutConfig) {
  const [showHelp, setShowHelp] = useState(false);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (isInputElement(e.target)) return;

      const mod = e.ctrlKey || e.metaKey;

      // Navigation shortcuts: Ctrl/Meta + 1-9
      if (mod && NAV_SHORTCUTS[e.key]) {
        e.preventDefault();
        config.navigate(NAV_SHORTCUTS[e.key]);
        return;
      }

      // Ctrl+R — refresh
      if (mod && (e.key === 'r' || e.key === 'R')) {
        e.preventDefault();
        config.onRefresh?.();
        return;
      }

      // Ctrl+E — export
      if (mod && (e.key === 'e' || e.key === 'E')) {
        e.preventDefault();
        config.onExport?.();
        return;
      }

      // Escape — close modals
      if (e.key === 'Escape') {
        config.onEscape?.();
        setShowHelp(false);
        return;
      }

      // ? (Shift+/) — toggle help
      if (e.key === '?' && !mod) {
        setShowHelp((prev) => !prev);
        return;
      }
    },
    [config],
  );

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  return { showHelp, setShowHelp };
}
