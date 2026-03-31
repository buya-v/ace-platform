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

export interface InstrumentDetail extends Instrument {
  contractSize: string;
  positionLimit: string;
  currency: string;
  exchange: string;
  firstTradeDate: string;
  lastTradeDate: string;
  settlementType: 'physical' | 'cash';
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
