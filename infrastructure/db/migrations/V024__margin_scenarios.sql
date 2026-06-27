-- V24: SPAN-like scenario-based margin model
-- Adds risk arrays (pre-computed P&L per scenario per instrument) and
-- inter-commodity spread credits for margin relief on offsetting positions.

CREATE TABLE margin.risk_arrays (
    instrument_id VARCHAR(64) NOT NULL,
    scenario_id INT NOT NULL,
    price_shift_pct DECIMAL(8,4) NOT NULL,
    vol_shift_pct DECIMAL(8,4) NOT NULL,
    pnl_impact DECIMAL(18,4) NOT NULL,
    PRIMARY KEY (instrument_id, scenario_id)
);

CREATE TABLE margin.spread_credits (
    id VARCHAR(64) PRIMARY KEY,
    long_instrument_id VARCHAR(64) NOT NULL,
    short_instrument_id VARCHAR(64) NOT NULL,
    credit_pct DECIMAL(8,4) NOT NULL DEFAULT 50.0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 16 standard scenarios per instrument (price +/-1sigma,2sigma,3sigma x vol +/-25%)
-- Seed for wheat
INSERT INTO margin.risk_arrays (instrument_id, scenario_id, price_shift_pct, vol_shift_pct, pnl_impact) VALUES
    ('WHT-HRW-2026M07-UB', 1, 3.0, 25.0, -150.00),
    ('WHT-HRW-2026M07-UB', 2, 3.0, -25.0, -140.00),
    ('WHT-HRW-2026M07-UB', 3, -3.0, 25.0, 160.00),
    ('WHT-HRW-2026M07-UB', 4, -3.0, -25.0, 150.00),
    ('WHT-HRW-2026M07-UB', 5, 6.0, 25.0, -310.00),
    ('WHT-HRW-2026M07-UB', 6, 6.0, -25.0, -290.00),
    ('WHT-HRW-2026M07-UB', 7, -6.0, 25.0, 320.00),
    ('WHT-HRW-2026M07-UB', 8, -6.0, -25.0, 300.00),
    ('WHT-HRW-2026M07-UB', 9, 10.0, 25.0, -520.00),
    ('WHT-HRW-2026M07-UB', 10, 10.0, -25.0, -490.00),
    ('WHT-HRW-2026M07-UB', 11, -10.0, 25.0, 530.00),
    ('WHT-HRW-2026M07-UB', 12, -10.0, -25.0, 500.00),
    ('WHT-HRW-2026M07-UB', 13, 15.0, 25.0, -780.00),
    ('WHT-HRW-2026M07-UB', 14, 15.0, -25.0, -740.00),
    ('WHT-HRW-2026M07-UB', 15, -15.0, 25.0, 790.00),
    ('WHT-HRW-2026M07-UB', 16, -15.0, -25.0, 750.00);
