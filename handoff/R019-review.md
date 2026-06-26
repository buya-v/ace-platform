APPROVED

# Review — R019: compliance-service MCSD/FRC money→decimal

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The change converts the residual `float64` money paths in compliance-service's MCSD
integration and FRC reporting packages to the shared
`github.com/garudax-platform/decimal` fixed-point type. I verified every new API call
against `src/shared/pkg/types/decimal/decimal.go`:

- `decimal.Zero()` (line 124), `decimal.DecimalFromInt` (112), `decimal.MustParse`
  (202), `decimal.NewFromFloat` returning `(Decimal, error)` (70), `TryMulInt64` (274),
  `IsPos` (217), `Equal` (234), `Float64` (63), `Add` (262) — all exist with the
  signatures the diff uses. No phantom methods.
- **Validation equivalence:** `req.SettlementAmount <= 0` → `!req.SettlementAmount.IsPos()`.
  `IsPos()` is `value > 0`, so the negation is `value <= 0` — exactly equivalent. ✓
- **Dividend math:** `round2(float64(qty) * RatioOrAmount)` →
  `NewFromFloat(RatioOrAmount).TryMulInt64(qty)`. Since `TryMulInt64` scales the
  fixed-point value by the integer share count, `2.50 (=25000 raw) × 1000 = 25,000,000
  raw = 2500.0000`, matching the test's expected 2500/1250. Computing in decimal before
  any rounding is strictly more precise than the old float×float→round2 path. ✓
- **FRC totals:** `TotalValue`/`TotalPenalty` are now exact `Decimal` sums; the old
  post-hoc `round2(TotalPenalty)` is correctly removed (the test `12.34 + 7.66 = 20`
  is a genuine precision edge case and passes exactly). ✓
- **Non-money floats correctly preserved:** `PriceChange`, `PercentOutstanding`,
  `RatioOrAmount` (dual-purpose: cash for DIVIDEND, share ratio for splits), and
  `ShareEntitlement` (int) are all left as their original types per the R006 audit
  scope. `PercentOutstanding`'s `round2` was replaced with `math.Round(pct*100)/100`,
  which preserves the prior half-away-from-zero 2-dp behaviour. ✓
- **No compile breaks:** A full-service grep for every touched field
  (`SettlementAmount`, `CashEntitlement`, `PenaltyAmount`, `TotalValue`, `FailValue`,
  `InstrumentVolume.Value`, `TotalPenalty`) confirms the only arithmetic sites are the
  ones the diff modifies. `frc_handlers.go` only JSON-decodes these structs and
  delegates to the reporting package — `Decimal.UnmarshalJSON` accepts bare JSON
  numbers, so the handler genuinely needs no change. ✓
- **Both `round2` helpers removed** (one per file), with no remaining callers.

### Security: PASS

This change improves the platform's money-handling robustness:
- Overflow is no longer silent — `NewFromFloat` rejects NaN/Inf and `TryMulInt64`
  reports overflow, both propagated out of `NotifyCorporateAction` rather than
  corrupting an entitlement. `RatioOrAmount` arrives from untrusted JSON, so the `Try*`
  form is the correct choice at this boundary.
- Banker's-rounding fixed-point replaces float accumulation, eliminating truncation
  bias on penalty/notional sums.
- No injection surface, no secrets, no auth changes. `Float64()` is used only for
  display-only CSV rendering, which is the sanctioned accessor.

### Code Quality: PASS

Follows the validated shared-decimal migration recipe (R002/R003/R004) exactly:
filesystem `replace` directive against `../shared/pkg/types/decimal`, no `go.sum`
churn, direct type use. The distinction between money (→Decimal) and non-money floats
is applied carefully and documented inline. The handoff is thorough and honestly
discloses behavioural deltas. No dead code, no over-engineering.

### Test Coverage: PASS

Critical money paths are covered: DvP settlement with a decimal amount, the
zero-amount validation path (`IsPos`), exact dividend entitlement (2500/1250), FRC
daily-summary totals, fractional settlement-fail penalties summing to an exact 20.00,
and CSV rendering. Test literals were correctly migrated to `DecimalFromInt`/
`MustParse` with `.Equal` assertions. The worker reports `go build`, `go vet`, and
`go test ./...` all clean.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Untested new error path:** the `NewFromFloat`/`TryMulInt64` error returns in
   `NotifyCorporateAction` (the only genuinely new behaviour in this change) have no
   test. A NaN/Inf or overflow dividend case would lock in that the error now surfaces
   instead of silently corrupting. Hard to trigger via JSON, but cheap to assert.
2. **Inert `,omitempty`:** `TransferStatus.SettlementAmount` keeps `,omitempty`, which
   is a no-op on a struct type, so FoP transfer status now always serializes
   `"settlement_amount":0` (previously omitted). The worker disclosed this; no test
   asserts its absence. Consider dropping the now-meaningless tag for a clean FoP wire
   shape.
3. **Precision regression test:** add a fractional dividend (e.g. 2.505 per share) to
   demonstrate the 4-dp benefit over the old 2-dp `round2`; the current tests all use
   whole-number expectations that the old code also satisfied.
4. The handoff's follow-up notes (corporate-actions and gateway fees/pre-trade risk
   still on `float64`) align with the R006 audit backlog and should become their own
   R-tasks.
