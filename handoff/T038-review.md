# Review — T038: Warehouse Service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation faithfully covers the core warehouse receipt lifecycle specified in T037:
- Inspection-gated receipt issuance (requires PASSED inspection)
- Receipt status transitions: ACTIVE -> PLEDGED -> ACTIVE, ACTIVE -> DELIVERY_PENDING -> DELIVERED/ACTIVE
- Lot uniqueness enforcement per facility/commodity/lot
- Double-entry inventory via event sourcing (deposit on issuance, withdrawal on cancel/delivery)
- Facility capacity enforcement at receipt creation time
- Delivery failure correctly reverts receipt to ACTIVE
- Collateralization blocks transfer and cancel while pledged
- Release validates clearing member ID matches

State machine transitions are correctly guarded — you cannot transfer/cancel a PLEDGED receipt, cannot pledge a non-ACTIVE receipt, cannot deliver a CANCELLED receipt.

The `usedCapacityLocked` function correctly counts ACTIVE, PLEDGED, and DELIVERY_PENDING receipts toward used capacity.

One minor note: `main.go:39` logs `srv.GRPCAddr()` (the bind address) instead of `lis.Addr()` for the health server portion of the log message — cosmetic, not a bug.

### Security: PASS

- No SQL injection risk — in-memory store with Go maps, and the handoff explicitly notes the SQL-ready interface pattern for future parameterized queries.
- No hardcoded secrets or credentials.
- Input validation at service boundary: all required fields checked, quantity validated as positive, capacity validated before issuance.
- Thread-safety via sync.RWMutex on all store operations with appropriate lock usage (RLock for reads, Lock for writes).
- The `atomic.AddUint64` for ID generation is safe under concurrent access (used outside the mutex for counter increment, but within mutex for map writes — acceptable since IDs only need uniqueness, not strict ordering).

No authorization layer exists, but this is expected for phase 3 — the service layer is a business logic core, and auth would be enforced at the gRPC interceptor or gateway level.

### Code Quality: PASS

- Follows the established zero-dependency Go module pattern from other ACE services.
- Clean separation: `types` (domain models) -> `store` (persistence) -> `service` (business logic + validation) -> `server` (transport).
- Port convention respected: gRPC 50058, health HTTP 8088.
- Copy-on-read pattern in store (returns `copy := *r`) prevents mutation of internal state — good defensive practice.
- Request types are cleanly separated from domain types.
- Naming is consistent with other services in the platform.
- No unnecessary complexity or dead code.

### Test Coverage: PASS

Comprehensive test suite with 40+ test functions covering:
- **Facility**: registration, duplicate code rejection, validation of all required fields, update, listing with filters.
- **Inspection**: scheduling, recording results (pass/fail), preventing double-completion.
- **Receipt lifecycle**: issuance, transfer, cancel, lot uniqueness, capacity check, inspection gating.
- **Collateralization**: pledge, release, wrong-member rejection, pledge of non-active receipt.
- **Delivery**: success flow, failure with revert, cancelled-receipt rejection, listing.
- **Inventory**: tracking after issuance, after cancel, after delivery.
- **Capacity**: used/available computation.
- **Full lifecycle**: end-to-end test covering inspect -> issue -> transfer -> pledge -> release -> deliver.
- **Store-level tests**: duplicate the service-level scenarios but test the store directly.
- **Server tests**: health/ready endpoints, config defaults, env override.

Coverage numbers from handoff (83.8% store, 70.7% service, 47.1% server) are appropriate — business-critical store and service layers are well-covered.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Server test duplicates handler logic**: `server_test.go` recreates handler closures inline rather than testing the actual `StartHealthServer` mux. Consider using `httptest.Server` with the real mux for higher-fidelity tests.

2. **Missing spec features noted in handoff**: Partial delivery / receipt splitting (mentioned in T037 spec) and idempotency keys are not implemented. The handoff correctly documents these as follow-ups — acceptable for phase 3.

3. **Atomic ID generation outside mutex**: `nextID()` uses `atomic.AddUint64` which is called both inside and outside the mutex (e.g., `appendReceiptEventLocked` calls `s.nextID()` while holding the lock). This works but is slightly inconsistent — the atomic is redundant when the mutex is held. Minor, no functional impact.

4. **Delivery from PLEDGED receipt**: `CreateDelivery` allows delivery initiation on PLEDGED receipts (`r.Status == types.ReceiptStatusPledged`). The T037 spec says pledge blocks transfer/cancel but is ambiguous about delivery. Consider whether delivery should require release first.
