-- V27: Securities trading tables
-- Orders and trades for securities (extends exchange schema)

-- Securities orders table (parallels exchange.orders for commodities)
CREATE TABLE securities.orders (
    id                  VARCHAR(64) PRIMARY KEY,
    client_order_id     VARCHAR(64),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    account_id          VARCHAR(64) NOT NULL,
    participant_id      VARCHAR(64) NOT NULL,
    side                VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    order_type          VARCHAR(20) NOT NULL CHECK (order_type IN (
        'LIMIT', 'MARKET', 'STOP_LIMIT', 'STOP_MARKET'
    )),
    tif                 VARCHAR(10) NOT NULL CHECK (tif IN ('DAY', 'GTC', 'GTD', 'IOC', 'FOK')),
    price               DECIMAL(18,4),
    stop_price          DECIMAL(18,4),
    quantity            BIGINT NOT NULL,
    filled_qty          BIGINT NOT NULL DEFAULT 0,
    remaining_qty       BIGINT NOT NULL,
    status              VARCHAR(20) NOT NULL CHECK (status IN (
        'NEW', 'PARTIALLY_FILLED', 'FILLED', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    is_short_sell       BOOLEAN NOT NULL DEFAULT FALSE,
    locate_id           VARCHAR(64),
    settlement_date     DATE NOT NULL,
    reject_reason       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_orders_instrument ON securities.orders(instrument_id);
CREATE INDEX idx_sec_orders_account ON securities.orders(account_id);
CREATE INDEX idx_sec_orders_participant ON securities.orders(participant_id);
CREATE INDEX idx_sec_orders_status ON securities.orders(status);
CREATE INDEX idx_sec_orders_settlement ON securities.orders(settlement_date);
CREATE INDEX idx_sec_orders_created ON securities.orders(created_at);

-- Securities trades table (append-only)
CREATE TABLE securities.trades (
    id                  VARCHAR(64) PRIMARY KEY,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    buy_order_id        VARCHAR(64) NOT NULL,
    sell_order_id       VARCHAR(64) NOT NULL,
    buyer_participant_id  VARCHAR(64) NOT NULL,
    seller_participant_id VARCHAR(64) NOT NULL,
    price               DECIMAL(18,4) NOT NULL,
    quantity            BIGINT NOT NULL,
    trade_value         DECIMAL(18,4) NOT NULL,
    accrued_interest    DECIMAL(18,4) NOT NULL DEFAULT 0,
    settlement_value    DECIMAL(18,4) NOT NULL,
    aggressor_side      VARCHAR(4) NOT NULL CHECK (aggressor_side IN ('BUY', 'SELL')),
    is_short_sell       BOOLEAN NOT NULL DEFAULT FALSE,
    settlement_date     DATE NOT NULL,
    traded_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only protection
CREATE RULE no_update_sec_trades AS ON UPDATE TO securities.trades DO INSTEAD NOTHING;
CREATE RULE no_delete_sec_trades AS ON DELETE TO securities.trades DO INSTEAD NOTHING;

CREATE INDEX idx_sec_trades_instrument ON securities.trades(instrument_id);
CREATE INDEX idx_sec_trades_traded_at ON securities.trades(traded_at);
CREATE INDEX idx_sec_trades_buyer ON securities.trades(buyer_participant_id);
CREATE INDEX idx_sec_trades_seller ON securities.trades(seller_participant_id);
CREATE INDEX idx_sec_trades_settlement_date ON securities.trades(settlement_date);

-- Securities execution reports (append-only)
CREATE TABLE securities.execution_reports (
    id                  VARCHAR(64) PRIMARY KEY,
    order_id            VARCHAR(64) NOT NULL,
    trade_id            VARCHAR(64),
    exec_type           VARCHAR(20) NOT NULL CHECK (exec_type IN (
        'NEW', 'PARTIAL_FILL', 'FILL', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    status              VARCHAR(20) NOT NULL,
    side                VARCHAR(4) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    price               DECIMAL(18,4),
    quantity            BIGINT NOT NULL,
    last_qty            BIGINT NOT NULL DEFAULT 0,
    last_price          DECIMAL(18,4),
    cum_qty             BIGINT NOT NULL DEFAULT 0,
    leaves_qty          BIGINT NOT NULL DEFAULT 0,
    reject_reason       TEXT,
    account_id          VARCHAR(64) NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE RULE no_update_sec_exec_reports AS ON UPDATE TO securities.execution_reports DO INSTEAD NOTHING;
CREATE RULE no_delete_sec_exec_reports AS ON DELETE TO securities.execution_reports DO INSTEAD NOTHING;

CREATE INDEX idx_sec_exec_order ON securities.execution_reports(order_id);
CREATE INDEX idx_sec_exec_trade ON securities.execution_reports(trade_id) WHERE trade_id IS NOT NULL;
CREATE INDEX idx_sec_exec_created ON securities.execution_reports(created_at);

-- Securities positions (net position per participant per instrument)
CREATE TABLE securities.positions (
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    net_qty             BIGINT NOT NULL DEFAULT 0,
    avg_price           DECIMAL(18,4) NOT NULL DEFAULT 0,
    market_value        DECIMAL(18,4) NOT NULL DEFAULT 0,
    unrealized_pnl      DECIMAL(18,4) NOT NULL DEFAULT 0,
    realized_pnl        DECIMAL(18,4) NOT NULL DEFAULT 0,
    total_buy_qty       BIGINT NOT NULL DEFAULT 0,
    total_sell_qty      BIGINT NOT NULL DEFAULT 0,
    short_qty           BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, instrument_id)
);

CREATE INDEX idx_sec_positions_instrument ON securities.positions(instrument_id);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT SELECT, INSERT ON securities.orders TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON securities.trades TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON securities.execution_reports TO garudax_exchange_svc;
        GRANT UPDATE (filled_qty, remaining_qty, status, updated_at) ON securities.orders TO garudax_exchange_svc;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON securities.positions TO garudax_clearing_svc;
    END IF;
END $$;
