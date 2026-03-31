-- V16: Market Data TimescaleDB tables for trades, candles, and tickers
-- Depends on: market_data schema (V4)

-- Ensure schema exists
CREATE SCHEMA IF NOT EXISTS market_data;

-- Ensure TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Trades: append-only time-series data from matching engine
CREATE TABLE IF NOT EXISTS market_data.trades (
    id VARCHAR(64) NOT NULL,
    instrument_id VARCHAR(64) NOT NULL,
    price DECIMAL(18,4) NOT NULL,
    quantity BIGINT NOT NULL,
    trade_value DECIMAL(18,4) NOT NULL DEFAULT 0,
    aggressor_side VARCHAR(10),
    trade_type VARCHAR(20),
    sequence_number BIGINT NOT NULL DEFAULT 0,
    traded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (traded_at, id)
);
SELECT create_hypertable('market_data.trades', 'traded_at', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_trades_instrument_time
    ON market_data.trades (instrument_id, traded_at DESC);

CREATE INDEX IF NOT EXISTS idx_trades_instrument_seq
    ON market_data.trades (instrument_id, sequence_number);

-- Candles: OHLCV aggregated candle data
CREATE TABLE IF NOT EXISTS market_data.candles (
    instrument_id VARCHAR(64) NOT NULL,
    interval VARCHAR(10) NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    open DECIMAL(18,4) NOT NULL,
    high DECIMAL(18,4) NOT NULL,
    low DECIMAL(18,4) NOT NULL,
    close DECIMAL(18,4) NOT NULL,
    volume BIGINT NOT NULL DEFAULT 0,
    trade_count INT NOT NULL DEFAULT 0,
    vwap DECIMAL(18,4),
    turnover DECIMAL(18,4) DEFAULT 0,
    PRIMARY KEY (instrument_id, interval, bucket)
);

-- Tickers: latest 24h summary per instrument (one row per instrument, upserted)
CREATE TABLE IF NOT EXISTS market_data.tickers (
    instrument_id VARCHAR(64) PRIMARY KEY,
    symbol VARCHAR(64),
    last_price DECIMAL(18,4),
    bid DECIMAL(18,4),
    ask DECIMAL(18,4),
    volume_24h BIGINT DEFAULT 0,
    turnover_24h DECIMAL(18,4) DEFAULT 0,
    high_24h DECIMAL(18,4),
    low_24h DECIMAL(18,4),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
