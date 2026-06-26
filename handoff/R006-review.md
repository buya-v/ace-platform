APPROVED

# Review — R006: Money-path audit

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Summary

R006 is a static money-path audit across all 21 `src/` Go modules, confirming the
post-R003/R004 state and surfacing residual float64 money math. It ships exactly
one code change — fixing a truncating haircut conversion in margin-engine — plus a
thorough per-service PASS/FAIL handoff (`handoff/R006.md`) and five scoped
follow-up task proposals (R007–R011). I verified the one code change against the
actual source and the audit's load-bearing claims by spot-check.

---

## Evaluation

### Correctness: PASS

The single code change (`margin-engine/internal/collateral/warehouse_source.go:127`)
is correct and is a genuine defect fix, not cosmetic:

- **The bug is real.** `int64(s.haircut * 10000)` truncates toward zero. For
  "nice" haircuts (0.60/0.70/0.80) the float product rounds to the right integer,
  but for values like 0.83 (`0.83*10000` = 8299.9999… → `int64` = **8299**) it
  silently understates collateral by one raw unit. Since haircut multiplies every
  pledged-receipt credit (`qty.MulDecimal(price).MulDecimal(haircut)`), this is a
  money-path truncation exactly in scope for the audit.
- **The fix is correct.** `decimal.NewFromFloat(0.83)` →
  `strconv.FormatFloat(f,'f',-1,64)` = "0.83" → `ParseDecimal` (half-even) = 8300
  raw. Half-even rounding replaces truncation, consistent with the R003/R004
  decimal semantics.
- **Type-safe.** `types.Decimal` is a pure alias of `decimal.Decimal`
  (`internal/types/decimal.go:13`), so `NewFromFloat`'s `decimal.Decimal` return
  assigns cleanly to the `types.Decimal`-typed local and feeds `MulDecimal`
  without conversion. The new `github.com/garudax-platform/decimal` import is
  already a dependency of the module (go.mod:11, `replace` at :21).
- **No regression.** Existing collateral tests exercise haircuts 0.60/0.70/0.80,
  all of which yield byte-identical raw values under old and new code, so the
  worker's "suite green" claim is consistent with the test fixtures I inspected.

The audit's core structural claims that I spot-checked hold up: the per-engine
`internal/types/decimal.go` is indeed a 23-line alias shim over the shared type,
and `DivInt`/`MulDecimal` call sites resolve to the shared checked-decimal
methods. The PASS/FAIL per-service classification is defensible and the out-of-scope
FAIL list (corporate-actions, compliance MCSD/FRC, gateway fees/risk, fix-gateway)
is correctly excluded from R003/R004 scope and routed to follow-up tasks rather
than silently fixed.

### Security: PASS

No security surface is touched. The haircut input is a bounded config float
`(0,1]` enforced by `WithHaircut`/`DefaultHaircut`; the added error branch is
defensive only (NaN/Inf can't reach it from a finite config value) and fails
safe to zero collateral credit (conservative — never inflates credit). No
secrets, no injection, no auth path, no external input. The defensive
`log.Printf` does not leak sensitive data.

### Code Quality: PASS

The change follows the file's existing conventions: shared-decimal usage matching
the rest of the engine, the established `log.Printf("collateral: …")` message
style, and a clear comment explaining *why* the conversion changed (citing the
R006 audit and the truncation-vs-half-even rationale). No dead code, no
over-engineering. The handoff is well-structured, cross-references downstream task
IDs with specific file:line sites, and clearly separates in-scope fixes from
documented/justified residuals.

### Test Coverage: PASS

The changed function (`calculateReceiptCollateral`) is covered by the existing
margin-engine collateral suite (default, custom 0.60/0.70, invalid-haircut, and
fractional-quantity cases), so the modified line executes under test. Adequate
for an audit task whose single change preserves existing behavior on all
currently-tested inputs. See the non-blocking suggestion below for the one gap.

---

## Required Fixes (if REJECTED)

None — approved.

## Suggestions (non-blocking)

1. **Add a truncation-regression test.** Every existing haircut fixture
   (0.60/0.70/0.80) happens to round-trip identically under both the old and new
   code, so none of them would have caught the original bug or would catch a
   regression to truncation. Add one case with a haircut that previously
   truncated — e.g. `WithHaircut(0.83)` over `100 * 1000` collateral, asserting
   `83000` (new, correct) rather than the old truncated `82999` — to lock in the
   fix. This is the highest-value follow-up.
2. **Multi-tenancy directive (N/A here, noted for completeness).** The 2026-04-23
   directive requires tenant context on new API surfaces. R006 adds no API surface
   — it corrects internal arithmetic in an existing function — so the rule does
   not apply and is not grounds for rejection. Flagging only so the next reviewer
   doesn't re-litigate it.
3. **Follow-ups R007–R011 are well-scoped.** The corporate-actions and
   compliance MCSD/FRC float money paths are real correctness debt; prioritize
   them before those modules are wired into a running binary (corporate-actions
   currently has no `cmd/`, so it is latent, not live). The gateway fees +
   pre-trade-risk migration (R009) is the most user-visible (fees are charged) and
   should lead that batch.
