APPROVED

# Review — R027: Live fresh-stack verification — confirm cross-service Kafka propagation (bug class #3) FIXED

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

This is a live deploy-tier *verification* task. The only committed code change is a one-line
match relaxation in `tests/kafka-e2e/propagation_test.go` (`e.CorrelationID == want` →
`strings.Contains(e.CorrelationID, want)`), plus a thorough explanatory comment. The change is
correct and well-justified:

- I confirmed `strings` is already imported (`propagation_test.go:27`), so the change compiles
  with no import churn.
- The rationale holds against the codebase's documented behavior: `want` is the unique
  nanosecond token `r024-e2e-<UnixNano>` (line 130). The settlement engine re-keys its downstream
  event with the deterministic idempotency key `cycle-<tradeID>` (the validated R024 pattern in
  CLAUDE.md) and carries that as `correlation_id` on `settlement.completed`, so a strict `==`
  match misses a genuinely-propagated event. Containment on the unique token is the right fix.
- No false-positive risk: each `required` topic has its own watcher reading only that topic
  (lines 159–160), and the token is nanosecond-unique, so containment cannot match an unrelated
  event. The worker proved the root cause directly on the broker before changing the assertion
  (not papering over a failure), which is the correct discipline.
- The verification methodology is genuinely end-to-end and matches what bug class #3 requires:
  image build from committed Dockerfiles (R026 confirmed), fresh-volume DB init (R025 confirmed),
  `kafka-topics --list` showing all four `ace-commodities.*` topics, real producer/consumer log
  lines on all four engines, `tests/kafka-e2e` PASS on the live broker, and `tests/e2e` PASS
  (32/0/8-skip) through the live gateway with lifecycle + tenant enforcement holding.

The verdict is appropriately scoped. Bug class #3 is a *transport/propagation* defect ("channel
stubs don't bridge separate processes"); the worker confirms transport works while explicitly
NOT overclaiming economic settlement. The newly-surfaced D1/D2/D3 (empty participant/order IDs
from matching, unseeded margin risk params, missing DLQ topics) are correctly classified as
downstream data/config/topic gaps — not propagation defects and not R024 regressions — and
filed as a follow-up (R028). This is an honest, accurate verdict, not an inflated one.

### Security: PASS

No security surface is touched. The host-port collision was resolved with an *ephemeral* compose
override in `/tmp` that remaps host-published ports only; internal `postgres:5432` / `kafka:9092`
service networking is untouched and no committed file (`docker-compose.yml`) was modified. Tenant
enforcement was re-verified live (`TestTenantEnforcement_*` PASS), preserving the platform
multi-tenancy invariant. The change is test-only and introduces no injection/authz/secret risk.

### Code Quality: PASS

The diff is minimal and disciplined — exactly one assertion change in a test module, plus a
high-quality comment that explains the *why* (the R024 `cycle-<tradeID>` re-keying), not just the
*what*. It follows existing conventions in the file. No committed non-test files were changed, in
keeping with the verification scope. The handoff and integration report are detailed, cite
concrete evidence (engine log lines, topic listings, test output), and include a reproduction
recipe.

### Test Coverage: PASS

The change makes the cross-process propagation test assert *real* behavior rather than an
over-strict correlation encoding, which strengthens (not weakens) the test's meaning. The
required-vs-best-effort watcher split (clearing.novated + settlement.completed required;
margin.call-issued best-effort) is sound and avoids conflating "no margin call issued" with "did
not propagate." The Go unit suite and `-race` tier were not re-run, which is justified: engine
source is unchanged from `main` (only a test file in a separate `tests/` module changed), and
both were green at the R016 baseline / `run-20260626-033236`.

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Pre-create the four `ace-commodities.*` topics in the kafka-e2e setup.** The worker noted
   the watchers create readers on topics that may not exist until first publish
   (`settlement.completed` / `margin.call-issued`), which made the first run flaky. Pre-creating
   topics (or asserting their existence) in the test fixture would make the suite deterministic.
2. **Schedule R028 to drain D1/D2/D3** so a real gateway-submitted trade settles end-to-end:
   D1 — populate participant/order IDs on the matched `types.Trade` before publish in
   matching-engine (clearing novation rejects empty IDs); D2 — seed margin risk params for
   `WHT-HRW-2026M07-UB` in the deployed margin-engine; D3 — pre-create `ace-commodities.dlq.*`
   topics so exhausted-retry events are dead-lettered rather than dropped. These are genuine
   platform gaps but correctly out of scope for R027 (they don't affect the bug-class-#3 verdict).
3. **Wire a non-pushing `docker compose build` + fresh-volume init smoke into the R014 CI gate.**
   This defect class (build-context / migration-ordering / image drift) is exactly the R015 root
   cause and is cheap to gate at PR time rather than discovering it days later at integration.
