APPROVED

# Review — R020: gateway fees + pre-trade risk → decimal

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The task — convert the gateway's fee calculation and pre-trade risk
(order-value/quantity/price-band) money math from `float64` to the shared
`github.com/garudax-platform/decimal` type — is implemented correctly and
completely for the two R006-enumerated residuals (fees charged, orders blocked).

Verified against the actual shared module (`src/shared/pkg/types/decimal/decimal.go`):

- **Every method used exists with the claimed semantics.** `MulDecimal`,
  `DivInt64`, `MulInt64`, `Add`, `Sub`, `Abs`, `IsPos`, `IsZero`, `Equal`,
  `LessThan`, `GreaterThan`, `TryMulDecimal`, `TryMulInt64`, `NewFromFloat`,
  `ParseDecimal`, `MustParse`, `DecimalFromRaw`, `Zero`, `Float64`, `String` are
  all present. `MulDecimal`/`DivInt64` round half-to-even and check a 128-bit
  intermediate, so the migration inherits the no-overflow / no-truncation-bias
  guarantees R006 was after.
- **Multiply-before-divide is the right call.** `tradeValue.MulDecimal(RateBPS).DivInt64(10000)`
  preserves a fractional basis-point rate that a naive `RateBPS/10000`-first
  decimal port would lose to the 4-dp scale. `TestCalculateFee_FractionalBasisPoints`
  (2.5 bps on 1,000,000 = 250 exactly) proves it.
- **Price-band rearrangement is mathematically equivalent.** For `lastPrice > 0`,
  `|price − last|/last·100 > bandPct` ⟺ `|price − last|·100 > bandPct·last`. The
  guard correctly short-circuits the `lastPrice <= 0` and market-order (`price <= 0`)
  cases first, so the multiply only runs when `last > 0`. Boundary inclusivity is
  preserved (110 vs 100 @ 10% passes; 110.01 rejects).
- **Overflow fails closed.** Untrusted notional/band multiplies use `TryMul*`
  and treat overflow as ORDER_VALUE_EXCEEDED / PRICE_BAND_EXCEEDED rather than
  panicking — the correct posture for client-supplied price/quantity.
- **`DefaultOrderLimits` sentinel** correctly swaps the unrepresentable
  `math.MaxFloat64` for `DecimalFromRaw(math.MaxInt64)` (~9.22e14), the largest
  representable Decimal, as the effectively-unlimited value.

Compile safety checked: the files that reference these symbols but are outside
the diff (`reporting/generator.go`, `bot/executor.go`, `handler/routes.go`) do
not construct the changed `fees`/`risk` structs or call the changed functions
with float arguments, and the now-decimal `PositionLimits.MaxLong/Short/Gross`
fields are dormant (defined but unused in logic) — so the type change does not
break compilation. No untouched float comparison against a now-decimal field
remains in `handler.go` (only the two `OrderLimits` validations, both updated).

### Security: PASS

- Untrusted client price/quantity are parsed through `ParseDecimal`, which
  returns an error on malformed input (propagated as a 400) — no silent
  coercion.
- Order-value and price-band checks use the `Try*` API and fail closed on
  overflow, so an astronomically large order cannot wrap an int64 into a passing
  notional. This is a genuine improvement over the prior float path.
- Parameterized SQL is preserved in all `PgStore` paths; no injection surface
  introduced. No hardcoded secrets.
- Transport/domain boundary is clean: JSON request DTOs (`CreateFeeRuleRequest`,
  `FeeRuleUpdate`) stay `float64`/`*float64` and convert at the handler edge; the
  decimal type's `MarshalJSON`/`UnmarshalJSON` keep the HTTP wire contract
  (plain JSON numbers) identical.

### Code Quality: PASS

- Follows the validated R-series migration recipe: relative `replace` directive
  for the shared module, `decFromFloat` shim at the `database/sql` float
  boundary (mirroring securities-service), domain = decimal / DTO = float64.
- Dead `roundTo4` removed from the calculator; fixed-point rounding now lives in
  the shared type.
- `omitempty` is used only on the pointer field `MaxFee` (correct — `omitempty`
  is a no-op on a struct value, and none of the non-pointer decimal fields rely
  on it).
- Decision rationale is documented inline (multiply-before-divide, band
  rearrangement, fail-closed overflow) and in a thorough handoff.

### Test Coverage: PASS

- Fee paths covered: per-tier rates, wildcard tier/instrument precedence, min
  applied/not-applied, max capped/not-capped, per-contract, zero/empty rules,
  and the new fractional-bps precision test.
- Risk paths covered: numeric and string quantity parsing, a new fractional
  numeric-quantity case, order-qty/value at-limit and over-limit (incl. a new
  fractional-notional case), price-band within/at/over (incl. fractional just
  -over), market-order and no-last-price skips, fail-open on nil store / store
  error, and `DefaultOrderLimits` positivity.
- Assertions are meaningful (`.Equal`/`.IsZero` on exact decimal values), not
  "runs without error."

---

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **`decFromFloat` swallows the `NewFromFloat` error.** The handoff states the
   only error is NaN/Inf, but `NewFromFloat` also returns `ErrOverflow` for
   values outside ±9.22e14 (via `ParseDecimal`). On the `CreateFeeRule` request
   path this means an out-of-range fee value would silently become `0` rather
   than being rejected. Realistic bps/fee inputs are nowhere near that bound, so
   severity is low — but consider returning a 400 from the handler when
   `NewFromFloat` errors on a client-supplied DTO field, rather than discarding
   it in the shared shim.

2. **Reporting fee aggregation is still float.** `internal/reporting/generator.go`
   keeps its own `FeeInput` (TradingFees/ClearingFees/…) and a local `roundTo4`,
   summing fees and PnL into a `net_amount` statement in `float64`. This was
   deliberately scoped out (and is a display/report surface, not the charge
   decision), so it does not block R020 — but it is residual money-float debt on
   a financial report and is a reasonable candidate for a follow-up task
   alongside the other R006 residuals (R007–R011).

3. **Untested overflow branches.** The `TryMul*` fail-closed paths in
   `CheckOrderSize` and `CheckPriceBand` are not directly exercised by a test.
   They are only reachable at astronomically large inputs, so this is minor — a
   single case with near-`MaxInt64`-raw price/quantity would lock in the
   fail-closed behavior.

4. **`PriceBandPct` / `Volume30D` remain float64.** Correctly left as
   percentages/statistics outside the money scope, per the handoff. No action
   needed unless a later task fully types the fees domain.
