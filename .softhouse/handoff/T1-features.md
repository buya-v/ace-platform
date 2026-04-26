# T1-features Handoff — Off-Book Confirmation, Node Hierarchy, CSV Bulk Import, Mass Amend

**Status:** DONE
**Agent:** Coder
**Build:** go build ./... — clean (securities-service + gateway)
**Tests:** go test ./... — all pass

---

## Summary of Changes

### Part A — Off-Book Trade Confirmation

**Files changed:**
- `internal/types/types.go` — Added `ConfirmedBy`, `RejectedBy`, `RejectionReason string` to `OffBookTrade` (omitempty JSON tags).
- `internal/store/store.go` — Extended `OffBookTradeStore` interface with `Confirm` and `Reject`; implemented on `InMemoryOffBookTradeStore`.
- `internal/server/handlers_off_book.go` — Added `handleConfirmOffBookTrade` and `handleRejectOffBookTrade`; updated `handleOffBookTrade` dispatcher to route `/confirm` and `/reject` sub-paths.
- `internal/server/server.go` — Added routes for confirm and reject.
- `src/gateway/cmd/gateway/main.go` — Registered all off-book routes including confirm and reject.

**API:**
- `POST /api/v1/securities/off-book-trades/{id}/confirm` — body `{"confirmed_by": "<actor>"}`.
- `POST /api/v1/securities/off-book-trades/{id}/reject` — body `{"rejected_by": "<actor>", "rejection_reason": "<reason>"}`. Both fields required.

---

### Part B — Node Hierarchy

**Files changed:**
- `internal/types/types.go` — Appended `Node` struct at end of file.
- `internal/store/store.go` — Added `NodeStore` interface and `InMemoryNodeStore` (with `SetPermissions` helper via type assertion). `GetEffectivePermissions` walks root→leaf, producing a deduplicated union.
- `internal/server/server.go` — Added `nodeStore` field, updated `New()` signature (inserted between `offBookTradeStore` and `locateStore`), registered `/nodes` routes.
- `internal/server/handlers_node.go` — New file with four handlers: list, create, get-permissions, put-permissions.
- `cmd/securities-service/main.go` — Passes `store.NewInMemoryNodeStore()`.
- `src/gateway/cmd/gateway/main.go` — Registered node routes.

**API:**
- `POST /api/v1/securities/nodes`
- `GET /api/v1/securities/nodes?firm_id=<id>`
- `GET /api/v1/securities/nodes/{id}/permissions` (effective/inherited)
- `PUT /api/v1/securities/nodes/{id}/permissions` (replace local permissions)

---

### Part C — CSV Bulk Import

**Files changed:**
- `internal/server/handlers_bulk.go` — Added `handleBulkInstrumentsCSV`; uses `encoding/csv`, case-insensitive header mapping.
- `internal/server/server.go` — Route `POST /api/v1/securities/bulk/instruments/csv`.
- `src/gateway/cmd/gateway/main.go` — Registered the CSV route.

---

### Part D — Mass Amend

**Files changed:**
- `internal/server/handlers_bulk.go` — Added `amendInstrumentItem`, `BulkAmendResult`, `handleBulkInstrumentsAmend`.
- `internal/server/server.go` — Route `POST /api/v1/securities/bulk/instruments/amend`.
- `src/gateway/cmd/gateway/main.go` — Registered the amend route.

---

## Test File Updates

All 30+ test files calling `server.New()` were updated with an extra `nil` for `nodeStore` (between `offBookTradeStore` and `locateStore`).

---

## Decisions Made

- `NodeStore.SetPermissions` is not in the interface — only on `InMemoryNodeStore`, called via type assertion. Production stores should add `UpdatePermissions` to the interface.
- `GetEffectivePermissions` uses an additive model (root first, no permission removal by inheritance).
- CSV `currency` defaults to `"MNT"` if empty, matching the JSON bulk endpoint.
- Mass amend uses zero-value semantics on `InstrumentUpdate` — omitted fields are no-ops.

---

## Suggested Follow-ups

- Add `NodeStore.UpdatePermissions(id string, perms []string) error` to the interface.
- Add `GET /api/v1/securities/nodes/{id}/children` handler (`ListChildren` already exists on store).
- Add unit tests for the four new feature groups.
- Move `BulkAmendResult` to `types` package if other packages need it.
