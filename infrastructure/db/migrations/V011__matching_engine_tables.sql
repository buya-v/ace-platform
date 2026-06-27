-- V11: Matching engine persistence tables (T103)
-- Adds: exchange.orders, exchange.trades, exchange.matching_execution_reports
-- The order book stays in-memory; these tables persist trade history and execution reports.

-- Orders table — persists order state for audit and recovery
CREATE TABLE IF NOT EXISTS exchange.orders (
    id              VARCHAR(64) PRIMARY KEY,
    instrument_id   VARCHAR(64) NOT NULL,
    account_id      VARCHAR(64) NOT NULL,
    side            VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    order_type      VARCHAR(20) NOT NULL CHECK (order_type IN ('LIMIT', 'MARKET', 'STOP_LIMIT', 'STOP_MARKET')),
    tif             VARCHAR(10) NOT NULL CHECK (tif IN ('DAY', 'GTC', 'GTD', 'IOC', 'FOK')),
    price           DECIMAL(18,4),
    quantity        DECIMAL(18,4) NOT NULL,
    filled_qty      DECIMAL(18,4) DEFAULT 0,
    status          VARCHAR(20) NOT NULL CHECK (status IN (
        'NEW', 'PARTIALLY_FILLED', 'FILLED', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_instrument_id ON exchange.orders(instrument_id);
CREATE INDEX IF NOT EXISTS idx_orders_account_id ON exchange.orders(account_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON exchange.orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON exchange.orders(created_at);

-- Trades table — append-only record of matched trades
CREATE TABLE IF NOT EXISTS exchange.trades (
    id              VARCHAR(64) PRIMARY KEY,
    instrument_id   VARCHAR(64) NOT NULL,
    buy_order_id    VARCHAR(64),
    sell_order_id   VARCHAR(64),
    price           DECIMAL(18,4) NOT NULL,
    quantity        DECIMAL(18,4) NOT NULL,
    buyer_id        VARCHAR(64),
    seller_id       VARCHAR(64),
    aggressor_side  VARCHAR(4) CHECK (aggressor_side IN ('BUY', 'SELL')),
    traded_at       TIMESTAMPTZ DEFAULT NOW()
);

-- Block DELETE/UPDATE on trades (append-only)
CREATE RULE no_update_trades AS ON UPDATE TO exchange.trades
    DO INSTEAD NOTHING;
CREATE RULE no_delete_trades AS ON DELETE TO exchange.trades
    DO INSTEAD NOTHING;

CREATE INDEX IF NOT EXISTS idx_trades_instrument_id ON exchange.trades(instrument_id);
CREATE INDEX IF NOT EXISTS idx_trades_traded_at ON exchange.trades(traded_at);
CREATE INDEX IF NOT EXISTS idx_trades_buy_order_id ON exchange.trades(buy_order_id);
CREATE INDEX IF NOT EXISTS idx_trades_sell_order_id ON exchange.trades(sell_order_id);
CREATE INDEX IF NOT EXISTS idx_trades_buyer_id ON exchange.trades(buyer_id);
CREATE INDEX IF NOT EXISTS idx_trades_seller_id ON exchange.trades(seller_id);

-- Matching execution reports — one per order state change from matching engine
CREATE TABLE IF NOT EXISTS exchange.matching_execution_reports (
    id              VARCHAR(64) PRIMARY KEY,
    order_id        VARCHAR(64) NOT NULL,
    trade_id        VARCHAR(64),
    exec_type       VARCHAR(20) NOT NULL CHECK (exec_type IN (
        'NEW', 'PARTIAL_FILL', 'FILL', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    status          VARCHAR(20) NOT NULL,
    price           DECIMAL(18,4),
    quantity        DECIMAL(18,4),
    leaves_qty      DECIMAL(18,4),
    cum_qty         DECIMAL(18,4),
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Block DELETE/UPDATE on execution reports (append-only)
CREATE RULE no_update_matching_exec_reports AS ON UPDATE TO exchange.matching_execution_reports
    DO INSTEAD NOTHING;
CREATE RULE no_delete_matching_exec_reports AS ON DELETE TO exchange.matching_execution_reports
    DO INSTEAD NOTHING;

CREATE INDEX IF NOT EXISTS idx_matching_exec_reports_order_id ON exchange.matching_execution_reports(order_id);
CREATE INDEX IF NOT EXISTS idx_matching_exec_reports_trade_id ON exchange.matching_execution_reports(trade_id) WHERE trade_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_matching_exec_reports_created_at ON exchange.matching_execution_reports(created_at);

-- Grant access to exchange service role (if role exists)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT SELECT, INSERT ON exchange.orders TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON exchange.trades TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON exchange.matching_execution_reports TO garudax_exchange_svc;
        GRANT UPDATE (filled_qty, status, updated_at) ON exchange.orders TO garudax_exchange_svc;
    END IF;
END $$;
