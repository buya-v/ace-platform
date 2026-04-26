# Handoff: T2 — Sprint 8 Test Writer

## Changes Made

### Store tests — appended to `src/securities-service/internal/store/store_test.go`
- `TestIndexStore_CRUD` — 10 sub-tests: create, get, get-non-existent (ErrNotFound), list, update, update-non-existent, delete, second-delete, copy isolation
- `TestEntityPermissionStore_SetGet_ListByRole` — 6 sub-tests: set+get, set-overwrites, get-non-existent, listByRole (2 roles + empty), delete, delete-non-existent
- `TestFolderStore_Create_ListChildren` — 8 sub-tests: create+get, duplicate, get-non-existent, list, listChildren (root→2 children, child→grandchild, unknown parent), delete, copy isolation
- `TestWarningStore_Create_List_Acknowledge` — 5 sub-tests: create+list(false), acknowledge+filter-split, ack-non-existent (ErrNotFound), duplicate create, empty store

### Handler tests — new files in `src/securities-service/internal/server/`

**`handlers_index_test.go`**
- `TestCreateIndex` — 5 sub-tests: 201 happy, auto-id, custom-id, 400 bad JSON, 409 duplicate
- `TestListIndices` — seeds 3 via store, verifies ≥3 returned
- `TestGetIndex` — 200 correct fields, 404 unknown
- `TestDeleteIndex` — 204, GET→404 after, 2nd DELETE→404, unknown→404
- `TestCalculateIndex` — 200 updated current_value + last_calculated_at, 404 unknown, JSON round-trip into `types.Index`
- `TestIndexHandlers_Unconfigured` — verifies 503 on all 5 routes when indexStore is nil

**`handlers_entity_permission_test.go`**
- `TestSetEntityPermission` — 5 sub-tests: 200 happy, overwrite, 400 missing role_id, 400 missing entity_type, 400 bad JSON
- `TestListByRole` — seeds 3 perms (2 for role-x, 1 for role-y); verifies role-x=2, role-y=1, unknown=0, missing param=400
- `TestDeleteEntityPermission` — 204 success, list-after-delete=0, 2nd DELETE=404, unknown=404, invalid path=400
- `TestEntityPermissionHandlers_Unconfigured` — 503 on all 3 routes when store is nil

**`handlers_folder_test.go`**
- `TestCreateFolder` — 7 sub-tests: 201, auto-id, custom-id, child with parent_id, 400 missing name, 400 bad JSON, 409 duplicate
- `TestListFolders` — seeds 3 via store, verifies ≥3 returned
- `TestGetFolder` — 200 correct fields, 404 unknown
- `TestDeleteFolder` — 204, GET→404, 2nd DELETE→404, unknown→404
- `TestListFolderChildren` — root→2 children, child1→grandchild, leaf→0, unknown parent→0 (all 200)
- `TestFolderHandlers_Unconfigured` — 503 on all 5 routes when store is nil

**`handlers_warning_test.go`**
- `TestListWarnings` — default lists unacked, acknowledged=false, acknowledged=true=0 initially, 405 on POST
- `TestAcknowledgeWarning` — 204 success, appears in acknowledged=true list, absent from unacked list, X-User-ID header recorded in acknowledged_by, 404 unknown, 404 invalid item path, 405 wrong method on /acknowledge
- `TestWarningHandlers_Unconfigured` — 503 on GET /warnings and POST /warnings/{id}/acknowledge when store is nil

## Decisions

- Followed the exact `newXxxTestServer(t)` pattern from `handlers_role_test.go` for each new feature: fresh stores, `New()` call with nil-padded args, `SetXxxStore()` after construction, `SetReady()`, register routes, wrap with TenantMiddleware.
- Store tests follow the `TestXxx_SubTestName` style from existing store_test.go (e.g., `t.Run("Create and Get", ...)`)
- `_Unconfigured` tests for each handler group verify 503 responses when the relevant store is left nil, matching `TestRoleHandlers_Unconfigured`.
- Used `seedWarnings` / direct store seeding for handler tests that need pre-existing data without going through HTTP.
- `TestCalculateIndex/JSON_round-trip_via_types.Index` decodes the response into `types.Index` to validate all JSON struct tags are correctly wired.

## Tests

```
go test ./... -race -count=1
```

Result: ALL PASS (0 failures, -race clean)

Package breakdown:
- `internal/store` — ok (includes 4 new TestXxx functions, 29 new sub-tests)
- `internal/server` — ok (includes 4 new test files, ~60 new sub-tests)
- All existing tests continue to pass unchanged

New test count (new files + appended):
- Store: +4 top-level tests, ~29 sub-tests
- Server handlers: +4 files, ~60 sub-tests

## Blockers

None.

## Follow-ups

- T3 (if planned): Coverage report — new handler and store code is well covered by these tests; `handleCalculateIndex` with a live instrument store (TickSize price proxy) is exercised by `TestCalculateIndex/returns_200_with_updated_current_value`.
- Consider adding `TestCalculateIndex/with_seeded_instrument` sub-test that pre-seeds an instrument store with a known TickSize to verify the weighted calculation numerically.
- `TestWarningHandlers_Unconfigured` does not test `POST /warnings` (405 before nil-store check) — this is intentional since the method check runs before store nil-check; documented in test comment.
