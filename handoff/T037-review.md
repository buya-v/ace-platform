APPROVED

# Review — T037: Warehouse Service Architecture Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec comprehensively covers the warehouse service domain: eWR lifecycle, facility registry, inventory management, delivery processing, quality inspection, and collateralization. Key observations:

- Receipt state machine is well-defined with clear transition rules and terminal states (CANCELLED, DELIVERED). The PLEDGED state correctly blocks transfers, deliveries, and cancellations.
- Double-entry inventory model is sound — quantities derived via aggregation rather than mutable balances, preventing drift.
- Facility capacity is computed (not stored), consistent with the inventory approach.
- Delivery workflow correctly integrates with the clearing engine's obligation model, including partial delivery via receipt splitting.
- Inspection-gated issuance enforces quality control at the data model level.
- Port assignment (50058/8088) follows the established convention.
- The protobuf definition matches the spec exactly — 21 RPCs, all enums use UNSPECIFIED=0, decimals as strings.
- SQL migration is consistent with the spec: 7 tables, 2 views, proper CHECK constraints, foreign keys, and indexes.
- The idempotency_keys table with TTL is a good addition for mutation safety.
- Receipt number format (`eWR-{FACILITY_CODE}-{YYYYMMDD}-{SEQ}`) is clearly specified.

Minor note: The spec mentions `idempotency_key` in gRPC metadata (section 14.1) but the protobuf doesn't include it as a message field — this is correct since it's metadata, not a request field.

### Security: PASS

- Database roles are properly scoped: `ace_warehouse_svc` gets SELECT/INSERT/UPDATE (no DELETE, consistent with append-only design), `ace_warehouse_ro` gets SELECT only.
- No hardcoded credentials — connection via Kubernetes secrets.
- Authorization rules are specified (holder must match for transfers, pledgee must authorize release).
- KYC checks are required for receipt issuance and transfer via compliance service integration.
- `SELECT ... FOR UPDATE` specified for capacity serialization to prevent race conditions.
- Receipt number uniqueness constraint prevents duplicate issuance.
- No SQL injection risk in the migration (no dynamic SQL).

### Code Quality: PASS

- Follows established project conventions: spec document + protobuf + SQL migration (same pattern as T007/T015/T035).
- Protobuf follows proto3 best practices: UNSPECIFIED=0 for all enums, string representation for decimals, Timestamp for time fields.
- SQL migration uses consistent naming, proper schema namespacing, and appropriate data types (NUMERIC(18,4) for quantities, matching the platform-wide Decimal pattern).
- Views for computed values (current_inventory, facility_utilization) are clean and correct.
- Handoff file is well-structured with clear follow-up instructions for T038.
- The spec document in `docs/` has consistent structure across all 14 sections with ASCII diagrams, tables, and code blocks.

### Test Coverage: PASS (N/A)

This is a specification task — no executable code to test. The spec provides sufficient detail for the implementation task (T038) to write tests against: state machine transitions, validation rules, capacity constraints, delivery workflow, and grading criteria are all explicitly specified with concrete values.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Idempotency key cleanup**: The spec mentions a 24-hour TTL on idempotency keys but doesn't specify the cleanup mechanism. T038 should implement a periodic cleanup job or use PostgreSQL's `pg_cron` extension.

2. **Receipt expiry enforcement**: The spec defines `expires_at` on receipts but doesn't specify what happens when a receipt expires. Consider adding an `EXPIRED` terminal state or documenting that expiry is handled by a batch job that transitions ACTIVE receipts to CANCELLED.

3. **Composite index on receipt_number**: The migration has a separate `idx_receipts_number` index, but `receipt_number` already has a UNIQUE constraint (which creates an implicit index). The explicit index is redundant — the spec's embedded SQL in section 10 includes it but the migration file correctly omits it.

4. **UpdateFacility partial updates**: The protobuf `UpdateFacilityRequest` doesn't use a `FieldMask` — all fields are sent on every update. For a spec task this is fine, but T038 may want to add `google.protobuf.FieldMask` to support partial updates cleanly.

5. **Event schema versioning**: The Kafka event schema (section 11.3) lacks a `schema_version` field. Adding one would make future schema evolution easier for downstream consumers.
