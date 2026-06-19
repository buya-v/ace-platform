# MSE-2 — Implement Corporate Actions Engine

**Status:** success
**Role:** coder
**Branch/worktree:** line/MSE-2

## Context

The pure calculation core (`engine.go`) and its spec (`engine_test.go`) already
existed in the zero-dep module `src/corporate-actions`, authored by the
test-writer (see `handoff/TEST-MSE-2.md`). That layer answers *what each holder
is entitled to* but has no orchestration that *generates and processes corporate
action events* end-to-end — which is what this task asks for. The previous coder
attempt was rejected for producing no file changes; this pass adds the missing
processing layer.

## What was added (`src/corporate-actions/process.go`)

A lifecycle/event orchestration layer on top of the calculations:

- **`ProcessDividend` / `ProcessSplit` / `ProcessRights`** — drive one action
  `ANNOUNCED → PROCESSING → COMPLETED`, run the matching calculation
  (`CalculateDividend` / `ApplySplit` / `CalculateRights`), and return a
  `ProcessResult` bundling the generated entitlements / adjusted positions plus
  the emitted events. If the calculation fails after the action entered
  `PROCESSING`, the action is **rolled back to `ANNOUNCED`** (the existing
  `PROCESSING → ANNOUNCED` transition) so a failed run leaves no half-processed
  state.
- **`Event` / `EventType`** — tenant-scoped lifecycle events
  (`corporate_action.processing|completed|cancelled`) carrying action ID,
  tenant, instrument, type, status, and affected holder count — ready to publish
  to the platform event bus without a lookup back to the action.
- **`Cancel`** — `ANNOUNCED → CANCELLED` with a cancellation event; rejects
  terminal/processing actions with `ErrInvalidTransition`.
- **`Process(ca, terms any, holders)`** — single runtime entry point that
  dispatches on `ca.ActionType`, with `ErrWrongActionType` for terms/type
  mismatch and for declared-but-unprocessed types (e.g. `Merger`).

## Multi-tenancy

Tenant ID stays a first-class, non-optional input: `begin` runs the existing
`validate(want)` (missing tenant → `ErrMissingTenant`) before any state change,
and only eligible (same tenant + instrument, qty > 0) holdings are acted upon.

## Verification

```
cd src/corporate-actions
go build ./...                  # clean
go vet ./...                    # clean
gofmt -l .                      # no output
go test ./... -cover -count=1   # ok — 100.0% of statements
```

New tests in `process_test.go`: happy paths, rollback-on-invalid-terms,
wrong-type / non-announced-state / missing-tenant rejections, cancellation,
runtime dispatch + terms-mismatch + unsupported-type (Merger).

## Suggested Follow-ups

- Wire this layer into `securities-service`'s `handleProcessCorporateAction`,
  replacing the inline dividend/split math and emitting `Event`s to Kafka.
- Persist `RightsEntitlement` (no table yet) via a migration alongside
  `securities.entitlements`.
- Implement real `Merger` processing (currently routed to `ErrWrongActionType`
  by `Process`).
