# R001 — Shared Decimal (Money) Type Specification

**Phase:** R0 (financial correctness)
**Status:** spec → implemented by R002, consumed by R003/R004
**Module:** `github.com/garudax-platform/decimal` at `src/shared/pkg/types/decimal/`

---

## 1. Problem

The platform had **six divergent copies** of a `Decimal` type (matching/clearing/margin/settlement engines, market-data, warehouse) plus the `securities-service` doing money math in raw `float64`. The copies disagreed on API surface and on three correctness-critical behaviours:

1. **Silent overflow.** `margin-engine` `MulDecimal` computed `(d.value * other.value) / scale` where both operands are already ×10⁴; the intermediate `int64` product wraps silently for large notionals — corrupting balances with no error.
2. **Truncation rounding bias.** All division (`DivInt`, `DivInt64`) and fixed-point multiply truncated toward zero. Over many trades this is a systematic one-directional bias that fails to reconcile to the cent.
3. **Silent divide-by-zero.** `DivInt/DivInt64` returned `DecimalZero()` on a zero divisor, masking errors (e.g. VWAP with zero volume silently → 0).

This spec defines **one** money type that fixes all three and is a **superset** of every existing API so migration (R003/R004) is largely an import swap.

## 2. Representation

- Signed fixed-point, **exactly 4 fractional digits** — `Decimal(18,4)`.
- Internal: a single `int64` holding `realValue × 10000`.
- `Scale = 10000`.
- Representable range ≈ `-922,337,203,685,477.5808 … 922,337,203,685,477.5807`.
- Zero value of the struct is a valid `0.0000` (so `var d Decimal` is usable).

## 3. Correctness rules (non-negotiable)

| Rule | Behaviour |
|------|-----------|
| **No silent overflow** | Every `+ − ×` and the fixed-point `÷` checks a 128-bit intermediate (`math/bits`). Overflow is reported, never wrapped. |
| **Banker's rounding** | Fixed-point multiply (`MulDecimal`) and integer division (`DivInt64`/`DivInt`) round **half-to-even**. No truncation toward zero. |
| **No silent divide-by-zero** | Division by zero returns `ErrDivideByZero` (Try* form) / panics (convenience form). Never returns `0`. |
| **Parse is lossless or rounded, never truncated** | `ParseDecimal` accepts >4 fractional digits and rounds the excess half-to-even; invalid input errors. |

## 4. API surface

Two parallel styles:

- **`Try*` methods** return `(Decimal, error)` — use at money boundaries reachable from untrusted/large inputs (settlement cash, P&L, margin). Errors: `ErrOverflow`, `ErrDivideByZero`, `ErrInvalidFormat`.
- **Convenience methods** (`Add`, `Sub`, `Mul*`, `Div*`, `Negate`, `Abs`) keep the **exact names/signatures of the old copies** and **panic** on overflow/divide-by-zero. A panic is a loud, stack-traced failure — strictly safer than the old silent corruption — and keeps R003 a drop-in rename.

**Constructors:** `NewDecimal(int,frac)`, `DecimalFromInt(int64)`, `DecimalZero()`, `Zero()`, `DecimalFromRaw(int64)`, `ParseDecimal(string)`, `MustParse(string)`.
**Arithmetic:** `Add`/`TryAdd`, `Sub`/`TrySub`, `MulInt64`/`TryMulInt64`, `MulUint64`/`TryMulUint64`, `MulDecimal`/`TryMulDecimal`, `DivInt64`/`TryDivInt64`, `DivInt` (alias), `Negate`/`TryNegate`, `Abs`.
**Comparison:** `Cmp`, `LessThan`, `GreaterThan`, `Equal`, `GreaterThanOrEqual`, `LessThanOrEqual`, `Max`, `Min`.
**Predicates/accessors:** `IsZero`, `IsNeg`, `IsPos`, `Raw() int64`, `String() string`.

`Raw()`/`DecimalFromRaw()` preserve wire/DB compatibility (the scaled int64 already stored in protobufs and Postgres `*_raw` columns is unchanged).

## 5. Migration contract (for R003/R004)

1. Add to each consumer `go.mod`:
   `require github.com/garudax-platform/decimal v0.0.0` and
   `replace github.com/garudax-platform/decimal => ../shared/pkg/types/decimal`.
2. Delete the local `internal/types/decimal.go`; re-export by aliasing in `internal/types` (`type Decimal = decimal.Decimal`, plus `var NewDecimal = decimal.NewDecimal`, …) so existing `types.Decimal` references compile unchanged.
3. `securities-service` (R004): replace every `float64` price/qty/cash/accrual with `Decimal`; settlement cash = `price.MulUint64(qty)` (or `MulInt64`), bond accrual via `MulDecimal`/`DivInt64`, so values reconcile with the int-based settlement engine.
4. Behavioural deltas to expect in tests: results that previously truncated now round half-even; previously-silent overflow/÷0 now panic (or error via `Try*`). R005 asserts these; R006 audits for any call site that *relied* on truncation (none expected for money).

## 6. Out of scope

- Arbitrary precision / variable scale (4dp is the platform standard).
- Currency tagging (handled at a higher layer).
- Float interop beyond an explicit, documented `ParseDecimal`/`String` boundary.
