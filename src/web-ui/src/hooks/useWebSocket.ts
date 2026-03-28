import { useEffect, useRef, useCallback } from 'react';
import { WebSocketManager, type WSMessage, type WSStatus } from '../services/ws';

interface UseWebSocketOptions {
  url: string | null;
  onMessage: (msg: WSMessage) => void;
  onStatusChange?: (status: WSStatus) => void;
  enabled?: boolean;
}

export function useWebSocket({ url, onMessage, onStatusChange, enabled = true }: UseWebSocketOptions) {
  const managerRef = useRef<WebSocketManager | null>(null);
  const onMessageRef = useRef(onMessage);
  const onStatusRef = useRef(onStatusChange);

  onMessageRef.current = onMessage;
  onStatusRef.current = onStatusChange;

  const disconnect = useCallback(() => {
    if (managerRef.current) {
      managerRef.current.disconnect();
      managerRef.current = null;
    }
  }, []);

  useEffect(() => {
    if (!url || !enabled) {
      disconnect();
      return;
    }

    const manager = new WebSocketManager({
      url,
      onMessage: (msg) => onMessageRef.current(msg),
      onStatusChange: (status) => onStatusRef.current?.(status),
    });

    managerRef.current = manager;
    manager.connect();

    return () => {
      manager.disconnect();
    };
  }, [url, enabled, disconnect]);

  return { disconnect };
}
