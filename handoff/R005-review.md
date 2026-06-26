# Review — R005: Decimal reconciliation & property tests

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The deliverable is a self-contained Go test module at `tests/reconciliation/` that
exercises the *real* shared money type (`github.com/garudax-platform/decimal`) via a
filesystem `replace` — verified the module path in `src/shared/pkg/types/decimal/go.mod`
matches the `require`/`replace` in the test `go.mod`. I checked every API reference
against `decimal.go`; all exist with the used signatures (`TryMulInt64`/`MulInt64`,
`TryMulDecimal`/`MulDecimal`, `TryMulUint64`, `TryDivInt64`/`DivInt64`/`DivInt`,
`TryAdd`/`TrySub`/`TryNegate`, `DecimalFromRaw`/`DecimalFromInt`/`MustParse`/
`ParseDecimal`/`NewFromFloat`, `Raw`/`Equal`/`Cmp`/comparators, `Scale`, `ErrOverflow`,
`ErrDivideByZero`).

The independent `math/big` oracle is the strongest part of the work. `roundHalfEvenBig`
(2r vs d, tie → even) is mathematically equivalent to the package's `mulDivRound`
(r vs rem=ud-r, tie → q&1), and both round the magnitude with sign applied after — so the
oracle is a genuine second implementation, not a mirror of the code under test. The
overflow contract in `requireOracle` ("fits int64 ⇒ exact value; doesn't fit ⇒ must
error") aligns with the package's pre-rounding `hi >= ud` guard and post-rounding
`signedFromMag` check; I worked through the boundary cases (positive MaxInt64+1 → error,
negative MaxInt64+1 → MinInt64) and oracle/package agree.

The reconciliation formulas are reproduced faithfully — I confirmed each against source:
- `securitiesNetAmount` ↔ `internal/settlement/engine.go:80` and `equities.go:186`
  (`trade.Price.MulInt64(int64(trade.Quantity))`) — exact match.
- `settlementEngineCash` ↔ `internal/valuation/valuation.go:91-96` (abs(qty) then
  `markPrice.MulInt64(qty)`) — exact match.
- `securitiesBondNetAmount` accrued path ↔ `engine.go:223-226`
  (`parValue.MulDecimal(couponFactor).MulInt64(30).MulInt64(qty).DivInt64(basis)`) — exact,
  including the divide-by-basis-LAST ordering, and the test's oracle correctly treats only
  the MulDecimal and final DivInt64 as rounding steps (the two MulInt64 are exact).

Spot-checked the table cases by hand (17.33×137=2374.21, 999.9999×4321=4320999.5679,
0.0001×1e6=100, bond accrued 41.0959) — all correct. The float-boundary regression is
sound: 4.015 rounds *down* to ~4.0149999996 in float64, so the old truncating path yields
401.4999 while the decimal path is exactly 401.5000; the precondition assertion guards the
premise. Round-trip bounds (exact for Mul→Div, ±q/2+1 for Div→Mul) are correctly derived.

### Security: PASS

Test-only module. No network (handoff confirms no `go.sum`, filesystem `replace` only), no
untrusted input, no secrets, no shell/SQL/command surface. No source files were modified
(scope respected: only `tests/` and `handoff/`). The single float boundary (`NewFromFloat`
for the coupon rate) mirrors production source rather than introducing new float math.

### Code Quality: PASS

Follows the established platform conventions: separate Go module under `tests/` with a
filesystem `replace` (consistent with the zero-dep module pattern and the e2e tier's
isolation). Deterministic randomness (fixed seed `0x6A52D5` + per-test offsets, 20k
iterations) matches the CLAUDE.md guidance on stable CI. Clear package doc, file:line
citations on every reproduced formula, sensible naming, and a `checked < iterations/2`
guard so a too-tight range can't silently no-op a property. Handoff is thorough and
honestly documents the central limitation (see below). Verified clean under
`go vet`/`gofmt`/`-race`/`-count=2` per the handoff (not independently re-run, but the
code is gofmt-clean on inspection).

### Test Coverage: PASS

All four required areas are covered with meaningful assertions, not "runs without error":
1. **Mul/Div round-trip & rounding direction** — 12 property sweeps vs the big.Int oracle,
   pinning banker's rounding on every random input plus exact and bounded round-trips.
2. **Overflow errors, not wraps** — both property-level (`requireOracle` across wide-range
   sweeps) and 7 focused boundary tests (Mul/Add/Sub/Negate/Parse at int64 extremes),
   asserting both the `Try*` error and the convenience `panic`.
3. **Divide-by-zero** — every entry point (`TryDivInt64` error, `DivInt64`/`DivInt` panic,
   0/0), plus a non-zero-divisor negative control proving the guard isn't overbroad.
4. **Cross-service reconciliation** — table + 20k-iteration sweep showing securities-service
   and settlement-engine cash legs agree exactly (or overflow identically), a batch-sum
   exactness check over 500 trades, the bond accrued path vs oracle, and a regression that
   pins *why* the float→decimal migration was needed.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

- **Formula duplication is the real risk.** Because the settlement math lives in service
  `internal/` packages, the suite reproduces the one-line formulas rather than calling the
  live functions. A future change to a service formula that doesn't touch the shared type
  would not be caught here. The handoff already flags this; promote the suggested
  per-service `_test.go` follow-ups (securities `internal/settlement` and settlement-engine
  `internal/valuation`) into a tracked R-series task so the reconciliation eventually binds
  to the live functions, not copies.
- **`TestReconcile_RandomizedCashLegAgreement` is near-tautological** — both sides call
  `MulInt64` on the same positive inputs, so it mainly validates the overflow-parity wrapper.
  Consider feeding the two sides through their actual differing intermediate representations
  (or negative quantities to exercise `settlementEngineCash`'s abs branch) to make it bite harder.
- **Tenant context (directive 2026-04-23):** the multi-tenancy directive requires tenant
  context on domain API surfaces. It does **not** apply here — `decimal` is a platform-level
  primitive shared across all tenants (analogous to "Auth is platform-level"), and this is a
  pure-arithmetic test module with no API surface. Noting explicitly so the exemption is on record.
