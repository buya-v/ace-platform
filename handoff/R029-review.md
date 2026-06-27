APPROVED

# Review — R029: Live e2e proof — a real gateway-submitted trade settles end-to-end across the bus

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The single source deliverable (`tests/kafka-e2e/gateway_seeded_test.go`,
`TestSeededGatewayTradeSettles`) does exactly what R027 asked for and what
R028's D1/D2/D3 fixes needed live verification of: it drives a **real,
authenticated, tenant-scoped order pair through the gateway** (no hand-set Kafka
payload, unlike the synthetic `TestCrossServiceTradePropagation`) and asserts the
resulting trade flows across the live broker through all four required hops.

Verified against the existing `propagation_test.go` (same `kafkae2e` package):

- **Symbol resolution is clean.** All referenced identifiers
  (`skipIfBrokerUnavailable`, `topicTradesExecuted`, `topicClearingNovated`,
  `topicMarginCallIssued`, `topicSettlementComplete`, `event`,
  `tradeExecutedPayload`) are defined in `propagation_test.go`. The new file's
  declarations (`seededTenant` vs the existing `tenant`, `gwDo`, `collector`,
  `newCollector`, `firstMatch`, `waitForMatch`, etc.) introduce **no collisions**.
  No unused imports (`bytes`, `sync`, `io`, `net/http` are all exercised).
- **Hop-correlation logic is sound and matches the documented payload shapes.**
  The decision to match `trades.executed`/`clearing.novated`/`margin.call-issued`
  by the **unique per-run participant ID** (present in those payloads) and
  `settlement.completed` by the server-assigned `trade_id` (re-keyed to
  `cycle-<tradeID>`, caught by containment) is correct — this avoids the false
  "HOP 3 BROKE" the handoff describes from a first cut that matched margin by
  `trade_id` (which the margin event does not carry).
- **`FirstOffset` (not `LastOffset`) is the right choice here** and is correctly
  justified: a brand-new consumer group reading "latest" races the producer on a
  fresh stack; reading from the start is race-free because the match token is a
  nanosecond-stamped per-run ID that cannot false-match prior-run residue.
- **D1 assertions are the meaningful ones**: non-empty buyer/seller participant
  IDs AND order IDs (`Fatalf`), plus equality-with-submitted (`Errorf`). The
  non-empty checks are the hard gate against the pre-R028 empty-ID regression;
  the equality check correctly records a failure while letting downstream hops
  run. Making `margin.call-issued` a REQUIRED hop (vs best-effort in the
  transport test) is justified: the freshly-generated zero-collateral
  participant guarantees a deficit → call for the seeded instrument, as the live
  log (`deficit=5025000`) confirms.
- **Goroutine lifecycle is correct.** `registerAndLogin` (which may `t.Skipf`) is
  called before collectors/ctx are created, so an auth-unavailable skip leaks
  nothing; later in-test skips (`order()` 502/503) run under `defer cancel()`,
  which stops the collector goroutines. Timeouts (75s ctx, 15s HTTP client) fit
  the observed 16.6s runtime.
- The integration run (`run-20260627-062805`) shows the test executed (not
  skipped) and passed with concrete evidence (`trade_id=id-33`, populated
  buyer/seller participant + order IDs, all four hops observed).

### Security: PASS

Test-only code. The hardcoded `SeededPass123!` is an ephemeral per-run test
credential (email is nanosecond-stamped), not a production secret. Passing
`participant_id` in the order body reflects pre-existing system behavior and is
not introduced by this test. No injection surface, no real credentials, no
authz bypass introduced.

### Code Quality: PASS

Follows the established conventions of the module precisely: graceful-skip
pattern (skips unless BOTH broker and gateway are reachable, keeping it CI-safe),
containment-based correlation matching, unique per-run tokens, and thorough
inline rationale for the non-obvious choices (`FirstOffset`, per-event match
tokens, in-network execution). Failure messages name the exact broken hop and
the likely cause, which makes this a genuine diagnostic, not a smoke test.
Correctly scoped to `tests/` + `handoff/` (Test Writer scope honored — no `src/`
changes), and the transient `.env.r029` port override was not committed.

### Test Coverage: PASS

This adds the precise high-value assertion the prior live-verification tasks
were missing: a **seeded, gateway-driven** end-to-end proof (rather than a
synthetic transport-only proof). It covers the critical economic-completion path
— populated participant/order IDs (D1), seeded margin params producing a real
call (D2), and full matching→clearing→margin→settlement propagation over a real
broker — and cannot pass vacuously in full-stack mode (every required hop
`Fatalf`s on absence; the match token is run-unique). It complements, rather
than duplicates, the existing transport test.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. The in-code hop ordering is 1 → 2 → 4 → 3 (settlement checked before margin).
   It's harmless (independent waits) and documented, but reordering to 1→2→3→4
   would read more naturally.
2. The exact-ID-match uses `t.Errorf` while the non-empty check uses `t.Fatalf`.
   The intent (hard-gate non-empty, soft-record exact match so downstream hops
   still run) is reasonable but would benefit from a one-line comment stating
   that deliberately.
3. Adopt the handoff's own follow-up: add a `trade_id` field to the
   `margin.call-issued` payload so the margin hop can be correlated to the
   originating trade without relying on the participant ID — improves both this
   test and operational tracing.
4. Wire the live fresh-stack run (`docker compose build` + fresh-volume `up` +
   in-network `tests/kafka-e2e`/`tests/e2e`) into the R014 CI gate, per the
   handoff — this is now hands-off reproducible and is the only tier that catches
   the Dockerfile-context / migration-ordering / cross-service-propagation defect
   classes.
