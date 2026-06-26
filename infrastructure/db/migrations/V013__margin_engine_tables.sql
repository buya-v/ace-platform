-- V13: Margin engine PostgreSQL persistence tables
-- Stores portfolio margins, margin calls, and margin parameters

CREATE SCHEMA IF NOT EXISTS margin;

-- Portfolio margin snapshots: one row per participant per calculation
CREATE TABLE margin.portfolio_margins (
    id                VARCHAR(64) PRIMARY KEY,
    participant_id    VARCHAR(64) NOT NULL,
    initial_margin    DECIMAL(18,4) NOT NULL DEFAULT 0,
    maintenance_margin DECIMAL(18,4) NOT NULL DEFAULT 0,
    collateral_value  DECIMAL(18,4) NOT NULL DEFAULT 0,
    excess_deficit    DECIMAL(18,4) NOT NULL DEFAULT 0,
    calculated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_portfolio_margins_participant ON margin.portfolio_margins (participant_id);
CREATE INDEX idx_portfolio_margins_calculated  ON margin.portfolio_margins (calculated_at);

-- Margin calls: issued when collateral < required margin
CREATE TABLE margin.margin_calls (
    id               VARCHAR(64) PRIMARY KEY,
    participant_id   VARCHAR(64) NOT NULL,
    call_amount      DECIMAL(18,4) NOT NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'ISSUED',
    deadline         TIMESTAMPTZ,
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at      TIMESTAMPTZ
);

CREATE INDEX idx_margin_calls_participant ON margin.margin_calls (participant_id);
CREATE INDEX idx_margin_calls_status      ON margin.margin_calls (status);

-- Margin parameters per instrument (initial/maintenance percentages)
CREATE TABLE margin.margin_parameters (
    instrument_id         VARCHAR(64) PRIMARY KEY,
    initial_margin_pct    DECIMAL(8,4) NOT NULL DEFAULT 10.0,
    maintenance_margin_pct DECIMAL(8,4) NOT NULL DEFAULT 7.5,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
