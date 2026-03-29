import React, { createContext, useContext, useCallback, useReducer } from 'react';

export type ToastType = 'success' | 'error' | 'warning' | 'info';

export interface ToastItem {
  id: string;
  message: string;
  type: ToastType;
}

interface ToastState {
  toasts: ToastItem[];
}

type ToastAction =
  | { type: 'ADD'; payload: ToastItem }
  | { type: 'DISMISS'; payload: string };

function toastReducer(state: ToastState, action: ToastAction): ToastState {
  switch (action.type) {
    case 'ADD':
      return { toasts: [...state.toasts, action.payload] };
    case 'DISMISS':
      return { toasts: state.toasts.filter(t => t.id !== action.payload) };
    default:
      return state;
  }
}

interface ToastContextValue {
  toasts: ToastItem[];
  showToast: (message: string, type: ToastType) => void;
  dismissToast: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(toastReducer, { toasts: [] });

  const showToast = useCallback((message: string, type: ToastType) => {
    const id = `toast-${++nextId}-${Date.now()}`;
    dispatch({ type: 'ADD', payload: { id, message, type } });
  }, []);

  const dismissToast = useCallback((id: string) => {
    dispatch({ type: 'DISMISS', payload: id });
  }, []);

  return (
    <ToastContext.Provider value={{ toasts: state.toasts, showToast, dismissToast }}>
      {children}
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within ToastProvider');
  return ctx;
}
