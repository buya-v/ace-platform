# TEST-MSE-2 — Tests for Corporate Actions Processing

**Status:** success
**Role:** test_writer
**Branch/worktree:** line/TEST-MSE-2

## Summary

Defined the corporate-actions domain via tests (TDD). Created a new
**zero-dependency Go module** `src/corporate-actions` (the platform's winning
"zero-dep Go module per service" pattern), whose `engine_test.go` specifies the
state transitions and calculations for **dividends, stock splits, and rights
issues**, backed by a clean reference `engine.go` that the tests pin down.

Constants are kept wire-compatible with
`securities-service/internal/types` (`CA_DIVIDEND`, `CA_STOCK_SPLIT`,
`CA_RIGHTS_ISSUE`, `ANNOUNCED/PROCESSING/COMPLETED/CANCELLED`, `PENDING/PAID`)
so values flow through the platform without translation.

## What the tests define

### State machine
- `ANNOUNCED → PROCESSING → COMPLETED`
- `ANNOUNCED → CANCELLED`
- `PROCESSING → ANNOUNCED` (rollback on processing failure — mirrors the existing
  `handlers_corporate_actions.go` rollback semantics)
- `COMPLETED` / `CANCELLED` are terminal
- Illegal transitions return `ErrInvalidTransition` and leave `Status` unchanged

### Dividend
- Per-holder cash entitlement = `quantity * amount_per_share`, rounded to 2 dp
  (half away from zero)
- Eligibility filter: same tenant **and** same instrument **and** quantity > 0
- Errors: negative dividend, wrong action type, missing tenant, missing instrument
- Zero dividend allowed; result slice never nil

### Stock split
- `SplitAdjustedQuantity` — forward (2:1, 3:1), reverse (1:10), fractional (3:2)
  with floored whole shares + fractional remainder (cash-in-lieu basis)
- `SplitAdjustedPrice` — inverse ratio, preserves total market value
- `ApplySplit` — adjusts only eligible positions, does not mutate the input slice
- Errors: zero/negative ratio shares, negative price, wrong action type

### Rights issue
- `CalculateRights` — `rights = floor(held * NewShares / OldShares)`,
  `cost = rights * subscription_price`
- `TheoreticalExRightsPrice` (TERP) =
  `(OldShares*cumPrice + NewShares*subscriptionPrice) / (OldShares + NewShares)`
- Errors: invalid ratio, negative price, wrong action type, missing tenant

## Multi-tenancy

Per the GarudaX directive, **tenant ID is a first-class, non-optional input**:
every action must carry a non-empty `TenantID` (else `ErrMissingTenant`), and
holdings are only acted upon when their tenant matches the action's tenant.

## Verification

```
cd src/corporate-actions
go build ./...                  # clean
go vet ./...                    # clean
gofmt -l .                      # no output (formatted)
go test ./... -cover -count=1   # ok — 100.0% of statements
```

26 test functions / 84 sub-cases, all passing. Go 1.22.

## Suggested Follow-ups

- A **coder** task can wire this engine into `securities-service`'s
  `handleProcessCorporateAction`, replacing the inline dividend/split math with
  `corporateactions.CalculateDividend` / `ApplySplit` and adding real
  `CA_RIGHTS_ISSUE` processing (currently "manual processing required").
- Persist `RightsEntitlement` (no store/table exists yet) — needs a migration
  alongside the existing `securities.entitlements` table.
- Consider promoting `corporateactions` types into a shared package if other
  services (clearing, settlement) need entitlement values.
