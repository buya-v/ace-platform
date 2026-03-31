export type OrderSide = 'buy' | 'sell';
export type OrderType = 'limit' | 'market' | 'stop-limit' | 'stop-market' | 'iceberg';
export type TimeInForce = 'day' | 'gtc' | 'gtd' | 'ioc' | 'fok';

export interface SubmitOrderRequest {
  instrument_id: string;
  side: OrderSide;
  order_type: OrderType;
  quantity: string;
  price?: string;
  stop_price?: string;
  display_qty?: string;
  time_in_force?: TimeInForce;
  expire_time?: string;
  client_order_id?: string;
}

export interface SubmitOrderResponse {
  order_id: string;
  client_order_id: string;
  status: 'pending' | 'accepted' | 'rejected';
  instrument_id: string;
  side: OrderSide;
  order_type: string;
  quantity: string;
  price: string;
  created_at: string;
  reject_reason?: string;
}

export interface OrderValidationError {
  field: string;
  message: string;
}

/** Order types that require a limit price */
export function requiresPrice(orderType: OrderType): boolean {
  return orderType === 'limit' || orderType === 'stop-limit' || orderType === 'iceberg';
}

/** Order types that require a stop price */
export function requiresStopPrice(orderType: OrderType): boolean {
  return orderType === 'stop-limit' || orderType === 'stop-market';
}

/** Order types that require a display quantity */
export function requiresDisplayQty(orderType: OrderType): boolean {
  return orderType === 'iceberg';
}

/** Whether time-in-force GTD requires an expiry date */
export function requiresExpiry(tif: TimeInForce): boolean {
  return tif === 'gtd';
}

export function validateOrder(req: Partial<SubmitOrderRequest>): OrderValidationError[] {
  const errors: OrderValidationError[] = [];

  if (!req.instrument_id) {
    errors.push({ field: 'instrument_id', message: 'Instrument is required' });
  }

  if (!req.side || (req.side !== 'buy' && req.side !== 'sell')) {
    errors.push({ field: 'side', message: 'Side must be buy or sell' });
  }

  if (!req.order_type) {
    errors.push({ field: 'order_type', message: 'Order type is required' });
  }

  if (!req.quantity || isNaN(Number(req.quantity)) || Number(req.quantity) <= 0) {
    errors.push({ field: 'quantity', message: 'Quantity must be a positive number' });
  }

  if (req.order_type && requiresPrice(req.order_type)) {
    if (!req.price || isNaN(Number(req.price)) || Number(req.price) <= 0) {
      errors.push({ field: 'price', message: 'Price is required for limit orders' });
    }
  }

  if (req.order_type && requiresStopPrice(req.order_type)) {
    if (!req.stop_price || isNaN(Number(req.stop_price)) || Number(req.stop_price) <= 0) {
      errors.push({ field: 'stop_price', message: 'Stop price is required for stop orders' });
    }
  }

  if (req.order_type && requiresDisplayQty(req.order_type)) {
    if (!req.display_qty || isNaN(Number(req.display_qty)) || Number(req.display_qty) <= 0) {
      errors.push({ field: 'display_qty', message: 'Display quantity is required for iceberg orders' });
    }
    if (req.display_qty && req.quantity && Number(req.display_qty) >= Number(req.quantity)) {
      errors.push({ field: 'display_qty', message: 'Display quantity must be less than total quantity' });
    }
  }

  if (req.time_in_force === 'gtd') {
    if (!req.expire_time) {
      errors.push({ field: 'expire_time', message: 'Expiry date is required for GTD orders' });
    } else {
      const expiry = new Date(req.expire_time);
      if (isNaN(expiry.getTime())) {
        errors.push({ field: 'expire_time', message: 'Invalid expiry date' });
      } else if (expiry.getTime() <= Date.now()) {
        errors.push({ field: 'expire_time', message: 'Expiry date must be in the future' });
      }
    }
  }

  return errors;
}
