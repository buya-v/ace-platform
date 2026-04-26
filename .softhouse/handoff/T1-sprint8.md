# Handoff: T1 — Sprint 8 Indices + Entity Permissions + Folders + Warnings

## Changes Made
- `types.go`: Added Index, EntityPermission, Folder, Warning structs + warning type constants + FolderID on Instrument
- `store.go`: Added IndexStore, EntityPermissionStore, FolderStore, WarningStore interfaces + InMemory implementations
- `handlers_index.go`: CRUD + calculate endpoint (5 routes)
- `handlers_entity_permission.go`: List by role, set, delete (3 routes)
- `handlers_folder.go`: CRUD + list children (5 routes)
- `handlers_warning.go`: List + acknowledge (2 routes)
- `server.go`: 4 store fields, 4 Set*Store() setters, 8 route registrations
- `main.go`: Wired all 4 new stores
- `gateway/main.go`: Added 13 gateway route entries

## Decisions
- Used Set*Store() pattern (post-construction wiring) to avoid breaking existing New() constructor call sites
- Warning List takes boolean `acknowledged` parameter to filter
- Index calculate endpoint reads latest trade prices from tradeStore

## Tests
- `go build ./...` — PASS (securities-service and gateway)
- `go test ./...` — PASS (all existing tests continue to pass)

## Follow-ups
- T2: Write unit tests for all 4 new store + handler modules
