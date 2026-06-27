-- V21: Participant tiering and membership management
-- Supports: farmer, hedger, speculator, market_maker, clearing_member tiers

CREATE SCHEMA IF NOT EXISTS participants;

CREATE TABLE participants.members (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    legal_name VARCHAR(255) NOT NULL,
    entity_type VARCHAR(50),              -- individual, corporate, cooperative, farmer_group
    tier VARCHAR(30) NOT NULL DEFAULT 'speculator', -- farmer, hedger, speculator, market_maker, clearing_member
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',  -- PENDING, ACTIVE, SUSPENDED, TERMINATED
    onboarded_at TIMESTAMPTZ,
    net_worth_category VARCHAR(20),       -- small, medium, large
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_members_user ON participants.members(user_id);
CREATE INDEX idx_members_status ON participants.members(status);

CREATE TABLE participants.membership_history (
    id VARCHAR(64) PRIMARY KEY,
    member_id VARCHAR(64) NOT NULL,
    action VARCHAR(50) NOT NULL,          -- CREATED, TIER_CHANGED, SUSPENDED, REINSTATED, TERMINATED
    old_value VARCHAR(100),
    new_value VARCHAR(100),
    reason TEXT,
    actor_id VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_membership_history_member ON participants.membership_history(member_id);
