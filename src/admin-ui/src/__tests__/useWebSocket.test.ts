import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useWebSocket } from '../hooks/useWebSocket';

// Mock getConfig
vi.mock('../config', () => ({
  getConfig: () => ({
    API_BASE_URL: '/api/v1',
    WS_BASE_URL: 'ws://localhost:8080/api/v1/ws',
    HEALTH_POLL_INTERVAL: 15000,
    AUTH_TOKEN_REFRESH_BUFFER: 60,
  }),
}));

// --- Mock WebSocket ---
type MockWSInstance = {
  url: string;
  onopen: ((ev: Event) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  close: ReturnType<typeof vi.fn>;
  readyState: number;
};

let mockInstances: MockWSInstance[] = [];

class MockWebSocket implements MockWSInstance {
  url: string;
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  close = vi.fn();
  readyState = 0; // CONNECTING

  constructor(url: string) {
    this.url = url;
    mockInstances.push(this);
  }
}

function latestWS(): MockWSInstance {
  return mockInstances[mockInstances.length - 1];
}

function simulateOpen(ws: MockWSInstance) {
  ws.readyState = 1;
  ws.onopen?.(new Event('open'));
}

function simulateMessage(ws: MockWSInstance, data: unknown) {
  ws.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }));
}

function simulateClose(ws: MockWSInstance) {
  ws.readyState = 3;
  ws.onclose?.(new CloseEvent('close'));
}

function simulateError(ws: MockWSInstance) {
  ws.onerror?.(new Event('error'));
}

describe('useWebSocket', () => {
  beforeEach(() => {
    mockInstances = [];
    vi.useFakeTimers();
    vi.stubGlobal('WebSocket', MockWebSocket);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('connects to the correct URL', () => {
    renderHook(() => useWebSocket('/book/WHT'));

    expect(mockInstances.length).toBe(1);
    expect(latestWS().url).toBe('ws://localhost:8080/api/v1/ws/book/WHT');
  });

  it('starts with connecting status', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    // After mount, a WS was created so status should be connecting
    expect(result.current.status).toBe('connecting');
    expect(result.current.data).toBeNull();
    expect(result.current.reconnectCount).toBe(0);
  });

  it('transitions to connected on open', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    act(() => {
      simulateOpen(latestWS());
    });

    expect(result.current.status).toBe('connected');
  });

  it('parses JSON messages and updates data', () => {
    const { result } = renderHook(() => useWebSocket<{ bids: number[] }>('/book/A'));

    act(() => {
      simulateOpen(latestWS());
    });

    act(() => {
      simulateMessage(latestWS(), { bids: [1, 2, 3] });
    });

    expect(result.current.data).toEqual({ bids: [1, 2, 3] });
  });

  it('ignores non-JSON messages', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    act(() => {
      simulateOpen(latestWS());
    });

    act(() => {
      const ws = latestWS();
      ws.onmessage?.(new MessageEvent('message', { data: 'not json {{{' }));
    });

    expect(result.current.data).toBeNull();
  });

  it('sets error status on WebSocket error', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    act(() => {
      simulateError(latestWS());
    });

    expect(result.current.status).toBe('error');
  });

  it('transitions to disconnected on close and schedules reconnect', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    act(() => {
      simulateOpen(latestWS());
    });
    expect(result.current.status).toBe('connected');

    act(() => {
      simulateClose(latestWS());
    });

    expect(result.current.status).toBe('disconnected');
    expect(result.current.reconnectCount).toBe(1);
  });

  it('reconnects with exponential backoff', () => {
    const { result } = renderHook(() => useWebSocket('/test'));

    // First close -> reconnect after 1s (2^0 * 1000)
    act(() => {
      simulateClose(latestWS());
    });
    expect(result.current.reconnectCount).toBe(1);
    expect(mockInstances.length).toBe(1); // No new WS yet

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(mockInstances.length).toBe(2); // Reconnected

    // Second close -> reconnect after 2s (2^1 * 1000)
    act(() => {
      simulateClose(latestWS());
    });
    expect(result.current.reconnectCount).toBe(2);

    act(() => {
      vi.advanceTimersByTime(1999);
    });
    expect(mockInstances.length).toBe(2); // Not yet

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(mockInstances.length).toBe(3); // Reconnected at 2s

    // Third close -> reconnect after 4s (2^2 * 1000)
    act(() => {
      simulateClose(latestWS());
    });
    expect(result.current.reconnectCount).toBe(3);

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(mockInstances.length).toBe(4);
  });

  it('resets backoff on successful connection', () => {
    renderHook(() => useWebSocket('/test'));

    // Close -> reconnect
    act(() => {
      simulateClose(latestWS());
    });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(mockInstances.length).toBe(2);

    // Open successfully -> resets attempt counter
    act(() => {
      simulateOpen(latestWS());
    });

    // Close again -> should reconnect after 1s (reset), not 2s
    act(() => {
      simulateClose(latestWS());
    });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(mockInstances.length).toBe(3); // Reconnected after 1s, confirming reset
  });

  it('caps backoff at 30 seconds', () => {
    renderHook(() => useWebSocket('/test'));

    // Simulate many disconnects to push backoff high
    for (let i = 0; i < 10; i++) {
      act(() => {
        simulateClose(latestWS());
      });
      act(() => {
        vi.advanceTimersByTime(30000);
      });
    }

    // After 10 attempts, backoff would be 2^10 * 1000 = 1,024,000 ms
    // But capped at 30,000 ms
    const countBefore = mockInstances.length;
    act(() => {
      simulateClose(latestWS());
    });
    act(() => {
      vi.advanceTimersByTime(30000);
    });
    expect(mockInstances.length).toBe(countBefore + 1);
  });

  it('cleans up WebSocket on unmount', () => {
    const { unmount } = renderHook(() => useWebSocket('/test'));

    const ws = latestWS();
    act(() => {
      simulateOpen(ws);
    });

    unmount();

    expect(ws.close).toHaveBeenCalled();
  });

  it('clears reconnect timer on unmount', () => {
    const { unmount } = renderHook(() => useWebSocket('/test'));

    // Trigger close to schedule reconnect
    act(() => {
      simulateClose(latestWS());
    });

    const countBefore = mockInstances.length;
    unmount();

    // Advance past backoff — should NOT create new WS
    act(() => {
      vi.advanceTimersByTime(60000);
    });
    expect(mockInstances.length).toBe(countBefore);
  });

  it('does not connect when enabled is false', () => {
    const { result } = renderHook(() => useWebSocket('/test', { enabled: false }));

    expect(mockInstances.length).toBe(0);
    expect(result.current.status).toBe('disconnected');
  });

  it('connects when enabled changes from false to true', () => {
    const { result, rerender } = renderHook(
      ({ enabled }) => useWebSocket('/test', { enabled }),
      { initialProps: { enabled: false } },
    );

    expect(mockInstances.length).toBe(0);
    expect(result.current.status).toBe('disconnected');

    rerender({ enabled: true });

    expect(mockInstances.length).toBe(1);
    expect(result.current.status).toBe('connecting');
  });

  it('reconnects when path changes', () => {
    const { rerender } = renderHook(
      ({ path }) => useWebSocket(path),
      { initialProps: { path: '/book/A' } },
    );

    expect(mockInstances.length).toBe(1);
    expect(latestWS().url).toBe('ws://localhost:8080/api/v1/ws/book/A');

    const firstWs = latestWS();
    rerender({ path: '/book/B' });

    expect(firstWs.close).toHaveBeenCalled();
    expect(mockInstances.length).toBe(2);
    expect(latestWS().url).toBe('ws://localhost:8080/api/v1/ws/book/B');
  });

  it('handles multiple messages updating data', () => {
    const { result } = renderHook(() => useWebSocket<{ value: number }>('/test'));

    act(() => {
      simulateOpen(latestWS());
    });

    act(() => {
      simulateMessage(latestWS(), { value: 1 });
    });
    expect(result.current.data).toEqual({ value: 1 });

    act(() => {
      simulateMessage(latestWS(), { value: 2 });
    });
    expect(result.current.data).toEqual({ value: 2 });
  });

  it('strips trailing/leading slashes when building URL', () => {
    renderHook(() => useWebSocket('///book/X'));

    expect(latestWS().url).toBe('ws://localhost:8080/api/v1/ws/book/X');
  });
});
