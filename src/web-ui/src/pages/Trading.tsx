import React, { useState, useEffect, useCallback } from 'react';
import { useTrading } from '../contexts/MarketContext';
import { useWebSocket } from '../hooks/useWebSocket';
import { useAuth } from '../contexts/AuthContext';
import { apiRequest } from '../services/api';
import { parseTradeMessage } from '../services/ws';
import { calculateTradeValue } from '../services/tradingUtils';
import { OrderBook } from '../components/OrderBook';
import { OrderEntry } from '../components/OrderEntry';
import { TradeHistory } from '../components/TradeHistory';
import { Chart } from '../components/Chart';
import { Positions } from '../components/Positions';
import { MarginStatusPanel } from '../components/MarginStatus';
import { InstrumentSelector } from '../components/InstrumentSelector';
import { InstrumentInfo } from '../components/InstrumentInfo';
import { ConnectionStatus } from '../components/ConnectionStatus';
import { ErrorBoundary } from '../components/ErrorBoundary';
import type { WSMessage } from '../services/ws';
import type { Position, PnlSummary, MarginStatus as MarginStatusType, TradeRecord } from '../types/trade';
import type { Instrument, Ticker } from '../types/instrument';
import styles from './Trading.module.css';

interface SecuritiesOrder {
  id: string;
  instrument_id: string;
  side: string;
  order_type: string;
  quantity: number;
  price: number;
  time_in_force: string;
  status: string;
  filled_quantity: number;
  created_at: string;
  updated_at: string;
}

export const Trading: React.FC = () => {
  const { state, dispatch, selectInstrument } = useTrading();
  const { logout } = useAuth();
  const [prefillPrice, setPrefillPrice] = useState<string | undefined>();
  const [tradeHistory, setTradeHistory] = useState<TradeRecord[]>([]);
  const [instruments, setInstruments] = useState<Instrument[]>([]);
  const [orders, setOrders] = useState<SecuritiesOrder[]>([]);
  const [tickers, setTickers] = useState<Map<string, Ticker>>(new Map());
  const [activeTab, setActiveTab] = useState<'positions' | 'history' | 'orders'>('orders');

  const instrumentId = state.selectedInstrument?.id ?? null;
  const wsBase = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`;

  const handleBookMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'snapshot') {
      const data = msg.data as { bids: []; asks: []; sequence: number };
      dispatch({
        type: 'BOOK_SNAPSHOT',
        book: {
          bids: data.bids || [],
          asks: data.asks || [],
          sequence: data.sequence || 0,
          lastUpdated: new Date().toISOString(),
        },
      });
    } else if (msg.type === 'update') {
      const data = msg.data as { side: 'bid' | 'ask'; price: string; quantity: string; orderCount: number; sequence: number };
      dispatch({ type: 'BOOK_UPDATE', update: data });
    }
  }, [dispatch]);

  const handleTradeMessage = useCallback((msg: WSMessage) => {
    const trade = parseTradeMessage(msg.data);
    if (trade) {
      dispatch({ type: 'NEW_TRADE', trade });
    }
  }, [dispatch]);

  const handleWsStatus = useCallback((status: 'connecting' | 'connected' | 'disconnected' | 'reconnecting') => {
    dispatch({ type: 'WS_STATUS_CHANGE', status });
  }, [dispatch]);

  // WebSocket feeds are not available for securities-service instruments.
  // Disabled to avoid connection errors in console.
  useWebSocket({
    url: null,
    onMessage: handleBookMessage,
    onStatusChange: handleWsStatus,
    enabled: false,
  });

  useWebSocket({
    url: null,
    onMessage: handleTradeMessage,
    enabled: false,
  });

  // Fetch instruments list
  useEffect(() => {
    apiRequest<{ data: Instrument[] }>('/securities/instruments')
      .then((res) => setInstruments(res.data || []))
      .catch(() => {});
  }, []);

  // Poll positions and margin — catch errors silently (these endpoints may not exist for securities)
  useEffect(() => {
    const poll = async () => {
      try {
        const [posData, marginData] = await Promise.allSettled([
          apiRequest<{ positions: Position[] }>('/clearing/positions'),
          apiRequest<MarginStatusType>('/margin'),
        ]);
        if (posData.status === 'fulfilled') dispatch({ type: 'SET_POSITIONS', positions: posData.value.positions || [] });
        if (marginData.status === 'fulfilled') dispatch({ type: 'SET_MARGIN', margin: marginData.value });
      } catch {
        // Silently fail polling — these commodity endpoints may not be available
      }
    };

    poll();
    const interval = setInterval(poll, 15000);
    return () => clearInterval(interval);
  }, [dispatch]);

  // Poll tickers for position current prices — silently handle missing market-data endpoint
  useEffect(() => {
    if (state.positions.length === 0) return;
    const pollTickers = () => {
      state.positions.forEach((pos) => {
        apiRequest<Ticker>(`/market-data/ticker?instrument_id=${pos.instrumentId}`)
          .then((ticker) => {
            setTickers((prev) => new Map(prev).set(pos.instrumentId, ticker));
          })
          .catch(() => {});
      });
    };
    pollTickers();
    const interval = setInterval(pollTickers, 15000);
    return () => clearInterval(interval);
  }, [state.positions]);

  // Settlement cycles — silently handle missing endpoint
  useEffect(() => {
    apiRequest<PnlSummary>('/settlement/cycles')
      .then((pnl) => dispatch({ type: 'SET_PNL', pnl }))
      .catch(() => {});

    const interval = setInterval(() => {
      apiRequest<PnlSummary>('/settlement/cycles')
        .then((pnl) => dispatch({ type: 'SET_PNL', pnl }))
        .catch(() => {});
    }, 15000);
    return () => clearInterval(interval);
  }, [dispatch]);

  // Poll trade history from securities service
  useEffect(() => {
    const fetchHistory = () => {
      apiRequest<{ data: Record<string, unknown>[] }>('/securities/trades')
        .then((res) => {
          const trades = (res.data || []).map((t) => {
            const instId = (t.instrument_id || t.instrumentId || '') as string;
            const inst = instruments.find((i) => i.id === instId);
            return {
              tradeId: (t.id || t.tradeId || '') as string,
              instrumentId: instId,
              instrumentSymbol: inst?.ticker || instId.slice(0, 8),
              side: ((t.side || '') as string).toLowerCase(),
              quantity: String(t.quantity || '0'),
              price: String(t.price || '0'),
              totalValue: calculateTradeValue(String(t.price || 0), String(t.quantity || 0)),
              timestamp: (t.created_at || t.timestamp || '') as string,
            };
          }) as TradeRecord[];
          setTradeHistory(trades);
        })
        .catch(() => {});
    };
    fetchHistory();
    const interval = setInterval(fetchHistory, 10000);
    return () => clearInterval(interval);
  }, [instruments]);

  // Poll orders from securities service
  useEffect(() => {
    const fetchOrders = () => {
      apiRequest<{ data: SecuritiesOrder[] }>('/securities/orders')
        .then((res) => setOrders(res.data || []))
        .catch(() => {});
    };
    fetchOrders();
    const interval = setInterval(fetchOrders, 5000);
    return () => clearInterval(interval);
  }, []);

  // Close position: submit opposing market order via securities endpoint
  const handleClosePosition = useCallback(async (posInstrumentId: string, side: 'long' | 'short', quantity: string) => {
    const opposingSide = side === 'long' ? 'sell' : 'buy';
    try {
      await apiRequest('/securities/orders', {
        method: 'POST',
        body: JSON.stringify({
          instrument_id: posInstrumentId,
          side: opposingSide,
          order_type: 'market',
          quantity: Math.abs(Number(quantity)).toString(),
        }),
      });
    } catch {
      // Error will show via next position poll
    }
  }, []);

  return (
    <div className={styles.layout}>
      <header className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.logo}>GarudaX</span>
          <InstrumentSelector
            selected={state.selectedInstrument}
            onSelect={selectInstrument}
          />
        </div>
        <div className={styles.headerRight}>
          <ConnectionStatus status={state.wsStatus} />
          <button className={styles.logoutBtn} onClick={logout}>
            Logout
          </button>
        </div>
      </header>

      <main className={styles.main}>
        <div className={styles.chartArea}>
          <ErrorBoundary>
            <Chart instrumentId={instrumentId} />
          </ErrorBoundary>
        </div>

        <div className={styles.tradingArea}>
          <div className={styles.bookPanel}>
            <ErrorBoundary>
              <OrderBook
                bids={state.orderBook.bids}
                asks={state.orderBook.asks}
                onPriceClick={setPrefillPrice}
              />
            </ErrorBoundary>
          </div>

          <div className={styles.entryPanel}>
            <ErrorBoundary>
              <OrderEntry instrumentId={instrumentId} prefillPrice={prefillPrice} />
              <InstrumentInfo instrumentId={instrumentId} />
            </ErrorBoundary>
          </div>

          <div className={styles.tradesPanel}>
            <ErrorBoundary>
              <TradeHistory trades={tradeHistory} instruments={instruments} />
            </ErrorBoundary>
          </div>
        </div>

        <div className={styles.bottomArea}>
          <div className={styles.bottomTabArea}>
            <div className={styles.tabBar}>
              <button
                className={`${styles.tab} ${activeTab === 'orders' ? styles.tabActive : ''}`}
                onClick={() => setActiveTab('orders')}
              >
                Orders ({orders.length})
              </button>
              <button
                className={`${styles.tab} ${activeTab === 'positions' ? styles.tabActive : ''}`}
                onClick={() => setActiveTab('positions')}
              >
                Positions ({state.positions.length})
              </button>
              <button
                className={`${styles.tab} ${activeTab === 'history' ? styles.tabActive : ''}`}
                onClick={() => setActiveTab('history')}
              >
                Trade History ({tradeHistory.length})
              </button>
            </div>
            <div className={styles.tabContent}>
              {activeTab === 'orders' && (
                <div className={styles.ordersTable}>
                  <div className={styles.ordersHeader}>
                    <span>Time</span>
                    <span>Instrument</span>
                    <span>Side</span>
                    <span>Type</span>
                    <span>Qty</span>
                    <span>Price</span>
                    <span>Filled</span>
                    <span>Status</span>
                  </div>
                  {orders.map((order) => (
                    <div key={order.id} className={styles.ordersRow}>
                      <span>{new Date(order.created_at).toLocaleTimeString()}</span>
                      <span>{instruments.find((i) => i.id === order.instrument_id)?.ticker || order.instrument_id?.slice(0, 8)}</span>
                      <span className={order.side?.toLowerCase() === 'buy' ? styles.buySide : styles.sellSide}>
                        {order.side || '-'}
                      </span>
                      <span>{order.order_type}</span>
                      <span>{order.quantity}</span>
                      <span>{order.price}</span>
                      <span>{order.filled_quantity}</span>
                      <span>{order.status}</span>
                    </div>
                  ))}
                  {orders.length === 0 && (
                    <div className={styles.empty}>No orders</div>
                  )}
                </div>
              )}
              {activeTab === 'positions' && (
                <ErrorBoundary>
                  <Positions
                    positions={state.positions}
                    tickers={tickers}
                    onClosePosition={handleClosePosition}
                  />
                </ErrorBoundary>
              )}
              {activeTab === 'history' && (
                <ErrorBoundary>
                  <TradeHistory trades={tradeHistory} instruments={instruments} />
                </ErrorBoundary>
              )}
            </div>
          </div>
          <div className={styles.marginPanel}>
            <ErrorBoundary>
              <MarginStatusPanel margin={state.margin} />
            </ErrorBoundary>
          </div>
        </div>
      </main>
    </div>
  );
};
