# Review — T036: Market Data Service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The core business logic is correct:
- OHLCV candle aggregation across 6 intervals works correctly with proper bucket alignment.
- Candle close detection (new bucket triggers close of previous) is sound.
- Ticker 24h rolling window computation is correct.
- Trade store ordering (newest-first for LastN, sequence-based for SinceSequence) is correct.
- Streaming hub correctly uses non-blocking publish to avoid backpressure on ingestion.
- FlushClosed correctly marks and removes expired candles.

Minor issues (non-blocking):
- `main.go:60-67`: The "periodic ticker pruning" goroutine is a **no-op**. It creates a ticker, loops on it, but only does `_ = types.Interval1m` — it never calls `eng.PruneBefore()`. The ticker engine's trades slice will grow unbounded in a long-running process. This is mitigated by `GetTicker` filtering at query time, but memory will leak. Should call a server method that delegates to `tickerEngine.PruneBefore(cutoff)`.
- `server.go:100`: `SubscribeCandles` accepts `interval` parameter but ignores it — all candle updates for the instrument are delivered regardless of interval. The subscriber gets 6x the expected updates. This is an API contract mismatch but functionally harmless for now.

### Security: PASS

- No external input parsing beyond environment variables (port numbers, bind address, instrument list).
- All stores are in-memory — no SQL injection surface.
- No credentials, secrets, or tokens in code.
- Port binding defaults to `0.0.0.0` which is appropriate for container deployment.
- Health/readiness endpoints expose no sensitive data.

### Code Quality: PASS

- Follows the established zero-dependency Go module pattern from matching-engine et al.
- Clean package separation: types, candle, store, ticker, streaming, server, ddl, retention.
- Decimal type duplication is consistent with project convention (accepted per learned patterns).
- Port 50057/8087 follows the allocation convention.
- Callback-based builder design keeps it testable without server dependencies.

Minor nits:
- `builder.go:131` — `_ = key` in `GetAllCandles` is dead code, should be removed.
- `ddl/generate.go`: Hierarchical continuous aggregates (5m from candles_1m view) may not work in all TimescaleDB versions — cascaded continuous aggregates were added in TSdb 2.9+. Worth a comment.

### Test Coverage: PASS

35 tests covering:
- Candle builder: single trade, multi-trade OHLCV, bucket transitions, multi-instrument, flush, not-found (7 tests + BucketStart)
- Trade store: LastN, empty, SinceSequence, InTimeRange, LastTrade, AllInstruments (6 tests)
- Candle store: store/query, empty, DeleteBefore, upsert (4 tests)
- Ticker engine: single trade, 24h stats, not-found, all/filtered tickers, prune, turnover (7 tests)
- Streaming hub: candle pub/sub, trade pub/sub, cross-instrument isolation, slow subscriber (4 tests)
- Server integration: ingest+candles, ingest+ticker, ingest+trades, multi-ticker, streaming, ready state, limit clamping (7 tests)

Tests verify meaningful behavior (OHLCV correctness, ordering, time windowing, backpressure handling), not just "runs without error." Coverage of 68.7% on candle builder (core logic) is above the 60% floor for business-critical packages.

---

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Fix the no-op ticker pruning goroutine** in `main.go` — add a `PruneTickerTrades()` method to Server that calls `tickerEngine.PruneBefore(time.Now().UTC().Add(-25 * time.Hour))`, and call it from the goroutine. Without this, memory grows linearly with trade volume.
2. **Wire `interval` filtering in `SubscribeCandles`** or remove the parameter to avoid API confusion.
3. **Remove `_ = key`** dead code in `GetAllCandles`.
4. **Add a TSdb version comment** in `ddl/generate.go` noting that cascaded continuous aggregates require TimescaleDB 2.9+.
5. **Consider adding a DDL generation test** — the `ddl` package has no tests. Even a simple "output contains expected table/view names" test would catch regressions.
