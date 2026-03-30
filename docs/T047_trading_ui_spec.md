# Trading Web UI Architecture Specification

**Document ID:** T047-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 4)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Technology Stack](#2-technology-stack)
3. [Application Architecture](#3-application-architecture)
4. [Page Routes](#4-page-routes)
5. [Component Tree](#5-component-tree)
6. [State Management](#6-state-management)
7. [Authentication Flow](#7-authentication-flow)
8. [REST API Integration](#8-rest-api-integration)
9. [WebSocket Integration](#9-websocket-integration)
10. [Instrument Selector](#10-instrument-selector)
11. [Order Book Display](#11-order-book-display)
12. [Order Entry Form](#12-order-entry-form)
13. [Trade History Feed](#13-trade-history-feed)
14. [Candlestick Chart](#14-candlestick-chart)
15. [Position & P&L Display](#15-position--pl-display)
16. [Account Balance & Margin Status](#16-account-balance--margin-status)
17. [Responsive Layout](#17-responsive-layout)
18. [Error Handling & Loading States](#18-error-handling--loading-states)
19. [Accessibility](#19-accessibility)
20. [Performance Requirements](#20-performance-requirements)

---

## 1. Overview

The GarudaX Trading Web UI is a single-page application (SPA) that provides commodity traders with a real-time trading interface for the AI Powered Commodity Exchange. It connects to the GarudaX API Gateway (`https://api.garudax.mn/api/v1/`) for REST operations and WebSocket streams.

### Design Principles

- **Real-time first**: Order book and trade data stream via WebSocket; REST is used for state queries and mutations (order submission, position lookup).
- **Minimal dependencies**: React 18, TypeScript, Vite. No Redux, no CSS-in-JS runtime. Lightweight charting library for candlestick charts.
- **Security-conscious**: JWT tokens stored in memory only (never localStorage/sessionStorage). Silent refresh via httpOnly cookie. PKCE for login.
- **Responsive**: Fully functional on desktop (1024px+) and tablet (768px+); mobile (< 768px) shows simplified single-panel layout.

### Scope

This spec covers:
- SPA architecture, routing, component tree, state management
- JWT auth flow (login, silent refresh, logout)
- Real-time order book and trade feed via WebSocket
- Order entry for limit, market, IOC, and FOK order types
- Candlestick chart with OHLCV data
- Position summary, P&L display, margin status
- Instrument selector for 14 commodities
- Responsive breakpoints and layout
- Error handling, loading states, accessibility

This spec does NOT cover:
- Admin panel (instrument halt/resume, circuit breakers, compliance)
- Account registration / KYC onboarding flow
- Mobile native apps
- Deployment / CI/CD pipeline for the frontend

---

## 2. Technology Stack

| Layer | Technology | Rationale |
|---|---|---|
| Framework | React 18 | Concurrent rendering, Suspense for loading states |
| Language | TypeScript 5.x (strict mode) | Type safety across components, API types, WS messages |
| Build | Vite 5.x | Fast HMR, ESBuild bundling, native ESM dev server |
| Routing | React Router v6 | Standard SPA routing, lazy route loading |
| State | React Context + `useReducer` | Sufficient for this app's complexity; no Redux overhead |
| Styling | CSS Modules + CSS custom properties | Scoped styles, theme variables, no runtime cost |
| Charts | lightweight-charts (TradingView) | Purpose-built for financial candlestick charts, <50KB gzip |
| HTTP | `fetch` API (native) | No axios dependency; wrapper with auth interceptor |
| WebSocket | Native `WebSocket` API | Custom reconnection manager with exponential backoff |
| Testing | Vitest + React Testing Library | Vite-native test runner, DOM testing |
| Linting | ESLint + Prettier | Standard TypeScript/React config |

### Project Structure

```
src/trading-ui/
  index.html
  vite.config.ts
  tsconfig.json
  package.json
  src/
    main.tsx                          # App entry point
    App.tsx                           # Root component, providers, router
    routes.tsx                        # Route definitions (lazy-loaded)
    vite-env.d.ts
    api/
      client.ts                       # Authenticated fetch wrapper
      endpoints.ts                    # REST endpoint definitions
      types.ts                        # API request/response TypeScript types
    auth/
      AuthContext.tsx                  # Auth state provider
      authReducer.ts                  # Auth state reducer
      tokenManager.ts                 # In-memory token storage + silent refresh
      LoginPage.tsx                   # Login form
    ws/
      WebSocketManager.ts             # Reconnection manager w/ exponential backoff
      useWebSocket.ts                 # React hook for WS subscriptions
      messageTypes.ts                 # WS message TypeScript types
    trading/
      TradingContext.tsx              # Trading state provider (instrument, book, trades)
      tradingReducer.ts              # Trading state reducer
      TradingPage.tsx                # Main trading layout
      InstrumentSelector.tsx         # Commodity instrument picker
      OrderBook.tsx                  # Real-time L2 order book
      OrderEntryForm.tsx             # Order submission form
      TradeHistory.tsx               # Real-time trade feed
      CandlestickChart.tsx           # OHLCV chart (lightweight-charts)
      PositionSummary.tsx            # Positions table
      PnlDisplay.tsx                 # P&L summary
      MarginStatus.tsx               # Account balance + margin info
    components/
      Layout.tsx                     # App shell (header, sidebar, content)
      Header.tsx                     # Top bar: instrument selector, account menu
      ErrorBoundary.tsx              # React error boundary
      LoadingSpinner.tsx             # Shared loading indicator
      Toast.tsx                      # Notification toasts
    hooks/
      useInterval.ts                 # Polling hook for REST data
      useMediaQuery.ts               # Responsive breakpoint hook
    styles/
      variables.css                  # CSS custom properties (colors, spacing)
      global.css                     # Reset + base styles
```

---

## 3. Application Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                          App.tsx                                  │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐  │
│  │ AuthProvider │→ │TradingProvider│→ │   React Router (v6)     │  │
│  │ (Context)    │  │ (Context)     │  │                         │  │
│  └─────────────┘  └──────────────┘  │  /login  → LoginPage    │  │
│                                      │  /trade  → TradingPage  │  │
│                                      │  /        → Redirect    │  │
│                                      └─────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
         │                    │
         │ fetch + JWT        │ WebSocket
         ▼                    ▼
┌─────────────────┐  ┌──────────────────┐
│  api/client.ts  │  │ WebSocketManager │
│  (REST calls)   │  │ (book, trades,   │
│                 │  │  executions)     │
└────────┬────────┘  └────────┬─────────┘
         │                    │
         ▼                    ▼
   GarudaX API Gateway (https://api.garudax.mn/api/v1/)
```

### Data Flow

1. **Auth**: User logs in → gateway returns JWT access token + httpOnly refresh cookie → token stored in `tokenManager` (closure variable, never in storage).
2. **Instrument selection**: User picks commodity → `TradingContext` dispatches `SET_INSTRUMENT` → WebSocket subscriptions update to new instrument.
3. **Order book**: `WebSocketManager` connects to `/api/v1/ws/book?instrument_id={id}` → book updates dispatched to `TradingContext` → `OrderBook` component re-renders.
4. **Order entry**: User fills form → `api/client.ts` POSTs to `/api/v1/orders` → response updates order state.
5. **Trade feed**: WebSocket `/api/v1/ws/trades?instrument_id={id}` → new trades prepended to trade history.
6. **Positions/P&L/Margin**: Polled via REST every 5s (not streamed) — these change less frequently.

---

## 4. Page Routes

| Route | Component | Auth Required | Description |
|---|---|---|---|
| `/` | `Redirect` | No | Redirects to `/trade` if authenticated, `/login` otherwise |
| `/login` | `LoginPage` | No | Login form; redirects to `/trade` on success |
| `/trade` | `TradingPage` | Yes | Main trading interface (default route) |
| `/trade/:instrumentId` | `TradingPage` | Yes | Trading with pre-selected instrument |

Route guards use `AuthContext` — unauthenticated requests to protected routes redirect to `/login` with `returnUrl` query param.

---

## 5. Component Tree

```
App
├── AuthProvider
│   └── TradingProvider
│       └── Router
│           ├── /login → LoginPage
│           │   ├── LoginForm
│           │   │   ├── EmailInput
│           │   │   ├── PasswordInput
│           │   │   └── SubmitButton
│           │   └── ErrorMessage
│           │
│           └── /trade → Layout
│               ├── Header
│               │   ├── InstrumentSelector
│               │   ├── ConnectionStatus       (WS connection indicator)
│               │   └── AccountMenu            (user name, logout)
│               │
│               └── TradingPage
│                   ├── CandlestickChart       (top, full width)
│                   ├── OrderBook              (left panel)
│                   │   ├── BookHeader         (spread, mid-price)
│                   │   ├── AskLevels          (sell side, reversed)
│                   │   └── BidLevels          (buy side)
│                   ├── OrderEntryForm         (center panel)
│                   │   ├── OrderTypeSelector  (limit/market/IOC/FOK tabs)
│                   │   ├── SideToggle         (buy/sell)
│                   │   ├── PriceInput         (disabled for market)
│                   │   ├── QuantityInput
│                   │   ├── TotalDisplay       (computed: price × qty)
│                   │   └── SubmitButton
│                   ├── TradeHistory           (right panel)
│                   │   └── TradeRow[]         (price, qty, time, side)
│                   ├── PositionSummary        (bottom left)
│                   │   └── PositionRow[]      (instrument, qty, avg price, unrealized P&L)
│                   ├── PnlDisplay             (bottom center)
│                   │   ├── RealizedPnl
│                   │   └── UnrealizedPnl
│                   └── MarginStatus           (bottom right)
│                       ├── AccountBalance
│                       ├── UsedMargin
│                       ├── AvailableMargin
│                       └── MarginUtilization  (progress bar)
```

---

## 6. State Management

### AuthContext

```typescript
interface AuthState {
  status: 'idle' | 'loading' | 'authenticated' | 'unauthenticated';
  user: User | null;
  error: string | null;
}

interface User {
  id: string;
  email: string;
  displayName: string;
  roles: string[];
  participantId: string;
}

type AuthAction =
  | { type: 'LOGIN_START' }
  | { type: 'LOGIN_SUCCESS'; user: User }
  | { type: 'LOGIN_FAILURE'; error: string }
  | { type: 'LOGOUT' }
  | { type: 'TOKEN_REFRESHED' }
  | { type: 'SESSION_EXPIRED' };
```

### TradingContext

```typescript
interface TradingState {
  selectedInstrument: Instrument | null;
  orderBook: OrderBookState;
  recentTrades: Trade[];
  positions: Position[];
  pnl: PnlSummary | null;
  margin: MarginStatus | null;
  wsStatus: 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
}

interface OrderBookState {
  bids: PriceLevel[];    // sorted descending by price
  asks: PriceLevel[];    // sorted ascending by price
  sequence: number;
  lastUpdated: string;
}

interface PriceLevel {
  price: string;         // decimal string (matches protobuf convention)
  quantity: string;      // decimal string
  orderCount: number;
}

interface Trade {
  tradeId: string;
  price: string;
  quantity: string;
  side: 'buy' | 'sell';  // aggressor side
  timestamp: string;     // ISO 8601
  sequence: number;
}

interface Instrument {
  instrumentId: string;  // UUID
  symbol: string;        // e.g. "WHEAT-2026-07-UB"
  commodityName: string; // e.g. "Wheat"
  deliveryMonth: string; // e.g. "2026-07"
  deliveryLocation: string;
  tickSize: string;      // e.g. "0.0001"
  lotSize: string;       // e.g. "1"
  status: 'active' | 'halted' | 'expired';
}

interface Position {
  instrumentId: string;
  instrumentSymbol: string;
  netQuantity: string;
  avgEntryPrice: string;
  unrealizedPnl: string;
  realizedPnl: string;
  side: 'long' | 'short' | 'flat';
}

interface PnlSummary {
  totalRealizedPnl: string;
  totalUnrealizedPnl: string;
  totalPnl: string;
  currency: string;
}

interface MarginStatus {
  accountBalance: string;
  usedMargin: string;
  availableMargin: string;
  marginUtilization: number;   // 0.0 - 1.0
  marginCalls: MarginCall[];
}

interface MarginCall {
  callId: string;
  amount: string;
  deadline: string;
  status: 'pending' | 'met' | 'breached';
}

type TradingAction =
  | { type: 'SET_INSTRUMENT'; instrument: Instrument }
  | { type: 'BOOK_SNAPSHOT'; book: OrderBookState }
  | { type: 'BOOK_UPDATE'; update: BookDelta }
  | { type: 'NEW_TRADE'; trade: Trade }
  | { type: 'SET_POSITIONS'; positions: Position[] }
  | { type: 'SET_PNL'; pnl: PnlSummary }
  | { type: 'SET_MARGIN'; margin: MarginStatus }
  | { type: 'WS_STATUS_CHANGE'; status: TradingState['wsStatus'] };
```

### Context Composition

```tsx
// App.tsx
function App() {
  return (
    <AuthProvider>
      <TradingProvider>
        <RouterProvider router={router} />
      </TradingProvider>
    </AuthProvider>
  );
}
```

`TradingProvider` internally creates the `WebSocketManager` and subscribes to the selected instrument's book/trade streams. When the instrument changes, it tears down old subscriptions and establishes new ones.

---

## 7. Authentication Flow

### Login Sequence

```
┌──────────┐     ┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│  Browser  │     │  Trading UI │     │  API Gateway  │     │ Auth Service  │
└─────┬─────┘     └──────┬──────┘     └──────┬───────┘     └──────┬───────┘
      │                  │                    │                    │
      │  Enter email +   │                    │                    │
      │  password        │                    │                    │
      │─────────────────>│                    │                    │
      │                  │                    │                    │
      │                  │  POST /api/v1/auth/login               │
      │                  │  { email, password }                   │
      │                  │───────────────────>│                    │
      │                  │                    │  gRPC Login()      │
      │                  │                    │───────────────────>│
      │                  │                    │                    │
      │                  │                    │  { access_token,   │
      │                  │                    │    expires_in }    │
      │                  │                    │<───────────────────│
      │                  │                    │                    │
      │                  │  200 OK            │                    │
      │                  │  Body: { access_token, expires_in }    │
      │                  │  Set-Cookie: refresh_token=...; HttpOnly; Secure; SameSite=Strict
      │                  │<───────────────────│                    │
      │                  │                    │                    │
      │                  │  Store access_token│                    │
      │                  │  in memory (closure│                    │
      │                  │  variable)         │                    │
      │                  │                    │                    │
      │  Redirect to     │                    │                    │
      │  /trade          │                    │                    │
      │<─────────────────│                    │                    │
```

### Token Storage

```typescript
// tokenManager.ts — closure-based in-memory token storage
function createTokenManager() {
  let accessToken: string | null = null;
  let expiresAt: number = 0;
  let refreshTimer: ReturnType<typeof setTimeout> | null = null;

  return {
    setToken(token: string, expiresInSeconds: number) {
      accessToken = token;
      expiresAt = Date.now() + expiresInSeconds * 1000;
      // Schedule silent refresh at 80% of token lifetime
      scheduleRefresh(expiresInSeconds * 0.8);
    },

    getToken(): string | null {
      if (!accessToken || Date.now() >= expiresAt) return null;
      return accessToken;
    },

    clear() {
      accessToken = null;
      expiresAt = 0;
      if (refreshTimer) clearTimeout(refreshTimer);
    },
  };
}
```

### Silent Refresh

- The refresh token is stored in an httpOnly, Secure, SameSite=Strict cookie (set by the gateway).
- At 80% of the access token's lifetime, `tokenManager` sends `POST /api/v1/auth/refresh` (cookie sent automatically).
- If refresh succeeds: new access token stored in memory, new refresh scheduled.
- If refresh fails (401): dispatch `SESSION_EXPIRED`, redirect to `/login`.
- On page load (F5/navigation): attempt silent refresh immediately — if the httpOnly cookie is valid, the user is restored without re-entering credentials.

### Logout

```
POST /api/v1/auth/logout
Cookie: refresh_token=...
```

- Server invalidates the refresh token.
- Client clears in-memory access token via `tokenManager.clear()`.
- Dispatch `LOGOUT` action, redirect to `/login`.

---

## 8. REST API Integration

### Authenticated Fetch Wrapper

```typescript
// api/client.ts
async function apiRequest<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const token = tokenManager.getToken();
  if (!token) throw new AuthError('No valid token');

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    credentials: 'include',  // send httpOnly cookie
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
      ...options.headers,
    },
  });

  if (response.status === 401) {
    // Attempt refresh, retry once
    const refreshed = await attemptRefresh();
    if (refreshed) return apiRequest<T>(path, options);
    throw new AuthError('Session expired');
  }

  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new ApiError(response.status, body.error || 'Unknown error', body.code);
  }

  return response.json();
}
```

### Endpoint Mapping

All endpoints are routed through the GarudaX API Gateway. The gateway translates REST → gRPC to the appropriate backend service.

| UI Feature | Method | Endpoint | Backend Service | Poll Interval |
|---|---|---|---|---|
| Submit order | POST | `/api/v1/orders` | matching-engine | — |
| Cancel order | DELETE | `/api/v1/orders/{order_id}` | matching-engine | — |
| Modify order | PATCH | `/api/v1/orders/{order_id}` | matching-engine | — |
| List open orders | GET | `/api/v1/orders?status=open` | matching-engine | 10s |
| Get order book (REST fallback) | GET | `/api/v1/instruments/{id}/book` | matching-engine | — |
| Get positions | GET | `/api/v1/clearing/positions` | clearing-engine | 5s |
| Get position (single) | GET | `/api/v1/clearing/positions/{instrument_id}` | clearing-engine | — |
| Get P&L | GET | `/api/v1/settlement/cycles` | settlement-engine | 5s |
| Get margin status | GET | `/api/v1/margin` | margin-engine | 5s |
| Get margin calls | GET | `/api/v1/margin/calls` | margin-engine | 10s |
| Get candles | GET | `/api/v1/market-data/candles?instrument_id={id}&interval={i}&from={ts}&to={ts}` | market-data-service | — |
| Get ticker | GET | `/api/v1/market-data/ticker?instrument_id={id}` | market-data-service | 5s |
| Login | POST | `/api/v1/auth/login` | auth-service | — |
| Refresh token | POST | `/api/v1/auth/refresh` | auth-service | — |
| Logout | POST | `/api/v1/auth/logout` | auth-service | — |
| Get profile | GET | `/api/v1/auth/me` | auth-service | — |

**Note on market-data endpoints**: The gateway spec (T033) defines routes for the market-data-service. The exact REST paths may be refined when the market-data gateway routes are implemented. The candle endpoint follows the pattern from the market-data spec (T035): `GetCandles(instrument_id, interval, time_range)`.

### Order Submission Request

```typescript
interface SubmitOrderRequest {
  instrument_id: string;        // UUID
  side: 'buy' | 'sell';
  order_type: 'limit' | 'market' | 'ioc' | 'fok';
  quantity: string;             // decimal string
  price?: string;               // required for limit; omitted for market
  time_in_force?: 'gtc' | 'ioc' | 'fok' | 'day';
  client_order_id?: string;     // optional idempotency key
}

interface SubmitOrderResponse {
  order_id: string;
  client_order_id: string;
  status: 'pending' | 'accepted' | 'rejected';
  instrument_id: string;
  side: 'buy' | 'sell';
  order_type: string;
  quantity: string;
  price: string;
  created_at: string;
  rejection_reason?: string;
}
```

---

## 9. WebSocket Integration

### Connection Manager

```typescript
// ws/WebSocketManager.ts
class WebSocketManager {
  private connections: Map<string, WebSocket> = new Map();
  private reconnectAttempts: Map<string, number> = new Map();

  private static readonly MAX_RECONNECT_ATTEMPTS = 10;
  private static readonly BASE_DELAY_MS = 1000;
  private static readonly MAX_DELAY_MS = 30000;

  connect(channel: string, url: string, onMessage: (msg: WsMessage) => void): void;
  disconnect(channel: string): void;
  disconnectAll(): void;

  private getReconnectDelay(attempt: number): number {
    // Exponential backoff with jitter
    const delay = Math.min(
      WebSocketManager.BASE_DELAY_MS * Math.pow(2, attempt),
      WebSocketManager.MAX_DELAY_MS
    );
    // Add ±20% jitter to prevent thundering herd
    const jitter = delay * 0.2 * (Math.random() * 2 - 1);
    return Math.round(delay + jitter);
  }
}
```

### Reconnection Strategy

1. On `WebSocket.onclose` or `WebSocket.onerror`: start reconnection.
2. Attempt 1: wait 1s. Attempt 2: wait 2s. Attempt 3: wait 4s. ... up to 30s max.
3. Add ±20% jitter to each delay.
4. After 10 failed attempts: stop reconnecting, dispatch `WS_STATUS_CHANGE('disconnected')`, show user notification.
5. User can manually trigger reconnection via UI button.
6. On successful reconnect: reset attempt counter, request full book snapshot (sequence gap may have occurred).

### WebSocket Channels

| Channel | URL | Auth | Direction | Purpose |
|---|---|---|---|---|
| Order Book | `/api/v1/ws/book?instrument_id={id}` | JWT query param | Server → Client | Real-time L2 book updates |
| Trade Feed | `/api/v1/ws/trades?instrument_id={id}` | JWT query param | Server → Client | Real-time trade tape |
| Executions | `/api/v1/ws/executions` | JWT query param | Server → Client | User's own execution reports |

**Auth for WebSocket**: Since browsers cannot set custom headers on WebSocket upgrade requests, the JWT is passed as a query parameter: `ws://host/api/v1/ws/book?instrument_id={id}&token={jwt}`. The gateway validates the token during the upgrade handshake.

### WebSocket Message Formats

All messages use the envelope defined in the gateway's `websocket.Message`:

```typescript
interface WsMessage {
  type: string;
  instrument_id?: string;
  sequence?: number;
  timestamp: string;          // ISO 8601
  data?: unknown;
}
```

#### Book Snapshot (`type: "book_snapshot"`)

Sent on initial connection and after reconnection.

```json
{
  "type": "book_snapshot",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
  "sequence": 10542,
  "timestamp": "2026-03-28T10:00:00.123Z",
  "data": {
    "bids": [
      { "price": "325.5000", "quantity": "150.0000", "order_count": 3 },
      { "price": "325.2500", "quantity": "200.0000", "order_count": 5 }
    ],
    "asks": [
      { "price": "325.7500", "quantity": "100.0000", "order_count": 2 },
      { "price": "326.0000", "quantity": "250.0000", "order_count": 4 }
    ]
  }
}
```

#### Book Delta (`type: "book_update"`)

Incremental update to a single price level.

```json
{
  "type": "book_update",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
  "sequence": 10543,
  "timestamp": "2026-03-28T10:00:00.456Z",
  "data": {
    "side": "bid",
    "price": "325.5000",
    "quantity": "200.0000",
    "order_count": 4
  }
}
```

If `quantity` is `"0.0000"`, the price level should be removed from the book.

#### Trade (`type: "trade"`)

```json
{
  "type": "trade",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
  "sequence": 8890,
  "timestamp": "2026-03-28T10:00:01.789Z",
  "data": {
    "trade_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "price": "325.5000",
    "quantity": "50.0000",
    "aggressor_side": "buy"
  }
}
```

#### Execution Report (`type: "execution_report"`)

Private to the authenticated user.

```json
{
  "type": "execution_report",
  "timestamp": "2026-03-28T10:00:01.800Z",
  "data": {
    "order_id": "f1e2d3c4-b5a6-7890-fedc-ba0987654321",
    "client_order_id": "my-order-001",
    "exec_type": "fill",
    "status": "filled",
    "side": "buy",
    "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
    "filled_quantity": "50.0000",
    "filled_price": "325.5000",
    "remaining_quantity": "0.0000",
    "cumulative_quantity": "50.0000"
  }
}
```

#### Heartbeat (`type: "heartbeat"`)

Sent every 30 seconds by the gateway.

```json
{
  "type": "heartbeat",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
  "timestamp": "2026-03-28T10:00:30.000Z"
}
```

Clients should track the last heartbeat time. If no heartbeat is received within 60 seconds, consider the connection stale and trigger reconnection.

### Sequence Gap Detection

Each book channel maintains a monotonically increasing `sequence` number. The client tracks the last received sequence. If a message arrives with `sequence > lastSequence + 1`, a gap has occurred (likely during a brief disconnection). The client should:

1. Continue processing the message (best-effort).
2. Request a full book snapshot via REST fallback: `GET /api/v1/instruments/{id}/book`.
3. Replace the local book state with the snapshot.

---

## 10. Instrument Selector

### Commodities List

The 14 tradeable commodities from the seed data (T004):

| Symbol | Name | Category |
|---|---|---|
| WHEAT | Wheat | Grains |
| CORN | Corn | Grains |
| SOYB | Soybeans | Oilseeds |
| RICE | Rice | Grains |
| COFF | Coffee | Softs |
| COCO | Cocoa | Softs |
| SUGR | Sugar | Softs |
| COTN | Cotton | Softs |
| PALM | Palm Oil | Oilseeds |
| RUBR | Rubber | Industrials |
| SMEA | Soybean Meal | Oilseeds |
| SOIL | Soybean Oil | Oilseeds |
| RAPE | Rapeseed | Oilseeds |
| BARL | Barley | Grains |

### Instrument Selection Behavior

Each commodity may have multiple active instruments (different delivery months/locations). The UI flow:

1. **Header dropdown** shows commodity names grouped by category (Grains, Oilseeds, Softs, Industrials).
2. On commodity selection, fetch active instruments: `GET /api/v1/instruments/{commodity_id}/contracts?status=active` (or filter client-side from a full instruments list).
3. Show sub-selector with available delivery months (e.g., "Jul 2026", "Sep 2026").
4. On instrument selection, dispatch `SET_INSTRUMENT` → WebSocket subscriptions switch → chart/book/trades reload.

### Instrument List Caching

- Fetch full instrument list on app load (`GET /api/v1/instruments?status=active`).
- Cache for 5 minutes (instruments don't change frequently).
- Group by commodity for the dropdown display.
- URL updates to `/trade/{instrumentId}` for deep-linking.

---

## 11. Order Book Display

### Layout

```
┌──────────────────────────┐
│  Order Book   WHEAT-JUL  │
│  Spread: 0.2500          │
├──────────────────────────┤
│  Price    Qty    Orders  │
│  326.00   250     4  ███ │  ← asks (red, ascending from mid)
│  325.75   100     2  ██  │
│──── mid: 325.6250 ──────│
│  325.50   200     4  ███ │  ← bids (green, descending from mid)
│  325.25   200     5  ████│
└──────────────────────────┘
```

### Features

- **Depth visualization**: Horizontal bars show cumulative depth at each level (proportional to max visible quantity).
- **Color coding**: Bids in green (#22c55e), asks in red (#ef4444).
- **Spread display**: Calculated as `bestAsk - bestBid`. Displayed in both absolute and basis points.
- **Mid-price**: `(bestBid + bestAsk) / 2`, shown at the center divider.
- **Click-to-fill**: Clicking a price level populates the order entry price field.
- **Configurable depth**: Dropdown to show 5, 10, 15, or 20 price levels per side. Default: 10.
- **Flash on update**: Price levels flash briefly (200ms) on update — green flash for quantity increase, red for decrease.

### Update Performance

- Book updates arrive at up to 100 messages/second during active trading.
- Use `requestAnimationFrame` batching: accumulate updates within a frame, apply all at once.
- The `OrderBook` component should use `React.memo` and only re-render when the visible price levels change.
- Virtualize rows if depth > 20 levels (unlikely to be displayed, but defensive).

---

## 12. Order Entry Form

### Order Types

| Type | Price Required | Behavior |
|---|---|---|
| **Limit** | Yes | Rests on the book at specified price until filled or cancelled |
| **Market** | No | Executes immediately at best available price; may partially fill |
| **IOC** (Immediate-or-Cancel) | Yes | Fills what's available at limit price, cancels remainder |
| **FOK** (Fill-or-Kill) | Yes | Fills entire quantity at limit price or cancels entire order |

### Form Fields

```
┌──────────────────────────────┐
│  [Limit] [Market] [IOC] [FOK]│  ← order type tabs
├──────────────────────────────┤
│  ┌─────────┐ ┌──────────┐   │
│  │   BUY   │ │   SELL   │   │  ← side toggle (green/red)
│  └─────────┘ └──────────┘   │
│                              │
│  Price  [________] USD/MT    │  ← disabled for market orders
│  Qty    [________] MT        │
│                              │
│  Total: 16,275.00 USD        │  ← computed: price × qty
│                              │
│  ┌──────────────────────┐    │
│  │    PLGarudaX BUY ORDER    │   │  ← colored by side
│  └──────────────────────┘    │
│                              │
│  Fee estimate: ~2.50 USD     │  ← optional, if fee schedule known
└──────────────────────────────┘
```

### Validation Rules

| Field | Validation |
|---|---|
| Price | Required for limit/IOC/FOK. Must be positive. Must be a multiple of `instrument.tickSize`. |
| Quantity | Required. Must be positive. Must be a multiple of `instrument.lotSize`. Min: 1 lot. |
| Order type | Required. One of: limit, market, ioc, fok. |
| Side | Required. One of: buy, sell. |

### Submission Flow

1. Validate all fields client-side.
2. Disable submit button, show spinner.
3. `POST /api/v1/orders` with `SubmitOrderRequest`.
4. On success (status: `accepted`): show success toast, clear form (or keep qty for rapid re-entry).
5. On rejection (status: `rejected`): show error with `rejection_reason`.
6. On network error: show error toast, re-enable button.

### Keyboard Shortcuts

| Key | Action |
|---|---|
| `B` | Set side to Buy (when form not focused) |
| `S` | Set side to Sell (when form not focused) |
| `Enter` | Submit order (when form focused) |
| `Escape` | Clear form |

---

## 13. Trade History Feed

### Layout

```
┌──────────────────────────┐
│  Recent Trades            │
├──────────────────────────┤
│  Price    Qty     Time   │
│  325.50   50    10:00:01 │  ← green (buyer aggressor)
│  325.75   25    10:00:00 │  ← red (seller aggressor)
│  325.50   100   09:59:58 │  ← green
│  ...                     │
└──────────────────────────┘
```

### Features

- Shows most recent 100 trades (configurable).
- New trades animate in from the top.
- Color by aggressor side: green for buy aggressor, red for sell aggressor.
- Timestamp shows `HH:mm:ss` format (local time).
- On initial load, fetch recent trades via REST: `GET /api/v1/instruments/{id}/trades/latest`.
- Subsequent updates via WebSocket trade messages.

---

## 14. Candlestick Chart

### Data Source

- Historical candles: `GET /api/v1/market-data/candles?instrument_id={id}&interval=1m&from={ts}&to={ts}`
- Real-time updates: use trade feed WebSocket to update the current (open) candle in-memory.

### Intervals

| Interval | Label | Default range loaded |
|---|---|---|
| 1m | 1 minute | Last 4 hours |
| 5m | 5 minutes | Last 24 hours |
| 15m | 15 minutes | Last 3 days |
| 1h | 1 hour | Last 7 days |
| 4h | 4 hours | Last 30 days |
| 1d | 1 day | Last 180 days |

### Features

- **Library**: TradingView `lightweight-charts` — renders on `<canvas>`, optimized for financial data.
- **OHLCV data**: Open, High, Low, Close, Volume per candle.
- **Volume overlay**: Volume bars at the bottom of the chart (semi-transparent).
- **Crosshair**: Shows price/time on hover.
- **Price line**: Current price shown as a horizontal dashed line.
- **Interval selector**: Tabs above the chart for 1m, 5m, 15m, 1h, 4h, 1d.
- **Auto-scroll**: Chart follows new candles as they form.
- **Lazy loading**: When user scrolls left (historical), fetch older candles on demand.

### Candle Response Format

```typescript
interface CandleData {
  timestamp: string;     // ISO 8601 start of interval
  open: string;          // decimal string
  high: string;
  low: string;
  close: string;
  volume: string;
}
```

---

## 15. Position & P&L Display

### Position Summary

Fetched via `GET /api/v1/clearing/positions` (polled every 5 seconds).

```
┌───────────────────────────────────────────────────┐
│  Positions                                         │
├───────────┬─────┬──────────┬──────────┬───────────┤
│ Instrument│ Side│ Quantity  │ Avg Price│ Unreal P&L│
├───────────┼─────┼──────────┼──────────┼───────────┤
│ WHEAT-JUL │ Long│   100 MT │  325.00  │  +50.00   │
│ CORN-SEP  │Short│   200 MT │  410.50  │  -25.00   │
└───────────┴─────┴──────────┴──────────┴───────────┘
```

### P&L Display

```
┌─────────────────────────────┐
│  P&L Summary                │
├─────────────────────────────┤
│  Realized:      +1,250.00   │  ← green
│  Unrealized:       +25.00   │  ← green
│  Total:         +1,275.00   │  ← green
└─────────────────────────────┘
```

- Green for positive, red for negative.
- Values formatted with thousand separators and 2 decimal places.
- Currency always USD (the exchange's settlement currency).

---

## 16. Account Balance & Margin Status

Fetched via `GET /api/v1/margin` (polled every 5 seconds).

```
┌──────────────────────────────────┐
│  Account & Margin                │
├──────────────────────────────────┤
│  Balance:       50,000.00 USD    │
│  Used Margin:   12,500.00 USD    │
│  Available:     37,500.00 USD    │
│                                  │
│  Utilization:   [████░░░░] 25%   │  ← progress bar
│                                  │
│  ⚠ Margin Call: 5,000.00 USD     │  ← shown only if active
│    Deadline: 2026-03-28 16:00    │
└──────────────────────────────────┘
```

### Margin Utilization Thresholds

| Utilization | Color | Behavior |
|---|---|---|
| 0% - 60% | Green (#22c55e) | Normal |
| 60% - 80% | Yellow (#eab308) | Warning |
| 80% - 100% | Red (#ef4444) | Critical, may trigger margin call |

---

## 17. Responsive Layout

### Breakpoints

| Breakpoint | Width | Layout |
|---|---|---|
| Mobile | < 768px | Single column, tabbed panels |
| Tablet | 768px - 1023px | Two-column, stacked bottom panels |
| Desktop | ≥ 1024px | Full three-column trading layout |

### Desktop Layout (≥ 1024px)

```
┌─────────────────────────────────────────────────────┐
│  Header: [Instrument Selector]  [Connection]  [User]│
├─────────────────────────────────────────────────────┤
│  Candlestick Chart (full width, ~250px height)       │
├──────────┬────────────────┬─────────────────────────┤
│ Order    │ Order Entry    │ Trade History            │
│ Book     │ Form           │                         │
│          │                │                         │
│          │                │                         │
├──────────┴───────┬────────┴─────────────────────────┤
│ Positions        │ P&L         │ Margin Status       │
└──────────────────┴─────────────┴────────────────────┘
```

### Tablet Layout (768px - 1023px)

```
┌───────────────────────────────────┐
│  Header                           │
├───────────────────────────────────┤
│  Candlestick Chart                │
├────────────────┬──────────────────┤
│ Order Book     │ Order Entry Form │
├────────────────┴──────────────────┤
│  Trade History (horizontal scroll)│
├───────────────────────────────────┤
│  Positions │ P&L │ Margin         │
└───────────────────────────────────┘
```

### Mobile Layout (< 768px)

```
┌────────────────────┐
│  Header (compact)  │
├────────────────────┤
│  [Chart][Book][Order][Trades]  │  ← tab bar
├────────────────────┤
│                    │
│  (selected panel   │
│   fills viewport)  │
│                    │
├────────────────────┤
│  Position/Margin   │
│  (collapsible)     │
└────────────────────┘
```

On mobile, only one main panel is visible at a time. The tab bar lets the user switch between Chart, Order Book, Order Entry, and Trade History. Position/P&L/Margin are in a collapsible footer section.

---

## 18. Error Handling & Loading States

### Loading States

| Component | Loading Behavior |
|---|---|
| TradingPage (initial) | Full-page skeleton with placeholder boxes |
| Order Book | Shimmer animation on price level rows |
| Candlestick Chart | "Loading chart data..." with spinner |
| Trade History | Shimmer on trade rows |
| Positions | Shimmer on table rows |
| Order submission | Button shows spinner, disabled during request |

### Error Categories

| Category | Handling | User-Facing |
|---|---|---|
| **Network error** | Retry with backoff; show toast | "Connection error. Retrying..." |
| **401 Unauthorized** | Silent refresh; if fails, redirect to login | "Session expired. Please log in." |
| **403 Forbidden** | Show error, no retry | "You don't have permission for this action." |
| **422 Validation** | Show field-level errors | Inline errors on the form field |
| **429 Rate Limited** | Backoff, retry after `Retry-After` header | "Too many requests. Please wait." |
| **500 Server Error** | Show toast, log to console | "Something went wrong. Please try again." |
| **WebSocket disconnect** | Auto-reconnect with exponential backoff | Connection status indicator turns red |
| **React render error** | `ErrorBoundary` catches, shows fallback UI | "Something went wrong. Reload the page." |

### Error Boundary

Each major panel (OrderBook, Chart, OrderEntry, TradeHistory, Positions) is wrapped in its own `ErrorBoundary`. A crash in one panel doesn't take down the entire trading page.

---

## 19. Accessibility

### ARIA Labels

| Component | ARIA | Details |
|---|---|---|
| Instrument selector | `role="combobox"`, `aria-expanded`, `aria-activedescendant` | Dropdown with search |
| Order type tabs | `role="tablist"`, `role="tab"`, `aria-selected` | Tab selection |
| Buy/Sell toggle | `role="radiogroup"`, `role="radio"`, `aria-checked` | Side selection |
| Order book table | `role="grid"`, `aria-label="Order book"` | Read-only data grid |
| Trade history | `role="log"`, `aria-live="polite"` | Live-updating trade log |
| Margin progress bar | `role="progressbar"`, `aria-valuenow`, `aria-valuemin`, `aria-valuemax` | Utilization meter |
| Connection status | `aria-live="assertive"` | Connection loss announcement |
| Toast notifications | `role="alert"`, `aria-live="assertive"` | Error/success messages |

### Keyboard Navigation

- **Tab order**: Header → Chart interval selector → Order Book → Order Entry Form → Trade History → Positions → P&L → Margin.
- **Order form**: Standard form tab order. Enter submits. Escape clears.
- **Instrument selector**: Arrow keys navigate, Enter selects, Escape closes.
- **Order book**: Arrow keys navigate price levels, Enter fills price into order form.
- **Focus trap**: Modal dialogs (confirmation, error) trap focus until dismissed.

### Color Contrast

- All text meets WCAG 2.1 AA contrast ratio (≥ 4.5:1 for normal text, ≥ 3:1 for large text).
- Red/green indicators are supplemented with icons (↑/↓ arrows) for color-blind users.
- Chart candlesticks use both color and fill pattern (solid up, hollow down) for color-blind accessibility.

---

## 20. Performance Requirements

| Metric | Target | Measurement |
|---|---|---|
| First Contentful Paint | < 1.5s | Lighthouse |
| Time to Interactive | < 3.0s | Lighthouse |
| Order book render (100 updates/s) | < 16ms per frame | Performance API |
| Order submission round-trip | < 200ms (UI → gateway) | Network timing |
| Bundle size (gzipped) | < 250KB (main), < 100KB (chart lib) | Build output |
| WebSocket message processing | < 5ms per message | Performance API |
| Memory (steady state) | < 100MB | Chrome DevTools |

### Optimization Strategies

- **Code splitting**: Login page and trading page are separate chunks (React.lazy).
- **Tree shaking**: Vite eliminates unused code at build time.
- **Memoization**: `React.memo` on OrderBook rows, TradeHistory rows. `useMemo` for computed values (spread, mid-price, totals).
- **RAF batching**: WebSocket updates batched per animation frame.
- **Virtual scrolling**: Not needed for default depth (10 levels), but available if extended.
- **Decimal formatting**: Pre-compute formatted strings; avoid re-formatting on every render.
