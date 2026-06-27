-- V23: Default fund management tables for CCP default waterfall
-- Implements IOSCO/PFMI default management principles

CREATE TABLE IF NOT EXISTS clearing.default_fund_contributions (
    participant_id VARCHAR(64) NOT NULL,
    amount DECIMAL(18,4) NOT NULL DEFAULT 0,
    currency VARCHAR(3) DEFAULT 'MNT',
    deposited_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (participant_id)
);

CREATE TABLE IF NOT EXISTS clearing.default_events (
    id VARCHAR(64) PRIMARY KEY,
    defaulting_participant_id VARCHAR(64) NOT NULL,
    total_loss DECIMAL(18,4) NOT NULL,
    waterfall_used JSONB NOT NULL, -- tracks which layers absorbed how much
    status VARCHAR(20) NOT NULL DEFAULT 'IN_PROGRESS',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS clearing.ccp_capital (
    id VARCHAR(64) PRIMARY KEY,
    amount DECIMAL(18,4) NOT NULL,
    purpose VARCHAR(100) NOT NULL, -- 'skin_in_the_game', 'additional_capital'
    deposited_at TIMESTAMPTZ DEFAULT NOW()
);
