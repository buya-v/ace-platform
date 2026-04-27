export interface Instrument {
  id: string;
  isin: string;
  ticker: string;
  name: string;
  asset_class: string;
  security_type: string;
  exchange_code: string;
  lot_size: number;
  tick_size: number;
  currency: string;
  listing_date: string;
  trading_status: string;
  outstanding_shares: number;
  created_at: string;
  updated_at: string;
}

export interface InstrumentDetail extends Instrument {
  // Securities instruments carry all fields in the base Instrument interface.
  // This type exists for compatibility with components that expect InstrumentDetail.
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
