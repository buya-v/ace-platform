-- V12: Clearing engine PostgreSQL persistence
-- Tables for obligations, positions, and netting results

CREATE SCHEMA IF NOT EXISTS clearing;

-- Clearing obligations: novated trade obligations between participants and the CCP
CREATE TABLE clearing.obligations (
    obligation_id   VARCHAR(64) PRIMARY KEY,
    trade_id        VARCHAR(64) NOT NULL,
    instrument_id   VARCHAR(64) NOT NULL,
    participant_id  VARCHAR(64) NOT NULL,
    side            SMALLINT NOT NULL,           -- 1=BUY, 2=SELL
    price           DECIMAL(18,4) NOT NULL,
    quantity        BIGINT NOT NULL,
    value           DECIMAL(18,4) NOT NULL,
    status          SMALLINT NOT NULL DEFAULT 1, -- 0=PENDING,1=NOVATED,2=NETTED,3=SETTLED,4=REJECTED
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    novated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_obligations_trade_id ON clearing.obligations (trade_id);
CREATE INDEX idx_obligations_participant_id ON clearing.obligations (participant_id);
CREATE INDEX idx_obligations_instrument_id ON clearing.obligations (instrument_id);
CREATE INDEX idx_obligations_status ON clearing.obligations (status);

-- Positions: net position per participant per instrument
CREATE TABLE clearing.positions (
    participant_id  VARCHAR(64) NOT NULL,
    instrument_id   VARCHAR(64) NOT NULL,
    net_qty         BIGINT NOT NULL DEFAULT 0,
    avg_price       DECIMAL(18,4) NOT NULL DEFAULT 0,
    total_buy_qty   BIGINT NOT NULL DEFAULT 0,
    total_sell_qty  BIGINT NOT NULL DEFAULT 0,
    realized_pnl    DECIMAL(18,4) NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, instrument_id)
);

CREATE INDEX idx_positions_instrument_id ON clearing.positions (instrument_id);

-- Netting results: output of multilateral netting runs
CREATE TABLE clearing.netting_results (
    id              VARCHAR(64) PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL,
    participant_id  VARCHAR(64) NOT NULL,
    instrument_id   VARCHAR(64) NOT NULL,
    net_qty         BIGINT NOT NULL DEFAULT 0,
    net_value       DECIMAL(18,4) NOT NULL DEFAULT 0,
    gross_long_qty  BIGINT NOT NULL DEFAULT 0,
    gross_short_qty BIGINT NOT NULL DEFAULT 0,
    obligations_count INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    netted_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_netting_results_run_id ON clearing.netting_results (run_id);
CREATE INDEX idx_netting_results_participant_id ON clearing.netting_results (participant_id);
CREATE INDEX idx_netting_results_instrument_id ON clearing.netting_results (instrument_id);
