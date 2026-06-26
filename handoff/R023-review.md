# Review — R023: gateway reporting generator → decimal

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The migration of `gateway/internal/reporting` money math from `float64` to the shared
fixed-point `decimal.Decimal` is correct and follows the validated R020 recipe exactly.

Verified against the codebase (not just the diff):

- **Every decimal API method used exists** in `src/shared/pkg/types/decimal/decimal.go`:
  `Zero()`, `Sub`, `MulDecimal` (half-even round to 4 dp), `Add`, `GreaterThan`, `LessThan`,
  `IsZero`, `Equal`, `MustParse`, `NewFromFloat` (returns `(Decimal, error)`), `Float64`.
- **P&L arithmetic is more correct than before.** Per-position `unrealized = (mark - avg) * qty`
  uses `Sub(...).MulDecimal(decFromFloat(qty))`; `MulDecimal` already rounds half-to-even to 4 dp,
  so the old separate `roundTo4` step is correctly dropped. Aggregation (`Add`) and `netAmount`
  (`totalUnrealized.Sub(totalFees)`) are exact int64 ops with overflow-checked Try* under the hood.
  Hand-verified: FractionalPrecision test → `0.9999` unrealized, `0.1235` fees, `0.8764` net ✓;
  WireShape test → `(260.25-250)*100 - 15.5 = 1009.5` ✓.
- **Money vs quantity split is sound.** Prices, margins, fees, P&L, net amount, and OHLC/settlement
  prices become `Decimal`; genuine contract counts (Quantity, Volume, OpenInterest, Long/ShortQty,
  Net/GrossPosition) and the percent-of-OI ratio stay `float64`. This mirrors the documented recipe
  (`fees.Volume30D` / risk percentages left as float). `GenerateLargeTraderReport` is correctly
  untouched — it is purely quantities/ratios.
- **No external breakage.** The `GenerateMarketSummary` signature change (`settlementPrice float64 →
  decimal.Decimal`) and all changed struct field types are only referenced within the `reporting`
  package (confirmed by grep across `src/gateway` — handlers/main do not call the generators; they
  serve pre-stored reports). The generator is genuinely latent, so migrating-before-wiring is the
  right call per the R022 directive.
- **No migration needed — claim verified.** `infrastructure/db/migrations/V22__reporting_tables.sql`
  already declares `net_amount`, `open_price`, `high/low/close_price`, `settlement_price` as
  `DECIMAL(18,4)`, so the `Decimal → Float64 → NUMERIC(18,4) → Float64 → Decimal` round-trip is
  lossless for all realistic settlement magnitudes.

### Security: PASS

- Parameterized SQL queries are unchanged; the only edits are `.Float64()` on bound args and
  `decFromFloat(...)` on scanned values. No injection surface introduced.
- `decFromFloat` swallows `NewFromFloat`'s error, which is safe at this boundary: the only error
  cases are NaN/Inf, which a `NUMERIC(18,4)` column cannot carry. There is no untrusted-client
  boundary in this package (generator inputs are typed `Decimal`; DB values are trusted), so the
  fail-closed `Try*` form is not required here — consistent with the recipe's rule.
- No secrets, no auth changes.

### Code Quality: PASS

- Follows the established platform recipe and the per-package `decFromFloat` convention (fees/store.go
  and risk/store.go carry identical helpers; reporting now adds its own — accepted duplication, no
  redeclaration since each is package-local).
- Wire contract preserved: `Decimal.MarshalJSON` emits a bare JSON number, and the new WireShape
  regression tests assert `net_amount`, `open_price`, `settlement_price` deserialize as numbers and
  that quantity fields stay numbers. No `json:` tags changed.
- Good inline documentation: the money-vs-quantity rationale and the explicit note that the retained
  `roundTo4` now covers only non-money values (so the platform `roundTo2|roundTo4` residual sweep
  doesn't false-positive) are exactly the kind of breadcrumbs the audit pattern asks for.

### Test Coverage: PASS

- All existing money literals/assertions ported to `Decimal` via a `dec()` helper.
- New, meaningful tests added: `FractionalPrecision` (proves exact fixed-point P&L aggregation with a
  non-trivial tie value) and two `WireShape` tests (lock in bare-number JSON marshaling for both the
  settlement statement and the market summary — the most likely regression for this change).
- 63.4% package coverage; the uncovered lines are the `PgStore` paths that require a live DB, which is
  the pre-existing baseline, not a regression.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

- **Extreme-magnitude DB round-trip:** the `Float64()` hop loses the least-significant digit above
  ~1e11. Correctly documented and flagged for the MSE-equities scale. If/when reporting is wired for
  MSE notionals, switch the money columns to raw `int64` (`*_raw`) storage via `Raw()` /
  `DecimalFromRaw()` (the R011 pattern) rather than the float shim. Not needed for ace-commodities.
- **Wire the generator before relying on it (R024-adjacent):** once a scheduler/binary actually
  invokes these generators, feed registered-instrument data so the tests stop passing vacuously —
  same gap as the engine Kafka wiring.
- **`decFromFloat` is now duplicated across three gateway packages** (fees, risk, reporting). Accepted
  per the duplication policy, but if a fourth appears, consider a small gateway-internal helper package.
