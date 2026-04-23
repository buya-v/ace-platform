// Package ddl provides TimescaleDB continuous aggregate DDL generation.
package ddl

import "fmt"

// GenerateTradesHypertable returns DDL for the trades hypertable.
func GenerateTradesHypertable() string {
	return `CREATE TABLE IF NOT EXISTS ace_market_data.trades (
    trade_id UUID PRIMARY KEY,
    instrument_id UUID NOT NULL,
    price NUMERIC(18,4) NOT NULL,
    quantity BIGINT NOT NULL,
    trade_value NUMERIC(18,4) NOT NULL,
    aggressor_side TEXT NOT NULL CHECK (aggressor_side IN ('BUY', 'SELL')),
    trade_type TEXT NOT NULL DEFAULT 'CONTINUOUS',
    sequence_number BIGINT NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL
);

SELECT create_hypertable('ace_market_data.trades', 'executed_at',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_trades_instrument_seq
    ON ace_market_data.trades (instrument_id, sequence_number DESC);`
}

// GenerateContinuousAggregate returns DDL for a continuous aggregate at the given interval.
func GenerateContinuousAggregate(interval string, bucketWidth string, sourceTable string) string {
	viewName := fmt.Sprintf("ace_market_data.candles_%s", interval)

	return fmt.Sprintf(`CREATE MATERIALIZED VIEW %s
WITH (timescaledb.continuous) AS
SELECT
    instrument_id,
    time_bucket('%s', executed_at) AS bucket,
    FIRST(price, executed_at) AS open,
    MAX(price) AS high,
    MIN(price) AS low,
    LAST(price, executed_at) AS close,
    SUM(quantity) AS volume,
    COUNT(*) AS trade_count,
    SUM(trade_value) / NULLIF(SUM(quantity), 0) AS vwap,
    SUM(trade_value) AS turnover
FROM %s
WHERE trade_type != 'BUST'
GROUP BY instrument_id, time_bucket('%s', executed_at)
WITH NO DATA;

SELECT add_continuous_aggregate_policy('%s',
    start_offset => INTERVAL '%s' * 2,
    end_offset => INTERVAL '1 minute',
    schedule_interval => INTERVAL '%s',
    if_not_exists => TRUE
);`, viewName, bucketWidth, sourceTable, bucketWidth, viewName, bucketWidth, bucketWidth)
}

// GenerateAllAggregates returns DDL for all candle interval continuous aggregates.
// Uses hierarchical aggregation: 1m from trades, 5m/15m from 1m, 1h from 1m, 4h/1d from 1h.
func GenerateAllAggregates() string {
	ddl := GenerateTradesHypertable() + "\n\n"

	// Base: 1-minute from trades
	ddl += "-- 1-minute candles (base aggregate from trades)\n"
	ddl += GenerateContinuousAggregate("1m", "1 minute", "ace_market_data.trades") + "\n\n"

	// 5-minute from 1m
	ddl += "-- 5-minute candles (from 1m)\n"
	ddl += GenerateContinuousAggregate("5m", "5 minutes", "ace_market_data.candles_1m") + "\n\n"

	// 15-minute from 1m
	ddl += "-- 15-minute candles (from 1m)\n"
	ddl += GenerateContinuousAggregate("15m", "15 minutes", "ace_market_data.candles_1m") + "\n\n"

	// 1-hour from 1m
	ddl += "-- 1-hour candles (from 1m)\n"
	ddl += GenerateContinuousAggregate("1h", "1 hour", "ace_market_data.candles_1m") + "\n\n"

	// 4-hour from 1h
	ddl += "-- 4-hour candles (from 1h)\n"
	ddl += GenerateContinuousAggregate("4h", "4 hours", "ace_market_data.candles_1h") + "\n\n"

	// 1-day from 1h
	ddl += "-- 1-day candles (from 1h)\n"
	ddl += GenerateContinuousAggregate("1d", "1 day", "ace_market_data.candles_1h") + "\n"

	return ddl
}

// GenerateRetentionPolicies returns DDL for data retention policies.
func GenerateRetentionPolicies() string {
	return `-- Retention policies
SELECT add_retention_policy('ace_market_data.trades', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('ace_market_data.candles_1m', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('ace_market_data.candles_5m', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('ace_market_data.candles_15m', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('ace_market_data.candles_1h', INTERVAL '2 years', if_not_exists => TRUE);
-- 4h and 1d candles: indefinite retention (no policy)`
}
