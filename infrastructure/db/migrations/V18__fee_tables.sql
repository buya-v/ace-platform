-- Fee management tables for GarudaX commodity exchange
CREATE SCHEMA IF NOT EXISTS fees;

CREATE TABLE fees.fee_schedules (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    status VARCHAR(20) DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE fees.fee_rules (
    id VARCHAR(64) PRIMARY KEY,
    schedule_id VARCHAR(64) REFERENCES fees.fee_schedules(id),
    fee_type VARCHAR(30) NOT NULL, -- trading, clearing, data, membership
    instrument_pattern VARCHAR(100) DEFAULT '*',
    participant_tier VARCHAR(30) DEFAULT '*',
    rate_bps DECIMAL(10,4) NOT NULL, -- basis points
    min_fee DECIMAL(18,4) DEFAULT 0,
    max_fee DECIMAL(18,4),
    per_contract_fee DECIMAL(18,4) DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE fees.fee_transactions (
    id VARCHAR(64) PRIMARY KEY,
    trade_id VARCHAR(64) NOT NULL,
    participant_id VARCHAR(64) NOT NULL,
    fee_type VARCHAR(30) NOT NULL,
    amount DECIMAL(18,4) NOT NULL,
    currency VARCHAR(3) DEFAULT 'MNT',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_fee_transactions_participant ON fees.fee_transactions(participant_id);
CREATE INDEX idx_fee_transactions_trade ON fees.fee_transactions(trade_id);

CREATE TABLE fees.participant_tiers (
    participant_id VARCHAR(64) PRIMARY KEY,
    tier VARCHAR(30) NOT NULL DEFAULT 'speculator',
    volume_30d DECIMAL(18,4) DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Default fee schedule for agricultural exchange
INSERT INTO fees.fee_schedules (id, name, effective_from, status) VALUES
    ('default', 'GarudaX Default Fee Schedule', '2026-01-01', 'ACTIVE');

INSERT INTO fees.fee_rules (id, schedule_id, fee_type, participant_tier, rate_bps) VALUES
    ('default-farmer-trading', 'default', 'trading', 'farmer', 10.0),
    ('default-hedger-trading', 'default', 'trading', 'hedger', 15.0),
    ('default-speculator-trading', 'default', 'trading', 'speculator', 25.0),
    ('default-market_maker-trading', 'default', 'trading', 'market_maker', 5.0),
    ('default-clearing', 'default', 'clearing', '*', 5.0);
