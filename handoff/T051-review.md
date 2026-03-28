APPROVED

# Review — T051: Kafka Event Wiring Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec comprehensively covers all 9 ACE platform services with accurate event flows. Key strengths:

- Event schemas correctly reference existing Go types (e.g., `types.Trade` in matching-engine, `types.ClearingObligation` in clearing-engine, `types.MarginCall` in margin-engine).
- Partition key choices are sound: `instrument_id` for trade-path topics (matching → clearing → settlement) ensures per-instrument ordering, `participant_id` for participant-scoped topics (margin calls, compliance, auth).
- Consumer group mapping is complete — every producer/consumer relationship from the flow diagram has a corresponding consumer group entry.
- The idempotency pattern correctly extends clearing-engine's existing `processedTrades` map pattern.
- DLQ design with 3-retry-then-dead-letter is a standard, well-understood pattern.

Minor observation: Section 6.4 notes the partition key for `ace.settlement.completed` is "`instrument_id` of the first settlement price (or `cycle_id` if multi-instrument)" — this ambiguity could cause inconsistent partitioning if not resolved before implementation. Non-blocking since this is a spec, but should be clarified before the implementation task.

### Security: PASS

- SASL_SSL with SCRAM-SHA-512 is appropriate for inter-service authentication.
- Per-service ACLs correctly restrict write access to owned topics only — no service can write to another service's topics.
- `auto.create.topics.enable=false` prevents accidental topic creation.
- `unclean.leader.election.enable=false` prevents data loss on broker failure.
- No hardcoded credentials in the spec; SASL credentials are referenced abstractly.
- The `ace.auth.user-registered` event schema includes `email` — implementers should ensure this topic has appropriate access controls since it contains PII. Non-blocking for a spec.

### Code Quality: PASS

- Well-structured document with clear table of contents, consistent formatting, and logical section ordering.
- Topic naming convention (`ace.{domain}.{event-type}`) is clean and consistent throughout.
- Consumer group naming convention (`{service-name}-{topic}`) is systematic and collision-free.
- The spec follows the project's spec-first pattern established by T007 and T015.
- Appendix A provides copy-pasteable topic creation scripts, which is practical.
- Schema evolution strategy (Appendix B) is concise and follows standard Kafka practices.

### Test Coverage: PASS (N/A)

This is a specification document — no code to test. The spec does include testable contracts (event schemas, partition keys, consumer groups) that downstream implementation tasks can verify against.

---

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Clarify settlement partition key**: Section 6.4's "instrument_id of the first settlement price (or cycle_id)" should pick one strategy before implementation. Recommendation: produce one message per instrument with `instrument_id` as key, rather than one message per cycle.
2. **PII in auth events**: Note that `ace.auth.user-registered` contains email addresses. Implementation tasks should ensure this topic's retention and access are handled per data protection requirements.
3. **Dedup map memory bound**: The 100,000 event ID in-memory dedup set is reasonable but the spec should note that services with multiple consumer groups need one dedup set per group (not shared), or the cap may be hit prematurely.
4. **DLQ replay command**: The Appendix replay command using `kafka-console-consumer | kafka-console-producer` will strip the original partition key. The implementation task for the replay tool should preserve partition keys.
