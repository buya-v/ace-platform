# MSE-5 — FRC and MCSD Integrations

**Status:** success
**Role:** coder
**Branch/worktree:** line/MSE-5

## Summary

Built the two external regulatory/depository integration surfaces for the
**mse-equities** flagship venue, both as zero-dependency Go packages inside
`src/compliance-service` (honouring the platform's "zero-dep Go module" pattern),
then wired them into the compliance-service HTTP server.

### 1. FRC reporting pipeline — `src/compliance-service/reporting/frc.go`

Turns platform data into the five FRC-mandated reports from
`docs/platform-architecture.md §10.5` and publishes each to its delivery targets:

- **Report types:** `DAILY_TRADING_SUMMARY`, `LARGE_TRADER_REPORT`,
  `SETTLEMENT_FAILS_REPORT`, `SUSPICIOUS_TRADING_ALERT`, `QUARTERLY_COMPLIANCE_REPORT`.
- **Rendering:** JSON for all; CSV additionally for the tabular Daily Trading
  Summary (format validated per report type — CSV rejected for non-tabular reports).
- **Delivery targets** (computed, matching §10.5):
  - Kafka topic `<tenant>.compliance.frc-report-generated`
  - S3 key `s3://garudax-<tenant>-reports/<report_type>/<date>/<report_id>.<ext>`
- **`Publisher` interface** with `NoopPublisher`, `RecordingPublisher` (in-memory
  sink used by the no-broker deployment mode and the GET listing). A real Kafka+S3
  publisher swaps in behind the interface at deployment — the established
  "interface + in-memory impl" pattern for integration-layer code.
- Auto-computes totals + top-5 movers (by abs price change) for the daily summary,
  total penalties for fails, percent-of-outstanding for large traders.

### 2. MCSD integration — `src/compliance-service/integration/mcsd.go`

Implements the `CSDAdapter` interface verbatim from
`docs/platform-architecture.md §10.6` plus an in-memory `StubAdapter`
("initial implementation — operations succeed immediately"):

- **Account management:** `CreateCustodyAccount`, `GetBalance`, plus a `Credit`
  helper to seed opening positions.
- **Transfers:** `InstructDvP` / `InstructFoP` with synchronous book-entry
  settlement (`AutoSettle`, default on). When `AutoSettle` is off, transfers stay
  `PENDING` with a pending reservation and are driven via `Affirm`/`Settle`/`Fail`
  — modelling the MCSD affirmation handshake (open question §13.4 of the MSE spec).
  `GetTransferStatus` returns the full lifecycle record.
- **Corporate actions:** `NotifyCorporateAction` snapshots record-date holdings
  and computes per-holder `Entitlement`s (cash for DIVIDEND, shares for
  split/rights), retrievable via `GetEntitlements`.
- **ISO 20022 message IDs** declared as constants (`sese.023`, `sese.024`,
  `semt.013`, `seev.031`) so the production wire adapter and the stub agree on
  the message contract.

### 3. Server + main wiring

- `internal/server/frc_handlers.go` — `POST/GET /frc/reports` (generate+publish,
  list).
- `internal/server/mcsd_handlers.go` — `/csd/accounts`, `/csd/accounts/balance`,
  `/csd/transfers/{dvp,fop}`, `/csd/transfers?id=`, `/csd/corporate-actions`.
  Domain errors mapped to proper HTTP codes (400/403/404/409/422).
- `server.go` gains optional `frc`/`csd` fields + `SetFRCReporter`/`SetCSDAdapter`
  setters; routes register **only when wired** (nil-guarded) so existing
  deployments/tests are unaffected (verified by `TestRoutesDisabledWhenUnwired`).
- `main.go` enables FRC reporting (tenant from `FRC_TENANT_ID`, default
  `mse-equities`) with a `RecordingPublisher`, and the MCSD stub adapter.

## Multi-tenancy (GarudaX directive)

Tenant ID is a first-class, non-optional input throughout:
`reporting.NewReporter("")` → `ErrMissingTenant`; every MCSD account/instruction/
corporate-action requires a non-empty `TenantID`; the adapter **enforces tenant
isolation** — a transfer whose accounts don't both match the instruction tenant
is rejected with `ErrTenantMismatch` (`TestTenantIsolationRejectsCrossTenantTransfer`).

## Verification

```
cd src/compliance-service
go build ./...        # clean
go vet ./...          # clean
go test ./...         # all pass
#   reporting   88.3% coverage
#   integration 89.7% coverage (also -race clean)
#   internal/server passes incl. new FRC/MCSD HTTP tests
```

## Suggested Follow-ups

- **Real publishers/adapters:** swap `RecordingPublisher` for a Kafka+S3 publisher
  and `StubAdapter` for an ISO 20022 HTTPS adapter behind the existing interfaces;
  no caller changes required.
- **Settlement-engine hook:** `mse-equities.securities.settlement-completed/-failed`
  events should drive `GenerateSettlementFailsReport` and DvP instructions
  (MSE spec §10 event table). Currently exposed via HTTP for the orchestrator.
- **DB persistence:** reports/transfers/entitlements are in-memory; a future task
  can persist to `mse_compliance` / `platform.audit` (migration) for durability.
- **FRC schedule:** Daily Summary (14:00), Settlement Fails (17:00), Quarterly
  reports should be triggered by a scheduler (cron/admin-bot) calling the
  generate endpoint.
