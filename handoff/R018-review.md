APPROVED

# Review — R018: corporate-actions float→decimal

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The migration is mechanically and numerically correct. I verified the diff against the
actual shared module (`src/shared/pkg/types/decimal/decimal.go`) rather than trusting the
handoff:

- **Every decimal method the diff calls exists with the expected signature:** `MulInt64(int64)`,
  `DivInt64(int64)`, `IsNeg()`, `IsZero()`, `Equal()`, `Add()`, `MustParse(string)`,
  `DecimalFromInt(int64)`, `Float64()`, `Zero()`, and `NewFromFloat(float64) (Decimal, error)`.
  The handler correctly consumes the two-value `NewFromFloat` return.
- **Type alias wiring is present.** `src/securities-service/internal/types/types.go:9` already
  declares `type Decimal = decimal.Decimal`, so `EntitlementValue Decimal` compiles. Both
  consumer `go.mod`s carry `require ... decimal v0.0.0` + relative `replace => ../shared/pkg/types/decimal`
  (corporate-actions adds it; securities-service had it from R004). The relative path resolves.
- **Money math spot-checks all hold:**
  - Dividend: `0.1250 × 333 = 41.6250` exact (the old `round2` lossily collapsed this to 41.63 —
    the new 4dp value is strictly more correct).
  - SplitAdjustedPrice = `price × Old / New`: 100@2:1→50, 5@1:10→50, 30@3:2→20. Algebraically
    identical to the old `price / ratio` but without float division.
  - SplitAdjustedQuantity remainder: 105@1:10 → 10 + 5/10 = 0.5; 101@3:2 → 151 + 1/2 = 0.5. Exact.
  - Rights floor via integer `held×New/Old`: 100@1:5→20, 100@1:3→33. Cost = price × rights.
  - TERP `(Old·cum + New·sub)/(Old+New)`: 58/6 → 9.6667 (half-even at 4dp). Correct.
- **Divide/overflow safety:** every `DivInt64`/`MulInt64` (which panic on divide-by-zero/overflow)
  is reached only after the `Ratio()`/`NewShares>0`/`OldShares>0` guards, so the panic paths are
  unreachable for valid terms. Integer share-count paths use exact `int64` arithmetic.
- **Sentinel-error behavior preserved:** negative-amount, wrong-type, invalid-ratio, missing-tenant
  and missing-instrument checks are all retained and still compared with `errors.Is` in tests.

### Security: PASS

- No SQL/command/template construction, no new external input parsing, no secrets.
- **Tenant invariant upheld.** `validate()` still rejects empty `TenantID`, and `eligible()` still
  requires `p.TenantID == ca.TenantID` before any holder is acted on. No tenant context was removed
  or bypassed — consistent with the 2026-04-23 multi-tenancy directive.
- Pure-function domain code with no auth surface; nothing to bypass.

### Code Quality: PASS

- Follows the validated R003/R004 migration recipe exactly: re-exported `type Decimal = decimal.Decimal`
  alias so domain structs declare `Decimal` without every caller importing the shared module; relative
  filesystem `replace`; kept `go 1.22` on the corporate-actions module so it builds with the host
  toolchain and stays zero-`go.sum`/zero-network.
- Dead code removed cleanly (`round2`, `math` import). Doc comments updated to describe the
  half-even/no-overflow/no-divide-by-zero guarantees.
- Converting integer-result paths (split share counts, rights counts) to exact integer arithmetic
  rather than Decimal is the right call — no spurious precision, no float floor.
- Retaining `SplitTerms.Ratio() float64` purely for the invalid-ratio validation check is reasonable;
  it carries no money.

### Test Coverage: PASS

- All call sites in `engine_test.go` and `process_test.go` migrated to `Decimal` fixtures via a
  `dec()`/`MustParse` helper, with assertions using `.Equal`/`.IsZero` (meaningful, not "runs without error").
- Precision-delta cases are explicit and documented (`TestCalculateDividend_Precision` 41.6250,
  TERP 9.6667), so the intentional behavioral change from lossy 2dp to exact 4dp is locked in by tests.
- securities-service tests updated at all three layers (handler, store, fixture constructor) including
  the total-value aggregation (`1500`) recomputed in the Decimal domain.
- Error paths, skip-ineligible, never-nil, and market-value-preservation invariants remain covered.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Swallowed error in the handler:** `dividendAmount, _ = decimal.NewFromFloat(val)` discards the
   `NaN/Inf` error, silently yielding a zero dividend. In practice `encoding/json` cannot produce
   NaN/Inf so this is unreachable from real input, but consider handling it (or asserting it can't
   occur) for defensiveness.
2. **Custody integer delta:** `s.custodyBalanceStore.GetOrUpdate(..., int(entitlementValue.Float64()), 0)`
   truncates sub-unit cash and round-trips through float. This is pre-existing behavior (was
   `int(float64)`), and the authoritative stored `EntitlementValue` remains exact Decimal, so there is
   no regression — but flag it for the eventual settlement-boundary cents-rounding work.
3. **Negative-quantity split behavior** changed from floor-toward-−∞ to integer truncation-toward-zero.
   Unreachable for eligible (non-negative) holdings, so harmless, but worth a one-line doc note.
4. **gofmt hygiene:** the handoff notes securities-service is repo-wide not gofmt-clean under the host
   toolchain. The edited files add no new violations; the broader cleanup is correctly deferred to an
   R017-class task.
5. The R006 residuals (compliance MCSD/FRC, gateway fees + pre-trade risk, fix-gateway price tags)
   remain on `float64` money and are still tracked in the deferred backlog — out of scope here.

---

**Process note:** A worker handoff (`handoff/R018.md`) is present on the branch and is thorough
(decisions, behavioral deltas, deferred residuals, deliverables), satisfying the handoff requirement
even though it was not visible in the pre-merge working tree.
