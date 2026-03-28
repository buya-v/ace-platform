export interface Trade {
  tradeId: string;
  price: string;
  quantity: string;
  side: 'buy' | 'sell';
  timestamp: string;
  sequence: number;
}

export interface PriceLevel {
  price: string;
  quantity: string;
  orderCount: number;
}

export interface OrderBookState {
  bids: PriceLevel[];
  asks: PriceLevel[];
  sequence: number;
  lastUpdated: string;
}

export interface BookDelta {
  side: 'bid' | 'ask';
  price: string;
  quantity: string;
  orderCount: number;
  sequence: number;
}

export interface Position {
  instrumentId: string;
  instrumentSymbol: string;
  netQuantity: string;
  avgEntryPrice: string;
  unrealizedPnl: string;
  realizedPnl: string;
  side: 'long' | 'short' | 'flat';
}

export interface PnlSummary {
  totalRealizedPnl: string;
  totalUnrealizedPnl: string;
  totalPnl: string;
  currency: string;
}

export interface MarginStatus {
  accountBalance: string;
  usedMargin: string;
  availableMargin: string;
  marginUtilization: number;
  marginCalls: MarginCall[];
}

export interface MarginCall {
  callId: string;
  amount: string;
  deadline: string;
  status: 'pending' | 'met' | 'breached';
}

export interface Candle {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}
