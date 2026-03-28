import { useEffect, useRef, useCallback, useReducer } from 'react';

export interface PollingState<T> {
  data: T | null;
  isLoading: boolean;
  error: string | null;
  lastUpdated: string | null;
}

type PollingAction<T> =
  | { type: 'FETCH_START' }
  | { type: 'FETCH_SUCCESS'; payload: T }
  | { type: 'FETCH_ERROR'; payload: string };

function createReducer<T>() {
  return (state: PollingState<T>, action: PollingAction<T>): PollingState<T> => {
    switch (action.type) {
      case 'FETCH_START':
        return { ...state, isLoading: true };
      case 'FETCH_SUCCESS':
        return {
          data: action.payload,
          isLoading: false,
          error: null,
          lastUpdated: new Date().toISOString(),
        };
      case 'FETCH_ERROR':
        return {
          ...state,
          isLoading: false,
          error: action.payload,
        };
      default:
        return state;
    }
  };
}

export function usePolling<T>(
  fetchFn: (signal: AbortSignal) => Promise<T>,
  intervalMs: number,
  enabled: boolean = true,
): PollingState<T> & { refresh: () => void } {
  const initialState: PollingState<T> = {
    data: null,
    isLoading: false,
    error: null,
    lastUpdated: null,
  };

  const reducer = useRef(createReducer<T>()).current;
  const [state, dispatch] = useReducer(reducer, initialState);
  const fetchRef = useRef(fetchFn);
  fetchRef.current = fetchFn;

  const doFetch = useCallback((signal: AbortSignal) => {
    dispatch({ type: 'FETCH_START' });
    fetchRef.current(signal)
      .then(data => {
        if (!signal.aborted) {
          dispatch({ type: 'FETCH_SUCCESS', payload: data });
        }
      })
      .catch(err => {
        if (!signal.aborted) {
          dispatch({ type: 'FETCH_ERROR', payload: err instanceof Error ? err.message : 'Fetch failed' });
        }
      });
  }, []);

  const refreshTrigger = useRef(0);
  const [, forceUpdate] = useReducer(x => x + 1, 0);

  const refresh = useCallback(() => {
    refreshTrigger.current++;
    forceUpdate();
  }, []);

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    doFetch(controller.signal);

    if (intervalMs > 0) {
      const id = setInterval(() => {
        if (document.visibilityState !== 'hidden') {
          doFetch(controller.signal);
        }
      }, intervalMs);
      return () => {
        controller.abort();
        clearInterval(id);
      };
    }

    return () => controller.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, intervalMs, doFetch, refreshTrigger.current]);

  return { ...state, refresh };
}
