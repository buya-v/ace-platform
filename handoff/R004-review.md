APPROVED

# Review — R004: Remove float money math in securities-service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Evaluation

### Correctness: PASS

The migration converts the enumerated money fields (`SecurityOrder.Price/StopPrice/AvgFillPrice`,
`SecurityTrade.Price`, `Position.AvgCost/MarketValue/UnrealizedPnl`,
`SettlementObligation.Price/NetAmount/AccruedInterest`, `Bond.ParValue`,
`BondDetails.ParValue`, `CSDTransfer.SettlementAmount`, `AuctionResult.ClearingPrice`,
`ClearingInstruction.Price/NetAmount`) from `float64` to the shared `decimal.Decimal`,
and replaces every in-scope arithmetic/comparison site with decimal operations.

Verified the highest-risk assumptions against the shared type
(`src/shared/pkg/types/decimal/decimal.go`):

1. **`==` / `!=` are safe.** `Decimal` is a single `int64 value` at a fixed scale of 4
   (`decimal.go:91-93`). A given numeric value has exactly one internal representation,
   so the worker's use of `result.ClearingPrice != decLit(100.0)`, `tr.Price != decLit(50.00)`,
   etc. is correct. Critically, the production zero value `types.Decimal{}` (value=0) equals
   `decLit(0)` / `NewFromFloat(0)` (value=0), so the auction "no clearing price"/sell-flat-PnL
   paths that set `types.Decimal{}` compare equal to the tests' `decLit(0)`. This is the one
   place a float→decimal port could silently break, and it is sound here.

2. **Accrued-interest precision is correct.** Both `calcAccruedInterest` (bond handler) and
   `calculateAccruedInterest` (settlement) were reworked to `parValue * couponRate * days / basis`
   with the **division applied last** (`MulDecimal(couponFactor).MulInt64(days).DivInt64(basis)`),
   converting only the genuine ratio `couponRate` via `NewFromFloat` and keeping days/basis/quantity
   as exact integers. Traced ACT/365 (90 days, 0.05, par 1000): 50.0000 → 4500.0000 → DivInt64(365)
   = 12.3288, within the test's 0.0001 tolerance and rounding to 12.33 at 2dp. The naive
   "fold day-fraction into one 4dp factor" alternative would have produced 12.33 raw and failed
   the tolerance — the worker's decomposition is necessary, not incidental.

3. **Weighted-average cost** uses `(oldAvgCost*oldQty + price*qty) / newQty` with the divide last,
   preserving precision (verified against `TestMatchOrder_AvgCostCalculation`).

4. **Boundary shims are placed only at genuine non-money or legacy edges** — circuit-breaker
   reference prices, tick-size validation, surveillance thresholds, `math.Remainder`, market-data
   display floats, and the pg_store DB scan/write. These are the correct float interop points.

Builds, vets, and tests are reported green (7 packages, plus `-race` on engine/settlement), and
the converted files on the worktree branch match the diff.

### Security: PASS

- **No SQL injection introduced.** `PgOrderStore`/`PgTradeStore`/`PgPositionStore` still pass all
  values as positional query parameters ($1..$N); the change only swaps `order.Price` for
  `order.Price.Float64()` as the bound argument — the statement text is unchanged and parameterized.
- **Input validation improved at the JSON boundary.** Request DTOs that switched to `types.Decimal`
  (`par_value`, `settlement_amount`, service-desk `price`) now parse through `UnmarshalJSON`, which
  rejects malformed input and handles `null`, where a raw `float64` previously accepted any number.
- **No hardcoded secrets/credentials.** None introduced.
- Money corruption now fails loudly: the convenience ops panic on overflow rather than wrapping
  int64 silently (per the R002 design). One residual consideration — a pathologically large
  order value could panic the matching engine via `MulInt64` — but order quantity is `int`-bounded
  and this matches the platform-wide R002 decision; noted as non-blocking.

### Code Quality: PASS

- Consistent with the established shared-decimal pattern (R003) and the platform's
  re-export convention (`type Decimal = decimal.Decimal` in the service's `types` package).
- Boundary shims carry comments explaining *why* float is retained (display, ratios, legacy DB),
  which makes the float↔decimal seam auditable.
- The `decLit` test helper is a clean, uniform ergonomic across the affected test packages.
- The diff was kept focused: ~24 files that only had gofmt realignment were reverted, leaving
  `types.go`'s realignment as the one deliberate exception (it is a primary deliverable).

### Test Coverage: PASS

- Every affected test was migrated, and the assertions remain meaningful (clearing prices,
  net amounts, avg cost, accrued interest), not merely "compiles/runs".
- The precision-sensitive paths — the real risk in a float→decimal port — have explicit
  tolerance assertions (accrued interest ACT/360, ACT/365, 30/360; avg-cost weighted average).
- The external wire contract is preserved (Decimal marshals as a bare JSON number) and is
  exercised by existing handler tests that still decode response fields as `float64`, plus the
  shared module's `TestJSONRoundTrip`/`TestFloatInterop`.

---

## Required Fixes (if REJECTED)

None — approved.

## Suggestions (non-blocking)

1. **Out-of-scope money floats remain `float64`.** Dividend/entitlement math
   (`handlers_corporate_actions.go:233` `entitlementValue := float64(pos.Quantity) * dividendAmount`,
   feeding `Entitlement.EntitlementValue`), plus `TradeCorrection.Original/CorrectedPrice`,
   `OffBookTrade.Price`, `ReferencePrice.Price`, `PriceLevel.Price`, `TickerData.*Price`,
   `CustodyBalance.AvgCost`, and `TradingParameterSet.MaxOrderValue`. These were correctly excluded
   per the task's enumerated field list, but `EntitlementValue` is genuine money — schedule a
   follow-up pass to convert it.
2. **DB persistence still round-trips through `float64`.** `pg_store` writes via `.Float64()` and
   reads via `decFromFloat`, so money is precise *in memory and in arithmetic* (the task's scope)
   but loses precision *at rest* against numeric columns. Migrate to `*_raw int64` columns using
   `Raw()`/`DecimalFromRaw()` in a later DB task for full end-to-end precision.
3. **Import grouping nit** in `pg_store.go`: `github.com/garudax-platform/decimal` is interleaved
   into the stdlib import block. It is gofmt-legal (alphabetical) but not goimports-grouped; tidy
   when next touched.
4. **R005 reconciliation.** As the worker notes, securities `SettlementObligation.NetAmount`
   (`price.MulInt64(qty)` + bond accrued) should be reconciled against the settlement-engine cash
   leg; a starter test exists in the shared decimal module.
5. **End-to-end validation gap.** All verification was unit-level (no live DB/stack on this host).
   The float↔decimal DB shim and the wire contract should be confirmed against a real Postgres and
   the SPAs/e2e suite when infrastructure is available before this is relied on in production.
