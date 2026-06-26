APPROVED

# Review — R021: fix-gateway price mapper → decimal

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The task (R006 audit residual: `mapper.go:39,40,121,204`, `parser.go:223`) is fully addressed.

- `InternalOrder.Price` / `StopPrice`, `MapExecutionReport(price ...)`, and `formatPrice` are migrated from `float64` to the shared `decimal.Decimal`. `GetFloatTag` → `GetDecimalTag`, parsing FIX tags (strings on the wire) via `decimal.ParseDecimal` instead of `strconv.ParseFloat`. This eliminates the binary-float drift the audit flagged.
- **Semantics preserved:** `GetDecimalTag` returns `decimal.Zero()` for absent/invalid tags, matching the old `GetFloatTag` "0 on missing/invalid" contract that `MapNewOrderSingle` relies on. Verified `decimal.Zero()`, `IsZero()`, `Equal()`, `MustParse()`, `ParseDecimal()`, `String()` all exist in `src/shared/pkg/types/decimal/decimal.go`.
- **Wire compatibility verified.** The only non-test consumer of `order.Price`/`StopPrice` is `internal/server/tcp.go:272-273`, which places them into a `map[string]interface{}` that `OrderRouter.SubmitOrder` serializes via `json.Marshal`. `Decimal.MarshalJSON` emits a bare JSON number (e.g. `275.5`, `0`), shape-identical to the previous `float64`, so the securities-service payload needs no change and the untouched file still compiles.
- **AvgPx format delta is contained.** `formatPrice` now returns `String()` (trailing-zero-trimmed, `"275.5"` not `"275.5000"`) — both are spec-valid FIX price strings. This only flows through `MapExecutionReport`, which is test-only today; production `sendExecutionReport` (`tcp.go:477`) builds `TagAvgPx` from the literal `"0.0000"` and was correctly left untouched (no float math).
- No dangling references: every `GetFloatTag` call site in code (mapper.go, parser.go, parser_test.go) is converted in the diff. Remaining matches are only in docs/task data (`handoff/R006.md`, `tasks.json`).

### Security: PASS

No injection, secret, or authz surface in this change. Input validation at the FIX boundary is preserved: malformed/absent price tags coerce to zero exactly as before. This silent-zero coercion is pre-existing behavior, deliberately retained to keep order-rejection semantics unchanged within this scoped task, and is flagged for follow-up — not a regression. The 4-dp clamp and overflow-to-error behavior of `ParseDecimal` are stricter (safer) than the prior unbounded float.

### Code Quality: PASS

- Follows the two validated repo patterns precisely: shared-decimal superset migration and filesystem `replace github.com/garudax-platform/decimal => ../shared/pkg/types/decimal` (zero-dep, no `go.sum`). Correctly notes fix-gateway has no `internal/types` package, so it imports `decimal` directly with no alias shim — appropriate.
- Clear doc comments on `GetDecimalTag` and `formatPrice` explaining the no-float-drift rationale. Naming consistent with the codebase.
- Incidental gofmt realignment of the tag-constant block is harmless hygiene and does not obscure the substantive change.

### Test Coverage: PASS

- Existing assertions updated from float comparisons to `Decimal.Equal`/`IsZero` across `mapper_test.go` and `parser_test.go`.
- New `decimal_roundtrip_test.go` adds two regression tests. `TestPriceRoundTripBeatsFloat64` is the meaningful one: it directly contrasts the Decimal path against the old `ParseFloat→FormatFloat` path on `9007199254740.9931`, proving the float path drifts (to `...9922`) while Decimal preserves the value — locking in the fix.
- Reported 95.3% statement coverage on `internal/fix/`; build/vet/gofmt clean.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Strengthen the outbound assertion.** In `TestPriceRoundTripNoFloatDrift`, the outbound check (`got == want.String()`) is tautological because both sides derive from `ParseDecimal(in).String()`. Asserting each case against a hard-coded expected literal (e.g. `"275.5"`, `"0.0001"`) would make it a stronger guard against a future `String()`/`formatPrice` regression.
2. **Consider error-returning `GetDecimalTag`.** As the worker noted, silently coercing a malformed price tag to zero could let a garbled limit order through as a market-priced order. Returning `(Decimal, error)` and rejecting the order on parse failure would be safer — but this changes rejection semantics and is correctly deferred out of this scoped task.
3. **AvgPx fixed-scale variant.** If any FIX counterparty requires fixed 4-dp `AvgPx`, add a zero-padding `formatPrice` variant before `MapExecutionReport` is wired into production. Non-blocking while it remains test-only.
