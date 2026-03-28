import React, { createContext, useContext, useReducer, useCallback } from 'react';
import type { Instrument } from '../types/instrument';
import type { OrderBookState, Trade, Position, PnlSummary, MarginStatus, BookDelta, PriceLevel } from '../types/trade';
import type { WSStatus } from '../services/ws';

interface TradingState {
  selectedInstrument: Instrument | null;
  orderBook: OrderBookState;
  recentTrades: Trade[];
  positions: Position[];
  pnl: PnlSummary | null;
  margin: MarginStatus | null;
  wsStatus: WSStatus;
}

type TradingAction =
  | { type: 'SET_INSTRUMENT'; instrument: Instrument }
  | { type: 'BOOK_SNAPSHOT'; book: OrderBookState }
  | { type: 'BOOK_UPDATE'; update: BookDelta }
  | { type: 'NEW_TRADE'; trade: Trade }
  | { type: 'SET_POSITIONS'; positions: Position[] }
  | { type: 'SET_PNL'; pnl: PnlSummary }
  | { type: 'SET_MARGIN'; margin: MarginStatus }
  | { type: 'WS_STATUS_CHANGE'; status: WSStatus };

const MAX_TRADES = 100;

export function applyBookDelta(levels: PriceLevel[], delta: BookDelta, ascending: boolean): PriceLevel[] {
  const newLevels = levels.filter((l) => l.price !== delta.price);
  if (Number(delta.quantity) > 0) {
    newLevels.push({
      price: delta.price,
      quantity: delta.quantity,
      orderCount: delta.orderCount,
    });
  }
  newLevels.sort((a, b) => {
    const diff = Number(a.price) - Number(b.price);
    return ascending ? diff : -diff;
  });
  return newLevels;
}

export function tradingReducer(state: TradingState, action: TradingAction): TradingState {
  switch (action.type) {
    case 'SET_INSTRUMENT':
      return {
        ...state,
        selectedInstrument: action.instrument,
        orderBook: { bids: [], asks: [], sequence: 0, lastUpdated: '' },
        recentTrades: [],
      };
    case 'BOOK_SNAPSHOT':
      return { ...state, orderBook: action.book };
    case 'BOOK_UPDATE': {
      const { update } = action;
      if (update.sequence <= state.orderBook.sequence) return state;
      const book = { ...state.orderBook, sequence: update.sequence, lastUpdated: new Date().toISOString() };
      if (update.side === 'bid') {
        book.bids = applyBookDelta(state.orderBook.bids, update, false);
      } else {
        book.asks = applyBookDelta(state.orderBook.asks, update, true);
      }
      return { ...state, orderBook: book };
    }
    case 'NEW_TRADE':
      return {
        ...state,
        recentTrades: [action.trade, ...state.recentTrades].slice(0, MAX_TRADES),
      };
    case 'SET_POSITIONS':
      return { ...state, positions: action.positions };
    case 'SET_PNL':
      return { ...state, pnl: action.pnl };
    case 'SET_MARGIN':
      return { ...state, margin: action.margin };
    case 'WS_STATUS_CHANGE':
      return { ...state, wsStatus: action.status };
    default:
      return state;
  }
}

const initialState: TradingState = {
  selectedInstrument: null,
  orderBook: { bids: [], asks: [], sequence: 0, lastUpdated: '' },
  recentTrades: [],
  positions: [],
  pnl: null,
  margin: null,
  wsStatus: 'disconnected',
};

interface TradingContextValue {
  state: TradingState;
  dispatch: React.Dispatch<TradingAction>;
  selectInstrument: (instrument: Instrument) => void;
}

const TradingContext = createContext<TradingContextValue | null>(null);

export function TradingProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(tradingReducer, initialState);

  const selectInstrument = useCallback((instrument: Instrument) => {
    dispatch({ type: 'SET_INSTRUMENT', instrument });
  }, []);

  return (
    <TradingContext.Provider value={{ state, dispatch, selectInstrument }}>
      {children}
    </TradingContext.Provider>
  );
}

export function useTrading(): TradingContextValue {
  const ctx = useContext(TradingContext);
  if (!ctx) throw new Error('useTrading must be used within TradingProvider');
  return ctx;
}
