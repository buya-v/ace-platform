-- Reference data schema for instrument definitions, commodities, and delivery infrastructure

CREATE SCHEMA IF NOT EXISTS reference;

CREATE TABLE reference.commodities (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(50) NOT NULL, -- grain, oilseed, livestock, dairy, fiber
    unit VARCHAR(20) NOT NULL, -- bushel, cwt, lb, kg, mt
    grade_specs JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE reference.instruments (
    id VARCHAR(64) PRIMARY KEY,
    commodity_id VARCHAR(64) REFERENCES reference.commodities(id),
    name VARCHAR(255) NOT NULL,
    delivery_month INT NOT NULL, -- 1-12
    delivery_year INT NOT NULL,
    contract_size DECIMAL(18,4) NOT NULL, -- e.g., 5000 bushels
    tick_size DECIMAL(18,8) NOT NULL, -- minimum price increment
    min_price_increment DECIMAL(18,8),
    currency VARCHAR(3) NOT NULL DEFAULT 'MNT',
    trading_hours VARCHAR(100), -- e.g., "09:00-15:00"
    first_trade_date DATE,
    last_trade_date DATE,
    delivery_start DATE,
    delivery_end DATE,
    settlement_type VARCHAR(20) DEFAULT 'PHYSICAL', -- PHYSICAL or CASH
    status VARCHAR(20) DEFAULT 'ACTIVE', -- ACTIVE, SUSPENDED, EXPIRED
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE reference.delivery_locations (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    address TEXT,
    latitude DECIMAL(10,7),
    longitude DECIMAL(10,7),
    capacity_mt DECIMAL(18,2),
    commodities_accepted TEXT[], -- array of commodity_ids
    status VARCHAR(20) DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE reference.trading_calendar (
    cal_date DATE PRIMARY KEY,
    is_trading_day BOOLEAN DEFAULT TRUE,
    market_open TIME,
    market_close TIME,
    notes TEXT
);

-- Seed 6 agricultural commodity instruments
INSERT INTO reference.commodities (id, name, category, unit) VALUES
    ('WHT-HRW', 'Hard Red Winter Wheat', 'grain', 'bushel'),
    ('CRN-YEL', 'Yellow Corn', 'grain', 'bushel'),
    ('SBN-NO2', 'No.2 Soybeans', 'oilseed', 'bushel'),
    ('BRL-MALT', 'Malting Barley', 'grain', 'bushel'),
    ('CSH-RAW', 'Raw Cashmere', 'fiber', 'kg'),
    ('LVS-CATTLE', 'Live Cattle', 'livestock', 'cwt');

INSERT INTO reference.instruments (id, commodity_id, name, delivery_month, delivery_year, contract_size, tick_size, currency, settlement_type, first_trade_date, last_trade_date, status) VALUES
    ('WHT-HRW-2026M07-UB', 'WHT-HRW', 'HRW Wheat Jul 2026', 7, 2026, 5000, 0.0025, 'MNT', 'PHYSICAL', '2026-01-15', '2026-06-30', 'ACTIVE'),
    ('CRN-YEL-2026M09-UB', 'CRN-YEL', 'Yellow Corn Sep 2026', 9, 2026, 5000, 0.0025, 'MNT', 'PHYSICAL', '2026-03-01', '2026-08-31', 'ACTIVE'),
    ('SBN-NO2-2026M11-UB', 'SBN-NO2', 'No.2 Soybeans Nov 2026', 11, 2026, 5000, 0.0025, 'MNT', 'PHYSICAL', '2026-05-01', '2026-10-31', 'ACTIVE'),
    ('BRL-MALT-2026M07-UB', 'BRL-MALT', 'Malting Barley Jul 2026', 7, 2026, 5000, 0.0025, 'MNT', 'PHYSICAL', '2026-01-15', '2026-06-30', 'ACTIVE'),
    ('CSH-RAW-2026M09-UB', 'CSH-RAW', 'Raw Cashmere Sep 2026', 9, 2026, 100, 0.01, 'MNT', 'PHYSICAL', '2026-03-01', '2026-08-31', 'ACTIVE'),
    ('LVS-CATTLE-2026M10-UB', 'LVS-CATTLE', 'Live Cattle Oct 2026', 10, 2026, 40000, 0.025, 'MNT', 'PHYSICAL', '2026-04-01', '2026-09-30', 'ACTIVE');
