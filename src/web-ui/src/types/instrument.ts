export interface Instrument {
  instrumentId: string;
  symbol: string;
  commodityName: string;
  deliveryMonth: string;
  deliveryLocation: string;
  tickSize: string;
  lotSize: string;
  status: 'active' | 'halted' | 'expired';
}

export interface Ticker {
  instrumentId: string;
  lastPrice: string;
  change24h: string;
  changePercent24h: string;
  high24h: string;
  low24h: string;
  volume24h: string;
}
