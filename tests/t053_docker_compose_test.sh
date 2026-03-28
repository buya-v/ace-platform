#!/usr/bin/env bash
# Tests for T053 — Docker Compose Production Stack
# Validates structure, dependencies, health checks, ports, and networks.

set -euo pipefail

COMPOSE_FILE="$(cd "$(dirname "$0")/.." && pwd)/docker-compose.yml"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  FAIL: $1"; }

check() {
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then pass "$desc"; else fail "$desc"; fi
}

check_grep() {
  local desc="$1" pattern="$2"
  if grep -qE "$pattern" "$COMPOSE_FILE" 2>/dev/null; then pass "$desc"; else fail "$desc"; fi
}

echo "=== T053 Docker Compose Validation ==="
echo

# ─── File existence ──────────────────────────────────────────────────
echo "--- File existence ---"
check "docker-compose.yml exists" test -f "$COMPOSE_FILE"
check ".env.example exists" test -f "$(dirname "$COMPOSE_FILE")/.env.example"

# ─── Infrastructure services ────────────────────────────────────────
echo "--- Infrastructure services ---"
for svc in postgres redis zookeeper kafka; do
  check_grep "$svc service defined" "^  ${svc}:"
done
check_grep "postgres uses timescaledb" "timescale/timescaledb:latest-pg15"
check_grep "redis uses alpine" "redis:7-alpine"
check_grep "kafka uses confluent" "confluentinc/cp-kafka:7.5.0"
check_grep "zookeeper uses confluent" "confluentinc/cp-zookeeper:7.5.0"

# ─── Application services ───────────────────────────────────────────
echo "--- Application services ---"
APP_SERVICES=(matching-engine clearing-engine margin-engine settlement-engine auth-service compliance-service market-data-service warehouse-service gateway)
for svc in "${APP_SERVICES[@]}"; do
  check_grep "$svc service defined" "^  ${svc}:"
done

# ─── Dockerfiles exist ──────────────────────────────────────────────
echo "--- Dockerfiles ---"
SRC_DIR="$(dirname "$COMPOSE_FILE")/src"
for svc in "${APP_SERVICES[@]}"; do
  check "$svc Dockerfile exists" test -f "$SRC_DIR/$svc/Dockerfile"
done

# ─── Port mappings ───────────────────────────────────────────────────
echo "--- Port mappings ---"
declare -A EXPECTED_PORTS=(
  [matching-engine]="50051:50051"
  [clearing-engine]="50052:50052"
  [margin-engine]="50053:50053"
  [settlement-engine]="50054:50054"
  [auth-service]="50055:50055"
  [compliance-service]="50056:50056"
  [market-data-service]="50057:50057"
  [warehouse-service]="50058:50058"
  [gateway]="8080:8080"
)
for svc in "${!EXPECTED_PORTS[@]}"; do
  port="${EXPECTED_PORTS[$svc]}"
  check_grep "$svc port $port" "\"${port}\""
done

# ─── Health checks ──────────────────────────────────────────────────
echo "--- Health checks ---"
# Count healthcheck blocks — should be at least 13 (4 infra + 9 app)
HC_COUNT=$(grep -c "healthcheck:" "$COMPOSE_FILE" || true)
if [ "$HC_COUNT" -ge 13 ]; then pass "at least 13 healthcheck blocks ($HC_COUNT found)"; else fail "expected >=13 healthcheck blocks, found $HC_COUNT"; fi

# ─── Dependency ordering ────────────────────────────────────────────
echo "--- Dependency ordering ---"
# Helper: extract a service block from compose file (from service name to next top-level service)
svc_block() {
  awk "
    /^  ${1}:\$/ { found=1; next }
    found && /^  [a-zA-Z]/ { found=0 }
    found { print }
  " "$COMPOSE_FILE"
}

# All app services should depend on postgres
for svc in "${APP_SERVICES[@]}"; do
  if [ "$svc" = "gateway" ]; then continue; fi
  if svc_block "$svc" | grep -q "postgres:" 2>/dev/null; then
    pass "$svc depends on postgres"
  else
    fail "$svc should depend on postgres"
  fi
done

# Gateway depends on auth-service
if svc_block "gateway" | grep -q "auth-service:" 2>/dev/null; then
  pass "gateway depends on auth-service"
else
  fail "gateway should depend on auth-service"
fi

# Kafka depends on zookeeper
if svc_block "kafka" | grep -q "zookeeper:" 2>/dev/null; then
  pass "kafka depends on zookeeper"
else
  fail "kafka should depend on zookeeper"
fi

# ─── Volumes ─────────────────────────────────────────────────────────
echo "--- Volumes ---"
check_grep "pgdata volume defined" "^  pgdata:"
check_grep "kafkadata volume defined" "^  kafkadata:"

# ─── Network ─────────────────────────────────────────────────────────
echo "--- Network ---"
check_grep "ace-network defined" "^  ace-network:"
check_grep "bridge driver" "driver: bridge"

# ─── Environment variables ──────────────────────────────────────────
echo "--- Environment variables ---"
check_grep "KAFKA_BROKERS set" "KAFKA_BROKERS:.*kafka:9092"
check_grep "JWT signing key set for auth-service" "AUTH_JWT_SIGNING_KEY"
check_grep "DB_HOST points to postgres" "DB_HOST:.*postgres"
check_grep "MATCHING_ENGINE_ADDR in gateway" "MATCHING_ENGINE_ADDR:.*matching-engine:50051"

# ─── .env.example completeness ──────────────────────────────────────
echo "--- .env.example ---"
ENV_EXAMPLE="$(dirname "$COMPOSE_FILE")/.env.example"
for var in POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB AUTH_JWT_SIGNING_KEY KAFKA_PORT REDIS_PORT; do
  if grep -q "^${var}=" "$ENV_EXAMPLE" 2>/dev/null; then
    pass ".env.example has $var"
  else
    fail ".env.example missing $var"
  fi
done

# ─── No frontend services ──────────────────────────────────────────
echo "--- No frontend services ---"
if grep -q "web-ui\|admin-ui" "$COMPOSE_FILE" 2>/dev/null; then
  fail "docker-compose.yml should not include web-ui or admin-ui"
else
  pass "no web-ui or admin-ui in compose"
fi

# ─── Summary ─────────────────────────────────────────────────────────
echo
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
