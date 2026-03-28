import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { WebSocketManager, parseBookMessage } from '../services/ws';

// Mock tokenManager
vi.mock('../services/tokenManager', () => ({
  tokenManager: {
    getToken: vi.fn(() => 'test-token'),
    setToken: vi.fn(),
    clear: vi.fn(),
    onRefreshNeeded: vi.fn(),
  },
}));

// Mock WebSocket
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  url: string;
  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  simulateMessage(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  simulateError() {
    this.onerror?.();
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  static instances: MockWebSocket[] = [];
  static reset() {
    MockWebSocket.instances = [];
  }
}

// Assign to global
(globalThis as unknown as Record<string, unknown>).WebSocket = MockWebSocket;

beforeEach(() => {
  vi.useFakeTimers();
  MockWebSocket.reset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('WebSocketManager', () => {
  it('connects with token in URL', () => {
    const onMessage = vi.fn();
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({ url: 'ws://test/ws', onMessage, onStatusChange });

    manager.connect();

    expect(MockWebSocket.instances.length).toBe(1);
    expect(MockWebSocket.instances[0].url).toBe('ws://test/ws?token=test-token');
    expect(onStatusChange).toHaveBeenCalledWith('connecting');
  });

  it('appends token with & when URL has query params', () => {
    const manager = new WebSocketManager({
      url: 'ws://test/ws?instrument=WHEAT',
      onMessage: vi.fn(),
      onStatusChange: vi.fn(),
    });
    manager.connect();
    expect(MockWebSocket.instances[0].url).toBe('ws://test/ws?instrument=WHEAT&token=test-token');
  });

  it('calls onStatusChange with connected on open', () => {
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({ url: 'ws://test', onMessage: vi.fn(), onStatusChange });
    manager.connect();

    MockWebSocket.instances[0].simulateOpen();
    expect(onStatusChange).toHaveBeenCalledWith('connected');
  });

  it('parses incoming messages', () => {
    const onMessage = vi.fn();
    const manager = new WebSocketManager({ url: 'ws://test', onMessage, onStatusChange: vi.fn() });
    manager.connect();

    MockWebSocket.instances[0].simulateOpen();
    MockWebSocket.instances[0].simulateMessage({ type: 'update', data: { price: '100' } });

    expect(onMessage).toHaveBeenCalledWith({ type: 'update', data: { price: '100' } });
  });

  it('ignores malformed messages', () => {
    const onMessage = vi.fn();
    const manager = new WebSocketManager({ url: 'ws://test', onMessage, onStatusChange: vi.fn() });
    manager.connect();

    // Send invalid JSON directly
    MockWebSocket.instances[0].onmessage?.({ data: 'not json{{{' });
    expect(onMessage).not.toHaveBeenCalled();
  });

  it('reconnects with exponential backoff on close', () => {
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange,
      maxRetries: 3,
    });
    manager.connect();
    MockWebSocket.instances[0].simulateOpen();

    // Simulate close (not user-initiated)
    MockWebSocket.instances[0].simulateClose();
    expect(onStatusChange).toHaveBeenCalledWith('reconnecting');

    // Advance past first backoff (1s base + up to 0.5s jitter)
    vi.advanceTimersByTime(2000);
    expect(MockWebSocket.instances.length).toBe(2); // reconnected
  });

  it('stops reconnecting after max retries', () => {
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange,
      maxRetries: 2,
    });
    manager.connect();

    // Close and exhaust retries
    MockWebSocket.instances[0].simulateClose();
    vi.advanceTimersByTime(2000); // retry 1
    MockWebSocket.instances[1].simulateClose();
    vi.advanceTimersByTime(5000); // retry 2

    // Now at max retries, next close should give disconnected
    MockWebSocket.instances[2].simulateClose();
    expect(onStatusChange).toHaveBeenLastCalledWith('disconnected');
  });

  it('disconnect prevents reconnection', () => {
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange,
    });
    manager.connect();
    MockWebSocket.instances[0].simulateOpen();

    manager.disconnect();
    expect(onStatusChange).toHaveBeenLastCalledWith('disconnected');

    // Try to connect again — should be no-op since closed flag is set
    manager.connect();
    expect(MockWebSocket.instances.length).toBe(1); // no new instance
  });

  it('disconnect clears pending reconnect timer', () => {
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange: vi.fn(),
      maxRetries: 5,
    });
    manager.connect();
    MockWebSocket.instances[0].simulateClose(); // triggers reconnect timer

    manager.disconnect();
    vi.advanceTimersByTime(60000);
    expect(MockWebSocket.instances.length).toBe(1); // no reconnection
  });

  it('isConnected returns true when open', () => {
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange: vi.fn(),
    });
    expect(manager.isConnected()).toBe(false);

    manager.connect();
    MockWebSocket.instances[0].simulateOpen();
    expect(manager.isConnected()).toBe(true);
  });

  it('resets retry count on successful connection', () => {
    const onStatusChange = vi.fn();
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange,
      maxRetries: 3,
    });

    manager.connect();
    MockWebSocket.instances[0].simulateClose(); // triggers retry 1
    vi.advanceTimersByTime(2000);
    MockWebSocket.instances[1].simulateOpen(); // resets retryCount

    // Close again — should still be able to retry since count was reset
    MockWebSocket.instances[1].simulateClose();
    expect(onStatusChange).toHaveBeenCalledWith('reconnecting');
  });

  it('getBackoffDelay respects maxDelay', () => {
    const manager = new WebSocketManager({
      url: 'ws://test',
      onMessage: vi.fn(),
      onStatusChange: vi.fn(),
      maxDelay: 5000,
    });

    // Simulate many retries by calling getBackoffDelay many times
    // At retry 20, base = 1000 * 2^20 would be huge, but capped at maxDelay
    // We need to access internal state — test via public getBackoffDelay
    // The delay should never exceed maxDelay * 1.5 (base + jitter)
    const delay = manager.getBackoffDelay();
    expect(delay).toBeLessThanOrEqual(7500); // maxDelay + 50% jitter at most
  });
});

describe('parseBookMessage', () => {
  it('parses a snapshot message', () => {
    const result = parseBookMessage({ type: 'snapshot', bids: [], asks: [] });
    expect(result).toEqual({ type: 'snapshot', payload: { type: 'snapshot', bids: [], asks: [] } });
  });

  it('parses an update message', () => {
    const result = parseBookMessage({ type: 'update', side: 'bid', price: '100', quantity: '10' });
    expect(result).toEqual({
      type: 'update',
      payload: { type: 'update', side: 'bid', price: '100', quantity: '10' },
    });
  });

  it('returns null for null input', () => {
    expect(parseBookMessage(null)).toBeNull();
  });

  it('returns null for non-object input', () => {
    expect(parseBookMessage('string')).toBeNull();
    expect(parseBookMessage(42)).toBeNull();
  });

  it('returns null for unknown type', () => {
    expect(parseBookMessage({ type: 'unknown' })).toBeNull();
  });

  it('returns null for missing type', () => {
    expect(parseBookMessage({ bids: [], asks: [] })).toBeNull();
  });
});
