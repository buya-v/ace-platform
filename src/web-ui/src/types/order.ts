export type OrderSide = 'buy' | 'sell';
export type OrderType = 'limit' | 'market' | 'ioc' | 'fok';
export type TimeInForce = 'gtc' | 'ioc' | 'fok' | 'day';

export interface SubmitOrderRequest {
  instrument_id: string;
  side: OrderSide;
  order_type: OrderType;
  quantity: string;
  price?: string;
  time_in_force?: TimeInForce;
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

  if (req.order_type === 'limit') {
    if (!req.price || isNaN(Number(req.price)) || Number(req.price) <= 0) {
      errors.push({ field: 'price', message: 'Price is required for limit orders' });
    }
  }

  return errors;
}
