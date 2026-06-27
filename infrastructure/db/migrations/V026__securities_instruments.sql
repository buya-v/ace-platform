-- V26: Securities instrument reference data
-- Extends the exchange to support equities, bonds, and ETFs

CREATE SCHEMA IF NOT EXISTS securities;

-- Securities instrument master table
CREATE TABLE securities.instruments (
    instrument_id       VARCHAR(64) PRIMARY KEY,
    isin                CHAR(12) NOT NULL UNIQUE,
    cusip               CHAR(9),
    sedol               CHAR(7),
    ticker              VARCHAR(12) NOT NULL,
    exchange_code       CHAR(4) NOT NULL DEFAULT 'MXUB',
    name                VARCHAR(255) NOT NULL,
    asset_class         VARCHAR(10) NOT NULL CHECK (asset_class IN ('EQUITY', 'BOND', 'ETF')),
    security_type       VARCHAR(20) NOT NULL CHECK (security_type IN (
        'COMMON', 'PREFERRED', 'GOVT_BOND', 'CORP_BOND', 'ZERO_COUPON', 'ETF'
    )),
    currency            CHAR(3) NOT NULL DEFAULT 'MNT',
    lot_size            INT NOT NULL DEFAULT 100,
    tick_size           DECIMAL(18,8) NOT NULL DEFAULT 1.0,
    listing_date        DATE NOT NULL,
    trading_status      VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (trading_status IN (
        'ACTIVE', 'HALTED', 'SUSPENDED', 'DELISTED'
    )),

    -- Issuer
    issuer_name         VARCHAR(255),
    issuer_country      CHAR(2) DEFAULT 'MN',
    sector              VARCHAR(100),

    -- Equity-specific
    shares_outstanding  BIGINT DEFAULT 0,
    market_cap          DECIMAL(18,4) DEFAULT 0,

    -- Bond-specific
    par_value           DECIMAL(18,4) DEFAULT 0,
    coupon_rate         DECIMAL(8,4) DEFAULT 0,
    coupon_frequency    VARCHAR(15) DEFAULT 'NONE' CHECK (coupon_frequency IN (
        'NONE', 'ANNUAL', 'SEMI_ANNUAL', 'QUARTERLY', 'MONTHLY'
    )),
    day_count_convention VARCHAR(10) DEFAULT 'ACT/365' CHECK (day_count_convention IN (
        'ACT/360', 'ACT/365', '30/360', 'ACT/ACT'
    )),
    maturity_date       DATE,
    issue_date          DATE,
    next_coupon_date    DATE,

    -- ETF-specific
    nav_per_share       DECIMAL(18,4) DEFAULT 0,
    fund_manager        VARCHAR(255),

    -- Trading controls
    price_band_pct      DECIMAL(5,2) NOT NULL DEFAULT 10.00,
    max_order_qty       BIGINT NOT NULL DEFAULT 10000,
    max_order_value     DECIMAL(18,4) NOT NULL DEFAULT 5000000000,
    short_sell_allowed  BOOLEAN NOT NULL DEFAULT TRUE,
    margin_eligible     BOOLEAN NOT NULL DEFAULT TRUE,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_instruments_isin ON securities.instruments(isin);
CREATE INDEX idx_sec_instruments_ticker ON securities.instruments(ticker);
CREATE INDEX idx_sec_instruments_asset_class ON securities.instruments(asset_class);
CREATE INDEX idx_sec_instruments_security_type ON securities.instruments(security_type);
CREATE INDEX idx_sec_instruments_trading_status ON securities.instruments(trading_status);
CREATE INDEX idx_sec_instruments_exchange_code ON securities.instruments(exchange_code);
CREATE INDEX idx_sec_instruments_maturity ON securities.instruments(maturity_date) WHERE maturity_date IS NOT NULL;

-- Short-sell restricted list
CREATE TABLE securities.short_sell_restricted_list (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    reason              VARCHAR(100) NOT NULL,
    restricted_from     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    restricted_until    TIMESTAMPTZ,
    added_by            VARCHAR(64) NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id, restricted_from)
);

-- Short-sell locate records
CREATE TABLE securities.locates (
    locate_id           VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    requested_qty       BIGINT NOT NULL,
    confirmed_qty       BIGINT NOT NULL DEFAULT 0,
    lender_id           VARCHAR(64),
    status              VARCHAR(15) NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
        'REQUESTED', 'CONFIRMED', 'DECLINED', 'EXPIRED', 'USED'
    )),
    valid_until         TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at        TIMESTAMPTZ
);

CREATE INDEX idx_locates_participant ON securities.locates(participant_id);
CREATE INDEX idx_locates_instrument ON securities.locates(instrument_id);
CREATE INDEX idx_locates_status ON securities.locates(status);

-- Position limits per security per participant
CREATE TABLE securities.position_limits (
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    max_long_qty        BIGINT NOT NULL DEFAULT 1000000,
    max_short_qty       BIGINT NOT NULL DEFAULT 500000,
    concentration_limit_pct DECIMAL(5,2) NOT NULL DEFAULT 5.00,
    max_order_value     DECIMAL(18,4) NOT NULL DEFAULT 1000000000,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, instrument_id)
);

-- Large trader reporting threshold
CREATE TABLE securities.large_trader_thresholds (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    threshold_qty       BIGINT NOT NULL DEFAULT 100000,
    threshold_pct       DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id)
);

-- SSR (Short-Sale Restriction) trigger tracking
CREATE TABLE securities.ssr_triggers (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    trigger_date        DATE NOT NULL,
    previous_close      DECIMAL(18,4) NOT NULL,
    trigger_price       DECIMAL(18,4) NOT NULL,
    decline_pct         DECIMAL(5,2) NOT NULL,
    active_until        DATE NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id, trigger_date)
);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA securities TO garudax_exchange_svc;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT ON securities.instruments TO garudax_clearing_svc;
        GRANT SELECT ON securities.position_limits TO garudax_clearing_svc;
    END IF;
END $$;
