import { describe, it, expect } from 'vitest';
import { tradingReducer, applyBookDelta } from '../contexts/MarketContext';
import type { PriceLevel } from '../types/trade';

const initialState = {
  selectedInstrument: null,
  orderBook: { bids: [], asks: [], sequence: 0, lastUpdated: '' },
  recentTrades: [],
  positions: [],
  pnl: null,
  margin: null,
  wsStatus: 'disconnected' as const,
};

describe('tradingReducer', () => {
  it('SET_INSTRUMENT resets book and trades', () => {
    const state = {
      ...initialState,
      orderBook: {
        bids: [{ price: '100', quantity: '10', orderCount: 1 }],
        asks: [],
        sequence: 5,
        lastUpdated: 'old',
      },
      recentTrades: [
        { tradeId: 't1', price: '100', quantity: '1', side: 'buy' as const, timestamp: '', sequence: 1 },
      ],
    };

    const result = tradingReducer(state, {
      type: 'SET_INSTRUMENT',
      instrument: {
        id: 'new',
        isin: 'MN0000000001',
        ticker: 'WHEAT',
        name: 'Wheat',
        asset_class: 'commodity',
        security_type: 'future',
        exchange_code: 'ACE',
        lot_size: 1,
        tick_size: 0.01,
        currency: 'MNT',
        listing_date: '2026-01-01',
        trading_status: 'active',
        outstanding_shares: 0,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    });

    expect(result.selectedInstrument?.id).toBe('new');
    expect(result.orderBook.bids).toEqual([]);
    expect(result.orderBook.asks).toEqual([]);
    expect(result.recentTrades).toEqual([]);
  });

  it('BOOK_SNAPSHOT replaces the order book', () => {
    const book = {
      bids: [{ price: '100', quantity: '10', orderCount: 2 }],
      asks: [{ price: '101', quantity: '5', orderCount: 1 }],
      sequence: 10,
      lastUpdated: '2026-01-01',
    };

    const result = tradingReducer(initialState, { type: 'BOOK_SNAPSHOT', book });
    expect(result.orderBook).toEqual(book);
  });

  it('BOOK_UPDATE ignores stale sequences', () => {
    const state = {
      ...initialState,
      orderBook: {
        bids: [{ price: '100', quantity: '10', orderCount: 1 }],
        asks: [],
        sequence: 5,
        lastUpdated: '',
      },
    };

    const result = tradingReducer(state, {
      type: 'BOOK_UPDATE',
      update: { side: 'bid', price: '99', quantity: '5', orderCount: 1, sequence: 3 },
    });

    // Should not change because sequence 3 < 5
    expect(result.orderBook.bids.length).toBe(1);
    expect(result.orderBook.sequence).toBe(5);
  });

  it('BOOK_UPDATE adds a new bid level', () => {
    const state = {
      ...initialState,
      orderBook: {
        bids: [{ price: '100', quantity: '10', orderCount: 1 }],
        asks: [],
        sequence: 5,
        lastUpdated: '',
      },
    };

    const result = tradingReducer(state, {
      type: 'BOOK_UPDATE',
      update: { side: 'bid', price: '99', quantity: '5', orderCount: 1, sequence: 6 },
    });

    expect(result.orderBook.bids.length).toBe(2);
    // Bids sorted descending
    expect(result.orderBook.bids[0].price).toBe('100');
    expect(result.orderBook.bids[1].price).toBe('99');
    expect(result.orderBook.sequence).toBe(6);
  });

  it('BOOK_UPDATE removes level when quantity is 0', () => {
    const state = {
      ...initialState,
      orderBook: {
        bids: [
          { price: '100', quantity: '10', orderCount: 1 },
          { price: '99', quantity: '5', orderCount: 1 },
        ],
        asks: [],
        sequence: 5,
        lastUpdated: '',
      },
    };

    const result = tradingReducer(state, {
      type: 'BOOK_UPDATE',
      update: { side: 'bid', price: '100', quantity: '0', orderCount: 0, sequence: 6 },
    });

    expect(result.orderBook.bids.length).toBe(1);
    expect(result.orderBook.bids[0].price).toBe('99');
  });

  it('NEW_TRADE prepends trade and caps at 100', () => {
    const trades = Array.from({ length: 100 }, (_, i) => ({
      tradeId: `t${i}`,
      price: '100',
      quantity: '1',
      side: 'buy' as const,
      timestamp: '',
      sequence: i,
    }));

    const state = { ...initialState, recentTrades: trades };
    const newTrade = {
      tradeId: 'new',
      price: '101',
      quantity: '2',
      side: 'sell' as const,
      timestamp: '',
      sequence: 100,
    };

    const result = tradingReducer(state, { type: 'NEW_TRADE', trade: newTrade });
    expect(result.recentTrades.length).toBe(100);
    expect(result.recentTrades[0].tradeId).toBe('new');
  });

  it('SET_POSITIONS updates positions', () => {
    const positions = [
      {
        instrumentId: 'abc',
        instrumentSymbol: 'WHEAT',
        netQuantity: '10',
        avgEntryPrice: '100',
        unrealizedPnl: '50',
        realizedPnl: '0',
        side: 'long' as const,
      },
    ];

    const result = tradingReducer(initialState, { type: 'SET_POSITIONS', positions });
    expect(result.positions).toEqual(positions);
  });

  it('SET_MARGIN updates margin', () => {
    const margin = {
      accountBalance: '10000',
      usedMargin: '2000',
      availableMargin: '8000',
      marginUtilization: 0.2,
      marginCalls: [],
    };

    const result = tradingReducer(initialState, { type: 'SET_MARGIN', margin });
    expect(result.margin).toEqual(margin);
  });

  it('WS_STATUS_CHANGE updates status', () => {
    const result = tradingReducer(initialState, { type: 'WS_STATUS_CHANGE', status: 'connected' });
    expect(result.wsStatus).toBe('connected');
  });
});

describe('applyBookDelta', () => {
  it('adds a new level in ascending order', () => {
    const levels: PriceLevel[] = [
      { price: '100', quantity: '10', orderCount: 1 },
      { price: '102', quantity: '5', orderCount: 1 },
    ];

    const result = applyBookDelta(levels, {
      side: 'ask',
      price: '101',
      quantity: '3',
      orderCount: 1,
      sequence: 1,
    }, true);

    expect(result.map((l) => l.price)).toEqual(['100', '101', '102']);
  });

  it('adds a new level in descending order', () => {
    const levels: PriceLevel[] = [
      { price: '102', quantity: '10', orderCount: 1 },
      { price: '100', quantity: '5', orderCount: 1 },
    ];

    const result = applyBookDelta(levels, {
      side: 'bid',
      price: '101',
      quantity: '3',
      orderCount: 1,
      sequence: 1,
    }, false);

    expect(result.map((l) => l.price)).toEqual(['102', '101', '100']);
  });

  it('updates an existing level', () => {
    const levels: PriceLevel[] = [
      { price: '100', quantity: '10', orderCount: 1 },
    ];

    const result = applyBookDelta(levels, {
      side: 'bid',
      price: '100',
      quantity: '15',
      orderCount: 2,
      sequence: 1,
    }, false);

    expect(result.length).toBe(1);
    expect(result[0].quantity).toBe('15');
    expect(result[0].orderCount).toBe(2);
  });

  it('removes level when quantity is 0', () => {
    const levels: PriceLevel[] = [
      { price: '100', quantity: '10', orderCount: 1 },
      { price: '99', quantity: '5', orderCount: 1 },
    ];

    const result = applyBookDelta(levels, {
      side: 'bid',
      price: '100',
      quantity: '0',
      orderCount: 0,
      sequence: 1,
    }, false);

    expect(result.length).toBe(1);
    expect(result[0].price).toBe('99');
  });
});
