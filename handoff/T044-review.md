APPROVED

# Review — T044: Margin Engine Test Coverage Improvement

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The 30 new tests correctly exercise the engine and scanner APIs. Key observations:

- **Engine tests** cover all previously-untested public methods: `SetMarginHandler`, `GetActiveMarginCall`, `GetAllActiveMarginCalls`, `GetMarginCallStats`, `GetParamStore`. Each test verifies meaningful state transitions (issue -> resolve lifecycle, deficit amounts, handler invocation counts).
- **Scanner tests** use table-driven patterns to validate scan risk across flat/long/short/large positions, verify scenario PnL counts match, confirm worst-scenario index correctness, and check linear risk scaling. MtM tests verify exact computed values, not just sign.
- The `TestCalculateMarginEmptyPositions` test at line 161 has a no-op `if` block (`if pm.TotalRequired.IsZero() == false { }`) — harmless but dead code. Non-blocking.
- Concurrency test (`TestConcurrentReadWrite`) exercises concurrent readers and writers, which is appropriate given the `sync.Mutex` in the engine.

### Security: PASS

Test-only changes — no new API surfaces, no input handling, no secrets. No security concerns.

### Code Quality: PASS

- Tests follow existing conventions: same package, same helper reuse (`setupEngine`, `newTestCollateral`, `testIDGen`, `cornParams`).
- New `wheatDeliveryParams()` helper in scanner tests mirrors the existing `cornParams()` pattern.
- Table-driven tests are well-structured with clear names.
- The `TestMarginCallDeficitAmount` test creates its own engine setup (custom `TEST-INST` params) rather than reusing `setupEngine` — this is appropriate since it needs specific params to verify exact deficit math.

### Test Coverage: PASS

- Engine coverage reported at 96.7% (up from 76.7%). The remaining gap (`callService.Evaluate()` error path) is unreachable without mocking — acceptable.
- Scanner was already at 95.2% and tests add redundant coverage via table-driven variants, which improves confidence without inflating numbers artificially.
- Tests verify meaningful behavior: exact deficit amounts, handler call counts, lifecycle transitions, linear scaling properties — not just "runs without error."

## Required Fixes

None.

## Suggestions (non-blocking)

1. `engine_coverage_test.go:168` — The empty `if` body (`if pm.TotalRequired.IsZero() == false { }`) should either assert something or be removed.
2. The `TestConcurrentReadWrite` test doesn't check for errors from `CalculateMargin` — adding error checks would catch any future regression in the mutex logic.
