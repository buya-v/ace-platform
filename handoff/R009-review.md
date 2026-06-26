APPROVED

# Review — R009: Concurrency race tests

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Scope note

The reviewed diff (`main...feature/R009-race-tests`) bundles three tasks' worth of
changes because of branch lineage: R007 (Kafka fail-fast wiring, already merged
as `d41c498`/`9f5971e`), R008 (engine concurrency fixes, already merged as
`a8ff164`/`fc7115c`), and the R009 work itself, plus CLAUDE.md learned-pattern
and tasks.json edits from the postmortem branch. **R009's own deliverables** are
the three new files under `tests/race/`:

- `tests/race/run_race_tests.sh` — the race-gate harness
- `tests/race/verify_prefix_failure.sh` — the "fails-on-pre-fix" soundness proof
- `tests/race/README.md` — documentation

R007/R008 were reviewed and merged separately; this review evaluates R009 only
and does not re-litigate the already-merged engine/Kafka changes.

---

## Evaluation

### Correctness: PASS

R009 is a Test Writer task. The worker correctly recognized the scope constraint:
the engine logic lives in `internal/` packages, which a separate `tests/` module
cannot import (Go internal-package rule), and the Test Writer must not modify
`src/` (owned by R008). R008's handoff explicitly anticipated that R009 would
"fold these into the CI race-gate." So R009's contribution is the runnable
harness + the pre-fix soundness proof + docs that drive R008's in-package tests —
the right decomposition, not duplicate tests.

I verified the in-package tests the harness drives actually compile against real
APIs:
- `clearing-engine/internal/engine`: `newTestEngine`, `makeTrade` exist; tests
  exercise `ClearTrade`, `SetTradeHandler`, `GetPositions`, `NetObligations`.
- `settlement-engine/internal/engine`: `setupEngine(t *testing.T) (*Engine,
  *valuation.Store, *payment.InMemoryGateway)` matches the `eng, priceStore, _`
  call sites; tests exercise `RegisterInstrument`, `getInstrumentConfig`,
  `RunSettlementCycle`, `GetAllCycles`.
- `margin-engine/internal/margincall`: `testIDGen` struct, `GetActive`,
  `GetAllActive` exist; tests exercise `Evaluate`, `SetHandler`.

The bash parsing in `run_race_tests.sh` is correct: the `|`-delimited SUITE
entries slice cleanly (`tests=("${parts[@]:2:$((local_n-3))}")` yields exactly
the test-name fields for the 2-, 3-, and 1-test entries), and the
exit-code capture (`local rc=$?` after a subshell / command substitution) avoids
the classic `local rc=$(...)` masking gotcha. The deadlock-regression tests
self-fail via an internal 2 s `select`/`time.After`, so a regression hangs and
fails rather than blocking CI forever.

All three required scenarios are covered: concurrent `ClearTrade` + callback,
`RegisterInstrument` during a settlement cycle, and concurrent handler
invocation.

### Security: PASS

These are test-only shell/Go artifacts. `verify_prefix_failure.sh` operates on a
`mktemp -d` throwaway copy and cleans up via `trap ... EXIT` — the committed
`src/` tree is never mutated. The `go mod edit -replace` rewrite uses a computed
repo-internal path; no untrusted input, no injection surface, no secrets. No
concerns.

### Code Quality: PASS

The harness and README are clear, well-commented, and faithful to the project's
conventions (relative-`replace` build model, `GOTOOLCHAIN=auto` note). The
README's scenario→test mapping table and CI snippet make the gate directly
usable by R014. The pre-fix patch is defensively guarded — it asserts its own
string substitutions landed (`"PRE-R008 SIMULATION" in s`) and the outer script
only accepts a failure that matches the specific `deadlocked` / `DATA RACE`
signatures, so a compile error from source drift is reported as UNEXPECTED rather
than silently "passing."

### Test Coverage: PASS

The standout value of R009 is `verify_prefix_failure.sh`: it proves the gate has
teeth by reconstructing bug #1 (handler-in-lock → deadlock) and bug #3
(unsynchronized handler write → `-race` DATA RACE) and asserting both fail on the
pre-R008 shape while passing on the committed code. A race-gate that can't fail
is worthless; this one demonstrably can. Two run modes (focused vs `--full`)
balance a fast developer loop against a true whole-package regression gate.

---

## Suggestions (non-blocking)

1. **Vacuous-pass risk in focused mode for the margin `engine` package.** The
   SUITE entry `src/margin-engine|./internal/engine/|TestCalculateMargin.*` runs
   `-run "^(TestCalculateMargin.*)$"`. If no matching test exists, `go test`
   exits 0 with "no tests to run" and the gate reports PASS without exercising
   anything. `--full` mode does cover that package, but consider asserting at
   least one test matched (e.g. `go test -run ... -v | grep -q '^=== RUN'`) or
   point this entry at a known-existing test name.

2. **`verify_prefix_failure.sh` depends on `python3` and on exact source text.**
   The self-guard handles drift gracefully (reports UNEXPECTED), but a one-line
   "requires python3" note in the README, and/or pinning to a sentinel comment in
   `engine.go` rather than a large literal block, would make it more robust to
   future edits of `ClearTrade`.

3. **Network-fetch deviation.** `GOTOOLCHAIN=auto` will fetch go1.25 on an older
   host, departing from the zero-network build ideal (already noted as a known
   deviation in CLAUDE.md). Fine for a CI gate; just keep it in mind when wiring
   R014 in a sandboxed runner.

4. **Orchestrator/merge note:** when merging `feature/R009-race-tests`, confirm
   the bundled R007/R008 hunks reconcile cleanly against the already-merged
   versions on main so the merge introduces only the `tests/race/` files (plus
   the intended CLAUDE.md/tasks.json postmortem edits).
