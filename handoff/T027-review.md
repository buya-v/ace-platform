APPROVED

# Review — T027: Clearing Engine

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation correctly covers the core clearing pipeline:

- **Novation** properly splits bilateral trades into two CCP-intermediated obligations with unique IDs, correct sides, and computed values.
- **Position management** correctly implements VWAP for position building, realized P&L on partial/full close, and position flips (long->short and vice versa). The P&L sign is correctly negated for short positions.
- **Netting** correctly aggregates obligations by participant+instrument, tracking gross long/short and net quantities/values.
- **Idempotency** via `processedTrades` map correctly rejects duplicate trade IDs.
- **Concurrency** is handled with a single engine mutex ensuring the novate->store->position pipeline is atomic, and the position manager has its own RWMutex for read-heavy queries. No deadlock risk (consistent lock ordering).

Minor note: `processedTrades` map grows unbounded. Acceptable for in-memory dev use; production will need eviction or a persistent store. Not blocking.

### Security: PASS

- No external user input handling beyond query parameters passed to in-memory lookups (no injection surface).
- No hardcoded secrets or credentials.
- HTTP endpoints are internal health/debug endpoints, not public API surface.
- No authentication on HTTP endpoints (`/positions`, `/netting`), but these are internal pod endpoints and gRPC will be the external API (noted as future work).

### Code Quality: PASS

- Follows matching-engine patterns consistently: separate Go module, zero external dependencies, identical Decimal type, same server/config structure, same port convention scheme.
- Clean separation of concerns across packages: `novation`, `position`, `netting`, `store`, `engine`, `server`, `types`.
- `ObligationStore` interface allows swapping in-memory for PostgreSQL without changing engine code.
- The in-memory store uses index maps (`byTrade`, `byParticipant`, `byInstrument`) for O(1) lookups — appropriate for the access patterns.
- `abs64` is duplicated in both `types/clearing.go` and `position/manager.go` — trivial, not blocking.
- Handoff file is thorough with clear downstream integration points for T028/T029.

### Test Coverage: PASS

42 tests across 5 test files covering:

- **Novation (5 tests):** happy path, validation (empty trade ID, zero quantity, missing participants), instrument preservation.
- **Position (12 tests):** new long/short, add to position with VWAP, partial close with P&L, full close to flat, position flip, short position P&L, lookups, validation.
- **Netting (9 tests):** single obligation, offsetting, full offset, multiple participants, multiple instruments, empty input, net value calculation, efficiency calculation, zero gross edge case.
- **Store (6 tests):** append/len, query by trade/participant/instrument/status, all.
- **Engine (10 tests):** basic clearing, idempotency, multi-trade position updates, trade handler callback, obligation retrieval, position queries, netting integration, concurrent clearing (100 goroutines).

Tests assert meaningful business behavior (VWAP values, P&L amounts, net quantities) rather than just "no error."

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Atomicity gap in `ClearTrade`**: If the second `oblStore.Append` (seller) fails after the first (buyer) succeeds, the system has a partial state. Consider a batch append method on `ObligationStore` when moving to PostgreSQL (transaction boundary).
2. **Unbounded `processedTrades` map**: For production, consider an LRU cache or moving idempotency checks to the persistent store layer.
3. **`abs64` duplication**: Minor — exists in both `types/clearing.go` and `position/manager.go`. Could be consolidated but not worth a separate change.
4. **`main.go` unused env var**: Line `_ = os.Getenv("MATCHING_ENGINE_ADDR")` is a no-op placeholder. Fine as documentation of intent, but a comment would be clearer than a discarded assignment.
