import React, { useState, useEffect, useCallback } from 'react';
import { useTrading } from '../contexts/MarketContext';
import { useWebSocket } from '../hooks/useWebSocket';
import { useAuth } from '../contexts/AuthContext';
import { apiRequest } from '../services/api';
import { parseTradeMessage } from '../services/ws';
import { OrderBook } from '../components/OrderBook';
import { OrderEntry } from '../components/OrderEntry';
import { TradeHistory } from '../components/TradeHistory';
import { Chart } from '../components/Chart';
import { Positions } from '../components/Positions';
import { MarginStatusPanel } from '../components/MarginStatus';
import { InstrumentSelector } from '../components/InstrumentSelector';
import { ConnectionStatus } from '../components/ConnectionStatus';
import { ErrorBoundary } from '../components/ErrorBoundary';
import type { WSMessage } from '../services/ws';
import type { Position, PnlSummary, MarginStatus as MarginStatusType } from '../types/trade';
import styles from './Trading.module.css';

export const Trading: React.FC = () => {
  const { state, dispatch, selectInstrument } = useTrading();
  const { logout } = useAuth();
  const [prefillPrice, setPrefillPrice] = useState<string | undefined>();

  const instrumentId = state.selectedInstrument?.instrumentId ?? null;
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

  useWebSocket({
    url: instrumentId ? `${wsBase}/api/v1/ws/book?instrument_id=${instrumentId}` : null,
    onMessage: handleBookMessage,
    onStatusChange: handleWsStatus,
    enabled: !!instrumentId,
  });

  useWebSocket({
    url: instrumentId ? `${wsBase}/api/v1/ws/trades?instrument_id=${instrumentId}` : null,
    onMessage: handleTradeMessage,
    enabled: !!instrumentId,
  });

  // Poll positions, P&L, margin
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
        // Silently fail polling
      }
    };

    poll();
    const interval = setInterval(poll, 5000);
    return () => clearInterval(interval);
  }, [dispatch]);

  useEffect(() => {
    apiRequest<PnlSummary>('/settlement/cycles')
      .then((pnl) => dispatch({ type: 'SET_PNL', pnl }))
      .catch(() => {});

    const interval = setInterval(() => {
      apiRequest<PnlSummary>('/settlement/cycles')
        .then((pnl) => dispatch({ type: 'SET_PNL', pnl }))
        .catch(() => {});
    }, 5000);
    return () => clearInterval(interval);
  }, [dispatch]);

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
            </ErrorBoundary>
          </div>

          <div className={styles.tradesPanel}>
            <ErrorBoundary>
              <TradeHistory trades={state.recentTrades} />
            </ErrorBoundary>
          </div>
        </div>

        <div className={styles.bottomArea}>
          <div className={styles.positionsPanel}>
            <ErrorBoundary>
              <Positions positions={state.positions} />
            </ErrorBoundary>
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
