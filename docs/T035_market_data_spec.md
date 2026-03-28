# Market Data Service Architecture Specification

**Document ID:** T035-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 3)

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Context](#2-system-context)
3. [Data Model](#3-data-model)
4. [OHLCV Candle Aggregation](#4-ohlcv-candle-aggregation)
5. [gRPC API Design](#5-grpc-api-design)
6. [WebSocket Streaming](#6-websocket-streaming)
7. [SQL Migrations](#7-sql-migrations)
8. [Data Retention Policy](#8-data-retention-policy)
9. [Performance Requirements](#9-performance-requirements)
10. [Deployment Architecture](#10-deployment-architecture)
11. [Failure Modes & Recovery](#11-failure-modes--recovery)
12. [Gateway Integration](#12-gateway-integration)

---

## 1. Overview

The Market Data Service is a dedicated microservice that consumes raw trade events from the matching engine and produces aggregated market data: OHLCV candles, ticker summaries, and trade tape feeds. It provides both historical query APIs and real-time streaming via gRPC server-side streaming.

### Design Principles

- **Read-optimized**: Backed by TimescaleDB hypertables and continuous aggregates for sub-millisecond historical queries.
- **Derived data only**: The service owns no primary business state. All data is derived from the matching engine's trade stream. The service can be rebuilt from scratch by replaying trades.
- **Zero-dependency Go module**: Following the established pattern (matching-engine, clearing-engine, etc.), the service is a standalone Go module with no shared library dependencies.
- **Separation from matching engine**: The matching engine's `MarketDataService` gRPC service (T007) provides raw order book snapshots and trade streams. This service (T035) consumes those streams and produces aggregated views (candles, tickers). The gateway routes accordingly.

### Scope

This spec covers:
- OHLCV candle aggregation from trade streams (1m, 5m, 15m, 1h, 4h, 1d intervals)
- TimescaleDB schema: trades hypertable + continuous aggregates
- gRPC API: `GetCandles`, `StreamCandles`, `GetTicker`, `GetTrades`, `StreamTrades`
- Real-time streaming via gRPC server-side streaming (gateway translates to WebSocket)
- Ticker/summary endpoint (last price, 24h change, 24h volume, best bid/ask)
- Trade tape (recent trades list with pagination)
- Data retention policies (tick 90d, 1m candles 1y, daily candles indefinite)
- SQL migrations for `market_data` schema

This spec does NOT cover:
- Order book snapshots or L2/L3 depth (served by matching-engine `MarketDataService`)
- Order management or execution (matching-engine `OrderService`)
- Historical tick replay for backtesting (future consideration)

---

## 2. System Context

```
                     +-------------------+
                     |  Matching Engine   |
                     |  :50051            |
                     +--------+----------+
                              |
                     gRPC StreamTrades /
                     Kafka: exchange.trades.*
                              |
                     +--------v----------+
                     | Market Data Svc   |
                     | gRPC :50057       |
                     | Health :8087      |
                     +--------+----------+
                              |
              +---------------+----------------+
              |               |                |
         GetCandles    StreamCandles     GetTicker
         GetTrades     StreamTrades
              |               |                |
              +-------+-------+--------+-------+
                      |                |
                +-----v-----+   +-----v------+
                | API Gateway|  | Internal    |
                | :8080      |  | Consumers   |
                +------------+  +-------------+
```

### Data Sources

| Source | Transport | Data |
|---|---|---|
| Matching Engine trades | gRPC `StreamTrades` (primary) | Real-time trade events |
| Matching Engine trades | Kafka `exchange.trades.{instrument_id}` (fallback) | Trade events for replay/recovery |
| Matching Engine order book | gRPC `GetOrderBook` | Best bid/ask for ticker enrichment |

### Ports

| Port | Protocol | Purpose |
|---|---|---|
| 50057 | gRPC | Market data API |
| 8087 | HTTP | Health checks (`/healthz`, `/readyz`) |

---

## 3. Data Model

### 3.1 Trade (Source Event)

Each trade from the matching engine is stored in a TimescaleDB hypertable for efficient time-range queries:

| Field | Type | Description |
|---|---|---|
| `trade_id` | UUID | Unique trade identifier (from matching engine) |
| `instrument_id` | UUID | Instrument identifier |
| `price` | NUMERIC(18,4) | Trade price |
| `quantity` | BIGINT | Trade quantity in lots |
| `trade_value` | NUMERIC(18,4) | price * quantity * lot_size |
| `aggressor_side` | VARCHAR(4) | BUY or SELL |
| `trade_type` | VARCHAR(12) | CONTINUOUS, AUCTION, BLOCK, BUST, CORRECTION |
| `sequence_number` | BIGINT | Monotonic sequence from matching engine |
| `executed_at` | TIMESTAMPTZ | Trade execution timestamp |
| `ingested_at` | TIMESTAMPTZ | When this service received the trade |

### 3.2 OHLCV Candle

Candles are materialized via TimescaleDB continuous aggregates over the trades hypertable:

| Field | Type | Description |
|---|---|---|
| `instrument_id` | UUID | Instrument identifier |
| `bucket` | TIMESTAMPTZ | Candle open time (time_bucket boundary) |
| `interval` | VARCHAR(3) | 1m, 5m, 15m, 1h, 4h, 1d |
| `open` | NUMERIC(18,4) | First trade price in bucket |
| `high` | NUMERIC(18,4) | Maximum trade price in bucket |
| `low` | NUMERIC(18,4) | Minimum trade price in bucket |
| `close` | NUMERIC(18,4) | Last trade price in bucket |
| `volume` | BIGINT | Total quantity traded |
| `trade_count` | INTEGER | Number of trades in bucket |
| `vwap` | NUMERIC(18,4) | Volume-weighted average price |
| `turnover` | NUMERIC(18,4) | Total trade value |

### 3.3 Ticker Summary

Computed in real-time (not persisted), assembled from latest candle + order book:

| Field | Type | Description |
|---|---|---|
| `instrument_id` | UUID | Instrument identifier |
| `symbol` | VARCHAR(30) | Human-readable symbol |
| `last_price` | NUMERIC(18,4) | Last trade price |
| `price_change_24h` | NUMERIC(18,4) | Absolute price change over 24h |
| `price_change_pct_24h` | NUMERIC(8,4) | Percentage price change over 24h |
| `high_24h` | NUMERIC(18,4) | 24h high |
| `low_24h` | NUMERIC(18,4) | 24h low |
| `volume_24h` | BIGINT | 24h volume in lots |
| `turnover_24h` | NUMERIC(18,4) | 24h turnover in quote currency |
| `best_bid` | NUMERIC(18,4) | Best bid price (from order book) |
| `best_ask` | NUMERIC(18,4) | Best ask price (from order book) |
| `open_interest` | BIGINT | Open interest (from clearing engine, future) |
| `last_trade_at` | TIMESTAMPTZ | Timestamp of last trade |

---

## 4. OHLCV Candle Aggregation

### 4.1 Supported Intervals

| Interval | Code | TimescaleDB time_bucket | Retention |
|---|---|---|---|
| 1 minute | `1m` | `time_bucket('1 minute', executed_at)` | 1 year |
| 5 minutes | `5m` | `time_bucket('5 minutes', executed_at)` | 1 year |
| 15 minutes | `15m` | `time_bucket('15 minutes', executed_at)` | 1 year |
| 1 hour | `1h` | `time_bucket('1 hour', executed_at)` | Indefinite |
| 4 hours | `4h` | `time_bucket('4 hours', executed_at)` | Indefinite |
| 1 day | `1d` | `time_bucket('1 day', executed_at)` | Indefinite |

### 4.2 Aggregation Strategy

**TimescaleDB Continuous Aggregates** are used for all intervals. This provides:
- Automatic incremental materialization as new trades arrive
- Efficient historical queries (reads from materialized view, not raw trades)
- Built-in refresh policies for near-real-time updates

**1-minute candle** is the base continuous aggregate over the `trades` hypertable. Higher intervals (5m, 15m, 1h, 4h, 1d) are hierarchical continuous aggregates over the 1m aggregate (supported in TimescaleDB 2.9+).

```
trades (hypertable)
  └── ohlcv_1m  (continuous aggregate, refresh every 10s, lag 30s)
        ├── ohlcv_5m  (hierarchical, refresh every 30s, lag 1m)
        ├── ohlcv_15m (hierarchical, refresh every 1m, lag 2m)
        ├── ohlcv_1h  (hierarchical, refresh every 5m, lag 5m)
        ├── ohlcv_4h  (hierarchical, refresh every 15m, lag 15m)
        └── ohlcv_1d  (hierarchical, refresh every 30m, lag 30m)
```

### 4.3 OHLCV Computation

For each bucket window:
- **Open** = `first(price, executed_at)` — price of the first trade by time
- **High** = `max(price)`
- **Low** = `min(price)`
- **Close** = `last(price, executed_at)` — price of the last trade by time
- **Volume** = `sum(quantity)`
- **VWAP** = `sum(trade_value) / sum(quantity * lot_size)` — volume-weighted average price
- **Turnover** = `sum(trade_value)`
- **Trade Count** = `count(*)`

**Empty buckets**: If no trades occur in a time bucket, no candle row is emitted. The API returns no candle for that period. Clients should interpolate using the previous candle's close price if needed.

### 4.4 Real-Time Candle Updates

For the current (incomplete) candle, the service maintains an in-memory accumulator per instrument per interval. Each new trade updates the accumulator:

```
on_trade(trade):
  for each interval in [1m, 5m, 15m, 1h, 4h, 1d]:
    bucket = floor(trade.executed_at, interval)
    candle = accumulators[instrument_id][interval]
    if candle.bucket != bucket:
      emit_completed_candle(candle)
      candle = new_candle(bucket, trade)
    else:
      candle.high = max(candle.high, trade.price)
      candle.low  = min(candle.low, trade.price)
      candle.close = trade.price
      candle.volume += trade.quantity
      candle.trade_count += 1
      candle.turnover += trade.trade_value
    broadcast_candle_update(candle)
```

StreamCandles subscribers receive both completed and in-progress candle updates, distinguished by an `is_closed` flag.

---

## 5. gRPC API Design

### 5.1 Service Definition

```protobuf
package ace.marketdata.v1;

service MarketDataAggregateService {
  // Historical OHLCV candles for an instrument
  rpc GetCandles(GetCandlesRequest) returns (GetCandlesResponse);

  // Real-time candle updates (server-side streaming)
  rpc StreamCandles(StreamCandlesRequest) returns (stream Candle);

  // Ticker summary (last price, 24h stats, bid/ask)
  rpc GetTicker(GetTickerRequest) returns (Ticker);

  // List of tickers for all or specified instruments
  rpc GetTickers(GetTickersRequest) returns (GetTickersResponse);

  // Recent trades (paginated)
  rpc GetTrades(GetTradesRequest) returns (GetTradesResponse);

  // Real-time trade stream (server-side streaming)
  rpc StreamTrades(StreamTradesRequest) returns (stream TradeEvent);
}
```

### 5.2 RPC Details

#### GetCandles

Retrieve historical OHLCV candles for an instrument within a time range.

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_id` | string (UUID) | Yes | Instrument to query |
| `interval` | CandleInterval enum | Yes | Candle interval (1m, 5m, 15m, 1h, 4h, 1d) |
| `start_time` | Timestamp | Yes | Start of time range (inclusive) |
| `end_time` | Timestamp | No | End of time range (exclusive); defaults to now |
| `limit` | uint32 | No | Max candles to return; default 500, max 5000 |

**Response**: `GetCandlesResponse` with repeated `Candle` messages, sorted by bucket time ascending.

**Pagination**: If more candles exist beyond the limit, `next_cursor` is set. Client sends `cursor` in next request.

#### StreamCandles

Subscribe to real-time candle updates for an instrument.

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_id` | string (UUID) | Yes | Instrument to stream |
| `interval` | CandleInterval enum | Yes | Candle interval |

**Behavior**:
- Immediately sends the current in-progress candle for the requested interval
- Sends updated candle on every trade (with `is_closed = false`)
- Sends final candle when the bucket closes (with `is_closed = true`)
- Heartbeat every 15 seconds if no trades occur

#### GetTicker

Get the current ticker summary for a single instrument.

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_id` | string (UUID) | Yes | Instrument to query |

**Response**: `Ticker` message with last price, 24h stats, best bid/ask.

#### GetTickers

Get ticker summaries for multiple instruments.

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_ids` | repeated string | No | Specific instruments; empty = all active |

**Response**: `GetTickersResponse` with repeated `Ticker` messages.

#### GetTrades

Get recent trades for an instrument (trade tape).

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_id` | string (UUID) | Yes | Instrument to query |
| `limit` | uint32 | No | Max trades; default 100, max 1000 |
| `since_sequence` | uint64 | No | Return trades after this sequence number |
| `start_time` | Timestamp | No | Filter by time range start |
| `end_time` | Timestamp | No | Filter by time range end |

**Response**: `GetTradesResponse` with repeated `TradeEvent` messages, sorted by sequence descending (newest first).

#### StreamTrades

Subscribe to real-time trade events for an instrument.

| Field | Type | Required | Description |
|---|---|---|---|
| `instrument_id` | string (UUID) | Yes | Instrument to stream |
| `since_sequence` | uint64 | No | Replay from this sequence (0 = live only) |

**Behavior**:
- If `since_sequence > 0`, replays historical trades from that sequence then switches to live
- Emits each trade as it occurs
- Heartbeat every 15 seconds if no trades occur

### 5.3 Message Definitions

```protobuf
enum CandleInterval {
  CANDLE_INTERVAL_UNSPECIFIED = 0;
  CANDLE_INTERVAL_1M = 1;
  CANDLE_INTERVAL_5M = 2;
  CANDLE_INTERVAL_15M = 3;
  CANDLE_INTERVAL_1H = 4;
  CANDLE_INTERVAL_4H = 5;
  CANDLE_INTERVAL_1D = 6;
}

message Candle {
  string instrument_id = 1;
  CandleInterval interval = 2;
  google.protobuf.Timestamp bucket = 3;       // candle open time
  string open = 4;                             // decimal string
  string high = 5;
  string low = 6;
  string close = 7;
  uint64 volume = 8;                           // total lots traded
  int32 trade_count = 9;
  string vwap = 10;                            // decimal string
  string turnover = 11;                        // decimal string
  bool is_closed = 12;                         // true when bucket is finalized
  google.protobuf.Timestamp timestamp = 13;    // server time of this update
}

message GetCandlesRequest {
  string instrument_id = 1;
  CandleInterval interval = 2;
  google.protobuf.Timestamp start_time = 3;
  google.protobuf.Timestamp end_time = 4;
  uint32 limit = 5;
  string cursor = 6;
}

message GetCandlesResponse {
  repeated Candle candles = 1;
  string next_cursor = 2;
}

message StreamCandlesRequest {
  string instrument_id = 1;
  CandleInterval interval = 2;
}

message Ticker {
  string instrument_id = 1;
  string symbol = 2;
  string last_price = 3;                       // decimal string
  string price_change_24h = 4;                 // decimal string
  string price_change_pct_24h = 5;             // decimal string (e.g. "2.5000")
  string high_24h = 6;
  string low_24h = 7;
  uint64 volume_24h = 8;
  string turnover_24h = 9;
  string best_bid = 10;
  string best_ask = 11;
  int64 open_interest = 12;
  google.protobuf.Timestamp last_trade_at = 13;
  google.protobuf.Timestamp timestamp = 14;    // server time
}

message GetTickerRequest {
  string instrument_id = 1;
}

message GetTickersRequest {
  repeated string instrument_ids = 1;
}

message GetTickersResponse {
  repeated Ticker tickers = 1;
}

message TradeEvent {
  string trade_id = 1;
  string instrument_id = 2;
  string price = 3;                            // decimal string
  uint64 quantity = 4;
  string trade_value = 5;                      // decimal string
  string aggressor_side = 6;                   // BUY or SELL
  string trade_type = 7;                       // CONTINUOUS, AUCTION, etc.
  uint64 sequence_number = 8;
  google.protobuf.Timestamp executed_at = 9;
}

message GetTradesRequest {
  string instrument_id = 1;
  uint32 limit = 2;
  uint64 since_sequence = 3;
  google.protobuf.Timestamp start_time = 4;
  google.protobuf.Timestamp end_time = 5;
}

message GetTradesResponse {
  repeated TradeEvent trades = 1;
}

message StreamTradesRequest {
  string instrument_id = 1;
  uint64 since_sequence = 2;
}

// Heartbeat sent on streams when no data for 15 seconds
message Heartbeat {
  google.protobuf.Timestamp timestamp = 1;
}
```

---

## 6. WebSocket Streaming

The API Gateway (T033) translates gRPC server-side streams into WebSocket connections. The market-data-service itself does NOT serve WebSocket — it exposes gRPC streams that the gateway wraps.

### 6.1 Gateway WebSocket Endpoints

These endpoints should be added to the gateway's routing table (extending T033):

| WebSocket Path | gRPC RPC | Auth | Description |
|---|---|---|---|
| `WS /api/v1/ws/candles/{instrument_id}?interval=1m` | `MarketDataAggregateService/StreamCandles` | public | Real-time candle updates |
| `WS /api/v1/ws/market-trades/{instrument_id}` | `MarketDataAggregateService/StreamTrades` | public | Aggregated trade stream |

**Note**: The existing gateway WS endpoints (`/ws/trades/`, `/ws/book/`) route to the matching engine's `MarketDataService` for raw data. The new endpoints route to this service for aggregated data.

### 6.2 WebSocket Message Format

Messages are JSON-encoded for WebSocket clients:

**Candle update:**
```json
{
  "type": "candle",
  "data": {
    "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
    "interval": "1m",
    "bucket": "2026-03-28T10:15:00Z",
    "open": "325.5000",
    "high": "326.2500",
    "low": "325.2500",
    "close": "326.0000",
    "volume": 1250,
    "trade_count": 47,
    "vwap": "325.8200",
    "turnover": "4072750.0000",
    "is_closed": false
  }
}
```

**Trade event:**
```json
{
  "type": "trade",
  "data": {
    "trade_id": "550e8400-e29b-41d4-a716-446655440099",
    "instrument_id": "550e8400-e29b-41d4-a716-446655440001",
    "price": "326.0000",
    "quantity": 50,
    "trade_value": "163000.0000",
    "aggressor_side": "BUY",
    "trade_type": "CONTINUOUS",
    "sequence_number": 1234567,
    "executed_at": "2026-03-28T10:15:23.456Z"
  }
}
```

**Heartbeat:**
```json
{
  "type": "heartbeat",
  "timestamp": "2026-03-28T10:15:30.000Z"
}
```

---

## 7. SQL Migrations

### V8 — Market Data Hypertable & Continuous Aggregates

This migration creates the `market_data` schema with TimescaleDB-specific features.

**Prerequisites**: TimescaleDB extension must be enabled (`CREATE EXTENSION IF NOT EXISTS timescaledb`).

```sql
-- V8: Market data hypertable and continuous aggregates (T035)
-- Requires: TimescaleDB extension

-- Enable TimescaleDB if not already enabled
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- ============================================================
-- Schema
-- ============================================================
CREATE SCHEMA IF NOT EXISTS market_data;

-- ============================================================
-- Trades hypertable (source of truth for all aggregates)
-- ============================================================
CREATE TABLE market_data.trades (
    trade_id            UUID NOT NULL,
    instrument_id       UUID NOT NULL,
    price               NUMERIC(18,4) NOT NULL,
    quantity            BIGINT NOT NULL,
    trade_value         NUMERIC(18,4) NOT NULL,
    aggressor_side      VARCHAR(4) NOT NULL CHECK (aggressor_side IN ('BUY', 'SELL')),
    trade_type          VARCHAR(12) NOT NULL DEFAULT 'CONTINUOUS'
                        CHECK (trade_type IN (
                            'CONTINUOUS','AUCTION','BLOCK','BUST','CORRECTION'
                        )),
    sequence_number     BIGINT NOT NULL,
    executed_at         TIMESTAMPTZ NOT NULL,
    ingested_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pk_market_data_trades PRIMARY KEY (executed_at, trade_id)
);

-- Convert to hypertable partitioned by executed_at (7-day chunks)
SELECT create_hypertable(
    'market_data.trades',
    'executed_at',
    chunk_time_interval => INTERVAL '7 days'
);

-- Unique index for deduplication
CREATE UNIQUE INDEX idx_trades_dedup
    ON market_data.trades (trade_id, executed_at);

-- Index for sequence-based queries (replay)
CREATE INDEX idx_trades_instrument_seq
    ON market_data.trades (instrument_id, sequence_number);

-- Index for time-range queries per instrument
CREATE INDEX idx_trades_instrument_time
    ON market_data.trades (instrument_id, executed_at DESC);

-- Append-only: block updates and deletes
CREATE RULE no_update_market_data_trades AS ON UPDATE TO market_data.trades
    DO INSTEAD NOTHING;
CREATE RULE no_delete_market_data_trades AS ON DELETE TO market_data.trades
    DO INSTEAD NOTHING;

-- ============================================================
-- 1-minute OHLCV continuous aggregate (base)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_1m
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('1 minute', executed_at) AS bucket,
    first(price, executed_at) AS open,
    max(price) AS high,
    min(price) AS low,
    last(price, executed_at) AS close,
    sum(quantity) AS volume,
    count(*) AS trade_count,
    CASE
        WHEN sum(quantity) > 0
        THEN sum(trade_value) / sum(quantity)
        ELSE NULL
    END AS vwap,
    sum(trade_value) AS turnover
FROM market_data.trades
WHERE trade_type NOT IN ('BUST', 'CORRECTION')
GROUP BY instrument_id, time_bucket('1 minute', executed_at)
WITH NO DATA;

-- Refresh policy: refresh every 10 seconds, lagging 30 seconds behind real-time
SELECT add_continuous_aggregate_policy('market_data.ohlcv_1m',
    start_offset    => INTERVAL '1 hour',
    end_offset      => INTERVAL '30 seconds',
    schedule_interval => INTERVAL '10 seconds'
);

-- ============================================================
-- 5-minute OHLCV (hierarchical over 1m)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_5m
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('5 minutes', bucket) AS bucket,
    first(open, bucket) AS open,
    max(high) AS high,
    min(low) AS low,
    last(close, bucket) AS close,
    sum(volume) AS volume,
    sum(trade_count)::INTEGER AS trade_count,
    CASE
        WHEN sum(volume) > 0
        THEN sum(turnover) / sum(volume)
        ELSE NULL
    END AS vwap,
    sum(turnover) AS turnover
FROM market_data.ohlcv_1m
GROUP BY instrument_id, time_bucket('5 minutes', bucket)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('market_data.ohlcv_5m',
    start_offset    => INTERVAL '2 hours',
    end_offset      => INTERVAL '1 minute',
    schedule_interval => INTERVAL '30 seconds'
);

-- ============================================================
-- 15-minute OHLCV (hierarchical over 1m)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_15m
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('15 minutes', bucket) AS bucket,
    first(open, bucket) AS open,
    max(high) AS high,
    min(low) AS low,
    last(close, bucket) AS close,
    sum(volume) AS volume,
    sum(trade_count)::INTEGER AS trade_count,
    CASE
        WHEN sum(volume) > 0
        THEN sum(turnover) / sum(volume)
        ELSE NULL
    END AS vwap,
    sum(turnover) AS turnover
FROM market_data.ohlcv_1m
GROUP BY instrument_id, time_bucket('15 minutes', bucket)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('market_data.ohlcv_15m',
    start_offset    => INTERVAL '6 hours',
    end_offset      => INTERVAL '2 minutes',
    schedule_interval => INTERVAL '1 minute'
);

-- ============================================================
-- 1-hour OHLCV (hierarchical over 1m)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_1h
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('1 hour', bucket) AS bucket,
    first(open, bucket) AS open,
    max(high) AS high,
    min(low) AS low,
    last(close, bucket) AS close,
    sum(volume) AS volume,
    sum(trade_count)::INTEGER AS trade_count,
    CASE
        WHEN sum(volume) > 0
        THEN sum(turnover) / sum(volume)
        ELSE NULL
    END AS vwap,
    sum(turnover) AS turnover
FROM market_data.ohlcv_1m
GROUP BY instrument_id, time_bucket('1 hour', bucket)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('market_data.ohlcv_1h',
    start_offset    => INTERVAL '1 day',
    end_offset      => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes'
);

-- ============================================================
-- 4-hour OHLCV (hierarchical over 1h)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_4h
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('4 hours', bucket) AS bucket,
    first(open, bucket) AS open,
    max(high) AS high,
    min(low) AS low,
    last(close, bucket) AS close,
    sum(volume) AS volume,
    sum(trade_count)::INTEGER AS trade_count,
    CASE
        WHEN sum(volume) > 0
        THEN sum(turnover) / sum(volume)
        ELSE NULL
    END AS vwap,
    sum(turnover) AS turnover
FROM market_data.ohlcv_1h
GROUP BY instrument_id, time_bucket('4 hours', bucket)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('market_data.ohlcv_4h',
    start_offset    => INTERVAL '3 days',
    end_offset      => INTERVAL '15 minutes',
    schedule_interval => INTERVAL '15 minutes'
);

-- ============================================================
-- 1-day OHLCV (hierarchical over 1h)
-- ============================================================
CREATE MATERIALIZED VIEW market_data.ohlcv_1d
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('1 day', bucket) AS bucket,
    first(open, bucket) AS open,
    max(high) AS high,
    min(low) AS low,
    last(close, bucket) AS close,
    sum(volume) AS volume,
    sum(trade_count)::INTEGER AS trade_count,
    CASE
        WHEN sum(volume) > 0
        THEN sum(turnover) / sum(volume)
        ELSE NULL
    END AS vwap,
    sum(turnover) AS turnover
FROM market_data.ohlcv_1h
GROUP BY instrument_id, time_bucket('1 day', bucket)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('market_data.ohlcv_1d',
    start_offset    => INTERVAL '7 days',
    end_offset      => INTERVAL '30 minutes',
    schedule_interval => INTERVAL '30 minutes'
);

-- ============================================================
-- Data Retention Policies
-- ============================================================

-- Raw tick trades: retain 90 days
SELECT add_retention_policy('market_data.trades', INTERVAL '90 days');

-- 1m candles: retain 1 year
SELECT add_retention_policy('market_data.ohlcv_1m', INTERVAL '1 year');

-- 5m candles: retain 1 year
SELECT add_retention_policy('market_data.ohlcv_5m', INTERVAL '1 year');

-- 15m candles: retain 1 year
SELECT add_retention_policy('market_data.ohlcv_15m', INTERVAL '1 year');

-- 1h, 4h, 1d candles: no retention policy (indefinite)

-- ============================================================
-- Service Role Grants
-- ============================================================
CREATE ROLE ace_marketdata_svc WITH LOGIN;
GRANT USAGE ON SCHEMA market_data TO ace_marketdata_svc;
GRANT SELECT, INSERT ON market_data.trades TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_5m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_15m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1h TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_4h TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1d TO ace_marketdata_svc;
```

---

## 8. Data Retention Policy

| Data Tier | Retention | Storage Estimate (50 instruments, 10k trades/day) |
|---|---|---|
| Raw trades (hypertable) | 90 days | ~450k rows, ~100 MB |
| 1m candles | 1 year | ~26M rows, ~2 GB |
| 5m candles | 1 year | ~5.3M rows, ~400 MB |
| 15m candles | 1 year | ~1.8M rows, ~150 MB |
| 1h candles | Indefinite | ~438k rows/year, ~35 MB/year |
| 4h candles | Indefinite | ~109k rows/year, ~9 MB/year |
| 1d candles | Indefinite | ~18k rows/year, ~2 MB/year |

TimescaleDB's `add_retention_policy` handles automatic chunk dropping. Compression policies can be added later for cold data (>30 day chunks compressed with `segmentby instrument_id, orderby bucket`).

---

## 9. Performance Requirements

| Metric | Target | Notes |
|---|---|---|
| Trade ingestion latency | < 10ms p99 | Time from matching engine trade to persisted in hypertable |
| Candle stream latency | < 50ms p99 | Time from trade to candle update delivered to subscriber |
| GetCandles query (500 candles) | < 20ms p99 | Reads from continuous aggregate |
| GetTicker query | < 10ms p99 | Computed from recent candles + cached bid/ask |
| GetTrades query (100 trades) | < 15ms p99 | Reads from hypertable with index |
| Concurrent stream subscribers | 5,000 | Per service instance |
| Trade throughput | 50,000 trades/sec | Burst capacity during high-volatility periods |

### Scaling Strategy

- **Horizontal scaling**: Multiple market-data-service replicas can consume the same Kafka topic (consumer group) and serve independent gRPC streams. Each replica maintains its own in-memory candle accumulators.
- **Read replicas**: TimescaleDB read replicas for query traffic isolation from ingestion.
- **Connection pooling**: pgBouncer for database connection management, sized at 20 connections per service instance.

---

## 10. Deployment Architecture

### Kubernetes Resources

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: market-data-service
  namespace: ace-exchange
spec:
  replicas: 2
  selector:
    matchLabels:
      app: market-data-service
  template:
    metadata:
      labels:
        app: market-data-service
      annotations:
        traffic.sidecar.istio.io/excludeInboundPorts: "50057"
    spec:
      containers:
      - name: market-data-service
        image: ace-platform/market-data-service:latest
        ports:
        - containerPort: 50057
          name: grpc
        - containerPort: 8087
          name: health
        env:
        - name: GRPC_PORT
          value: "50057"
        - name: HEALTH_PORT
          value: "8087"
        - name: TIMESCALEDB_DSN
          valueFrom:
            secretKeyRef:
              name: market-data-db-credentials
              key: dsn
        - name: MATCHING_ENGINE_ADDR
          value: "matching-engine.ace-exchange.svc.cluster.local:50051"
        - name: KAFKA_BROKERS
          value: "kafka.ace-infra.svc.cluster.local:9092"
        - name: KAFKA_TOPIC_PREFIX
          value: "exchange.trades"
        - name: KAFKA_CONSUMER_GROUP
          value: "market-data-service"
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: market-data-service
  namespace: ace-exchange
spec:
  selector:
    app: market-data-service
  ports:
  - name: grpc
    port: 50057
    targetPort: 50057
  - name: health
    port: 8087
    targetPort: 8087
```

### Configuration

| Env Variable | Default | Description |
|---|---|---|
| `GRPC_PORT` | `50057` | gRPC listen port |
| `HEALTH_PORT` | `8087` | Health HTTP port |
| `TIMESCALEDB_DSN` | (required) | TimescaleDB connection string |
| `MATCHING_ENGINE_ADDR` | `localhost:50051` | Matching engine gRPC address |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses |
| `KAFKA_TOPIC_PREFIX` | `exchange.trades` | Kafka trade topic prefix |
| `KAFKA_CONSUMER_GROUP` | `market-data-service` | Kafka consumer group |
| `CANDLE_STREAM_HEARTBEAT_SEC` | `15` | Heartbeat interval for streams |
| `DIRECT_POD_COMMS` | `true` | Bypass Istio sidecar for gRPC |

---

## 11. Failure Modes & Recovery

| Failure | Detection | Recovery |
|---|---|---|
| Matching engine gRPC stream drops | gRPC status / keepalive timeout | Reconnect with exponential backoff (1s → 30s). On reconnect, use `since_sequence` to replay missed trades from Kafka. |
| Kafka consumer lag | Consumer group lag metric > 10k | Scale up consumer instances. Alert if lag > 60 seconds. |
| TimescaleDB write failure | INSERT error / connection pool exhaustion | Buffer trades in memory (bounded queue, 100k max). Retry inserts. If buffer fills, drop oldest and log gap. |
| Continuous aggregate refresh delay | Materialization lag metric | Manual refresh via `CALL refresh_continuous_aggregate()`. Alert if lag > 5 minutes for 1m aggregates. |
| Service restart / crash | Pod termination | On startup: (1) query max `sequence_number` from `market_data.trades`, (2) resume consuming from that sequence via Kafka, (3) rebuild in-memory candle accumulators from most recent continuous aggregate row + any trades since last aggregate refresh. |
| Sequence gap detected | Gap in `sequence_number` on ingestion | Log warning, request replay from Kafka for the gap range. Do not block ingestion of later trades. |
| Trade bust received | `trade_type = 'BUST'` event | Persist the bust event. Busted trades are excluded from continuous aggregate WHERE clause (`trade_type NOT IN ('BUST', 'CORRECTION')`). The continuous aggregate automatically recomputes on next refresh. |

### Startup Sequence

```
1. Connect to TimescaleDB, verify schema version
2. Query max(sequence_number) from market_data.trades → resume_seq
3. Connect to Kafka consumer group, seek to resume_seq
4. Start gRPC server (returns SERVING on readyz only after step 5)
5. Start consuming trades (Kafka primary, gRPC fallback)
6. Build in-memory candle accumulators from DB state
7. Set readyz = true
8. Begin accepting gRPC requests
```

---

## 12. Gateway Integration

The API Gateway (T033) should add these routes to its endpoint mapping table:

### REST Endpoints (→ market-data-service :50057)

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/instruments/{instrument_id}/candles` | `MarketDataAggregateService/GetCandles` | public | Historical OHLCV candles |
| `GET` | `/api/v1/instruments/{instrument_id}/ticker` | `MarketDataAggregateService/GetTicker` | public | Ticker summary |
| `GET` | `/api/v1/tickers` | `MarketDataAggregateService/GetTickers` | public | All tickers |
| `GET` | `/api/v1/instruments/{instrument_id}/trades` | `MarketDataAggregateService/GetTrades` | public | Recent trades (tape) |

### WebSocket Endpoints (→ market-data-service :50057)

| WebSocket Path | gRPC RPC | Auth | Description |
|---|---|---|---|
| `WS /api/v1/ws/candles/{instrument_id}?interval=1m` | `MarketDataAggregateService/StreamCandles` | public | Real-time candle updates |
| `WS /api/v1/ws/market-trades/{instrument_id}` | `MarketDataAggregateService/StreamTrades` | public | Aggregated trade stream |

### Query Parameter Mapping (REST → gRPC)

```
GET /api/v1/instruments/{instrument_id}/candles?interval=1m&start=2026-03-27T00:00:00Z&end=2026-03-28T00:00:00Z&limit=500

→ GetCandlesRequest {
    instrument_id: "{instrument_id}",
    interval: CANDLE_INTERVAL_1M,
    start_time: "2026-03-27T00:00:00Z",
    end_time: "2026-03-28T00:00:00Z",
    limit: 500
  }

GET /api/v1/instruments/{instrument_id}/trades?limit=100&since_sequence=12000

→ GetTradesRequest {
    instrument_id: "{instrument_id}",
    limit: 100,
    since_sequence: 12000
  }
```

---

*End of Market Data Service Architecture Specification*
