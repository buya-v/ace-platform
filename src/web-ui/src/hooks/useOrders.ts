import { useState, useCallback } from 'react';
import { apiRequest } from '../services/api';
import type { SubmitOrderRequest, SubmitOrderResponse } from '../types/order';

interface UseOrdersReturn {
  submitOrder: (req: SubmitOrderRequest) => Promise<SubmitOrderResponse>;
  submitting: boolean;
  lastError: string | null;
  lastResult: SubmitOrderResponse | null;
  clearError: () => void;
}

export function useOrders(): UseOrdersReturn {
  const [submitting, setSubmitting] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<SubmitOrderResponse | null>(null);

  const submitOrder = useCallback(async (req: SubmitOrderRequest): Promise<SubmitOrderResponse> => {
    setSubmitting(true);
    setLastError(null);
    try {
      const result = await apiRequest<SubmitOrderResponse>('/securities/orders', {
        method: 'POST',
        body: JSON.stringify(req),
      });
      setLastResult(result);
      return result;
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Order submission failed';
      setLastError(message);
      throw err;
    } finally {
      setSubmitting(false);
    }
  }, []);

  const clearError = useCallback(() => setLastError(null), []);

  return { submitOrder, submitting, lastError, lastResult, clearError };
}
