#!/bin/bash
set -e

# ── GarudaX Platform — Kafka Topic Bootstrap ─────────────────────────────
# Runs once via garudax-kafka-init container after garudax-kafka is healthy.

BOOTSTRAP="garudax-kafka:9092"

echo "=== GarudaX Platform: Creating Kafka topics ==="

# Wait for broker to be fully ready
sleep 5

create_topic() {
  local name="$1"
  local partitions="${2:-3}"
  local retention_ms="${3:-604800000}"  # default 7 days

  echo "  Creating topic: $name (partitions=$partitions, retention=${retention_ms}ms)"
  /opt/kafka/bin/kafka-topics.sh --bootstrap-server "$BOOTSTRAP" \
    --create \
    --if-not-exists \
    --topic "$name" \
    --partitions "$partitions" \
    --replication-factor 1 \
    --config retention.ms="$retention_ms"
}

# ── Exchange Topics ──────────────────────────────────────────────────
create_topic "orders"              6   604800000    # 7 days, high throughput
create_topic "trades"              6   2592000000   # 30 days, append-only ledger
create_topic "order-book-updates"  3   86400000     # 1 day, real-time snapshots

# ── Clearing & Settlement ────────────────────────────────────────────
create_topic "positions"           3   2592000000   # 30 days
create_topic "margin-calls"        3   2592000000   # 30 days
create_topic "settlement"          3   2592000000   # 30 days

# ── Market Data ──────────────────────────────────────────────────────
create_topic "market-data-ticks"   6   604800000    # 7 days, high throughput

# ── Compliance & Audit ───────────────────────────────────────────────
create_topic "compliance-events"   3   7776000000   # 90 days, regulatory
create_topic "audit-log"           3   7776000000   # 90 days, regulatory

# ── Participants & Warehouse ─────────────────────────────────────────
create_topic "account-events"      3   2592000000   # 30 days
create_topic "warehouse-receipts"  3   2592000000   # 30 days

# ── System ───────────────────────────────────────────────────────────
create_topic "notifications"       3   604800000    # 7 days

# ── ace-commodities cross-service topics (R024) ──────────────────────
# The trading engines publish/consume tenant-prefixed topics for the
# matching -> clearing -> margin/settlement event flow. In Compose these
# auto-create, but production MSK sets auto.create.topics.enable=false, so they
# must be enumerated here.
create_topic "ace-commodities.trades.executed"       6   2592000000   # 30 days, trade ledger
create_topic "ace-commodities.clearing.novated"      3   2592000000   # 30 days
create_topic "ace-commodities.margin.call-issued"    3   2592000000   # 30 days
create_topic "ace-commodities.settlement.completed"  3   2592000000   # 30 days

# ── ace-commodities dead-letter topics (R028 D3) ─────────────────────
# Consumers route permanently-failed records to ace-commodities.dlq.<topic>
# (see internal/kafka consumer.sendToDLQ). These must exist or exhausted-retry
# events are silently dropped. Naming matches topicWithoutPrefix(): the tenant
# prefix is stripped and re-prefixed with "<tenant>.dlq.". Longer retention so
# failures survive for investigation.
create_topic "ace-commodities.dlq.trades.executed"       3   7776000000   # 90 days
create_topic "ace-commodities.dlq.clearing.novated"      3   7776000000   # 90 days
create_topic "ace-commodities.dlq.margin.call-issued"    3   7776000000   # 90 days
create_topic "ace-commodities.dlq.settlement.completed"  3   7776000000   # 90 days

echo ""
echo "=== GarudaX Platform: All Kafka topics created ==="
/opt/kafka/bin/kafka-topics.sh --bootstrap-server "$BOOTSTRAP" --list
