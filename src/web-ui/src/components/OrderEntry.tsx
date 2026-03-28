import React, { useState, useCallback } from 'react';
import type { OrderSide, OrderType, SubmitOrderRequest } from '../types/order';
import { validateOrder } from '../types/order';
import { useOrders } from '../hooks/useOrders';
import styles from './OrderEntry.module.css';

interface OrderEntryProps {
  instrumentId: string | null;
  prefillPrice?: string;
}

const ORDER_TYPES: OrderType[] = ['limit', 'market', 'ioc', 'fok'];

export const OrderEntry: React.FC<OrderEntryProps> = ({ instrumentId, prefillPrice }) => {
  const [side, setSide] = useState<OrderSide>('buy');
  const [orderType, setOrderType] = useState<OrderType>('limit');
  const [price, setPrice] = useState(prefillPrice ?? '');
  const [quantity, setQuantity] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const { submitOrder, submitting, lastError, lastResult, clearError } = useOrders();

  React.useEffect(() => {
    if (prefillPrice) setPrice(prefillPrice);
  }, [prefillPrice]);

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!instrumentId) return;

    const req: SubmitOrderRequest = {
      instrument_id: instrumentId,
      side,
      order_type: orderType,
      quantity,
      ...(orderType === 'limit' ? { price } : {}),
    };

    const validationErrors = validateOrder(req);
    if (validationErrors.length > 0) {
      const errMap: Record<string, string> = {};
      validationErrors.forEach((e) => (errMap[e.field] = e.message));
      setErrors(errMap);
      return;
    }

    setErrors({});
    clearError();

    try {
      await submitOrder(req);
      setQuantity('');
      if (orderType !== 'limit') setPrice('');
    } catch {
      // Error is captured in lastError
    }
  }, [instrumentId, side, orderType, price, quantity, submitOrder, clearError]);

  const total = orderType === 'limit' && price && quantity
    ? (Number(price) * Number(quantity)).toFixed(4)
    : '';

  return (
    <form className={styles.orderEntry} onSubmit={handleSubmit}>
      <div className={styles.sideToggle}>
        <button
          type="button"
          className={`${styles.sideBtn} ${side === 'buy' ? styles.buyActive : ''}`}
          onClick={() => setSide('buy')}
        >
          Buy
        </button>
        <button
          type="button"
          className={`${styles.sideBtn} ${side === 'sell' ? styles.sellActive : ''}`}
          onClick={() => setSide('sell')}
        >
          Sell
        </button>
      </div>

      <div className={styles.typeSelector}>
        {ORDER_TYPES.map((t) => (
          <button
            key={t}
            type="button"
            className={`${styles.typeBtn} ${orderType === t ? styles.typeActive : ''}`}
            onClick={() => setOrderType(t)}
          >
            {t.toUpperCase()}
          </button>
        ))}
      </div>

      {orderType === 'limit' && (
        <div className={styles.field}>
          <label htmlFor="price">Price</label>
          <input
            id="price"
            type="text"
            inputMode="decimal"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            placeholder="0.0000"
          />
          {errors.price && <span className={styles.error}>{errors.price}</span>}
        </div>
      )}

      <div className={styles.field}>
        <label htmlFor="quantity">Quantity</label>
        <input
          id="quantity"
          type="text"
          inputMode="decimal"
          value={quantity}
          onChange={(e) => setQuantity(e.target.value)}
          placeholder="0"
        />
        {errors.quantity && <span className={styles.error}>{errors.quantity}</span>}
      </div>

      {total && (
        <div className={styles.total}>
          Total: {total}
        </div>
      )}

      <button
        type="submit"
        disabled={submitting || !instrumentId}
        className={`${styles.submitBtn} ${side === 'buy' ? styles.buySubmit : styles.sellSubmit}`}
      >
        {submitting ? 'Submitting...' : `${side === 'buy' ? 'Buy' : 'Sell'}`}
      </button>

      {lastError && <div className={styles.errorBanner}>{lastError}</div>}
      {lastResult && lastResult.status === 'accepted' && (
        <div className={styles.successBanner}>Order accepted</div>
      )}
    </form>
  );
};
