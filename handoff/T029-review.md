APPROVED

# Review — T029: Daily Settlement

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation correctly models daily mark-to-market settlement for commodity futures:

- **Variation margin formula** is correct: `(currentPrice - previousPrice) * netQuantity` for existing positions, `(currentPrice - avgEntryPrice) * netQuantity` for new positions. Long positions profit on price increases; short positions profit on price decreases — verified in tests.
- **Net settlement per participant** correctly aggregates P&L across instruments before generating a single PAY_IN or PAY_OUT instruction. Zero-P&L participants are excluded.
- **Zero-sum invariant** is maintained and tested: total pay-in equals total pay-out.
- **Cycle lifecycle** (PENDING → VALUING → CALCULATED → SETTLING → COMPLETED/FAILED) transitions correctly, including failure paths when the payment gateway rejects.
- **Flat position handling** (NetQuantity == 0) returns zero variation margin.
- **Missing price error propagation** is correct — batch calculation fails fast on first missing price.

Minor note: The previous-day price lookup in `valuation.Store.SetSettlementPrice` uses `date.AddDate(0, 0, -1)`, which only finds calendar-adjacent days. Over weekends/holidays this would yield no previous price, falling back to entry price. This is acceptable for the current in-memory implementation but should be addressed when wiring to production data (noted in follow-ups).

### Security: PASS

- No SQL, no user-facing HTTP input parsing beyond `cycle_id` query param (used as map key lookup, not interpolated into queries).
- No secrets or credentials hardcoded.
- Payment gateway is interface-based; the in-memory implementation has no external side effects.
- The `/cycles` HTTP endpoint exposes settlement data without authentication, but this is consistent with the health/status pattern in other engines (matching-engine, clearing-engine, margin-engine) and is appropriate for internal pod-to-pod communication.

### Code Quality: PASS

- **Follows project conventions exactly**: separate Go module with zero external dependencies, identical `Decimal` type, same project layout (`cmd/`, `internal/types/`, `internal/server/`), same config-from-env pattern, same port scheme (50054/8084 follows 50051-53/8081-83).
- **Clean package boundaries**: `valuation` → `pnl` → `settlement` → `payment` → `engine` orchestration. Each package has a single responsibility.
- **Concurrency**: Engine uses `sync.Mutex`, Store uses `sync.RWMutex`, InMemoryGateway uses `sync.Mutex` — all appropriate.
- One minor redundancy: `atomic.AddUint64` in `InMemoryGateway.ProcessPayment` is used inside a mutex lock — the atomic is unnecessary since the mutex already protects the counter. Not a bug, just redundant.

### Test Coverage: PASS

35 tests covering all packages:

- **Valuation (7)**: set/get prices, missing price error, previous price tracking, position valuation for long/short/flat.
- **P&L (8)**: long profit, long loss, short profit, new position (entry price fallback), flat position, missing price error, batch calculation, net-by-participant aggregation.
- **Settlement (6)**: pay-in/pay-out generation, zero-P&L exclusion, multi-instrument netting, cycle ID propagation, totals calculation, empty totals.
- **Payment (5)**: gateway success, gateway failure injection, payment tracking, processor all-success, processor partial failure.
- **Engine (9)**: full cycle success, payment failure → FAILED status, missing price error, multi-instrument cycle, get cycle, get all cycles, cycle handler callback, set settlement price convenience, concurrent cycles (10 goroutines).

Tests verify meaningful behavior (exact P&L values, zero-sum invariant, correct status transitions) rather than just "runs without error."

## Suggestions (non-blocking)

1. **Remove redundant atomic in InMemoryGateway**: The `counter` field in `InMemoryGateway` uses `atomic.AddUint64` but is always accessed under the mutex. Either drop the atomic (use plain `g.counter++`) or drop the mutex for the counter. Low priority since it's a test utility.

2. **DivInt64 silent zero**: `Decimal.DivInt64(0)` returns `DecimalZero()` silently. This matches the other engines but is a latent data-corruption risk if a caller accidentally divides by zero. Consider returning an error or panicking in a future pass.

3. **Weekend/holiday price gaps**: As noted above, the previous-day lookup is calendar-based. When integrating with real market data, consider a "most recent prior settlement price" lookup instead of strict day-1.
