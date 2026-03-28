# Review — T007: Exchange Engine Architecture Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec comprehensively covers what the task asked for: CLOB architecture, price-time priority matching algorithm, price discovery (opening/closing auctions), trade ledger design, and API contracts. Specific strengths:

- The matching algorithm in Section 5 is correct: price-time priority with proper FOK pre-check, IOC partial cancellation, and MARKET order handling.
- The auction equilibrium price algorithm correctly maximizes volume, then minimizes imbalance, then ties to previous close.
- The protobuf definitions are well-structured and align with the spec's data model.
- The V6 migration correctly follows T004's append-only pattern (DELETE/UPDATE rules on execution_reports).
- The handoff file references T004 constraints and provides clear guidance for downstream tasks (T008, T027).

Minor note: The spec doc's Section 10 shows `execution_reports.order_id` with a FK reference to `exchange.orders`, but the actual V6 migration omits this FK. This is intentional — the migration comment says it avoids cross-table FK to keep the append-only pattern clean — but the spec and migration should be consistent. Non-blocking since this is a spec doc, not running code.

### Security: PASS

- No credentials or secrets hardcoded.
- Pre-trade validation includes authorization checks (participant permissions, account ownership).
- Self-trade prevention is specified with configurable modes.
- Rate limiting is specified (50 orders/sec per participant default).
- Admin operations (BustTrade, HaltInstrument, DisableParticipant) are in a separate AdminService — appropriate for RBAC separation.
- SQL migration grants least-privilege access via `ace_exchange_svc` role (SELECT+INSERT only on append-only tables, UPDATE allowed only on `circuit_breaker_events` for `resumed_at`).
- Price band validation and fat-finger protection prevent erroneous order injection.

### Code Quality: PASS

- Protobuf follows standard conventions: enum values prefixed with type name, `_UNSPECIFIED = 0` sentinel values, `go_package` option set correctly.
- SQL migration uses appropriate types (`NUMERIC(18,4)` for prices, `TIMESTAMPTZ`, `UUID`), CHECK constraints, and partial indexes (`WHERE trade_id IS NOT NULL`).
- Spec document is well-organized with clear sections, pseudocode for algorithms, and explicit scope boundaries.
- Instrument identifier convention (`WHT-HRW-2026M07-UB`) is practical and references T004 seed data.

### Test Coverage: PASS

This is an architecture spec task (`agent_role: architect`), not a code implementation task. No executable code was produced that requires tests. The protobuf definitions and SQL migration will be tested when T008 (Matching Engine) implements against them. This is the expected pattern for spec tasks.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **FK consistency between spec and migration**: The spec doc Section 10 shows `execution_reports.order_id REFERENCES exchange.orders(order_id)`, but the V6 migration omits this FK. Either add the FK to the migration or update the spec to note it's intentionally omitted. The omission is defensible (avoids locking on the orders table during high-throughput inserts), but should be documented.

2. **Missing FK on instruments**: `instruments.commodity_id` and `instruments.delivery_location_id` have no FK constraints to `reference.commodities` and `reference.delivery_locations`. The spec doc shows these FKs. Consider adding them since instruments are low-write/high-read and the referential integrity is valuable.

3. **V6 migration numbering**: The handoff assumes V6 follows V5, but this should be validated at migration time. If another task creates a V6 first, there will be a conflict. Consider noting this dependency explicitly.
