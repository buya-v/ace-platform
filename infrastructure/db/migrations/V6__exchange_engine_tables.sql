-- V6: Exchange engine tables (T007)
-- Adds: instruments, execution_reports, circuit_breaker_events
-- These tables support the CLOB matching engine spec.

-- Tradeable instrument definitions (commodity + delivery month + location)
CREATE TABLE exchange.instruments (
    instrument_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol              VARCHAR(30) NOT NULL UNIQUE,
    commodity_id        UUID NOT NULL,
    delivery_location_id UUID NOT NULL,
    delivery_month      DATE NOT NULL,
    lot_size            NUMERIC(18,4) NOT NULL,
    tick_size           NUMERIC(18,4) NOT NULL,
    price_band_pct      NUMERIC(5,2) NOT NULL DEFAULT 20.00,
    max_order_qty       BIGINT NOT NULL DEFAULT 1000,
    max_order_value     NUMERIC(18,4) NOT NULL DEFAULT 500000000,
    status              VARCHAR(10) NOT NULL DEFAULT 'ACTIVE'
                        CHECK (status IN ('ACTIVE','SUSPENDED','EXPIRED','DELISTED')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_instrument_contract
        UNIQUE (commodity_id, delivery_location_id, delivery_month)
);

CREATE INDEX idx_instruments_commodity ON exchange.instruments(commodity_id);
CREATE INDEX idx_instruments_status ON exchange.instruments(status);

-- Execution reports — one per order state change (append-only)
CREATE TABLE exchange.execution_reports (
    exec_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id            UUID NOT NULL,
    client_order_id     VARCHAR(64),
    exec_type           VARCHAR(20) NOT NULL
                        CHECK (exec_type IN (
                            'NEW','PARTIAL_FILL','FILL','CANCELLED',
                            'REJECTED','EXPIRED','TRADE_BUST'
                        )),
    order_status        VARCHAR(20) NOT NULL,
    side                VARCHAR(4) NOT NULL CHECK (side IN ('BUY','SELL')),
    instrument_id       UUID NOT NULL,
    price               NUMERIC(18,4),
    quantity            BIGINT NOT NULL,
    last_qty            BIGINT NOT NULL DEFAULT 0,
    last_price          NUMERIC(18,4),
    cumulative_qty      BIGINT NOT NULL DEFAULT 0,
    leaves_qty          BIGINT NOT NULL DEFAULT 0,
    trade_id            UUID,
    transact_time       TIMESTAMPTZ NOT NULL DEFAULT now(),
    reject_reason       TEXT,
    account_id          UUID NOT NULL
);

-- Block DELETE/UPDATE on execution_reports (same pattern as trades in T004)
CREATE RULE no_update_execution_reports AS ON UPDATE TO exchange.execution_reports
    DO INSTEAD NOTHING;
CREATE RULE no_delete_execution_reports AS ON DELETE TO exchange.execution_reports
    DO INSTEAD NOTHING;

CREATE INDEX idx_exec_reports_order ON exchange.execution_reports(order_id);
CREATE INDEX idx_exec_reports_account ON exchange.execution_reports(account_id);
CREATE INDEX idx_exec_reports_time ON exchange.execution_reports(transact_time);
CREATE INDEX idx_exec_reports_trade ON exchange.execution_reports(trade_id) WHERE trade_id IS NOT NULL;

-- Circuit breaker event log
CREATE TABLE exchange.circuit_breaker_events (
    event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrument_id       UUID NOT NULL REFERENCES exchange.instruments(instrument_id),
    trigger_price       NUMERIC(18,4) NOT NULL,
    reference_price     NUMERIC(18,4) NOT NULL,
    deviation_pct       NUMERIC(5,2) NOT NULL,
    halt_level          SMALLINT NOT NULL CHECK (halt_level IN (1, 2, 3)),
    halted_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    resumed_at          TIMESTAMPTZ,
    auction_price       NUMERIC(18,4),
    reason              TEXT
);

CREATE INDEX idx_cb_events_instrument ON exchange.circuit_breaker_events(instrument_id);
CREATE INDEX idx_cb_events_time ON exchange.circuit_breaker_events(halted_at);

-- Grant access to exchange service role
GRANT SELECT, INSERT ON exchange.instruments TO garudax_exchange_svc;
GRANT SELECT, INSERT ON exchange.execution_reports TO garudax_exchange_svc;
GRANT SELECT, INSERT, UPDATE ON exchange.circuit_breaker_events TO garudax_exchange_svc;
