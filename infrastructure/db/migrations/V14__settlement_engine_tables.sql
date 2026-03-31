-- Settlement engine tables
-- Stores settlement cycles, payment instructions, and settlement prices

CREATE SCHEMA IF NOT EXISTS settlement;

-- Settlement cycles (daily end-of-day settlement runs)
CREATE TABLE settlement.cycles (
    id              VARCHAR(64)     PRIMARY KEY,
    status          VARCHAR(20)     NOT NULL DEFAULT 'VALUING',
    settle_date     DATE            NOT NULL,
    total_payin     DECIMAL(18,4)   DEFAULT 0,
    total_payout    DECIMAL(18,4)   DEFAULT 0,
    error_message   TEXT,
    started_at      TIMESTAMPTZ     DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_cycles_status ON settlement.cycles (status);
CREATE INDEX idx_cycles_settle_date ON settlement.cycles (settle_date);

-- Settlement instructions (net payment instructions per participant per cycle)
CREATE TABLE settlement.instructions (
    id              VARCHAR(64)     PRIMARY KEY,
    cycle_id        VARCHAR(64)     NOT NULL REFERENCES settlement.cycles(id),
    participant_id  VARCHAR(64)     NOT NULL,
    amount          DECIMAL(18,4)   NOT NULL,
    direction       VARCHAR(10)     NOT NULL,
    status          VARCHAR(20)     NOT NULL DEFAULT 'PENDING',
    error_message   TEXT,
    created_at      TIMESTAMPTZ     DEFAULT NOW(),
    submitted_at    TIMESTAMPTZ,
    confirmed_at    TIMESTAMPTZ
);

CREATE INDEX idx_instructions_cycle_id ON settlement.instructions (cycle_id);
CREATE INDEX idx_instructions_participant_id ON settlement.instructions (participant_id);
CREATE INDEX idx_instructions_status ON settlement.instructions (status);

-- Settlement prices (mark/settlement price per instrument per date)
CREATE TABLE settlement.prices (
    instrument_id       VARCHAR(64)     NOT NULL,
    settlement_price    DECIMAL(18,4)   NOT NULL,
    previous_price      DECIMAL(18,4)   DEFAULT 0,
    price_date          DATE            NOT NULL,
    created_at          TIMESTAMPTZ     DEFAULT NOW(),
    PRIMARY KEY (instrument_id, price_date)
);

CREATE INDEX idx_prices_instrument ON settlement.prices (instrument_id);
CREATE INDEX idx_prices_date ON settlement.prices (price_date);
