import { useEffect, useRef, useState, useCallback } from 'react';
import { getConfig } from '../config';

export type WebSocketStatus = 'connecting' | 'connected' | 'disconnected' | 'error';

export interface WebSocketResult<T> {
  data: T | null;
  status: WebSocketStatus;
  reconnectCount: number;
}

export interface WebSocketOptions {
  enabled?: boolean;
}

const MAX_BACKOFF_MS = 30000;

function getBackoffMs(attempt: number): number {
  const ms = Math.min(1000 * Math.pow(2, attempt), MAX_BACKOFF_MS);
  return ms;
}

export function useWebSocket<T = unknown>(
  path: string,
  options?: WebSocketOptions,
): WebSocketResult<T> {
  const enabled = options?.enabled ?? true;

  const [data, setData] = useState<T | null>(null);
  const [status, setStatus] = useState<WebSocketStatus>('disconnected');
  const [reconnectCount, setReconnectCount] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptRef = useRef(0);
  const unmountedRef = useRef(false);

  const cleanup = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.onopen = null;
      wsRef.current.onclose = null;
      wsRef.current.onerror = null;
      wsRef.current.onmessage = null;
      wsRef.current.close();
      wsRef.current = null;
    }
  }, []);

  useEffect(() => {
    unmountedRef.current = false;

    if (!enabled) {
      cleanup();
      setStatus('disconnected');
      return;
    }

    function connect() {
      if (unmountedRef.current) return;

      cleanup();

      const config = getConfig();
      const url = config.WS_BASE_URL.replace(/\/+$/, '') + '/' + path.replace(/^\/+/, '');

      setStatus('connecting');

      let ws: WebSocket;
      try {
        ws = new WebSocket(url);
      } catch {
        setStatus('error');
        scheduleReconnect();
        return;
      }

      wsRef.current = ws;

      ws.onopen = () => {
        if (unmountedRef.current) return;
        attemptRef.current = 0;
        setStatus('connected');
      };

      ws.onmessage = (event: MessageEvent) => {
        if (unmountedRef.current) return;
        try {
          const parsed = JSON.parse(event.data) as T;
          setData(parsed);
        } catch {
          // Ignore non-JSON messages
        }
      };

      ws.onerror = () => {
        if (unmountedRef.current) return;
        setStatus('error');
      };

      ws.onclose = () => {
        if (unmountedRef.current) return;
        setStatus('disconnected');
        scheduleReconnect();
      };
    }

    function scheduleReconnect() {
      if (unmountedRef.current) return;
      const backoff = getBackoffMs(attemptRef.current);
      attemptRef.current++;
      setReconnectCount((c) => c + 1);
      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        connect();
      }, backoff);
    }

    connect();

    return () => {
      unmountedRef.current = true;
      cleanup();
    };
  }, [path, enabled, cleanup]);

  return { data, status, reconnectCount };
}
