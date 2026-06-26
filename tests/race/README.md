# Concurrency race-gate (R009)

This suite proves the three concurrency fixes shipped in **R008** with the Go
race detector (`go test -race`) and a set of deadlock-regression tests. It is
the `tests/`-side gate that runs those tests as one suite — wire it into CI
(see **R014**) so handler-in-lock / map-race regressions are caught
automatically.

## What R008 fixed (and what this gate guards)

| # | Bug class | Engine(s) | How a regression is detected |
|---|-----------|-----------|------------------------------|
| 1 | Handler callback invoked **while holding the engine mutex** — a re-entrant callback deadlocks the non-reentrant lock | clearing, margin, settlement | Deadlock-regression tests: the handler re-enters the engine; if it runs in-lock the test hangs and **fails on a 2 s timeout** |
| 2 | **Unguarded concurrent map read** (`getInstrumentConfig` read `e.instruments` with no lock while `RegisterInstrument` wrote it) | settlement | `go test -race` reports `DATA RACE` |
| 3 | **Unsynchronized writes** to the `Set*Handler` field, racing with reads | clearing, margin, settlement | `go test -race` reports `DATA RACE` |

R008 placed the tests inside each engine's `internal/` package (the engine
logic is internal, so the tests that exercise `ClearTrade`, `RegisterInstrument`,
and the handler callbacks must live there). This directory drives them.

### Tests exercised, by task requirement

- **concurrent `ClearTrade` + callback** —
  `clearing-engine/internal/engine`:
  `TestClearTradeHandlerRunsOutsideLock` (deadlock regression) and
  `TestClearTradeConcurrentRace` (16 workers clearing while the handler is
  mutated and positions are read).
- **concurrent `RegisterInstrument` during a settlement cycle** —
  `settlement-engine/internal/engine`:
  `TestGetInstrumentConfigConcurrentWithRegister` (register vs. config read) and
  `TestRunSettlementCycleConcurrentRace` (cycles + `RegisterInstrument` +
  handler mutation + `GetAllCycles` all concurrent), plus
  `TestRunSettlementCycleHandlerOutsideLock` (deadlock regression).
- **concurrent handler invocation** —
  `margin-engine/internal/margincall`:
  `TestEvaluateHandlerRunsOutsideLock` (deadlock regression) and
  `TestEvaluateConcurrentRace`. The margin `internal/engine` package's
  `CalculateMargin` + `SetMarginHandler` path is covered under `-race` in
  `--full` mode.

## Running it

From the repo root:

```bash
# Focused: just the R008/R009 concurrency tests, all under -race.
./tests/race/run_race_tests.sh

# Whole affected packages under -race (the regression gate; what CI should run).
./tests/race/run_race_tests.sh --full

# Stream full `go test -v` output.
./tests/race/run_race_tests.sh --verbose
```

Equivalent manual commands (run inside each module directory):

```bash
cd src/clearing-engine    && go test -race ./internal/engine/
cd src/settlement-engine  && go test -race ./internal/engine/
cd src/margin-engine      && go test -race ./internal/engine/ ./internal/margincall/
```

Exit status is `0` only when every package is race-clean and all tests pass.

### Toolchain note

The engine modules declare `go 1.25.x`. If the host `go` is older, the scripts
export `GOTOOLCHAIN=auto` so the correct toolchain is fetched on first use
(consistent with the repo's relative-`replace`, no-`go.sum` build model). Set
`GOTOOLCHAIN` yourself to override.

## Proving the tests have teeth (fail on pre-fix code)

A green race-gate is only meaningful if the tests would **fail** on the buggy
pre-R008 code. `verify_prefix_failure.sh` proves exactly that: it copies the
clearing engine to a throwaway temp dir (committed `src/` is never touched),
reconstructs bug #1 (handler-in-lock) and bug #3 (unsynchronized handler
write), and asserts both tests fail:

```bash
./tests/race/verify_prefix_failure.sh
```

Expected output:

```
==> [pre-fix] bug #1: handler-in-lock should DEADLOCK the test
  ✓ expected FAIL observed (deadlock detected by the test)
==> [pre-fix] bug #3: unsynchronized handler write should trip -race
  ✓ expected FAIL observed (WARNING: DATA RACE)

==> Both tests FAIL on pre-fix code and PASS on the committed (R008-fixed) code.
    The race-gate is sound.
```

On the pre-fix code the detector prints a `WARNING: DATA RACE` naming
`(*Engine).SetTradeHandler`, and the deadlock test reports
`ClearTrade deadlocked — handler is still being invoked inside the lock`. On the
committed code both pass.

## CI integration (R014)

Add to the build+test gate (one step per engine module):

```yaml
- run: cd src/clearing-engine    && go test -race ./internal/engine/
- run: cd src/settlement-engine  && go test -race ./internal/engine/
- run: cd src/margin-engine      && go test -race ./internal/engine/ ./internal/margincall/
```

or simply `./tests/race/run_race_tests.sh --full`.

## Follow-ups

- Extend the deadlock-regression pattern to the **matching engine** if it grows
  a handler-callback path (none today).
- The cross-service event-propagation e2e failures (channel-based Kafka stubs
  not bridging separate processes) are a **different** concurrency concern and
  are out of scope here — they require real Kafka infrastructure, not `-race`.
