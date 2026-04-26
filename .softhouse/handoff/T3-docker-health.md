# T3 Handoff — Docker Compose Health Check & Fix

**Date:** 2026-04-26
**Status:** COMPLETE
**Agent:** Softhouse Coder (T3)

---

## Summary

All four key services were running when the task started. The gateway and securities-service had been built from stale images (Apr 25) while Sprint 8 source code changes (demo reset endpoint, schema additions) were committed on Apr 26. Rebuilt both images and fixed a database schema gap that caused securities instrument queries to fail.

---

## Container Status at Task Start

| Service | Port | Status |
|---|---|---|
| gateway | 8080 | Up (healthy) — stale image |
| securities-service | 8089 (API), 9089 (health) | Up (healthy) — stale image |
| auth-service | 8085 | Up (healthy) |
| demo-runner | 13002 (ext) / 3002 (int) | Up (healthy) |

All 19 services were running. No service was down at start.

---

## Issues Found

### 1. Gateway image was stale — missing demo reset route

The `POST /api/v1/securities/demo/reset` route was added to `src/gateway/cmd/gateway/main.go` in commit `69bc844` (2026-04-26 14:47). The running gateway container was built on 2026-04-25. The route returned 404 via the gateway (the underlying securities-service had the route, but the gateway didn't proxy it).

**Fix:** `docker compose build gateway securities-service && docker compose up -d gateway securities-service`

### 2. Database schema `ace_securities` missing

After rebuild, the securities-service connected to PostgreSQL (via `DATABASE_URL` env var) and returned `ERROR: relation "ace_securities.instruments" does not exist`. The database volume was created before migrations V26–V30 were applied. The `docker-entrypoint-initdb.d/` init scripts only run on first DB initialization.

**Fix:** Manually applied migrations V26–V28 (create `securities` schema + tables) and V30 (rename `securities` → `ace_securities`):

```bash
docker compose exec -T postgres psql -U garudax -d garudax -f /docker-entrypoint-initdb.d/V26__securities_instruments.sql
docker compose exec -T postgres psql -U garudax -d garudax -f /docker-entrypoint-initdb.d/V27__securities_trading.sql
docker compose exec -T postgres psql -U garudax -d garudax -f /docker-entrypoint-initdb.d/V28__securities_settlement.sql
docker compose exec -T postgres psql -U garudax -d garudax -f /docker-entrypoint-initdb.d/V30__ace_schema_renames.sql
```

V30 also attempts to rename other schemas (ace_exchange, ace_clearing, etc.) but those migrations (V1–V9) were not applied to this DB volume. The errors for those renames are pre-existing gaps unrelated to this task. The critical rename (`securities` → `ace_securities`) succeeded.

---

## Verification Results

### Health endpoints

| Endpoint | Expected | Result |
|---|---|---|
| `GET http://localhost:8080/healthz` | `{"service":"ace-gateway","status":"ok"}` | PASS |
| `GET http://localhost:9089/healthz` | `{"status":"ok","service":"securities-service","database":"ok"}` | PASS |
| `GET http://localhost:8085/healthz` | `ok` | PASS |
| `GET http://localhost:13002/` | HTTP 200 (SPA HTML) | PASS |

### Functional endpoints

| Endpoint | Result |
|---|---|
| `POST http://localhost:8080/api/v1/securities/demo/reset` (with `X-GarudaX-Tenant: mse-equities`) | `{"message":"All securities data cleared","status":"reset"}` — PASS |
| `GET http://localhost:8089/api/v1/securities/instruments` (with `X-GarudaX-Tenant: mse-equities`) | `{"total":0,"data":[],...}` — PASS (empty after reset) |

---

## Files Changed

No source files were modified. Changes were:

1. **Docker images rebuilt:** `ace-platform-gateway` and `ace-platform-securities-service` (new images from Sprint 8 source)
2. **Database migrations applied:** V26, V27, V28, V30 (applied to running postgres container's volume)

---

## Key Decisions

1. **Did not rebuild other services** — Only gateway and securities-service were stale per the task spec. The other 17 services were not touched.

2. **Applied DB migrations manually, did not modify docker-compose.yml** — The correct fix is to run the migrations (schema must exist for the service to work), not to remove the `DATABASE_URL` env var. The securities-service PG stores are production-path code that should be wired correctly.

3. **V30 partial failure is pre-existing** — V30 fails on other schema renames (`ace_exchange`, `ace_clearing`, etc.) because V1–V9 were never applied to this DB volume. This is a pre-existing state issue, not introduced by this task.

---

## Suggested Follow-ups

- **Migration runner**: The platform needs a proper migration runner (Flyway, golang-migrate, or custom script) that applies pending migrations on service startup or via an explicit `make migrate` target. The current `docker-entrypoint-initdb.d` approach only works on fresh DB volumes.
- **Missing DB schemas**: Migrations V1–V9 (creating `exchange`, `clearing`, `participants`, etc. schemas) were never applied to the current DB volume. Services that depend on those schemas (matching-engine, clearing-engine, etc.) are using in-memory stores. This works for demo purposes but should be tracked as tech debt.
- **Health endpoint port clarity**: The securities-service exposes health on port 9089 (not 8089). The `GET /healthz` on port 8089 returns 404. Any monitoring infrastructure should point to port 9089 for health checks.
