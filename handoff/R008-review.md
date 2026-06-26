# Review — R008: Fix engine concurrency bugs

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The change fixes three real, distinct concurrency defects across the clearing, margin, and settlement engines, and the chosen idiom is sound and consistent.

1. **Handler-invoked-under-lock (deadlock/contention).** All three engines previously called an arbitrary user callback while holding the engine mutex. A callback that re-entered the engine (e.g. `NetObligations`, `GetAllCycles`, `GetActive`) would deadlock on the non-reentrant mutex. The fix — snapshot the handler field inside the critical section, release the lock, then invoke — is correct and is the right idiom for `func`-typed fields (preferable to `atomic.Value`, which has typed-nil/type-consistency footguns for funcs). The `*Locked` helper extraction (`clearTradeLocked`, `calculateMarginLocked`, `runSettlementCycleLocked`, `runMultiInstrumentCycleLocked`) cleanly owns `defer Unlock()` and returns a handler snapshot for the thin public method to fire after unlock.

2. **Unsynchronized handler writes.** `SetTradeHandler`/`SetMarginHandler`/`SetHandler`/`SetCycleHandler` now take the same mutex that guards the read path, closing the data race between concurrent `Set*Handler` and the calculation path. Verified the read side snapshots the field under the lock, so set/read is fully ordered.

3. **Settlement unguarded map read + non-reentrant RLock.** `getInstrumentConfig` now takes `RLock` (safe vs. `RegisterInstrument`'s write). Critically, the in-engine caller `runMultiInstrumentCycleLocked` already holds the **write** lock, so it correctly calls the new lock-free `lookupInstrumentConfig` instead of `getInstrumentConfig` — avoiding the `RLock`-under-`Lock` self-deadlock that Go's non-reentrant `RWMutex` would otherwise produce. `GetCycle`/`GetAllCycles` downgraded to `RLock` for concurrent reads. All correct.

Behavioral preservation verified against the diff:
- Settlement's early P&L-failure path still returns a nil handler (`types.SettlementCycle{}, nil`), so the handler does **not** fire on failure — matching prior semantics.
- The handler receives a value copy (`*cycle` / `snapshot`) taken under the lock, so it never observes a concurrently-mutated struct.
- margincall's `issueOrUpdateCall` return changed from `(*MarginCall, error)` (error always nil) to `(*MarginCall, bool)`, where `bool` flags a *newly issued* call. The update-existing path returns `false` (no handler) and the resolve/surplus path returns `nil` — both preserve the original "only fire on new issue" semantics. The snapshot (`snapshot = *result`) is taken under the lock before unlock, which is essential since `result` aliases the `s.calls` map entry.
- Margin engine ordering preserved: `marginHandler` fires before `callService.Evaluate`, both now outside `e.mu`.

I confirmed all referenced test helpers (`newTestEngine`, `makeTrade`, `setupEngine`, `deficitPortfolio`, `surplusPortfolio`, `testIDGen`) exist in the respective packages, and the new package-level `participantID(int)` helper does not collide with any package-level identifier (existing uses are function parameters, legally shadowing it).

### Security: PASS

No security surface. These are internal correctness/liveness fixes — no input handling, auth, or external boundaries are touched. Moving callbacks out of the lock reduces availability risk (deadlock) rather than introducing any.

### Code Quality: PASS

Follows existing conventions (zero-dep modules, existing mutex idiom, gofmt alignment). The `*Locked` helper pattern is readable and keeps each error path intact. Comments explain *why* the handler runs outside the lock and why `lookupInstrumentConfig` is lock-free. The handoff (`handoff/R008.md`, present in the diff despite the prompt header) documents decisions clearly.

One non-correctness note: the branch bundles unrelated bookkeeping — CLAUDE.md learned-pattern additions (R018–R022) and `tasks.json` / `tasks-remediation.json` status flips for R018–R023. These are not part of the concurrency fix and add merge-conflict risk against whatever else is touching those files. Non-blocking, but the branch would be cleaner scoped to the four engine files + tests.

### Test Coverage: PASS

Each fix has a targeted regression test plus a `-race` stress test:
- clearing: `TestClearTradeHandlerRunsOutsideLock` (re-enters via `NetObligations`, 2s deadlock guard) + `TestClearTradeConcurrentRace`.
- margincall: `TestEvaluateHandlerRunsOutsideLock` (re-enters via `GetActive`) + `TestEvaluateConcurrentRace` exercising both issue and resolve paths.
- settlement: `TestGetInstrumentConfigConcurrentWithRegister` (the exact map-read race), `TestRunSettlementCycleHandlerOutsideLock`, `TestRunSettlementCycleConcurrentRace`.

The deadlock tests assert real behavior (timeout-based liveness, re-entrant callbacks), not just "runs without error," and the race tests are meaningful only under `-race`. This is the right shape for concurrency tests.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Margin same-participant atomicity.** Moving `callService.Evaluate` outside `e.mu` is correct for avoiding cross-service deadlock, but it also drops the previous atomicity between caching the portfolio and evaluating its margin call. Two concurrent `CalculateMargin` calls for the *same* participant can now cache pm1 then pm2 but `Evaluate` them in either order, leaving the margin call reflecting a stale snapshot. `margincall.Evaluate` serializes on its own mutex so there's no race, only a benign ordering ambiguity — but if same-participant concurrent recalculation is expected, consider a per-participant lock or documenting that recalculation must be serialized upstream.

2. **margin-engine `engine` package coverage.** The `marginCallHandler` / `SetMarginCallHandler` path in `src/margin-engine/internal/engine` has no added concurrency test (only the margincall service is covered). The handoff defers this to R009 — acceptable, but worth folding in when R009 extends the race gate.

3. **Scope the branch.** Drop the CLAUDE.md / tasks.json / tasks-remediation.json edits from this branch (or land them via the postmortem/planner branch) to keep the concurrency fix isolated and conflict-free.

4. **CI `-race` gate.** Wire `go test -race` for these three modules into R014's CI gate so handler-in-lock / map-race regressions are caught automatically (already noted in the handoff).
