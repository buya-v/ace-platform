import { tokenManager } from './tokenManager';

export type WSStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

export interface WSMessage {
  type: string;
  data: unknown;
}

export interface WebSocketManagerOptions {
  url: string;
  onMessage: (msg: WSMessage) => void;
  onStatusChange: (status: WSStatus) => void;
  maxRetries?: number;
  maxDelay?: number;
}

export class WebSocketManager {
  private ws: WebSocket | null = null;
  private retryCount = 0;
  private maxRetries: number;
  private maxDelay: number;
  private url: string;
  private onMessage: (msg: WSMessage) => void;
  private onStatusChange: (status: WSStatus) => void;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private closed = false;

  constructor(options: WebSocketManagerOptions) {
    this.url = options.url;
    this.onMessage = options.onMessage;
    this.onStatusChange = options.onStatusChange;
    this.maxRetries = options.maxRetries ?? 10;
    this.maxDelay = options.maxDelay ?? 30000;
  }

  connect(): void {
    if (this.closed) return;

    const token = tokenManager.getToken();
    const separator = this.url.includes('?') ? '&' : '?';
    const wsUrl = token ? `${this.url}${separator}token=${token}` : this.url;

    this.onStatusChange('connecting');
    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.retryCount = 0;
      this.onStatusChange('connected');
    };

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data) as WSMessage;
        this.onMessage(msg);
      } catch {
        // Ignore malformed messages
      }
    };

    this.ws.onclose = () => {
      if (this.closed) {
        this.onStatusChange('disconnected');
        return;
      }
      this.scheduleReconnect();
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  private scheduleReconnect(): void {
    if (this.retryCount >= this.maxRetries) {
      this.onStatusChange('disconnected');
      return;
    }

    this.onStatusChange('reconnecting');
    const delay = this.getBackoffDelay();
    this.retryCount++;

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }

  getBackoffDelay(): number {
    const base = Math.min(1000 * Math.pow(2, this.retryCount), this.maxDelay);
    const jitter = base * 0.5 * Math.random();
    return base + jitter;
  }

  disconnect(): void {
    this.closed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.onStatusChange('disconnected');
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }
}

export function parseBookMessage(data: unknown): { type: 'snapshot' | 'update'; payload: unknown } | null {
  if (typeof data !== 'object' || data === null) return null;
  const d = data as Record<string, unknown>;

  if (d.type === 'snapshot' || d.type === 'update') {
    return { type: d.type as 'snapshot' | 'update', payload: d };
  }
  return null;
}

export function parseTradeMessage(data: unknown): { tradeId: string; price: string; quantity: string; side: 'buy' | 'sell'; timestamp: string; sequence: number } | null {
  if (typeof data !== 'object' || data === null) return null;
  const d = data as Record<string, unknown>;

  if (typeof d.tradeId !== 'string' || typeof d.price !== 'string' ||
      typeof d.quantity !== 'string' || typeof d.timestamp !== 'string') {
    return null;
  }

  const side = d.side as string;
  if (side !== 'buy' && side !== 'sell') return null;

  return {
    tradeId: d.tradeId,
    price: d.price,
    quantity: d.quantity,
    side,
    timestamp: d.timestamp,
    sequence: typeof d.sequence === 'number' ? d.sequence : 0,
  };
}
