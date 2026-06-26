# Review — R024: Implement real cross-service Kafka event propagation

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent
**Date:** 2026-06-26

---

## Evaluation

### Correctness: PASS

The change does exactly what the task asks: it wires the already-existing real
wire-protocol Kafka adapters (`KafkaProducer`/`KafkaConsumer`, selected by
`NewProducerFromEnv`/`NewConsumerFromEnv` when `KAFKA_BROKERS` is set per R007)
into the four engine binaries via a per-engine `internal/eventbus` composition
package, closing the historical "engines never call `internal/kafka` from
`cmd/main.go`" gap (bug class #3).

I independently verified the integration points that determine whether this
actually propagates across processes:

- **Topic strings match across all modules.** This is the single highest-risk
  item for cross-process wiring, and it checks out. Matching publishes
  `TopicTradesExecuted` (`TenantID + ".trades.executed"`, TenantID =
  `ace-commodities`); clearing/margin/settlement define the same topics as
  literal `"ace-commodities.*"` strings (`config.go:12-15` in each). So
  matching→`ace-commodities.trades.executed`→clearing,
  clearing→`ace-commodities.clearing.novated`→{margin, settlement}, and the
  terminal `settlement.completed`/`margin.call-issued` all align byte-for-byte.
- **Fan-out is correct.** Clearing publishes one `clearing.novated`; margin and
  settlement each consume it under distinct consumer-group IDs (`ServiceName` =
  `margin-engine` vs `settlement-engine`), so both receive every novation rather
  than competing for partitions. Correct.
- **Engine signatures match exactly:** `ClearTrade(types.Trade)
  (*ClearingResult, error)`, `CalculateMargin(string, []types.Position)`,
  `RunSettlementCycle(string, time.Time, []types.Position)`,
  `SetSettlementPrice`, `SetMarginCallHandler`, `SetCycleHandler` — all present
  with the used shapes. `ParseDecimal`, `MulUint64`, `DecimalZero`, `NewDecimal`
  resolve through the shared decimal module / type aliases.
- **Single-handler composition** for margin/settlement (engines support one
  `MarginCallHandler`/`CycleHandler`, last-write-wins) is handled correctly by
  exposing `PublishMarginCall`/`PublishCycle` and composing persist+log+publish
  in `main.go`, rather than the Runtime clobbering the existing handler.
- **R008 snapshot-after-unlock preserved:** all publish calls fire from handlers
  the engines already invoke outside their locks; worker ran `go test -race` on
  all four engines.
- **Idempotency:** deterministic downstream keys (`cycle-<tradeID>`) and the
  inherited consumer `processedIDs` dedupe make reprocessing safe.

Semantic simplifications are documented and acceptable for a propagation-wiring
task (margin/settlement reconstruct positions per-trade, not per-portfolio;
settlement marks a fresh novation at its own fill price → zero same-day
variation). These do not affect the propagation contract.

### Security: PASS

No secrets, no injection surface. All inbound Kafka data is parsed into typed
payload structs; parse failures are handled rather than panicking. Bridges
return `nil` (commit + drop) for permanently-malformed records to avoid
poison-pill retry storms, and `err` (retry → inherited DLQ/retry policy) for
transient engine/publish failures — a sound fail-handling split. The R007
fail-fast is preserved: `eventbus` only calls the `*FromEnv` constructors when
`Enabled()` (= `KAFKA_BROKERS` set) is true, and those constructors still
`log.Fatal` outside tests, so there is no silent in-process fallback in a
multi-process deployment.

### Code Quality: PASS

Follows the codebase's established idioms: composition-root `internal/eventbus`
package keeps the `kafka` package free of an `engine`/`types` dependency and
provides an injectable test seam (`newRuntime`/`NewPublisherWith`); the
graceful-skip cross-process e2e test follows the documented pattern; the four
near-identical `enabled.go` files match the accepted template-duplication
convention. Good doc comments explaining the why (single-handler composition,
drop-vs-retry, R007/R008 interactions). The dead `MATCHING_ENGINE_ADDR` no-op in
clearing `main.go` is correctly removed.

### Test Coverage: PASS

Each `eventbus` package has meaningful unit tests asserting real behavior, not
just "runs without error": direct-bridge publish with payload/topic/field
assertions, bad-price-dropped (no downstream emitted), and an end-to-end
in-process channel bridge (consume→process→publish). Matching additionally
covers publish-error-does-not-panic, trade-type mapping, and `Enabled()`
env-gating. The cross-process `tests/kafka-e2e` module asserts required
`clearing.novated`/`settlement.completed` propagation correlated by ID, with
`margin.call-issued` correctly treated as best-effort.

Note (not a blocker): the e2e test SKIPs without a broker, so the *actual*
cross-process propagation is not exercised by the unit/CI tier — the in-process
channel bridge tests cover the bridge logic, and live verification is an
integration-agent step. The worker is honest about this: the claim that bug
class #3 is "FIXED" only fully holds once the integration agent runs
`tests/kafka-e2e` against the live stack (handoff follow-up #1).

## Required Fixes (if REJECTED)

None.

## Suggestions (non-blocking)

1. **Startup ordering race (margin & settlement `main.go`):** the consumer
   goroutine (`rt.Start`) is launched before `SetMarginCallHandler` /
   `SetCycleHandler` is composed. An event arriving in that startup window could
   be processed with the default (non-publishing) handler. The gap is effectively
   nil in practice, but moving the handler composition *before* `rt.Start(ctx)`
   would close it deterministically.
2. **Inconsistent drop logging:** clearing's bridge logs un-parseable trades
   before dropping; margin/settlement bridges silently `return nil` on parse /
   missing-participant. Add a `log.Printf` for symmetry, and consider routing
   genuinely-poison records to the DLQ rather than silently committing them.
3. **Stale topic comments (pre-existing, out of scope):** margin/settlement
   `wiring.go` header comments say `ace.clearing.novated` while the constants are
   `ace-commodities.clearing.novated`. Not introduced by this diff, but worth a
   cleanup follow-up so the comments don't imply a mismatch that doesn't exist.
4. **Live verification gating:** confirm with the integration agent that
   `tests/kafka-e2e` is run against the full `docker compose` stack (and that the
   e2e instrument is registered/seeded in the running matching-engine) before
   declaring bug class #3 retired. This depends on R025 (clean DB init) and R026
   (buildable images from `./src`) landing first, as the handoff notes.
5. The defined-but-unbridged back-edges (warehouse→margin `receipt-pledged`,
   settlement→clearing) are reasonable to defer; track them when those flows are
   exercised end-to-end.
