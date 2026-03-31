import React, { useState, useCallback } from 'react';
import type { OrderSide, OrderType, TimeInForce, SubmitOrderRequest } from '../types/order';
import { validateOrder, requiresPrice, requiresStopPrice, requiresDisplayQty, requiresExpiry } from '../types/order';
import { useOrders } from '../hooks/useOrders';
import styles from './OrderEntry.module.css';

interface OrderEntryProps {
  instrumentId: string | null;
  prefillPrice?: string;
}

const ORDER_TYPES: { value: OrderType; label: string }[] = [
  { value: 'limit', label: 'Limit' },
  { value: 'market', label: 'Market' },
  { value: 'stop-limit', label: 'Stop-Limit' },
  { value: 'stop-market', label: 'Stop-Mkt' },
  { value: 'iceberg', label: 'Iceberg' },
];

const TIME_IN_FORCE_OPTIONS: { value: TimeInForce; label: string }[] = [
  { value: 'day', label: 'DAY' },
  { value: 'gtc', label: 'GTC' },
  { value: 'gtd', label: 'GTD' },
  { value: 'ioc', label: 'IOC' },
  { value: 'fok', label: 'FOK' },
];

export const OrderEntry: React.FC<OrderEntryProps> = ({ instrumentId, prefillPrice }) => {
  const [side, setSide] = useState<OrderSide>('buy');
  const [orderType, setOrderType] = useState<OrderType>('limit');
  const [timeInForce, setTimeInForce] = useState<TimeInForce>('day');
  const [price, setPrice] = useState(prefillPrice ?? '');
  const [stopPrice, setStopPrice] = useState('');
  const [quantity, setQuantity] = useState('');
  const [displayQty, setDisplayQty] = useState('');
  const [expireTime, setExpireTime] = useState('');
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
      time_in_force: timeInForce,
      ...(requiresPrice(orderType) ? { price } : {}),
      ...(requiresStopPrice(orderType) ? { stop_price: stopPrice } : {}),
      ...(requiresDisplayQty(orderType) ? { display_qty: displayQty } : {}),
      ...(requiresExpiry(timeInForce) ? { expire_time: expireTime } : {}),
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
      if (!requiresPrice(orderType)) setPrice('');
      setStopPrice('');
      setDisplayQty('');
    } catch {
      // Error is captured in lastError
    }
  }, [instrumentId, side, orderType, timeInForce, price, stopPrice, quantity, displayQty, expireTime, submitOrder, clearError]);

  const total = requiresPrice(orderType) && price && quantity
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

      <div className={styles.field}>
        <label htmlFor="orderType">Order Type</label>
        <select
          id="orderType"
          className={styles.select}
          value={orderType}
          onChange={(e) => setOrderType(e.target.value as OrderType)}
        >
          {ORDER_TYPES.map((t) => (
            <option key={t.value} value={t.value}>{t.label}</option>
          ))}
        </select>
      </div>

      <div className={styles.field}>
        <label htmlFor="tif">Time in Force</label>
        <select
          id="tif"
          className={styles.select}
          value={timeInForce}
          onChange={(e) => setTimeInForce(e.target.value as TimeInForce)}
        >
          {TIME_IN_FORCE_OPTIONS.map((t) => (
            <option key={t.value} value={t.value}>{t.label}</option>
          ))}
        </select>
      </div>

      {requiresExpiry(timeInForce) && (
        <div className={styles.field}>
          <label htmlFor="expireTime">Expiry Date</label>
          <input
            id="expireTime"
            type="datetime-local"
            className={styles.dateInput}
            value={expireTime}
            onChange={(e) => setExpireTime(e.target.value)}
          />
          {errors.expire_time && <span className={styles.error}>{errors.expire_time}</span>}
        </div>
      )}

      {requiresStopPrice(orderType) && (
        <div className={styles.field}>
          <label htmlFor="stopPrice">Stop Price</label>
          <input
            id="stopPrice"
            type="text"
            inputMode="decimal"
            value={stopPrice}
            onChange={(e) => setStopPrice(e.target.value)}
            placeholder="0.0000"
          />
          {errors.stop_price && <span className={styles.error}>{errors.stop_price}</span>}
        </div>
      )}

      {requiresPrice(orderType) && (
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

      {requiresDisplayQty(orderType) && (
        <div className={styles.field}>
          <label htmlFor="displayQty">Display Qty</label>
          <input
            id="displayQty"
            type="text"
            inputMode="decimal"
            value={displayQty}
            onChange={(e) => setDisplayQty(e.target.value)}
            placeholder="Visible quantity"
          />
          {errors.display_qty && <span className={styles.error}>{errors.display_qty}</span>}
        </div>
      )}

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
