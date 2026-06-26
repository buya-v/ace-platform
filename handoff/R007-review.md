APPROVED

# Review — R007: Kafka fail-fast wiring

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The change does exactly what the deferred backlog item asked: it removes the
silent in-process Kafka fallback that cannot cross process boundaries, replacing
it with fail-fast (`log.Fatal`) behaviour when `KAFKA_BROKERS` is unset outside
of unit tests.

Verified:
- **Consistent logic across all 9 modules.** Each `NewProducerFromEnv` /
  `NewConsumerFromEnv` now: (1) returns the real Kafka adapter when
  `kafkaBrokersConfigured()` is true, (2) `log.Fatal`s when brokers are absent and
  `!testing.Testing()`, (3) returns the in-process channel adapter only under
  `testing.Testing()`. No inverted conditions, no dropped branches.
- **`kafkaBrokersConfigured()` is behaviour-preserving.** `strings.TrimSpace(os.Getenv(...)) != ""`
  is equivalent to the prior `brokers != "" && len(strings.TrimSpace(brokers)) > 0`
  (whitespace-only → not configured), and is now centralised + unit-tested.
- **matching-engine assertion is valid.** I confirmed `NewTradeProducer` returns
  `*ChannelProducer` (`src/matching-engine/internal/kafka/wiring.go:48`), so
  `TestNewProducerFromEnv_FailFastFallbackUnderTest`'s `p.(*ChannelProducer)` type
  assertion holds. matching-engine correctly reuses its existing exported
  constructor instead of introducing a duplicate `newInProcessProducer`.
- **securities-service exclusion is correct.** I confirmed
  `src/securities-service/internal/kafka/wiring.go` has no `*FromEnv` constructor —
  its `Producer` is passed in as a parameter (`PublishTradeExecuted(p Producer, ...)`),
  so there is no env-absence fallback to harden. The worker's scope note is accurate.
- **Test-mode signal choice is sound.** `testing.Testing()` (stdlib, Go 1.21+) is
  the correct guard: it is true only inside a `go test` binary and cannot be set by
  an operator. Choosing it over a `KAFKA_ALLOW_INPROCESS` env var avoids
  re-introducing the exact silent-drop foot-gun being removed.

The one genuine limitation — the production fatal path is unreachable from any test
binary because `testing.Testing()` is always true under `go test` (even in a
subprocess test harness) — is real and correctly documented rather than papered
over. This is inherent to the chosen mechanism, not a defect.

### Security: PASS

This is a reliability/correctness hardening, not a new attack surface. It closes a
silent-failure mode where a misconfigured production deployment would drop all
cross-service events (matching→clearing→margin→settlement) instead of refusing to
start. `log.Fatal` at startup wiring is the appropriate fail-closed response and is
explicitly sanctioned by the task. No secrets, no injection vectors, no
authn/authz surface touched. The fatal log messages are descriptive without
leaking sensitive data.

### Code Quality: PASS

- Clean, idiomatic, and uniform across all 9 services. The extraction of
  `kafkaBrokersConfigured()`, `newInProcessProducer()`, and `newInProcessConsumer()`
  gives the test-only adapter a clearly-named entry point rather than an inlined
  env-absence branch.
- Comments explain *why* (Go channels don't cross process boundaries) at each
  fatal/fallback site — appropriate density, matches surrounding style.
- The pre-existing no-brokers tests remain green unmodified, confirming the change
  is additive on the test path.

Minor (non-blocking) noise in the diff:
- gofmt struct-field realignments in `TradeExecutedPayload` /
  `SettlementCompletedPayload` (clearing/market-data/matching/settlement) are
  unrelated to R007 but are harmless gofmt cleanup.
- The branch diff also carries CLAUDE.md learned-pattern additions and
  `tasks.json` / `tasks-remediation.json` R018–R023 status edits that are *not*
  R007's work — these are a merge-base artifact (branch cut before those commits
  landed). See merge note below.

### Test Coverage: PASS

Each service gains a `failfast_test.go` covering the testable surface:
- `TestKafkaBrokersConfigured` — full env matrix (unset / empty / whitespace /
  single / multiple). Directly exercises the new predicate.
- `TestNewInProcessProducer` / `TestNewInProcessConsumer` — explicit constructors
  return usable, closeable adapters.
- `TestNewProducerFromEnv_FailFastFallbackUnderTest` (and consumer variant for
  gateway) — asserts the under-test fallback returns the in-process
  `*ChannelProducer` / `*ChannelConsumer`.

Coverage reported 63.8%–69.6% on the kafka packages. The asserted behaviour (the
predicate, the test-mode fallback type) is the behaviour that matters and is fully
covered. The fatal path is untestable by construction (documented), which is an
acceptable gap given the mechanism.

## Required Fixes (if REJECTED)

None — approved.

## Suggestions (non-blocking)

1. **`testing` import in production files** may trip linters (e.g. depguard /
   forbidigo) that forbid importing `testing` outside `_test.go`. If such a linter
   is added under R014's CI gate, add an allowlist entry or a short rationale
   comment so this doesn't get flagged later.
2. **Use `t.Setenv` instead of `os.Setenv`/`os.Unsetenv`** in the new tests.
   `t.Setenv` (Go 1.17+) auto-restores on cleanup and guards against accidental
   `t.Parallel`, avoiding any cross-test env pollution of `KAFKA_BROKERS`.
3. **Template the failfast wiring + test.** Per the project's validated
   template-duplication pattern for cross-cutting kafka concerns, fold the
   identical `kafkaBrokersConfigured` / `newInProcess*` helpers and
   `failfast_test.go` into the shared kafka template so the 9 copies don't drift.
4. **Merge hygiene:** confirm the branch is rebased onto current `main` before
   merge so the carried CLAUDE.md / `tasks.json` / `tasks-remediation.json` deltas
   (R018–R023 postmortem state) don't clobber newer main state — those edits are
   not part of R007's scope.
5. **Follow-through (already noted in the handoff):** ensure `KAFKA_BROKERS` is set
   in every service env across `deploy/` and `infrastructure/` so the new
   fail-fast aborts loudly on misconfig rather than silently — track under
   R008/R009 when `*FromEnv` is wired into `cmd/*/main.go`.
