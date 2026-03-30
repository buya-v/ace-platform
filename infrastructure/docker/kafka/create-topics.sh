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

echo ""
echo "=== GarudaX Platform: All Kafka topics created ==="
/opt/kafka/bin/kafka-topics.sh --bootstrap-server "$BOOTSTRAP" --list
