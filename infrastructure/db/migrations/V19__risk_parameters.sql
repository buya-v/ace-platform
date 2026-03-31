-- V19: Pre-trade risk parameters
-- Position limits per participant per instrument and order-level limits per instrument.

CREATE SCHEMA IF NOT EXISTS risk;

CREATE TABLE risk.position_limits (
    participant_id VARCHAR(64) NOT NULL,
    instrument_id VARCHAR(64) NOT NULL,
    max_long DECIMAL(18,4) NOT NULL DEFAULT 1000,
    max_short DECIMAL(18,4) NOT NULL DEFAULT 1000,
    max_gross DECIMAL(18,4) NOT NULL DEFAULT 2000,
    PRIMARY KEY (participant_id, instrument_id)
);

CREATE TABLE risk.order_limits (
    instrument_id VARCHAR(64) PRIMARY KEY,
    max_order_qty DECIMAL(18,4) NOT NULL DEFAULT 100,
    max_order_value DECIMAL(18,4) NOT NULL DEFAULT 1000000,
    price_band_pct DECIMAL(8,4) NOT NULL DEFAULT 20.0
);

-- Default order limits for seeded instruments
INSERT INTO risk.order_limits (instrument_id, max_order_qty, max_order_value, price_band_pct) VALUES
    ('WHT-HRW-2026M07-UB', 500, 5000000, 10.0),
    ('CRN-YEL-2026M09-UB', 500, 5000000, 10.0),
    ('SBN-NO2-2026M11-UB', 500, 5000000, 10.0),
    ('BRL-MALT-2026M07-UB', 500, 5000000, 10.0),
    ('CSH-RAW-2026M09-UB', 200, 2000000, 15.0),
    ('LVS-CATTLE-2026M10-UB', 100, 4000000, 10.0);
