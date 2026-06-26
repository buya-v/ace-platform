APPROVED

# Review — R022: R0-extension money-path re-audit

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

This is an audit-only task; its deliverable is the report in `handoff/R022.md`, and its
correctness is measured by whether its findings hold up against the actual code. I
spot-verified the load-bearing claims:

- **New residual (gateway reporting generator) — CONFIRMED REAL.** `generator.go`
  computes unrealized P&L `(MarkPrice-AvgPrice)*Quantity`, the `totalFees` sum, and
  `netAmount = totalUnrealized - totalFees` in `float64` (lines 72-88), OHLC/volume/
  settlement price (lines 132-159), and net/gross large-trader positions (lines 202-214),
  all rounded by a float `roundTo4` (line 231). Every file:line citation in the handoff
  matches.
- **"Latent / no live caller" — CONFIRMED TRUE.** A repo grep shows `GenerateSettlementStatement`,
  `GenerateMarketSummary`, and `GenerateLargeTraderReport` are referenced only by
  `generator.go` itself and `generator_test.go`. No HTTP handler invokes them, so this is
  dormant code, not a live correctness defect — the classification is accurate.
- **DB-at-rest residual — CONFIRMED.** `store.go` carries `DailyStatement.NetAmount float64`
  (line 19) and `MarketSummary` OHLC columns as `float64` (lines 28-34), scanned/stored as
  such. Correctly flagged for folding into R023/R011.
- **R018-R021 closures — CONFIRMED.** A platform-wide `round2|roundTo4|roundTo2` sweep
  leaves no money-rounding float helper in corporate-actions, compliance, gateway fees/risk,
  or fix-gateway source (the lone corporate-actions test match is a comment describing the
  *removed* helper). The only gateway `roundTo4` left is the flagged generator. This matches
  the handoff's per-service CLOSED verdicts.

The decision to defer the migration to a named follow-up (R023) rather than perform a
generator+store+DTO rewrite inside an audit task correctly follows the established
"audit's own diff stays minimal; emit scoped follow-ups" learned pattern.

### Security: PASS

No code changes, so no new attack surface. The audit does not introduce or mask any
security issue. (The pre-existing `itoa`-built SQL placeholder indices in `store.go` are
parameterized via `args` and are out of this task's scope.)

### Code Quality: PASS

Zero source diff — appropriate for an audit task. The handoff is high quality: explicit
audit method, per-service PASS/CLOSED verdicts, justified non-money floats called out with
reasoning (e.g. compliance `RatioOrAmount` dual-use, `PriceChange` as sort-key-only,
margin `haircut` as half-even-consumed config), exact file:line citations for the one
residual, and a single concretely-scoped follow-up (R023) with a migration recipe mirroring
R020. This is a model audit handoff.

### Test Coverage: PASS

No production code changed, so no new tests are required. The handoff documents `go build`
+ targeted `go test` runs on each remediated money package; these are consistent with the
no-diff outcome. The latent residual it identifies is already covered by an existing
`generator_test.go` suite that R023 can lean on during migration.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

- **Name the securities accrued-interest display rounding.** `securities-service/internal/
  server/handlers_bond.go:180` serves `roundTo2DP(accrued.Float64())` — a `float64` round of
  a money value (bond accrued interest) at the JSON response boundary. The audit declared
  securities "clean" without citing it. The classification is defensible (the accrued-interest
  *computation* is on the shared decimal per R004; this is display-boundary formatting in the
  R004-deferred-display class), but the audit's stated method (a `roundTo2` helper sweep) would
  have surfaced it, so it should have been listed explicitly under the securities deferred
  exceptions for completeness. Fold it into R023 or the R011 cleanup: marshal the decimal
  directly instead of round-tripping through `float64`.
- **R023 scope is correct as written** — when implementing, convert the generator inputs/
  outputs and `store.go` columns together so persisted settlement net-amounts are exact, and
  keep the HTTP wire shape (decimal marshals as a bare JSON number).
