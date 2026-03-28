APPROVED

# Review — T035: Market Data Service Architecture Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec is comprehensive and well-aligned with the task requirements:

- OHLCV candle aggregation for 6 intervals (1m, 5m, 15m, 1h, 4h, 1d) is fully specified with TimescaleDB continuous aggregates and hierarchical rollups.
- gRPC API covers all required RPCs: GetCandles, StreamCandles, GetTicker, GetTickers, GetTrades, StreamTrades.
- The protobuf file is consistent with the spec document. Stream RPCs correctly use `oneof` wrappers (`CandleUpdate`, `TradeStreamMessage`) to multiplex data and heartbeats — an improvement over the spec doc which showed `stream Candle` directly.
- The service name `MarketDataAggregateService` correctly avoids collision with the matching engine's `MarketDataService`.
- VWAP computation in the continuous aggregates (`sum(trade_value) / sum(quantity)`) is correct only if `trade_value = price * quantity` (i.e., lot_size = 1 or already factored in). The spec's data model section says `trade_value = price * quantity * lot_size`, so VWAP = `sum(price * qty * lot_size) / sum(qty)` which is not truly VWAP unless divided by `sum(qty * lot_size)`. This is a minor inconsistency but acceptable for a spec — the implementation task should clarify.
- Port assignment (50057/8087) follows the established convention.
- Gateway integration section provides clear REST and WebSocket route mappings.
- Retention policies are well-tiered (90d raw, 1y minute candles, indefinite hourly+).
- The SQL migration correctly uses `WITH NO DATA` for all continuous aggregates, which is the right approach for production.
- Trade bust/correction handling via WHERE clause exclusion in continuous aggregates is sound.

Minor note: The spec doc's SQL creates the role with bare `CREATE ROLE` while the migration file uses the safer `DO $$ IF NOT EXISTS` pattern — the migration is correct.

### Security: PASS

- No credentials or secrets hardcoded. Database DSN is sourced from Kubernetes secrets.
- The service role `ace_marketdata_svc` follows least-privilege: SELECT+INSERT on trades, SELECT-only on aggregates.
- Append-only rules (no UPDATE/DELETE) on the trades table protect data integrity.
- All streaming endpoints are marked as public (read-only market data), which is appropriate for an exchange.
- No SQL injection risk — the migration is declarative DDL, and the protobuf contract uses typed fields.
- The `since_sequence` replay mechanism could theoretically be used to scrape historical data, but since the endpoints are public anyway this is by design.

### Code Quality: PASS

- The spec document is well-structured with 12 clearly delineated sections.
- The protobuf file follows proto3 conventions: proper package naming, `go_package` option, zero-value `UNSPECIFIED` enum entries, detailed field comments.
- SQL migration follows the existing project pattern (schema-qualified names, CHECK constraints, explicit index names).
- The hierarchical continuous aggregate chain (trades -> 1m -> 5m/15m/1h -> 4h/1d) is cleanly organized.
- Handoff file includes clear follow-up items with specific references to upstream tasks and artifacts.
- Deployment YAML and configuration table are complete and production-ready.

### Test Coverage: PASS (N/A)

This is an architecture spec task, not an implementation task. There is no executable code to test. The spec provides sufficient detail for the implementation task to write tests against (protobuf contracts, SQL schema, expected behavior for each RPC, performance targets).

## Required Fixes

None.

## Suggestions (non-blocking)

1. **VWAP formula clarity**: The spec defines `trade_value = price * quantity * lot_size` but the SQL computes VWAP as `sum(trade_value) / sum(quantity)`. If lot_size != 1, this gives `price * lot_size`, not true VWAP. The implementation task should either normalize `trade_value` to exclude lot_size or divide by `sum(quantity * lot_size)`. Recommend adding a note in the spec.

2. **Compression policy**: Section 8 mentions compression can be added later. Consider specifying the compression policy in the spec so the implementation task includes it from the start (e.g., `ALTER TABLE market_data.trades SET (timescaledb.compress, timescaledb.compress_segmentby = 'instrument_id', timescaledb.compress_orderby = 'executed_at DESC')`).

3. **StreamCandles return type**: The spec doc section 5.1 shows `returns (stream Candle)` but the actual proto file correctly uses `returns (stream CandleUpdate)` with a `oneof` wrapper. The spec doc should be updated to match the proto for consistency.

4. **Sequence number uniqueness**: The trades table has no unique constraint on `(instrument_id, sequence_number)`. If the matching engine guarantees global sequence uniqueness, an index exists (`idx_trades_instrument_seq`) but it's not unique. Consider whether a unique constraint would be appropriate for gap detection.

5. **Kafka topic naming**: The spec uses `exchange.trades.{instrument_id}` (UUID in topic name). Consider whether a more human-readable topic naming scheme would be preferable for operational tooling (e.g., `exchange.trades.WHT-HRW-2026M07`).
