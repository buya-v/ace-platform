APPROVED

# Review — R028: Economic end-to-end completion (drain D1/D2/D3)

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

All three sub-fixes were verified against the actual codebase, not just the handoff narrative.

**D1 — participant/order IDs (matching engine):** Confirmed sound.
- `types.Order.ParticipantID` (order.go:141) and `types.Trade.{Buyer,Seller}ParticipantID`/`{Buy,Sell}OrderID` (order.go:187–190) exist, and the matching loop already copies `incoming/resting.ParticipantID` and `OrderID` onto the trade (orderbook.go:474–502). The root cause was correctly diagnosed: `server.SubmitOrder` built `types.Order` without `ParticipantID`, so trades carried empty IDs and clearing novation rejected them. The fix populates `order.ParticipantID` (fallback `ParticipantID → AccountID`) and `orderbook.SubmitOrder` now assigns `order.OrderID = ob.idGen.NewID()` when empty (placed before `validateOrder`, consistent with the existing `idGen` usage throughout the package). **The novation guard was not weakened** — only the data feeding it was fixed, which is the correct approach.
- The gateway genuinely forwards `meta["x-participant-id"] = claims.ParticipantID` from the JWT (handler.go:69,149), so the engine reading `X-Participant-Id` matches the real upstream identity. Go's `Header.Get` canonicalizes case, so the `x-participant-id`→`X-Participant-Id` mapping is correct.
- The `handleSubmitOrder` resolution chain (header → body `account_id` → `X-User-Id` → `anonymous`, with `participantID` ultimately defaulting to `accountID`) guarantees both IDs are non-empty in every path, so novation cannot be re-broken by a degraded request.

**D2 — margin risk-param seeding:** Confirmed sound. `params.{Store,InstrumentParams,NewStore,Set,Get,DefaultScenarios}` all exist with the field names used (params.go:19–28, 85–105); `Set` auto-fills the 16 SPAN scenarios when `Scenarios` is empty. `types.{ParseDecimal,DecimalZero,DecimalFromInt}` are valid re-exports (decimal.go:18–22). Demo values reuse the proven-safe wheat numbers. Seeding is applied once at startup in `main.go` after `params.NewStore()`, fail-fast on a bad `MARGIN_RISK_PARAMS_FILE`.

**D3 — DLQ + cross-service topics:** Confirmed sound. The DLQ names added to `create-topics.sh` exactly match the consumer's `dlqTopic := TenantID + ".dlq." + topicWithoutPrefix(topic)` (consumer.go:164), with `TenantID == "ace-commodities"` (pinned by tenant_topics_test.go). `--if-not-exists` makes creation idempotent; the four app topics + four DLQ topics are a correct superset of what the matching/clearing/margin/settlement consumers route.

### Security: PASS
No new exposure. The participant identity is sourced from the gateway-set `X-Participant-Id` header, which the matching engine already trusted (alongside `X-User-Id`) prior to this change — the gateway remains the JWT trust boundary (R011). No secrets, no injection surface (seed JSON parsed with `encoding/json`, decimals validated at the boundary, `os.ReadFile` on an operator-supplied path only). R007 Kafka fail-fast and R008 concurrency invariants are untouched (seed runs once at startup, no new locking).

### Code Quality: PASS
Follows established conventions: the `internal/seed` package mirrors the existing collateral-source convention (in-memory default + env override) and correctly keeps instrument-specific data out of the SPAN business logic. Decimal values use the shared type at boundaries (`ParseDecimal`/`DecimalFromInt`). The `server.go` package-doc reformatting is a benign gofmt alignment. DLQ topics carry longer (90-day) retention with a clear rationale comment.

### Test Coverage: PASS
- **D1:** 4 meaningful tests — participant IDs on a matched trade (the core novation precondition), explicit `ParticipantID` precedence over `AccountID`, generated `OrderID` when omitted, and the HTTP path resolving the participant from `X-Participant-Id`. Assertions are specific (exact ID values), not "runs without error." Verified `tradeStore.Trades`, `RegisterInstrument`, and `newTestServer` all exist.
- **D2:** Thorough — default seed populates the store with scenarios, `FromEnv` default + file paths, `LoadFile` happy path, and 6 distinct error cases (missing file, invalid JSON, empty array, missing instrument_id, bad decimal, non-positive contract size), plus an example-config parse guard.
- **D3:** No automated test, which is acceptable and consistent with the existing untested `create-topics.sh`; live topic verification is correctly deferred to R029.

## Required Fixes (if REJECTED)
None.

## Suggestions (non-blocking)
1. **Auction path coverage gap.** The handoff reasons that the call-auction fill path inherits the generated `OrderID`/`ParticipantID` because it operates on orders already placed via `OrderBook.SubmitOrder`. This is plausible but untested — consider one auction-cross test asserting populated trade participant/order IDs to lock the invariant.
2. **Single-instrument demo seed.** The built-in default only covers `WHT-HRW-2026M07-UB`; a novated trade on any other instrument will fail margin calc on a fresh stack. This is the documented intent (production uses `MARGIN_RISK_PARAMS_FILE`), but worth a startup log line that enumerates seeded instruments so operators notice an unseeded instrument early.
3. **R029 seeded assertion.** Per the handoff's own follow-up: the live verification should drive a real gateway-submitted trade that does *not* hand-set `P-BUY`/`P-SELL` (unlike `tests/kafka-e2e`), to prove the D1 data flow rather than the transport.
4. **Production MSK topics.** There is no enumerated Terraform topic list; if topic creation moves into Terraform, mirror the `ace-commodities.*` and `ace-commodities.dlq.*` set there.
