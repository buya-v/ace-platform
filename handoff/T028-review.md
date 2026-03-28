# Review — T028: SPAN Margin Calculation

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The SPAN scanning algorithm is correctly implemented: 16 scenarios (14 standard at 1/3, 2/3, 3/3 price scan range x vol up/down, plus 2 extreme at 30% weight), worst-case loss selection, and mark-to-market P&L calculation. The margin formula (initialMargin = scanRisk + deliveryCharge, totalRequired = initialMargin - MtM floored at zero) is sound. Margin call lifecycle (issue, update on changed deficit, resolve on surplus, breach on deadline) is correct.

The delivery month charge formula (`rate * spotPrice * contractSize * |qty|`) produces correct results as verified by the test (`0.05 * 600 * 5000 * 5 = 750,000`).

Minor: the `worstLoss.IsNeg()` guard at `scanner.go:61` is dead code since `worstLoss` starts at zero and only increases, but it's harmless.

### Security: PASS

No SQL, no command execution, no secrets hardcoded. HTTP endpoints use query params only as in-memory map keys — no injection surface. The `/margin` and `/margin-calls` endpoints are read-only. No authentication is present, but this is consistent with the other services at this stage (matching-engine, clearing-engine) which also have unauthenticated health/query endpoints. The `CollateralSource` interface correctly decouples from any real treasury system.

The `MulUint64` has a theoretical overflow risk on very large `uint64` values cast to `int64`, but this matches the identical pattern in `matching-engine/internal/types/decimal.go:129-131` and is acceptable for the value ranges in commodity trading.

### Code Quality: PASS

Follows project conventions exactly:
- Separate Go module with `go.mod`, zero external dependencies
- Same `internal/types/decimal.go` pattern as matching-engine (compatible API surface)
- Same directory structure (`cmd/`, `internal/{types,engine,server}/`)
- Port allocation avoids collisions (50053/8083 after 50051/8081 and 50052/8082)
- Clean separation: types → params → scanner → calculator → margin call service → engine → server
- `CollateralSource` and `PositionSource` as interfaces for downstream integration

The Decimal type extends the matching-engine version with needed methods (`Add`, `MulDecimal`, `MulInt64`, `Negate`, `IsNeg`, `Max`, `DivInt64`) while maintaining the same internal representation — good for cross-service compatibility.

### Test Coverage: PASS

40 tests across 4 packages covering:
- **params** (7): scenario count, symmetry, store CRUD, spot price update, missing instrument error
- **scanner** (8): flat/long/short positions, scan risk symmetry, MtM profit/loss/flat/short-profit
- **calculator** (8): single position, delivery month charge, missing instrument, portfolio with multiple instruments, flat position skip, excess/deficit
- **engine** (10): basic calculation, margin call trigger, no-call with sufficient collateral, portfolio cache hit/miss, spot price update, deadline checking, 20-goroutine concurrency, multi-instrument portfolio

Tests verify specific numeric outcomes (e.g., scan risk = $150,000 for 10 long corn contracts at $3 scan range x 5000 bushels) rather than just asserting non-zero. Edge cases (flat positions, zero collateral, missing instruments, deadline boundaries) are covered.

## Suggestions (non-blocking)

1. **Dead code**: `scanner.go:61` — the `if worstLoss.IsNeg()` check can never be true since `worstLoss` is initialized to zero and only set to higher values. Can be removed for clarity.

2. **`DivInt64` silent zero-on-divide-by-zero**: `decimal.go:108-111` returns `DecimalZero()` on division by zero rather than panicking or returning an error. This matches the "don't crash" philosophy but could mask bugs. Consider at minimum a comment noting this is intentional.

3. **Overflow documentation**: Large positions with large contract sizes could theoretically overflow `int64` in `MulInt64` chains (e.g., a scan risk calculation with price_move * contract_size * quantity all as large values). For production, consider adding overflow checks or documenting the safe value ranges. This matches the existing matching-engine limitation.
