# Exchange Engine Architecture Specification

**Document ID:** T007-SPEC-001
**Version:** 1.0
**Date:** 2026-03-27
**Status:** DRAFT
**Author:** Coder Agent (Phase 1)

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Context](#2-system-context)
3. [Order Book Architecture (CLOB)](#3-order-book-architecture-clob)
4. [Order Lifecycle](#4-order-lifecycle)
5. [Matching Algorithm](#5-matching-algorithm)
6. [Price Discovery Mechanism](#6-price-discovery-mechanism)
7. [Trade Ledger Design](#7-trade-ledger-design)
8. [Circuit Breakers & Risk Controls](#8-circuit-breakers--risk-controls)
9. [API Contracts](#9-api-contracts)
10. [Data Model Mapping](#10-data-model-mapping)
11. [Performance Requirements](#11-performance-requirements)
12. [Deployment Architecture](#12-deployment-architecture)
13. [Failure Modes & Recovery](#13-failure-modes--recovery)

---

## 1. Overview

The ACE Exchange Engine is the core trading system for the Agriculture Commodity Exchange of Mongolia. It implements a Central Limit Order Book (CLOB) with price-time priority matching for agricultural commodities including grains, oilseeds, livestock, and fiber products.

### Design Principles

- **Deterministic matching**: Given the same sequence of order events, the engine must produce identical trade output. This enables replay-based recovery and audit.
- **Append-only trade ledger**: Per T004, trades are immutable once written. No DELETE or UPDATE on `exchange.trades`.
- **Single-threaded matching per instrument**: Each order book is processed sequentially to guarantee determinism. Instruments are parallelized across books.
- **Event-sourced state**: The order book state is derived from the ordered sequence of order events. Snapshots accelerate recovery but are not authoritative.

### Scope

This spec covers:
- The in-memory CLOB data structure and matching logic
- Order types and their handling
- Price discovery (continuous trading + auction sessions)
- Trade ledger persistence
- gRPC and REST API contracts for order submission, book queries, and trade feeds

This spec does NOT cover:
- Clearing and settlement (T027)
- Market data distribution (separate service consumes trade events)
- User authentication (T005)
- Warehouse receipt linkage (covered by warehouse service)

---

## 2. System Context

```
                    +------------------+
                    |   API Gateway    |
                    +--------+---------+
                             |
                   gRPC / REST (order submit, cancel, query)
                             |
                    +--------v---------+
                    | Exchange Engine   |
                    | (matching-engine) |
                    +--+-----+------+--+
                       |     |      |
          +------------+     |      +-------------+
          |                  |                    |
  +-------v-------+  +------v--------+  +--------v--------+
  | PostgreSQL     |  | Kafka/NATS    |  | Redis            |
  | (exchange      |  | (trade events,|  | (session cache,  |
  |  schema)       |  |  order events)|  |  rate limits)    |
  +----------------+  +---------------+  +-----------------+
```

### Service Dependencies

| Dependency | Purpose | Protocol |
|---|---|---|
| PostgreSQL (`exchange` schema) | Order persistence, trade ledger | SQL via `ace_exchange_svc` role |
| Kafka/NATS | Trade event publishing, order event sourcing | Async messaging |
| Redis | Pre-trade risk cache, rate limiting, session state | Key-value |
| Clearing Service | Post-trade position updates, margin checks | gRPC (async) |
| Market Data Service | Consumes trade events for OHLCV aggregation | Kafka consumer |
| Compliance Service | Trade surveillance, audit trail | Kafka consumer |

---

## 3. Order Book Architecture (CLOB)

### 3.1 Data Structure

Each tradeable instrument (commodity + delivery month + delivery location) has one order book instance.

```
OrderBook {
    instrument_id:    UUID
    symbol:           string          // e.g., "WHT-HRW-2026M07-UB"
    bids:             PriceLevel[]    // sorted descending by price
    asks:             PriceLevel[]    // sorted ascending by price
    last_trade_price: Decimal
    sequence_number:  uint64          // monotonic, per-book
    state:            BookState       // PREOPEN | AUCTION | CONTINUOUS | HALTED | CLOSED
}

PriceLevel {
    price:       Decimal
    orders:      OrderQueue          // FIFO queue of orders at this price
    total_qty:   uint64              // sum of remaining quantities
    order_count: uint32
}

OrderQueue = DoublyLinkedList<Order>  // O(1) insert at tail, O(1) remove by pointer

Order {
    order_id:          UUID
    client_order_id:   string
    participant_id:    UUID
    account_id:        UUID
    side:              BUY | SELL
    order_type:        LIMIT | MARKET | STOP_LIMIT | STOP_MARKET | IOC | FOK | GTC | GTD
    price:             Decimal         // null for MARKET orders
    stop_price:        Decimal         // null unless STOP_*
    quantity:          uint64          // original quantity (lots)
    remaining_qty:     uint64          // unfilled quantity
    filled_qty:        uint64
    time_in_force:     GTC | GTD | IOC | FOK | DAY
    expire_at:         Timestamp       // null for GTC
    created_at:        Timestamp
    sequence_number:   uint64          // global sequence assigned on acceptance
    status:            NEW | PARTIALLY_FILLED | FILLED | CANCELLED | REJECTED | EXPIRED
}
```

### 3.2 Price Level Organization

- **Bid side**: Price levels sorted in descending order. Best bid = highest price.
- **Ask side**: Price levels sorted in ascending order. Best ask = lowest price.
- Within each price level, orders are queued in FIFO order (time priority).
- Empty price levels are removed from the tree to avoid memory leaks.

### 3.3 Instrument Identifier Convention

Format: `{COMMODITY}-{GRADE}-{DELIVERY_YYYYMMM}-{LOCATION_CODE}`

Examples:
- `WHT-HRW-2026M07-UB` — Hard Red Winter Wheat, July 2026 delivery, Ulaanbaatar
- `CRN-STD-2026M09-DK` — Corn, Standard, September 2026, Darkhan
- `CTL-LIV-2026M06-KH` — Live Cattle, June 2026, Khentii

Location codes are derived from `reference.delivery_locations` (see T004 seed data).

---

## 4. Order Lifecycle

### 4.1 State Machine

```
                    +----------+
         submit --> | PENDING  |
                    |VALIDATION|
                    +----+-----+
                         |
              +----------+----------+
              |                     |
        (valid)               (invalid)
              |                     |
        +-----v-----+        +-----v-----+
        |    NEW     |        |  REJECTED |
        +-----+------+        +-----------+
              |
     +--------+--------+
     |                  |
  (partial fill)    (full fill)
     |                  |
+----v----------+  +----v-----+
|PARTIALLY_FILLED|  |  FILLED  |
+----+-----------+  +----------+
     |
+----+--------+--------+
|             |         |
(more fills) (cancel)  (expire)
|             |         |
v        +----v----+ +--v------+
(FILLED) |CANCELLED| |EXPIRED  |
         +---------+ +---------+
```

### 4.2 Order Types

| Type | Behavior |
|---|---|
| **LIMIT** | Rests on the book at the specified price if not immediately matched. |
| **MARKET** | Matches against the best available prices until filled or book exhausted. No price specified. Rejected if no liquidity. |
| **STOP_LIMIT** | Converts to a LIMIT order when the last trade price crosses `stop_price`. |
| **STOP_MARKET** | Converts to a MARKET order when the last trade price crosses `stop_price`. |

### 4.3 Time-in-Force

| TIF | Behavior |
|---|---|
| **DAY** | Cancelled at end of trading session. Default. |
| **GTC** (Good-Till-Cancel) | Remains until explicitly cancelled or filled. Max 90 days. |
| **GTD** (Good-Till-Date) | Remains until `expire_at` timestamp, then auto-cancelled. |
| **IOC** (Immediate-or-Cancel) | Fill whatever is available immediately; cancel remainder. |
| **FOK** (Fill-or-Kill) | Fill the entire quantity immediately or reject entirely. |

### 4.4 Pre-Trade Validation

Before an order enters the book, the engine validates:

1. **Instrument exists and is active** — check `reference.commodities` and trading calendar
2. **Session state allows orders** — book must be in CONTINUOUS or AUCTION state
3. **Participant is authorized** — valid `participant_id` with active trading permissions
4. **Price within band** — limit price within daily price limit (reference price +/- band %)
5. **Quantity within limits** — min lot size, max order size, max position exposure
6. **Self-trade prevention** — reject if it would immediately match against same participant
7. **Margin sufficiency** — pre-trade margin check via clearing service (async with timeout)
8. **Rate limiting** — max orders/second per participant (configurable, default 50/s)

---

## 5. Matching Algorithm

### 5.1 Price-Time Priority

The ACE exchange uses strict **price-time priority** (also called FIFO):

1. **Price priority**: An order offering a better price is matched first. For buys, higher price has priority. For sells, lower price has priority.
2. **Time priority**: Among orders at the same price level, the order that arrived first (lowest sequence number) is matched first.

### 5.2 Matching Procedure (Continuous Trading)

```
function match(incoming_order):
    while incoming_order.remaining_qty > 0:
        opposite_book = (incoming is BUY) ? asks : bids

        if opposite_book is empty:
            break

        best_level = opposite_book.front()

        if incoming is LIMIT and not price_crosses(incoming.price, best_level.price, incoming.side):
            break  // no match at this price

        while incoming_order.remaining_qty > 0 and best_level has orders:
            resting_order = best_level.front()
            fill_qty = min(incoming_order.remaining_qty, resting_order.remaining_qty)
            fill_price = resting_order.price  // resting order's price (price improvement for incoming)

            execute_trade(incoming_order, resting_order, fill_qty, fill_price)

            if resting_order.remaining_qty == 0:
                best_level.dequeue(resting_order)

        if best_level is empty:
            opposite_book.remove_level(best_level.price)

    // After matching, if incoming still has remaining quantity:
    if incoming_order.remaining_qty > 0:
        if incoming_order.type is MARKET:
            cancel_remaining(incoming_order)  // MARKET orders don't rest
        elif incoming_order.tif is IOC:
            cancel_remaining(incoming_order)
        elif incoming_order.tif is FOK:
            // Should not reach here — FOK is checked atomically before matching
            unreachable()
        else:
            add_to_book(incoming_order)  // LIMIT order rests on the book

function price_crosses(incoming_price, resting_price, side):
    if side == BUY:  return incoming_price >= resting_price
    if side == SELL: return incoming_price <= resting_price
```

### 5.3 FOK Handling

FOK orders require special handling — they must be checked for full fillability BEFORE executing any trades:

```
function can_fill_fok(order):
    available = 0
    for each level in opposite_book:
        if order is LIMIT and not price_crosses(order.price, level.price, order.side):
            break
        available += level.total_qty
        if available >= order.quantity:
            return true
    return false
```

### 5.4 Self-Trade Prevention (STP)

When an incoming order would match against a resting order from the same participant:

| STP Mode | Behavior |
|---|---|
| **CANCEL_NEWEST** | Cancel the incoming (aggressor) order. Default. |
| **CANCEL_OLDEST** | Cancel the resting (passive) order. |
| **CANCEL_BOTH** | Cancel both orders. |

STP mode is configured per participant account.

---

## 6. Price Discovery Mechanism

### 6.1 Trading Sessions

Each trading day follows this session schedule (Mongolia Time, UTC+8):

| Session | Time | Book State | Description |
|---|---|---|---|
| Pre-Open | 08:30 - 09:00 | PREOPEN | Orders accepted but not matched |
| Opening Auction | 09:00 - 09:05 | AUCTION | Equilibrium price calculation |
| Continuous Trading AM | 09:05 - 11:30 | CONTINUOUS | Normal price-time matching |
| Midday Break | 11:30 - 13:00 | CLOSED | No orders accepted |
| Continuous Trading PM | 13:00 - 15:25 | CONTINUOUS | Normal price-time matching |
| Closing Auction | 15:25 - 15:30 | AUCTION | Settlement price calculation |
| Post-Close | 15:30 - 15:45 | CLOSED | Trade reporting only |

### 6.2 Opening Auction

The opening auction determines a single opening price that maximizes executed volume.

**Algorithm (Equilibrium Price Calculation):**

```
function calculate_auction_price(book):
    // Collect all price points from both sides
    price_points = sorted(unique(all bid prices + all ask prices))

    best_price = null
    max_volume = 0
    min_imbalance = infinity

    for price in price_points:
        buy_volume = sum of qty for all bids where bid.price >= price
        sell_volume = sum of qty for all asks where ask.price <= price
        matched_volume = min(buy_volume, sell_volume)
        imbalance = abs(buy_volume - sell_volume)

        if matched_volume > max_volume:
            max_volume = matched_volume
            min_imbalance = imbalance
            best_price = price
        elif matched_volume == max_volume and imbalance < min_imbalance:
            min_imbalance = imbalance
            best_price = price
        elif matched_volume == max_volume and imbalance == min_imbalance:
            // Tie-break: price closest to previous close
            if abs(price - prev_close) < abs(best_price - prev_close):
                best_price = price

    return best_price, max_volume
```

**Auction execution:**
- All matched orders execute at the single auction price.
- Pro-rata allocation when supply/demand exceeds at the equilibrium price.
- Unmatched orders carry over into continuous trading.

### 6.3 Closing Auction

Same algorithm as opening auction. The resulting price becomes the **official settlement price** for the instrument for that day, used for:
- Mark-to-market margin calculations
- Daily P&L reporting
- Continuous aggregate seeding in `market_data` schema

### 6.4 Reference Price

The reference price is used for circuit breaker calculations and price band validation:

- At session start: previous day's settlement price
- After a circuit breaker halt: last auction price
- For newly listed instruments: IPO reference price set by exchange admin

---

## 7. Trade Ledger Design

### 7.1 Trade Record

Every match produces a trade record persisted to `exchange.trades` (append-only per T004).

```
Trade {
    trade_id:           UUID (v7, time-ordered)
    instrument_id:      UUID
    buy_order_id:       UUID
    sell_order_id:      UUID
    buyer_participant:  UUID
    seller_participant: UUID
    price:              Decimal(18,4)
    quantity:           uint64          // in lots
    trade_value:        Decimal(18,4)   // price * quantity * lot_size
    aggressor_side:     BUY | SELL      // who initiated the match
    trade_type:         CONTINUOUS | AUCTION | BLOCK
    sequence_number:    uint64          // monotonic per instrument
    executed_at:        Timestamp       // nanosecond precision
    clearing_status:    PENDING | CLEARED | FAILED
}
```

### 7.2 Persistence Strategy

1. **Synchronous write to WAL**: Trade is written to a local write-ahead log before acknowledgment.
2. **Batch flush to PostgreSQL**: Trades are batched (configurable: 100 trades or 10ms, whichever first) and written to `exchange.trades` in a single transaction.
3. **Kafka publish**: Trade event published to `exchange.trades.{instrument_id}` topic after DB commit.
4. **Sequence guarantees**: The `sequence_number` is assigned by the matching engine (not the DB) to ensure deterministic replay.

### 7.3 Trade Corrections

Since `exchange.trades` is append-only (DELETE/UPDATE blocked), corrections are handled via:

- **Bust trade**: Insert a new trade record with `trade_type = BUST` and negative quantity, referencing the original `trade_id`.
- **Price adjustment**: Insert a correction record with `trade_type = CORRECTION`, referencing original `trade_id`, with the adjusted price.

Both operations require exchange admin authorization and generate compliance audit events.

### 7.4 Execution Reports

For every state change on an order, an execution report is generated:

```
ExecutionReport {
    exec_id:            UUID
    order_id:           UUID
    client_order_id:    string
    exec_type:          NEW | PARTIAL_FILL | FILL | CANCELLED | REJECTED | EXPIRED | TRADE_BUST
    order_status:       (current order status)
    last_qty:           uint64          // quantity of this fill (0 if not a fill)
    last_price:         Decimal         // price of this fill
    cumulative_qty:     uint64          // total filled so far
    leaves_qty:         uint64          // remaining quantity
    trade_id:           UUID            // if this is a fill
    timestamp:          Timestamp
}
```

---

## 8. Circuit Breakers & Risk Controls

### 8.1 Price Limits (per T004)

| Trigger | Action | Duration |
|---|---|---|
| Last trade >= reference +/- 5% | Trading halt | 15 minutes |
| Last trade >= reference +/- 10% | Trading halt | 60 minutes |
| Last trade >= reference +/- 15% | Trading halt | Remainder of session |

During a halt:
- Book state transitions to HALTED
- No new orders accepted
- Existing orders remain on the book
- After halt expires, a volatility auction (same as opening auction) determines the new reference price
- Continuous trading resumes after the volatility auction

### 8.2 Order-Level Controls

| Control | Default | Configurable |
|---|---|---|
| Max order quantity | 1,000 lots | Per instrument |
| Max order value | MNT 500,000,000 | Per instrument |
| Price band (limit orders) | Reference +/- 20% | Per instrument |
| Max orders per second | 50/s | Per participant |
| Max open orders | 500 | Per participant per instrument |
| Fat finger protection | Last trade +/- 3% for MARKET orders | Per instrument |

### 8.3 Market-Wide Controls

- **Kill switch**: Exchange admin can halt all instruments immediately
- **Participant disable**: Suspend a participant's ability to submit orders
- **Mass cancel**: Cancel all open orders for a participant or instrument

---

## 9. API Contracts

### 9.1 gRPC Service Definitions

See `src/matching-engine/proto/exchange.proto` for the full protobuf definitions.

#### OrderService

| RPC | Request | Response | Description |
|---|---|---|---|
| `SubmitOrder` | `SubmitOrderRequest` | `ExecutionReport` | Submit a new order |
| `CancelOrder` | `CancelOrderRequest` | `ExecutionReport` | Cancel an open order |
| `CancelAllOrders` | `CancelAllRequest` | `CancelAllResponse` | Cancel all orders for account |
| `ModifyOrder` | `ModifyOrderRequest` | `ExecutionReport` | Modify price/qty of open order |
| `GetOrder` | `GetOrderRequest` | `OrderDetail` | Query single order |
| `GetOpenOrders` | `GetOpenOrdersRequest` | `OrderList` | List open orders for account |

#### MarketDataService

| RPC | Request | Response | Description |
|---|---|---|---|
| `GetOrderBook` | `BookRequest` | `BookSnapshot` | L2 order book snapshot (aggregated) |
| `GetOrderBookL3` | `BookRequest` | `BookSnapshotL3` | L3 order book (individual orders) |
| `StreamTrades` | `TradeStreamRequest` | stream `Trade` | Real-time trade stream |
| `StreamOrderBook` | `BookStreamRequest` | stream `BookUpdate` | Incremental book updates |
| `GetLastTrade` | `LastTradeRequest` | `Trade` | Most recent trade for instrument |

#### AdminService

| RPC | Request | Response | Description |
|---|---|---|---|
| `HaltInstrument` | `HaltRequest` | `HaltResponse` | Manual trading halt |
| `ResumeInstrument` | `ResumeRequest` | `ResumeResponse` | Resume after halt |
| `BustTrade` | `BustTradeRequest` | `Trade` | Bust an erroneous trade |
| `SetCircuitBreaker` | `CBConfig` | `CBConfig` | Update circuit breaker params |
| `DisableParticipant` | `DisableRequest` | `DisableResponse` | Suspend participant trading |

### 9.2 REST API (Gateway Pass-through)

The gateway exposes REST endpoints that translate to gRPC calls:

```
POST   /api/v1/orders                    -> SubmitOrder
DELETE /api/v1/orders/{order_id}          -> CancelOrder
DELETE /api/v1/orders?account_id={id}     -> CancelAllOrders
PATCH  /api/v1/orders/{order_id}          -> ModifyOrder
GET    /api/v1/orders/{order_id}          -> GetOrder
GET    /api/v1/orders?account_id={id}     -> GetOpenOrders
GET    /api/v1/instruments/{id}/book      -> GetOrderBook
GET    /api/v1/instruments/{id}/book?level=3  -> GetOrderBookL3
GET    /api/v1/instruments/{id}/trades    -> GetLastTrade
WS     /api/v1/ws/trades/{instrument_id}  -> StreamTrades
WS     /api/v1/ws/book/{instrument_id}    -> StreamOrderBook
```

### 9.3 Message Schemas

See `src/matching-engine/proto/` for canonical protobuf definitions. Key messages:

#### SubmitOrderRequest
```protobuf
message SubmitOrderRequest {
  string client_order_id = 1;
  string instrument_id = 2;
  Side side = 3;
  OrderType order_type = 4;
  TimeInForce time_in_force = 5;
  string price = 6;           // decimal as string, empty for MARKET
  string stop_price = 7;      // decimal as string, empty unless STOP_*
  uint64 quantity = 8;         // in lots
  google.protobuf.Timestamp expire_at = 9;
  STPMode stp_mode = 10;
}
```

#### ExecutionReport
```protobuf
message ExecutionReport {
  string exec_id = 1;
  string order_id = 2;
  string client_order_id = 3;
  ExecType exec_type = 4;
  OrderStatus order_status = 5;
  Side side = 6;
  string price = 7;
  uint64 quantity = 8;
  uint64 last_qty = 9;
  string last_price = 10;
  uint64 cumulative_qty = 11;
  uint64 leaves_qty = 12;
  string trade_id = 13;
  google.protobuf.Timestamp transact_time = 14;
  string reject_reason = 15;
}
```

---

## 10. Data Model Mapping

### Mapping to `exchange` Schema (T004)

| Spec Concept | DB Table | Notes |
|---|---|---|
| Order | `exchange.orders` | `remaining_quantity` is a generated column |
| Trade | `exchange.trades` | Append-only (DELETE/UPDATE blocked) |
| Order Book | In-memory only | Rebuilt from `exchange.orders` on startup |
| Execution Report | `exchange.execution_reports` | New table (migration needed) |
| Instrument | `reference.commodities` + `reference.delivery_locations` | Composite key |
| Price Level | In-memory only | Derived from order state |

### New Tables Required

The following tables are not yet in the T004 migrations and need to be added:

```sql
-- Execution reports (append-only)
CREATE TABLE exchange.execution_reports (
    exec_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL REFERENCES exchange.orders(order_id),
    exec_type           VARCHAR(20) NOT NULL,
    order_status        VARCHAR(20) NOT NULL,
    last_qty            BIGINT DEFAULT 0,
    last_price          NUMERIC(18,4),
    cumulative_qty      BIGINT NOT NULL DEFAULT 0,
    leaves_qty          BIGINT NOT NULL DEFAULT 0,
    trade_id            UUID REFERENCES exchange.trades(trade_id),
    transact_time       TIMESTAMPTZ NOT NULL DEFAULT now(),
    reject_reason       TEXT
);

-- Instrument definitions (tradeable contracts)
CREATE TABLE exchange.instruments (
    instrument_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol              VARCHAR(30) NOT NULL UNIQUE,
    commodity_id        UUID NOT NULL REFERENCES reference.commodities(commodity_id),
    delivery_location_id UUID NOT NULL REFERENCES reference.delivery_locations(location_id),
    delivery_month      DATE NOT NULL,
    lot_size            NUMERIC(18,4) NOT NULL,
    tick_size           NUMERIC(18,4) NOT NULL,
    price_band_pct      NUMERIC(5,2) NOT NULL DEFAULT 20.00,
    max_order_qty       BIGINT NOT NULL DEFAULT 1000,
    status              VARCHAR(10) NOT NULL DEFAULT 'ACTIVE',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Circuit breaker events log
CREATE TABLE exchange.circuit_breaker_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrument_id       UUID NOT NULL REFERENCES exchange.instruments(instrument_id),
    trigger_price       NUMERIC(18,4) NOT NULL,
    reference_price     NUMERIC(18,4) NOT NULL,
    deviation_pct       NUMERIC(5,2) NOT NULL,
    halt_level          SMALLINT NOT NULL,  -- 1=5%, 2=10%, 3=15%
    halted_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    resumed_at          TIMESTAMPTZ,
    auction_price       NUMERIC(18,4)
);
```

---

## 11. Performance Requirements

| Metric | Target | Rationale |
|---|---|---|
| Order-to-ack latency (p50) | < 1 ms | Competitive with regional exchanges |
| Order-to-ack latency (p99) | < 5 ms | Acceptable for agriculture commodities |
| Matching throughput | 10,000 orders/sec | Peak capacity; normal load ~500/sec |
| Trade publish latency | < 10 ms | Time from match to Kafka publish |
| Book recovery time | < 30 sec | Cold start from DB replay |
| Book snapshot interval | Every 60 sec | For accelerated recovery |

### Scaling Strategy

- One matching engine instance per instrument partition (no horizontal scaling per book)
- Instruments partitioned across engine instances by hash
- Each instance runs on a dedicated CPU core (pinned) on `exchange-core` node group (T001)
- State is recovered from the event log; no shared state between instances

---

## 12. Deployment Architecture

```
exchange-core node group (c5.2xlarge, CPU-optimized)
  +------------------------------------------+
  | matching-engine pod (1 per partition)     |
  |   - main: matching engine process        |
  |   - sidecar: gRPC-to-REST proxy (envoy)  |
  |   - sidecar: metrics exporter            |
  +------------------------------------------+
  | Resources:                               |
  |   CPU: 2 cores (dedicated, no overcommit)|
  |   Memory: 4 Gi                           |
  |   Network: host networking (low latency) |
  +------------------------------------------+
```

- **Pod anti-affinity**: No two matching engine pods on the same node
- **PDB**: minAvailable = 1 per partition
- **Service mesh**: Istio mTLS for all gRPC calls (per T001)
- **Database connection**: Direct to TimescaleDB, not through pgbouncer (for LISTEN/NOTIFY)

---

## 13. Failure Modes & Recovery

### 13.1 Engine Crash

1. New pod starts on the same partition.
2. Load latest book snapshot from Redis/disk.
3. Replay all order events from Kafka after snapshot sequence number.
4. Book state is fully reconstructed.
5. Resume accepting orders.

**Recovery time target**: < 30 seconds.

### 13.2 Database Unavailable

- Engine continues matching in-memory and writing to local WAL.
- Trades queue in memory (bounded buffer: 10,000 trades).
- When DB recovers, flush queued trades in order.
- If buffer fills: halt the affected instruments, trigger alert.

### 13.3 Kafka Unavailable

- Trades are persisted to DB first (synchronous).
- Kafka publish retries with exponential backoff.
- Consumers see delayed but complete event stream.
- No data loss; eventual consistency for downstream systems.

### 13.4 Split Brain Prevention

- Only one engine instance per instrument partition can be active (leader election via Redis/etcd).
- Fencing token (sequence number) prevents stale writes.
- If leadership is lost mid-batch, the new leader replays from the last committed sequence.

---

## Appendix A: Glossary

| Term | Definition |
|---|---|
| **CLOB** | Central Limit Order Book — an order-driven market where buy and sell orders are matched by price-time priority |
| **Lot** | Minimum tradeable unit for an instrument (e.g., 1 lot = 10 metric tons of wheat) |
| **Tick** | Minimum price increment (e.g., MNT 100 per ton) |
| **Aggressor** | The incoming order that initiates a match against a resting order |
| **Resting order** | An order sitting on the book waiting to be matched |
| **STP** | Self-Trade Prevention — mechanism to prevent a participant from trading with themselves |
| **Reference price** | Baseline price for circuit breaker calculations |

## Appendix B: Related Documents

- T001: Cloud Architecture Design (ADR-001)
- T004: Core Database Schema (migrations V1-V5)
- T027: Clearing & Settlement (future)
- FIX 4.4 / FIX 5.0 SP2: Industry standard for order/execution report fields
