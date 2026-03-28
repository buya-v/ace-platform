-- V8: Market data hypertable and continuous aggregates (T035)
-- Requires: TimescaleDB extension
-- Creates: market_data schema with trades hypertable, OHLCV continuous aggregates,
--          retention policies, and service role grants.

-- ============================================================
-- TimescaleDB Extension
-- ============================================================
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
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ace_marketdata_svc') THEN
        CREATE ROLE ace_marketdata_svc WITH LOGIN;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA market_data TO ace_marketdata_svc;
GRANT SELECT, INSERT ON market_data.trades TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_5m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_15m TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1h TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_4h TO ace_marketdata_svc;
GRANT SELECT ON market_data.ohlcv_1d TO ace_marketdata_svc;
