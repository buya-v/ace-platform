# Review — T053: Docker Compose Production Stack

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The compose file correctly defines all 13 services (4 infrastructure + 9 application). Key findings:

- **Port assignments** match the established convention from CLAUDE.md: matching=50051, clearing=50052, margin=50053, settlement=50054, auth=50055, compliance=50056, market-data=50057, warehouse=50058, gateway=8080.
- **Gateway health port** correctly set to 8090, matching `src/gateway/internal/config/config.go:47` (`envInt("HEALTH_PORT", 8090)`).
- **Dependency chain** is correct: kafka→zookeeper, all app services→postgres+kafka, clearing→matching, margin/settlement→clearing, gateway→all core services+auth.
- **Build contexts** all point to correct `src/<service>` directories with existing `cmd/<service>/main.go` entry points.
- **Dockerfiles** correctly distinguish auth-service (has `go.sum`, uses `go mod download`) from zero-dep services (skip both). All 8 new Dockerfiles follow the correct pattern for zero-dep Go modules.
- **DB migrations** mount to `docker-entrypoint-initdb.d` — note there are duplicate V8 migration files (`V8__market_data_timescaledb.sql` and `V8__warehouse_tables.sql`) in the migrations directory, which is a pre-existing issue not introduced by this task but will cause Flyway/postgres init ordering issues.

### Security: PASS

- **Non-root containers**: All Dockerfiles create and use `appuser` (UID 1000) — good.
- **CGO disabled**: All builds use `CGO_ENABLED=0` reducing attack surface.
- **Secrets handling**: JWT signing key and DB password use env vars with defaults clearly marked "change in production" in `.env.example`. The defaults are appropriate for local dev; production deployment would use proper secret management. No actual secrets are hardcoded.
- **Network isolation**: All services on a single bridge network (`ace-network`), which is appropriate for a compose dev stack. Production K8s deployment (T054) handles real network policies.
- **DB SSL disabled**: `DB_SSL_MODE: disable` is acceptable for local Docker networking but should be `require` in production.

### Code Quality: PASS

- **YAML anchors** (`x-common-env`, `x-healthcheck-http`) reduce duplication effectively across 9 service blocks.
- **Consistent structure**: Every service block follows the same pattern (build, depends_on with conditions, environment, ports, healthcheck, networks).
- **Naming convention** matches the established `-engine` / `-service` pattern.
- **`.env.example`** is well-organized with section headers and includes all referenced variables.
- **Compose version 3.9** is appropriate for the `depends_on: condition` syntax used.

### Test Coverage: PASS

The `tests/t053_docker_compose_test.sh` script performs 63 structural validation checks covering:
- File existence (compose + .env.example)
- All 13 services defined
- All 9 Dockerfiles exist
- Port mappings for all services
- Healthcheck block count (≥13)
- Dependency ordering (app→postgres, kafka→zookeeper, gateway→auth)
- Volume and network definitions
- Environment variable wiring (Kafka brokers, JWT key, DB host, service addresses)
- `.env.example` completeness
- Negative test: no frontend services included

The tests are structural (grep-based, no Docker required) which is the right approach — they validate the compose file's correctness without requiring a running Docker daemon.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Duplicate V8 migrations**: `infrastructure/db/migrations/` has both `V8__market_data_timescaledb.sql` and `V8__warehouse_tables.sql`. PostgreSQL `docker-entrypoint-initdb.d` runs files alphabetically, so both will execute, but this is fragile. Consider renaming one to V9 in a separate task.
2. **Gateway health port not exposed**: The gateway Dockerfile only `EXPOSE 8080` but the compose file maps port 8080 only. The health endpoint on port 8090 is not exposed in compose — the healthcheck uses `localhost:8090` which works inside the container, but external health probes (e.g., from a load balancer) won't reach it. Consider adding `"8090:8090"` to gateway ports.
3. **Redis unused**: Redis is provisioned but no service references it. The handoff correctly notes this — just confirming it's intentional placeholder infrastructure.
4. **`latest-pg15` tag**: Consider pinning to a specific TimescaleDB version (e.g., `2.13.1-pg15`) for reproducible builds.
