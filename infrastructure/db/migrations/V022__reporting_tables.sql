-- V22: Reporting tables for settlement statements, market summaries, and large trader positions.

CREATE SCHEMA IF NOT EXISTS reporting;

CREATE TABLE reporting.daily_statements (
    id VARCHAR(64) PRIMARY KEY,
    participant_id VARCHAR(64) NOT NULL,
    report_date DATE NOT NULL,
    positions JSONB,
    margin JSONB,
    pnl JSONB,
    fees JSONB,
    net_amount DECIMAL(18,4),
    generated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(participant_id, report_date)
);

CREATE TABLE reporting.market_summaries (
    id VARCHAR(64) PRIMARY KEY,
    instrument_id VARCHAR(64) NOT NULL,
    report_date DATE NOT NULL,
    open_price DECIMAL(18,4),
    high_price DECIMAL(18,4),
    low_price DECIMAL(18,4),
    close_price DECIMAL(18,4),
    volume DECIMAL(18,4),
    open_interest DECIMAL(18,4),
    settlement_price DECIMAL(18,4),
    generated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(instrument_id, report_date)
);

CREATE TABLE reporting.large_trader_positions (
    id VARCHAR(64) PRIMARY KEY,
    participant_id VARCHAR(64) NOT NULL,
    instrument_id VARCHAR(64) NOT NULL,
    report_date DATE NOT NULL,
    net_position DECIMAL(18,4),
    gross_position DECIMAL(18,4),
    pct_of_open_interest DECIMAL(8,4),
    UNIQUE(participant_id, instrument_id, report_date)
);
