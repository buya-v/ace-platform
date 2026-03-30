# Kafka Event Wiring Specification

**Document ID:** T051-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 4)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Topic Naming Convention](#2-topic-naming-convention)
3. [Event Envelope Schema](#3-event-envelope-schema)
4. [Event Flow Diagram](#4-event-flow-diagram)
5. [Topic Catalog](#5-topic-catalog)
6. [Event Schemas](#6-event-schemas)
7. [Consumer Group Mapping](#7-consumer-group-mapping)
8. [Partition Key Strategy](#8-partition-key-strategy)
9. [Idempotency Strategy](#9-idempotency-strategy)
10. [Dead Letter Queue Design](#10-dead-letter-queue-design)
11. [Retention Policy](#11-retention-policy)
12. [Kafka Cluster Configuration](#12-kafka-cluster-configuration)
13. [Operational Runbook](#13-operational-runbook)

---

## 1. Overview

This document specifies the event-driven integration layer for the GarudaX (AI Powered Commodity Exchange) platform. All inter-service asynchronous communication flows through Apache Kafka. The design provides:

- **Loose coupling** — services produce and consume events without direct RPC dependencies
- **Durability** — events are persisted to Kafka and replayable within the retention window
- **Ordering** — partition keys guarantee per-instrument or per-participant ordering
- **Idempotency** — every consumer can safely re-process events via deduplication

### Services in Scope

| Service | Role | Type |
|---|---|---|
| matching-engine | Executes trades from the order book (CLOB) | Producer |
| clearing-engine | Novates trades, manages positions and netting | Producer + Consumer |
| margin-engine | Calculates margin requirements, issues margin calls | Producer + Consumer |
| settlement-engine | Runs daily MtM settlement cycles | Producer + Consumer |
| gateway | API gateway, WebSocket push to traders | Consumer |
| compliance-service | KYC/AML checks, risk monitoring | Producer + Consumer |
| market-data-service | Aggregates candles, distributes market data | Consumer |
| warehouse-service | Manages warehouse receipts and physical delivery | Producer |
| auth-service | User registration, authentication, authorization | Producer + Consumer |

---

## 2. Topic Naming Convention

```
ace.{domain}.{event-type}
```

Rules:
- **`ace`** — fixed namespace prefix for all GarudaX platform events
- **`{domain}`** — the owning service's business domain (not the service name): `trades`, `clearing`, `margin`, `settlement`, `compliance`, `market-data`, `warehouse`, `auth`
- **`{event-type}`** — past-tense verb describing what happened: `executed`, `novated`, `call-issued`, `completed`, `status-changed`, `trade-ingested`, `receipt-pledged`, `delivery-completed`, `user-registered`
- Hyphens separate multi-word segments within a level
- All lowercase

Dead letter topics follow: `ace.dlq.{original-topic-without-ace-prefix}`

Examples:
```
ace.trades.executed
ace.clearing.novated
ace.dlq.trades.executed
```

---

## 3. Event Envelope Schema

Every event published to any GarudaX topic MUST use this envelope. The `payload` field contains the event-specific data.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "ace-event-envelope-v1",
  "type": "object",
  "required": ["id", "type", "timestamp", "source", "correlation_id", "payload"],
  "properties": {
    "id": {
      "type": "string",
      "format": "uuid",
      "description": "Unique event ID (UUID v4). Used for idempotency deduplication."
    },
    "type": {
      "type": "string",
      "description": "Fully-qualified event type matching the topic name (e.g., ace.trades.executed)."
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 UTC timestamp of when the event was produced."
    },
    "source": {
      "type": "string",
      "description": "Producing service name (e.g., matching-engine)."
    },
    "correlation_id": {
      "type": "string",
      "format": "uuid",
      "description": "Traces a business operation across services. All events in a trade lifecycle share one correlation_id."
    },
    "schema_version": {
      "type": "integer",
      "minimum": 1,
      "default": 1,
      "description": "Payload schema version for forward compatibility."
    },
    "payload": {
      "type": "object",
      "description": "Event-type-specific data. Schema defined per topic."
    }
  },
  "additionalProperties": false
}
```

### Serialization

- **Wire format:** JSON (UTF-8). Protobuf may be adopted later for high-throughput topics.
- **Key format:** Plain string (the partition key value).
- **Timestamp:** Kafka record timestamp is set to the envelope `timestamp` value (CreateTime).

---

## 4. Event Flow Diagram

```
                        ┌──────────────────┐
                        │  matching-engine  │
                        └────────┬─────────┘
                                 │ ace.trades.executed
                                 ▼
                        ┌──────────────────┐
              ┌─────────│  clearing-engine  │─────────┐
              │         └──────────────────┘          │
              │ ace.clearing.novated                   │ ace.clearing.novated
              ▼                                        ▼
   ┌──────────────────┐                     ┌──────────────────┐
   │   margin-engine   │                     │ settlement-engine │
   └────────┬─────────┘                     └────────┬─────────┘
            │ ace.margin.call-issued                  │ ace.settlement.completed
            ▼                                         ▼
   ┌─────────────┐ ┌───────────────────┐    ┌──────────────────┐ ┌────────────────────┐
   │   gateway    │ │ compliance-service│    │  clearing-engine  │ │ market-data-service│
   │  (WS push)  │ │  (risk monitor)   │    │  (position close) │ │ (settlement price) │
   └─────────────┘ └────────┬──────────┘    └──────────────────┘ └────────────────────┘
                            │ ace.compliance.status-changed
                            ▼
                   ┌──────────────┐  ┌─────────┐
                   │ auth-service  │  │ gateway │
                   │(enable/disable│  │(notify) │
                   │   trading)    │  └─────────┘
                   └──────┬───────┘
                          │ ace.auth.user-registered
                          ▼
                   ┌───────────────────┐
                   │ compliance-service │
                   │   (trigger KYC)    │
                   └───────────────────┘

   ┌──────────────────┐
   │ warehouse-service │
   └────┬────────┬────┘
        │        │
        │        │ ace.warehouse.delivery-completed
        │        ▼
        │  ┌──────────────────┐
        │  │  clearing-engine  │
        │  │ (delivery fulfilled)│
        │  └──────────────────┘
        │
        │ ace.warehouse.receipt-pledged
        ▼
   ┌──────────────────┐
   │   margin-engine   │
   │(collateral update)│
   └──────────────────┘

   ┌────────────────────┐
   │ market-data-service │──── ace.market-data.trade-ingested (internal)
   └────────────────────┘
```

### Event Flow Summary (numbered)

| # | Producer | Topic | Consumer(s) | Purpose |
|---|---|---|---|---|
| 1 | matching-engine | `ace.trades.executed` | clearing-engine | Trigger trade novation |
| 2 | clearing-engine | `ace.clearing.novated` | margin-engine, settlement-engine | Position update for margin recalc + settlement obligation |
| 3 | margin-engine | `ace.margin.call-issued` | gateway, compliance-service | WebSocket push to trader + risk monitoring |
| 4 | settlement-engine | `ace.settlement.completed` | clearing-engine, market-data-service | Position close + settlement price distribution |
| 5 | compliance-service | `ace.compliance.status-changed` | auth-service, gateway | Enable/disable trading + notification |
| 6 | market-data-service | `ace.market-data.trade-ingested` | market-data-service (internal) | Internal candle aggregation trigger |
| 7 | warehouse-service | `ace.warehouse.receipt-pledged` | margin-engine | Collateral update |
| 8 | warehouse-service | `ace.warehouse.delivery-completed` | clearing-engine | Delivery obligation fulfilled |
| 9 | auth-service | `ace.auth.user-registered` | compliance-service | Trigger KYC onboarding |

---

## 5. Topic Catalog

| Topic | Partitions | Replication Factor | Retention | Partition Key | Compaction |
|---|---|---|---|---|---|
| `ace.trades.executed` | 16 | 3 | 7 days | `instrument_id` | delete |
| `ace.clearing.novated` | 16 | 3 | 7 days | `instrument_id` | delete |
| `ace.margin.call-issued` | 8 | 3 | 7 days | `participant_id` | delete |
| `ace.settlement.completed` | 8 | 3 | 7 days | `instrument_id` | delete |
| `ace.compliance.status-changed` | 4 | 3 | 7 days | `participant_id` | delete |
| `ace.market-data.trade-ingested` | 16 | 3 | 7 days | `instrument_id` | delete |
| `ace.warehouse.receipt-pledged` | 4 | 3 | 7 days | `participant_id` | delete |
| `ace.warehouse.delivery-completed` | 4 | 3 | 7 days | `instrument_id` | delete |
| `ace.auth.user-registered` | 4 | 3 | 7 days | `participant_id` | delete |
| `ace.dlq.trades.executed` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.clearing.novated` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.margin.call-issued` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.settlement.completed` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.compliance.status-changed` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.warehouse.receipt-pledged` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.warehouse.delivery-completed` | 4 | 3 | 30 days | (original key) | delete |
| `ace.dlq.auth.user-registered` | 4 | 3 | 30 days | (original key) | delete |

### Partition Count Rationale

- **16 partitions** for high-throughput topics (trades, clearing, market-data): supports up to 16 parallel consumers and high message rates during trading hours.
- **8 partitions** for medium-throughput topics (margin calls, settlement): margin calls and settlement cycles are less frequent but still require parallelism.
- **4 partitions** for low-throughput topics (compliance, warehouse, auth, all DLQs): these events are infrequent; 4 partitions provide adequate parallelism.

---

## 6. Event Schemas

### 6.1 `ace.trades.executed`

Produced by **matching-engine** when a trade is executed. Mirrors the `types.Trade` struct in `src/matching-engine/internal/types/order.go`.

```json
{
  "payload": {
    "trade_id": "TRD-20260328-000001",
    "instrument_id": "WHEAT-2026Q3",
    "buy_order_id": "ORD-B-001",
    "sell_order_id": "ORD-S-001",
    "buyer_participant_id": "PART-001",
    "seller_participant_id": "PART-002",
    "price": "1850.0000",
    "quantity": 100,
    "trade_value": "185000.0000",
    "aggressor_side": "BUY",
    "trade_type": "REGULAR",
    "sequence_number": 42,
    "executed_at": "2026-03-28T09:15:00.123Z"
  }
}
```

**Partition key:** `instrument_id` — ensures all trades for the same instrument are ordered.

### 6.2 `ace.clearing.novated`

Produced by **clearing-engine** after successful novation. Mirrors `types.ClearingObligation` from `src/clearing-engine/internal/types/clearing.go`.

```json
{
  "payload": {
    "obligation_id": "OBL-20260328-000001",
    "trade_id": "TRD-20260328-000001",
    "instrument_id": "WHEAT-2026Q3",
    "buyer_participant_id": "PART-001",
    "seller_participant_id": "PART-002",
    "ccp_id": "GarudaX-CCP",
    "price": "1850.0000",
    "quantity": 100,
    "status": "NOVATED",
    "buyer_position": {
      "participant_id": "PART-001",
      "instrument_id": "WHEAT-2026Q3",
      "net_quantity": 500,
      "avg_entry_price": "1840.0000"
    },
    "seller_position": {
      "participant_id": "PART-002",
      "instrument_id": "WHEAT-2026Q3",
      "net_quantity": -300,
      "avg_entry_price": "1855.0000"
    },
    "novated_at": "2026-03-28T09:15:00.456Z"
  }
}
```

**Partition key:** `instrument_id` — keeps all novation events for the same instrument ordered for downstream margin and settlement processing.

### 6.3 `ace.margin.call-issued`

Produced by **margin-engine** when a margin call is triggered. Mirrors `types.MarginCall` from `src/margin-engine/internal/types/margin.go`.

```json
{
  "payload": {
    "margin_call_id": "MC-20260328-000001",
    "participant_id": "PART-001",
    "instrument_id": "WHEAT-2026Q3",
    "call_type": "VARIATION",
    "required_amount": "25000.0000",
    "current_margin": "50000.0000",
    "maintenance_margin": "75000.0000",
    "deficit": "25000.0000",
    "status": "ISSUED",
    "deadline": "2026-03-28T11:00:00Z",
    "issued_at": "2026-03-28T09:30:00Z"
  }
}
```

**Partition key:** `participant_id` — all margin calls for the same participant are ordered so the gateway can present them sequentially.

### 6.4 `ace.settlement.completed`

Produced by **settlement-engine** when a daily settlement cycle completes. Mirrors `types.SettlementCycle` from `src/settlement-engine/internal/types/settlement.go`.

```json
{
  "payload": {
    "cycle_id": "SETTLE-20260328",
    "settle_date": "2026-03-28",
    "status": "COMPLETED",
    "settlement_prices": [
      {
        "instrument_id": "WHEAT-2026Q3",
        "settlement_price": "1855.0000",
        "previous_price": "1840.0000"
      }
    ],
    "total_pay_in": "1250000.0000",
    "total_pay_out": "1250000.0000",
    "instructions_count": 42,
    "started_at": "2026-03-28T16:00:00Z",
    "completed_at": "2026-03-28T16:05:30Z"
  }
}
```

**Partition key:** `instrument_id` of the first settlement price (or `cycle_id` if multi-instrument). For per-instrument settlement price updates, produce one message per instrument.

### 6.5 `ace.compliance.status-changed`

Produced by **compliance-service** when a participant's compliance status changes.

```json
{
  "payload": {
    "participant_id": "PART-001",
    "previous_status": "PENDING_REVIEW",
    "new_status": "APPROVED",
    "kyc_level": "ENHANCED",
    "aml_check_passed": true,
    "restrictions": [],
    "reason": "KYC documents verified",
    "changed_at": "2026-03-28T10:00:00Z"
  }
}
```

**Partition key:** `participant_id` — ensures ordered status transitions per participant.

### 6.6 `ace.market-data.trade-ingested`

Produced by **market-data-service** internally for candle aggregation. Derived from `ace.trades.executed` consumption.

```json
{
  "payload": {
    "instrument_id": "WHEAT-2026Q3",
    "trade_id": "TRD-20260328-000001",
    "price": "1850.0000",
    "quantity": 100,
    "side": "BUY",
    "ingested_at": "2026-03-28T09:15:00.789Z"
  }
}
```

**Partition key:** `instrument_id` — ensures all trades for the same instrument hit the same candle aggregator partition.

### 6.7 `ace.warehouse.receipt-pledged`

Produced by **warehouse-service** when a warehouse receipt is pledged as collateral.

```json
{
  "payload": {
    "receipt_id": "WR-20260328-000001",
    "participant_id": "PART-001",
    "commodity": "WHEAT",
    "quantity_mt": 500.0,
    "warehouse_id": "WH-UB-001",
    "grade": "GRADE_A",
    "collateral_value": "925000.0000",
    "pledged_at": "2026-03-28T08:00:00Z"
  }
}
```

**Partition key:** `participant_id` — ensures the margin-engine processes all collateral updates for a participant sequentially.

### 6.8 `ace.warehouse.delivery-completed`

Produced by **warehouse-service** when physical delivery is fulfilled.

```json
{
  "payload": {
    "delivery_id": "DEL-20260328-000001",
    "receipt_id": "WR-20260328-000001",
    "instrument_id": "WHEAT-2026Q3",
    "buyer_participant_id": "PART-001",
    "seller_participant_id": "PART-002",
    "quantity_mt": 500.0,
    "warehouse_id": "WH-UB-001",
    "completed_at": "2026-03-28T14:00:00Z"
  }
}
```

**Partition key:** `instrument_id` — aligns with clearing-engine's instrument-based processing.

### 6.9 `ace.auth.user-registered`

Produced by **auth-service** when a new user account is created.

```json
{
  "payload": {
    "user_id": "USR-20260328-000001",
    "participant_id": "PART-001",
    "email": "trader@example.mn",
    "roles": ["TRADER"],
    "registered_at": "2026-03-28T07:00:00Z"
  }
}
```

**Partition key:** `participant_id` — routes to the correct compliance-service partition for KYC processing.

---

## 7. Consumer Group Mapping

Consumer group naming: `{service-name}-{topic}`

| Consumer Group ID | Topic | Service | Purpose |
|---|---|---|---|
| `clearing-engine-ace.trades.executed` | `ace.trades.executed` | clearing-engine | Consume executed trades for novation |
| `margin-engine-ace.clearing.novated` | `ace.clearing.novated` | margin-engine | Update positions, recalculate margin |
| `settlement-engine-ace.clearing.novated` | `ace.clearing.novated` | settlement-engine | Track settlement obligations |
| `gateway-ace.margin.call-issued` | `ace.margin.call-issued` | gateway | Push margin calls to traders via WebSocket |
| `compliance-service-ace.margin.call-issued` | `ace.margin.call-issued` | compliance-service | Risk monitoring for repeated margin calls |
| `clearing-engine-ace.settlement.completed` | `ace.settlement.completed` | clearing-engine | Close settled positions |
| `market-data-service-ace.settlement.completed` | `ace.settlement.completed` | market-data-service | Ingest settlement prices |
| `auth-service-ace.compliance.status-changed` | `ace.compliance.status-changed` | auth-service | Enable/disable trading permissions |
| `gateway-ace.compliance.status-changed` | `ace.compliance.status-changed` | gateway | Notify traders of compliance status |
| `market-data-service-ace.trades.executed` | `ace.trades.executed` | market-data-service | Ingest trades for candle aggregation |
| `market-data-service-ace.market-data.trade-ingested` | `ace.market-data.trade-ingested` | market-data-service | Internal candle aggregation |
| `margin-engine-ace.warehouse.receipt-pledged` | `ace.warehouse.receipt-pledged` | margin-engine | Collateral update |
| `clearing-engine-ace.warehouse.delivery-completed` | `ace.warehouse.delivery-completed` | clearing-engine | Delivery obligation fulfilled |
| `compliance-service-ace.auth.user-registered` | `ace.auth.user-registered` | compliance-service | Trigger KYC onboarding |

### Consumer Group Properties

All consumer groups use the following defaults (overridable per group):

```properties
auto.offset.reset=earliest
enable.auto.commit=false
max.poll.records=100
session.timeout.ms=30000
heartbeat.interval.ms=10000
max.poll.interval.ms=300000
```

- **Manual offset commit** — consumers commit offsets only after successful processing to avoid data loss.
- **`earliest` reset** — on first join or offset expiration, start from the beginning to avoid missing events.

---

## 8. Partition Key Strategy

| Domain | Partition Key | Rationale |
|---|---|---|
| Trades (executed) | `instrument_id` | All trades for an instrument are ordered — clearing-engine processes them in sequence |
| Clearing (novated) | `instrument_id` | Margin and settlement both process per-instrument — consistent ordering |
| Margin (call-issued) | `participant_id` | Margin calls are per-participant — gateway and compliance process per-participant |
| Settlement (completed) | `instrument_id` | Settlement prices are per-instrument |
| Compliance (status-changed) | `participant_id` | Compliance status is per-participant |
| Market data (trade-ingested) | `instrument_id` | Candle aggregation is per-instrument |
| Warehouse (receipt-pledged) | `participant_id` | Collateral is tracked per-participant |
| Warehouse (delivery-completed) | `instrument_id` | Delivery obligations are per-instrument position |
| Auth (user-registered) | `participant_id` | KYC onboarding is per-participant |

### Key Format

Partition keys are plain strings — the raw value of the field (e.g., `"WHEAT-2026Q3"` or `"PART-001"`).

---

## 9. Idempotency Strategy

### Producer Idempotency

All Kafka producers MUST enable:
```properties
enable.idempotence=true
acks=all
retries=2147483647
max.in.flight.requests.per.connection=5
```

This guarantees exactly-once delivery to Kafka from each producer.

### Consumer Idempotency

Each consumer maintains a set of processed event IDs to prevent duplicate processing. This pattern is already implemented in `clearing-engine` (`processedTrades map[string]bool` in `src/clearing-engine/internal/engine/engine.go:31`).

**Standard implementation pattern:**

```go
type EventProcessor struct {
    mu             sync.Mutex
    processedIDs   map[string]bool  // In-memory dedup (hot path)
    maxTracked     int              // Cap to prevent unbounded growth
}

func (p *EventProcessor) ProcessEvent(event Event) error {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.processedIDs[event.ID] {
        return nil // Already processed — idempotent skip
    }

    // Process the event...

    p.processedIDs[event.ID] = true
    // Evict oldest if over capacity (LRU or time-based)
    return nil
}
```

**Deduplication window:** Each service keeps the last 100,000 event IDs in memory (covers ~7 days of events at peak throughput). For services that restart, the dedup set is rebuilt by:
1. Reading the last committed offset
2. Re-consuming from that offset (events are re-processed idempotently via application-level checks like clearing-engine's `processedTrades`)

**Database-backed dedup (for critical services):** clearing-engine, margin-engine, and settlement-engine SHOULD also persist processed event IDs to PostgreSQL for crash recovery:

```sql
CREATE TABLE processed_events (
    event_id    UUID PRIMARY KEY,
    topic       TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Cleanup: DELETE FROM processed_events WHERE processed_at < NOW() - INTERVAL '7 days';
```

---

## 10. Dead Letter Queue Design

### DLQ Topic Naming

```
ace.dlq.{domain}.{event-type}
```

Every event topic has a corresponding DLQ topic. For example:
- `ace.trades.executed` → `ace.dlq.trades.executed`
- `ace.clearing.novated` → `ace.dlq.clearing.novated`

### Retry Policy

```
Attempt 1 → immediate
Attempt 2 → 1 second delay
Attempt 3 → 5 second delay
→ DLQ after 3 failed attempts
```

### DLQ Message Format

The DLQ message wraps the original event with failure metadata:

```json
{
  "id": "dlq-uuid-001",
  "type": "ace.dlq.trades.executed",
  "timestamp": "2026-03-28T09:16:00Z",
  "source": "clearing-engine",
  "correlation_id": "original-correlation-id",
  "payload": {
    "original_event": { "...original event envelope..." },
    "error": "novation failed: insufficient counterparty credit",
    "consumer_group": "clearing-engine-ace.trades.executed",
    "attempt_count": 3,
    "first_attempt_at": "2026-03-28T09:15:01Z",
    "last_attempt_at": "2026-03-28T09:15:07Z"
  }
}
```

### DLQ Processing

- DLQ messages are NOT auto-retried. They require manual investigation or an operator-triggered replay.
- A DLQ consumer dashboard (admin-dashboard) should surface DLQ depth per topic.
- Replay tool: a CLI command to move DLQ messages back to the original topic after the root cause is fixed.

---

## 11. Retention Policy

| Topic Type | Retention | Rationale |
|---|---|---|
| Event topics (`ace.*`) | 7 days | Sufficient for consumer catch-up, replay debugging, and audit trail. Longer retention handled by warehouse-service archival to object storage. |
| DLQ topics (`ace.dlq.*`) | 30 days | Failed events need longer investigation window. 30 days aligns with compliance audit requirements. |

### Segment Settings

```properties
# Event topics
log.retention.hours=168          # 7 days
log.segment.bytes=1073741824     # 1 GB segments
log.cleanup.policy=delete

# DLQ topics
log.retention.hours=720          # 30 days
log.segment.bytes=268435456      # 256 MB segments (lower volume)
log.cleanup.policy=delete
```

---

## 12. Kafka Cluster Configuration

### Cluster Sizing (Production)

| Parameter | Value |
|---|---|
| Brokers | 3 (minimum for RF=3) |
| Zookeeper / KRaft | KRaft mode (no Zookeeper) |
| Min ISR | 2 |
| Default replication factor | 3 |
| `unclean.leader.election.enable` | `false` |
| `auto.create.topics.enable` | `false` |
| `message.max.bytes` | 1 MB |
| `num.io.threads` | 8 |
| `num.network.threads` | 3 |

### Security

```properties
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
ssl.endpoint.identification.algorithm=https
```

Each service authenticates with its own SASL credentials. ACLs restrict:
- Producers can only write to their owned topics
- Consumers can only read from topics they subscribe to
- No service can delete topics (admin-only operation)

### ACL Table

| Principal | Topic Pattern | Operation |
|---|---|---|
| `matching-engine` | `ace.trades.executed` | WRITE |
| `clearing-engine` | `ace.trades.executed` | READ |
| `clearing-engine` | `ace.clearing.novated` | WRITE |
| `clearing-engine` | `ace.settlement.completed` | READ |
| `clearing-engine` | `ace.warehouse.delivery-completed` | READ |
| `margin-engine` | `ace.clearing.novated` | READ |
| `margin-engine` | `ace.margin.call-issued` | WRITE |
| `margin-engine` | `ace.warehouse.receipt-pledged` | READ |
| `settlement-engine` | `ace.clearing.novated` | READ |
| `settlement-engine` | `ace.settlement.completed` | WRITE |
| `gateway` | `ace.margin.call-issued` | READ |
| `gateway` | `ace.compliance.status-changed` | READ |
| `compliance-service` | `ace.margin.call-issued` | READ |
| `compliance-service` | `ace.compliance.status-changed` | WRITE |
| `compliance-service` | `ace.auth.user-registered` | READ |
| `market-data-service` | `ace.trades.executed` | READ |
| `market-data-service` | `ace.settlement.completed` | READ |
| `market-data-service` | `ace.market-data.trade-ingested` | WRITE, READ |
| `warehouse-service` | `ace.warehouse.receipt-pledged` | WRITE |
| `warehouse-service` | `ace.warehouse.delivery-completed` | WRITE |
| `auth-service` | `ace.auth.user-registered` | WRITE |
| `auth-service` | `ace.compliance.status-changed` | READ |
| All services | `ace.dlq.*` (own topics) | WRITE |

---

## 13. Operational Runbook

### Monitoring Metrics

Each service should expose these Kafka consumer/producer metrics:

| Metric | Type | Labels | Alert Threshold |
|---|---|---|---|
| `kafka_consumer_lag` | gauge | `consumer_group`, `topic`, `partition` | > 10,000 (warn), > 100,000 (critical) |
| `kafka_consumer_messages_total` | counter | `consumer_group`, `topic` | — |
| `kafka_producer_messages_total` | counter | `topic` | — |
| `kafka_consumer_errors_total` | counter | `consumer_group`, `topic`, `error_type` | > 0 sustained for 5 min |
| `kafka_dlq_messages_total` | counter | `topic` | > 0 (alert immediately) |
| `kafka_consumer_processing_duration_seconds` | histogram | `consumer_group`, `topic` | p99 > 1s |

### Consumer Lag Alerting

```
CRITICAL: Consumer lag > 100,000 on any trade-path topic
          (ace.trades.executed, ace.clearing.novated)
          → Clearing/margin may be processing stale data

WARNING:  Consumer lag > 10,000 on any topic
          → Consumer may be falling behind; check processing throughput

INFO:     DLQ message count > 0
          → Manual investigation required
```

### Recovery Procedures

**Consumer restart:** Consumers resume from the last committed offset. Idempotency dedup handles any reprocessed messages.

**Broker failure:** With RF=3 and min.ISR=2, one broker can fail without data loss. The cluster continues to accept writes.

**Full cluster recovery:** Restore from the most recent broker snapshots. Consumers replay from their last committed offsets.

**DLQ replay:**
```bash
# List DLQ messages for a topic
kafka-console-consumer --bootstrap-server $BROKERS \
  --topic ace.dlq.trades.executed --from-beginning --max-messages 10

# Replay DLQ messages back to original topic (after fixing root cause)
kafka-console-consumer --bootstrap-server $BROKERS \
  --topic ace.dlq.trades.executed --from-beginning |
kafka-console-producer --bootstrap-server $BROKERS \
  --topic ace.trades.executed
```

---

## Appendix A: Topic Creation Commands

```bash
#!/bin/bash
BROKERS="kafka-0:9092,kafka-1:9092,kafka-2:9092"

# Event topics (7-day retention)
for topic in \
  ace.trades.executed:16 \
  ace.clearing.novated:16 \
  ace.margin.call-issued:8 \
  ace.settlement.completed:8 \
  ace.compliance.status-changed:4 \
  ace.market-data.trade-ingested:16 \
  ace.warehouse.receipt-pledged:4 \
  ace.warehouse.delivery-completed:4 \
  ace.auth.user-registered:4; do
  name="${topic%%:*}"
  partitions="${topic##*:}"
  kafka-topics.sh --bootstrap-server $BROKERS \
    --create --topic "$name" \
    --partitions "$partitions" \
    --replication-factor 3 \
    --config retention.ms=604800000 \
    --config min.insync.replicas=2
done

# DLQ topics (30-day retention, 4 partitions each)
for topic in \
  ace.dlq.trades.executed \
  ace.dlq.clearing.novated \
  ace.dlq.margin.call-issued \
  ace.dlq.settlement.completed \
  ace.dlq.compliance.status-changed \
  ace.dlq.warehouse.receipt-pledged \
  ace.dlq.warehouse.delivery-completed \
  ace.dlq.auth.user-registered; do
  kafka-topics.sh --bootstrap-server $BROKERS \
    --create --topic "$topic" \
    --partitions 4 \
    --replication-factor 3 \
    --config retention.ms=2592000000 \
    --config min.insync.replicas=2
done
```

## Appendix B: Schema Version Evolution

When a payload schema changes:

1. **Backward-compatible changes** (adding optional fields): increment `schema_version`, deploy consumers first, then producers.
2. **Breaking changes** (removing/renaming fields): create a new topic version (e.g., `ace.trades.executed.v2`), run both topics in parallel during migration, then deprecate the old topic.

Consumers MUST ignore unknown fields in the payload (forward compatibility). Producers MUST NOT remove required fields without a major version bump.
