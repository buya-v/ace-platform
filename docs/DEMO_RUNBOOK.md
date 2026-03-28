# ACE Platform — Demo Runbook

Step-by-step guide for demonstrating and probing the full Agriculture Commodity Exchange platform.

> **Base URL:** All `curl` commands assume the gateway is on `http://localhost:8080`.
> Adjust if running behind a different host/port.

---

## Table of Contents

1. [Environment Setup](#1-environment-setup)
2. [User Registration & KYC Flow](#2-user-registration--kyc-flow)
3. [Trading Flow Demo](#3-trading-flow-demo)
4. [Post-Trade Flow](#4-post-trade-flow)
5. [Physical Delivery Flow](#5-physical-delivery-flow)
6. [Market Data Flow](#6-market-data-flow)
7. [Compliance & Risk](#7-compliance--risk)
8. [Admin Operations](#8-admin-operations)
9. [Production Readiness Checklist](#9-production-readiness-checklist)

---

## 1. Environment Setup

### 1.1 Start the Stack

```bash
# From the project root
docker compose up -d --build
```

Docker Compose brings up 13 containers: postgres (TimescaleDB), redis, zookeeper, kafka, and 9 Go services.

**Wait for all services to become healthy** (~60-90 seconds):

```bash
docker compose ps --format "table {{.Name}}\t{{.Status}}"
```

All services should show `(healthy)` in the status column.

### 1.2 Verify Service Health Checks

Each Go service exposes `/healthz` (liveness) and `/readyz` (readiness) on its health port.

| Service | Health Port | gRPC Port |
|---|---|---|
| matching-engine | 8081 | 50051 |
| clearing-engine | 8082 | 50052 |
| margin-engine | 8083 | 50053 |
| settlement-engine | 8084 | 50054 |
| auth-service | 8085 | 50055 |
| compliance-service | 8086 | 50056 |
| market-data-service | 8087 | 50057 |
| warehouse-service | 8088 | 50058 |
| gateway | 8080 (HTTP API) / 8090 (health) | N/A |

**Check all health endpoints:**

```bash
# Gateway (serves API on 8080, health on 8090 inside container, proxied on 8080)
curl -s http://localhost:8080/healthz | jq .
# Expected: {"status":"ok","service":"ace-gateway"}

# Core engines
curl -s http://localhost:8081/healthz
# Expected: ok

curl -s http://localhost:8082/healthz
# Expected: ok

curl -s http://localhost:8083/healthz
# Expected: ok

curl -s http://localhost:8084/healthz
# Expected: ok

# Supporting services
curl -s http://localhost:8085/healthz
# Expected: ok

curl -s http://localhost:8086/healthz
# Expected: ok

curl -s http://localhost:8087/healthz
# Expected: ok

curl -s http://localhost:8088/healthz
# Expected: ok
```

> **Note:** The gateway's `/healthz` returns JSON (`{"status":"ok","service":"ace-gateway"}`).
> The individual services return plain text `ok`.

**One-liner to check all services:**

```bash
for port in 8080 8081 8082 8083 8084 8085 8086 8087 8088; do
  printf "port %s: " "$port"
  curl -s -o /dev/null -w "%{http_code}" "http://localhost:${port}/healthz"
  echo
done
# All should print: 200
```

### 1.3 Verify Frontend Apps

If built (requires Node.js):

- **Trading Web UI:** http://localhost:3000
- **Admin Dashboard:** http://localhost:3001

> **Note:** The `docker-compose.yml` does not include frontend services. Run them locally:
> ```bash
> cd src/web-ui && npm install && npm run dev -- --port 3000
> cd src/admin-ui && npm install && npm run dev -- --port 3001
> ```

### 1.4 Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Port conflict on 5432/8080/etc | Host service using the port | `POSTGRES_PORT=5433 docker compose up -d` or stop conflicting service |
| Service shows `starting` for >2 min | Database not ready | `docker compose logs postgres` — check for init errors |
| Gateway returns 502 on all routes | Backend services not healthy | `docker compose ps` — restart unhealthy: `docker compose restart matching-engine` |
| `POSTGRES_PASSWORD` auth error | Stale volume with old creds | `docker compose down -v && docker compose up -d` (destroys data) |
| Kafka `broker not available` | Zookeeper still starting | Wait 30s; check `docker compose logs kafka` |

---

## 2. User Registration & KYC Flow

### 2.1 Register a Trader

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader1@example.com",
    "password": "SecurePass123!",
    "role": "trader"
  }' | jq .
```

**Expected response (HTTP 201):**

```json
{
  "id": "usr_abc123...",
  "email": "trader1@example.com",
  "role": "trader"
}
```

**What this proves:** Auth service is running, user registration works, bcrypt password hashing (cost 12) is functional.

**Troubleshooting:**
- `409 Conflict` — email already registered; use a different email.
- `400 Bad Request` — check that all three fields are present, password >= 8 chars, role is one of: `trader`, `admin`, `viewer`, `compliance_officer`, `super_admin`.
- `502 Bad Gateway` — auth-service not reachable from gateway.

### 2.2 Register a Second Trader (for trading demo later)

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader2@example.com",
    "password": "SecurePass123!",
    "role": "trader"
  }' | jq .
```

### 2.3 Register an Admin User

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "AdminPass123!",
    "role": "admin"
  }' | jq .
```

### 2.4 Login and Extract JWT Token

```bash
# Login as trader1
TRADER1_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader1@example.com",
    "password": "SecurePass123!"
  }')

echo "$TRADER1_RESPONSE" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "AccessToken": "eyJhbGciOiJIUzI1NiIs...",
  "RefreshToken": "rft_...",
  "ExpiresIn": 3600
}
```

> **Known issue:** The `TokenPair` struct in auth-service lacks `json:` struct tags, so fields use Go naming (`AccessToken` not `access_token`). This may be fixed in a future release.

**Extract the token:**

```bash
# Handle both casing variants
TRADER1_TOKEN=$(echo "$TRADER1_RESPONSE" | jq -r '.AccessToken // .access_token')
echo "Token: ${TRADER1_TOKEN:0:20}..."
```

**Login as trader2:**

```bash
TRADER2_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader2@example.com",
    "password": "SecurePass123!"
  }')
TRADER2_TOKEN=$(echo "$TRADER2_RESPONSE" | jq -r '.AccessToken // .access_token')
```

**Login as admin:**

```bash
ADMIN_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "AdminPass123!"
  }')
ADMIN_TOKEN=$(echo "$ADMIN_RESPONSE" | jq -r '.AccessToken // .access_token')
```

### 2.5 Submit KYC / Participant Onboarding Application

```bash
curl -s -X POST http://localhost:8080/api/v1/participants \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER1_TOKEN" \
  -d '{
    "entity_name": "Acme Grain Trading LLC",
    "entity_type": "CORPORATE",
    "jurisdiction": "US",
    "tax_id": "12-3456789",
    "contact_email": "trader1@example.com",
    "contact_phone": "+1-555-0100"
  }' | jq .
```

**Expected response (HTTP 200 or 201):** A participant object with an `id` and `status: "PENDING"`.

**Save the participant ID:**

```bash
PARTICIPANT_ID="<id from response>"
```

### 2.6 Admin Approves KYC Application

```bash
curl -s -X POST "http://localhost:8080/api/v1/participants/${PARTICIPANT_ID}/approve" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{}' | jq .
```

**Expected:** Participant status changes to `APPROVED`.

**Verify:**

```bash
curl -s "http://localhost:8080/api/v1/participants/${PARTICIPANT_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

**What this proves:** Full KYC lifecycle — application submission, admin review, approval — works end-to-end through the gateway.

**Troubleshooting:**
- `401 Unauthorized` — token expired or missing; re-login.
- `403 Forbidden` — role lacks `compliance:manage` permission; use an `admin` or `super_admin` account.
- `502 Bad Gateway` — compliance-service not reachable.

---

## 3. Trading Flow Demo

### 3.1 Instrument

The default instrument is `WHT-HRW-2026M07-UB` (Hard Red Winter wheat, July 2026, US Bushels).
This is configured via the `INSTRUMENTS` environment variable in `docker-compose.yml`.

### 3.2 Submit a Limit Buy Order

```bash
curl -s -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER1_TOKEN" \
  -d '{
    "instrument_id": "WHT-HRW-2026M07-UB",
    "side": 1,
    "order_type": 1,
    "time_in_force": 2,
    "price": "550.0000",
    "quantity": 10
  }' | jq .
```

**Field reference:**
- `side`: 1 = BUY, 2 = SELL
- `order_type`: 1 = LIMIT, 2 = MARKET, 3 = STOP_LIMIT, 4 = STOP_MARKET
- `time_in_force`: 1 = DAY, 2 = GTC, 3 = GTD, 4 = IOC, 5 = FOK
- `price`: Decimal string with 4 decimal places
- `quantity`: Integer number of lots

**Expected response (HTTP 200):** An execution report confirming the order was accepted:

```json
{
  "exec_type": "NEW",
  "order_status": "NEW",
  "order_id": "ord_...",
  "instrument_id": "WHT-HRW-2026M07-UB",
  "side": "BUY",
  "price": "550.0000",
  "quantity": 10,
  "leaves_qty": 10,
  "cumulative_qty": 0
}
```

**What this proves:** Gateway routes to matching-engine, order is accepted into the order book.

### 3.3 Submit a Matching Limit Sell Order (from second account)

```bash
curl -s -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER2_TOKEN" \
  -d '{
    "instrument_id": "WHT-HRW-2026M07-UB",
    "side": 2,
    "order_type": 1,
    "time_in_force": 2,
    "price": "550.0000",
    "quantity": 10
  }' | jq .
```

**Expected response:** An execution report with `exec_type: "FILL"` — the sell crosses the resting buy at $550.

```json
{
  "exec_type": "FILL",
  "order_status": "FILLED",
  "order_id": "ord_...",
  "trade_id": "trd_...",
  "last_price": "550.0000",
  "last_qty": 10,
  "cumulative_qty": 10,
  "leaves_qty": 0
}
```

**What this proves:** Price-time priority matching works. Two orders at the same price cross immediately.

### 3.4 View the Order Book

```bash
# Level 2 order book (aggregated by price)
curl -s "http://localhost:8080/api/v1/instruments/WHT-HRW-2026M07-UB/book" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .

# Level 3 order book (individual orders)
curl -s "http://localhost:8080/api/v1/instruments/WHT-HRW-2026M07-UB/book/l3" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** After the full fill above, the book should be empty (both orders fully matched). Submit a new order without a counterparty to see it resting in the book.

### 3.5 View Last Trade

```bash
curl -s "http://localhost:8080/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** The trade from step 3.3 with price 550.0000, quantity 10.

### 3.6 List Open Orders

```bash
curl -s "http://localhost:8080/api/v1/orders" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Empty list (both orders filled) or any resting orders.

### 3.7 Cancel an Order

```bash
# First, place a new order that won't match
curl -s -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER1_TOKEN" \
  -d '{
    "instrument_id": "WHT-HRW-2026M07-UB",
    "side": 1,
    "order_type": 1,
    "time_in_force": 2,
    "price": "500.0000",
    "quantity": 5
  }' | jq .

# Save the order_id from the response, then cancel:
ORDER_ID="<order_id from response>"
curl -s -X DELETE "http://localhost:8080/api/v1/orders/${ORDER_ID}" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Execution report with `exec_type: "CANCELLED"`, `order_status: "CANCELLED"`.

**Troubleshooting:**
- `502 Bad Gateway` — matching-engine not reachable from gateway.
- `401 Unauthorized` — JWT expired; re-login.

---

## 4. Post-Trade Flow

### 4.1 View Clearing Positions

After a trade executes, the clearing engine novates the trade into buyer/seller positions.

```bash
curl -s "http://localhost:8080/api/v1/clearing/positions" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected response (HTTP 200):** List of positions showing the novated trade.

```json
[
  {
    "instrument_id": "WHT-HRW-2026M07-UB",
    "participant_id": "...",
    "side": "BUY",
    "quantity": 10,
    "avg_price": "550.0000"
  }
]
```

### 4.2 View Position for Specific Instrument

```bash
curl -s "http://localhost:8080/api/v1/clearing/positions/WHT-HRW-2026M07-UB" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 4.3 View Netting Obligations

```bash
curl -s "http://localhost:8080/api/v1/clearing/netting" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**What this proves:** Trade flows from matching-engine to clearing-engine. Novation creates CCP-intermediated positions.

### 4.4 View Margin Requirements

```bash
curl -s "http://localhost:8080/api/v1/margin" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Portfolio margin showing initial margin, maintenance margin, and available balance.

### 4.5 Calculate Margin (explicit)

```bash
curl -s -X POST http://localhost:8080/api/v1/margin/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER1_TOKEN" \
  -d '{}' | jq .
```

### 4.6 View Margin Calls

```bash
curl -s "http://localhost:8080/api/v1/margin/calls" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 4.7 Trigger End-of-Day Settlement

```bash
# View existing settlement cycles
curl -s "http://localhost:8080/api/v1/settlement/cycles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .

# View a specific cycle
curl -s "http://localhost:8080/api/v1/settlement/cycles/{cycle_id}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

> **Known gap:** There is no `POST /api/v1/settlement/cycle` endpoint in the gateway routes.
> The settlement-engine receives trade events via Kafka and processes settlements internally.
> To trigger a settlement cycle manually, you would need to call the settlement-engine gRPC API directly:
> ```bash
> # Direct gRPC call (requires grpcurl)
> grpcurl -plaintext localhost:50054 SettlementService/RunSettlementCycle
> ```
> This is a known API contract gap identified by e2e tests (T055).

**What this proves:** Settlement cycles are queryable via gateway. Manual triggering requires direct gRPC access.

**Troubleshooting:**
- Empty positions/margin — no trades have been executed yet; run Section 3 first.
- `502` on clearing/margin/settlement — check that the respective engine is healthy.

---

## 5. Physical Delivery Flow

The warehouse-service manages electronic warehouse receipts (eWR), inspections, and physical delivery workflows.

> **Note:** The gateway does not currently route to warehouse-service. These endpoints are accessed
> directly via the warehouse-service gRPC API (port 50058). If the gateway is extended to include
> warehouse routes, update the URLs below to use port 8080.

### 5.1 Register a Warehouse Facility (via gRPC)

```bash
# Using grpcurl:
grpcurl -plaintext -d '{
  "facility_code": "WH-KS-001",
  "name": "Kansas City Grain Terminal",
  "operator_id": "op_001",
  "license_number": "KS-2026-0001",
  "region": "US-KS",
  "total_capacity": "50000.0000",
  "capacity_unit": "MT"
}' localhost:50058 WarehouseService/RegisterFacility
```

### 5.2 Issue a Warehouse Receipt

```bash
grpcurl -plaintext -d '{
  "facility_id": "<facility_id>",
  "holder_id": "<participant_id>",
  "commodity_id": "WHT-HRW",
  "grade": "US No.1",
  "quantity": "1000.0000",
  "unit": "BU",
  "lot_number": "LOT-2026-001",
  "harvest_year": 2026
}' localhost:50058 WarehouseService/IssueReceipt
```

**Expected:** Receipt object with `status: "ACTIVE"` and a unique `receipt_id`.

### 5.3 Pledge Receipt as Collateral

```bash
grpcurl -plaintext -d '{
  "receipt_id": "<receipt_id>",
  "pledged_to": "ACE-CCP"
}' localhost:50058 WarehouseService/PledgeReceipt
```

**Expected:** Receipt status changes to `PLEDGED`, `pledged_to: "ACE-CCP"`.

### 5.4 Initiate Delivery

```bash
grpcurl -plaintext -d '{
  "receipt_id": "<receipt_id>",
  "obligation_id": "<from settlement>",
  "seller_id": "<seller_participant_id>",
  "buyer_id": "<buyer_participant_id>",
  "delivery_type": "PHYSICAL",
  "quantity": "1000.0000",
  "scheduled_date": "2026-07-15"
}' localhost:50058 WarehouseService/InitiateDelivery
```

**Expected:** Delivery instruction with `status: "PENDING"`.

### 5.5 Complete Delivery

```bash
grpcurl -plaintext -d '{
  "delivery_id": "<delivery_id>"
}' localhost:50058 WarehouseService/CompleteDelivery
```

**Expected:** Delivery status changes to `COMPLETED`. Receipt ownership transfers to buyer.

**What this proves:** Full physical delivery lifecycle — receipt issuance, collateral pledge, delivery initiation, and completion.

**Troubleshooting:**
- `grpcurl` not installed: `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`
- Connection refused on 50058 — warehouse-service not running or port not exposed.

---

## 6. Market Data Flow

### 6.1 Get Order Book via Gateway

```bash
curl -s "http://localhost:8080/api/v1/instruments/WHT-HRW-2026M07-UB/book" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Bid/ask levels with price and quantity.

### 6.2 Get Last Trade

```bash
curl -s "http://localhost:8080/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 6.3 WebSocket: Live Trade Feed

```bash
# Using websocat (or wscat):
websocat "ws://localhost:8080/api/v1/ws/trades/WHT-HRW-2026M07-UB"
```

**Expected:** Each trade produces a JSON message:

```json
{
  "trade_id": "trd_...",
  "instrument_id": "WHT-HRW-2026M07-UB",
  "price": "550.0000",
  "quantity": 10,
  "aggressor_side": "SELL",
  "executed_at": "2026-03-28T..."
}
```

### 6.4 WebSocket: Live Order Book Updates

```bash
websocat "ws://localhost:8080/api/v1/ws/book/WHT-HRW-2026M07-UB"
```

### 6.5 WebSocket: Execution Reports (authenticated)

```bash
websocat "ws://localhost:8080/api/v1/ws/executions"
```

> **Note:** Market-data-service (OHLCV candles, ticker data) is accessed via gRPC on port 50057.
> The gateway does not currently proxy market-data-service REST endpoints.
> Future gateway routes: `GET /api/v1/market-data/candles?instrument=...&interval=1m`

**What this proves:** Real-time data distribution via WebSockets. Market data updates push to connected clients.

**Troubleshooting:**
- WebSocket upgrade fails with 400 — ensure client sends proper `Upgrade: websocket` headers.
- No messages — submit orders (Section 3) while connected to generate events.

---

## 7. Compliance & Risk

### 7.1 Screen a Participant

```bash
curl -s -X POST http://localhost:8080/api/v1/screening/check \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "participant_id": "'"$PARTICIPANT_ID"'",
    "screening_type": "SANCTIONS"
  }' | jq .
```

**Expected response (HTTP 200):**

```json
{
  "screening_id": "scr_...",
  "participant_id": "...",
  "status": "CLEAR",
  "risk_level": "LOW",
  "checked_at": "2026-03-28T..."
}
```

### 7.2 Batch Screening

```bash
curl -s -X POST http://localhost:8080/api/v1/screening/batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "participant_ids": ["'"$PARTICIPANT_ID"'"],
    "screening_type": "PEP"
  }' | jq .
```

### 7.3 Get Risk Score

```bash
curl -s "http://localhost:8080/api/v1/risk-scores/${PARTICIPANT_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

**Expected:** Risk score object with `score`, `factors`, `last_updated`.

### 7.4 View Compliance Alerts

```bash
curl -s "http://localhost:8080/api/v1/compliance/alerts" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 7.5 View Audit Trail

```bash
curl -s "http://localhost:8080/api/v1/compliance/audit-trail" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 7.6 File a Suspicious Activity Report (SAR)

```bash
curl -s -X POST http://localhost:8080/api/v1/compliance/sar \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "participant_id": "'"$PARTICIPANT_ID"'",
    "reason": "Unusual trading pattern detected",
    "details": "High-frequency wash trades between related accounts"
  }' | jq .
```

### 7.7 Suspend a Participant

```bash
curl -s -X POST "http://localhost:8080/api/v1/compliance/participants/${PARTICIPANT_ID}/suspend" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"reason": "Pending investigation"}' | jq .
```

### 7.8 Demonstrate a Margin Call Scenario

To trigger a margin call:

1. **Establish a position** — buy 100 lots at $550 (Section 3).
2. **Move the market against the position** — submit sell orders at progressively lower prices ($540, $530, $520) from another account, causing mark-to-market losses.
3. **Check margin calls:**
   ```bash
   curl -s "http://localhost:8080/api/v1/margin/calls" \
     -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
   ```
4. When mark-to-market loss exceeds maintenance margin, a margin call is generated.

### 7.9 Circuit Breaker

The matching-engine supports circuit breakers to halt trading when prices move beyond configured limits.

```bash
# Set circuit breaker for an instrument (admin only)
curl -s -X PUT "http://localhost:8080/api/v1/admin/instruments/WHT-HRW-2026M07-UB/circuit-breaker" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "upper_limit": "600.0000",
    "lower_limit": "500.0000",
    "cooldown_seconds": 300
  }' | jq .
```

When a trade executes outside the limits, the instrument halts automatically.

**What this proves:** Compliance screening, risk scoring, SAR filing, circuit breakers, and margin call mechanisms are functional.

---

## 8. Admin Operations

### 8.1 Admin Dashboard (port 3001)

The admin dashboard (if running) provides:
- **Service Health** — real-time status of all 9 services (calls each `/healthz`)
- **Settlement Cycles** — view and manage via `GET /api/v1/settlement/cycles`
- **Circuit Breakers** — configure per-instrument via `PUT /api/v1/admin/instruments/{id}/circuit-breaker`
- **Compliance Alerts** — queue of pending alerts via `GET /api/v1/compliance/alerts`
- **Participant Management** — KYC review via `GET/POST /api/v1/participants`

### 8.2 Halt/Resume Trading on an Instrument

```bash
# Halt
curl -s -X POST "http://localhost:8080/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .

# Resume
curl -s -X POST "http://localhost:8080/api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.3 Bust a Trade (error trade correction)

```bash
curl -s -X POST "http://localhost:8080/api/v1/admin/trades/{trade_id}/bust" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.4 Disable a Participant from Trading

```bash
curl -s -X POST "http://localhost:8080/api/v1/admin/participants/{participant_id}/disable" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.5 Mass Cancel All Orders

```bash
curl -s -X POST "http://localhost:8080/api/v1/admin/mass-cancel" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "instrument_id": "WHT-HRW-2026M07-UB"
  }' | jq .
```

### 8.6 Margin Call Statistics

```bash
curl -s "http://localhost:8080/api/v1/margin/calls/stats" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

**What this proves:** Exchange admin has full operational control — halt/resume, trade busting, participant management, mass cancel.

---

## 9. Production Readiness Checklist

### 9.1 Service Summary

| Service | Health Endpoint | Tests | Coverage | Known Gaps |
|---|---|---|---|---|
| matching-engine | :8081/healthz | ~89 | 89.3% (orderbook) | No persistence (in-memory only) |
| clearing-engine | :8082/healthz | ~80 | 79.9% avg | Novation coverage improved to 100% |
| margin-engine | :8083/healthz | ~60 | 96.7% (engine) | — |
| settlement-engine | :8084/healthz | ~55 | PnL coverage improved | No `POST /settlement/cycle` in gateway |
| auth-service | :8085/healthz | 43 | 88.5% (business logic) | TokenPair missing json tags; HMAC JWT (should be RSA for prod) |
| compliance-service | :8086/healthz | ~50 | 80.1% (onboarding) | — |
| market-data-service | :8087/healthz | ~40 | 68.7% (candle) | No gateway REST routes |
| warehouse-service | :8088/healthz | ~45 | 83.8% (store) | No gateway REST routes |
| gateway | :8080/healthz | ~60 | 56.7%-93% | WebSocket 0% coverage |
| web-ui | :3000 | 84 | 60-100% biz logic | Component coverage 0% (needs React Testing Library) |
| admin-ui | :3001 | 64 | 50-100% biz logic | Component coverage 0% |

**Totals:** 689 Go unit tests + 148 frontend tests + 20 e2e tests = **857 tests**

**Average Go coverage:** 66.5% (statement-weighted)

### 9.2 Infrastructure Readiness

| Component | Status | Notes |
|---|---|---|
| DB Migrations | Partial | V8 conflict: duplicate `V8__market_data_timescaledb.sql` and `V8__warehouse_tables.sql` — renumber one |
| Kafka Topics | Channel-based | In-memory Go channels; no wire-protocol Kafka client yet. Real adapters needed for production |
| K8s Manifests | Reworked (T054) | Namespace, ConfigMap, and Secret cross-references fixed |
| Docker Compose | Working (T053) | All 9 services + infra; V8 migration conflict noted |
| Helm Charts | Not created | — |

### 9.3 Security Checklist

| Item | Status | Detail |
|---|---|---|
| JWT Signing | HMAC-SHA256 | **Must change to RSA/ECDSA for production** (asymmetric keys allow verification without sharing signing key) |
| mTLS | Configured (Istio) | K8s manifests include Istio sidecar injection |
| RBAC | 5 roles | `super_admin`, `admin`, `trader`, `viewer`, `compliance_officer` with fine-grained permissions |
| Password Hashing | bcrypt cost 12 | Industry standard |
| SQL Injection | Parameterized queries | All SQL uses parameterized queries |
| Token Storage | No localStorage | Tokens managed server-side; frontend uses httpOnly cookie pattern |
| PKCE | S256 only | `plain` method rejected |
| Rate Limiting | Gateway middleware | Configurable per-route rate limits |
| Input Validation | System boundaries | Email regex, password length, role validation |

### 9.4 Performance Baselines

| Metric | Target | Status |
|---|---|---|
| Matching engine latency | <50ms per match | **No benchmarks exist** — flag as gap |
| Gateway throughput | TBD | No load testing performed |
| WebSocket fan-out | TBD | No benchmarks |
| Order book depth | TBD | In-memory; scales with available RAM |

> **Gap:** No performance benchmarks exist. Before production, run:
> - `wrk` or `vegeta` load tests against gateway
> - Matching engine microbenchmarks (Go `testing.B`)
> - WebSocket connection limit tests

### 9.5 Monitoring Gaps

| Gap | Priority | Notes |
|---|---|---|
| Prometheus metrics | HIGH | No `/metrics` endpoints on any service |
| Grafana dashboards | HIGH | No dashboards; recommended: latency, throughput, error rates |
| Distributed tracing | MEDIUM | No OpenTelemetry/Jaeger integration |
| Alerting | HIGH | No PagerDuty/OpsGenie integration |
| Log aggregation | MEDIUM | Structured logging (JSON) exists but no ELK/Loki |

### 9.6 Data Integrity

| Feature | Status |
|---|---|
| Append-only trades table | Implemented (T004 schema) |
| SHA-256 audit hash chain | Implemented — each row references previous hash |
| Sequence numbers | Global sequence on orders and trades |
| Double-entry inventory | Warehouse service uses double-entry booking |

### 9.7 Disaster Recovery

| Item | Status |
|---|---|
| RTO/RPO targets | **Not defined** — gap |
| Backup strategy | **Not defined** — gap (rely on PG base backup + WAL archiving) |
| Multi-region | Not implemented |
| Failover | Not implemented |

### 9.8 Regulatory Compliance

| Feature | Status |
|---|---|
| KYC/AML workflow | Implemented — application, document upload, approval/rejection |
| Sanctions screening | Implemented — single + batch screening |
| PEP screening | Implemented |
| SAR filing | Implemented — `POST /api/v1/compliance/sar` |
| Audit trail | Append-only + SHA-256 hash chain |
| Participant suspension | Implemented — suspend/reinstate via compliance admin |
| Risk scoring | Implemented — per-participant risk scores |

---

## Quick Reference: All Gateway API Routes

### Auth (public)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/register` | Register new user |
| POST | `/api/v1/auth/login` | Login, get JWT |
| POST | `/api/v1/auth/refresh` | Refresh token |
| POST | `/api/v1/auth/logout` | Logout |
| GET | `/api/v1/auth/me` | Get profile |
| POST | `/api/v1/auth/password/change` | Change password |
| POST | `/api/v1/auth/password/reset` | Request password reset |

### Orders (authenticated — trader)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/orders` | Submit order |
| GET | `/api/v1/orders` | List open orders |
| GET | `/api/v1/orders/{order_id}` | Get order |
| PATCH | `/api/v1/orders/{order_id}` | Modify order |
| DELETE | `/api/v1/orders/{order_id}` | Cancel order |
| DELETE | `/api/v1/orders` | Cancel all orders |

### Market Data (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/instruments/{instrument_id}/book` | L2 order book |
| GET | `/api/v1/instruments/{instrument_id}/book/l3` | L3 order book |
| GET | `/api/v1/instruments/{instrument_id}/trades/latest` | Last trade |

### WebSocket (unauthenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/ws/trades/{instrument_id}` | Live trade feed |
| GET | `/api/v1/ws/book/{instrument_id}` | Live book updates |
| GET | `/api/v1/ws/executions` | Execution reports |

### Clearing (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/clearing/positions` | All positions |
| GET | `/api/v1/clearing/positions/{instrument_id}` | Position by instrument |
| GET | `/api/v1/clearing/netting` | Netting obligations |

### Margin (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/margin` | Portfolio margin |
| POST | `/api/v1/margin/calculate` | Calculate margin |
| GET | `/api/v1/margin/calls` | Active margin calls |
| GET | `/api/v1/margin/calls/stats` | Margin call stats |

### Settlement (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/settlement/cycles` | List cycles |
| GET | `/api/v1/settlement/cycles/{cycle_id}` | Get cycle |

### Compliance — Onboarding (authenticated)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/participants` | Submit application |
| GET | `/api/v1/participants` | List applications |
| GET | `/api/v1/participants/{participant_id}` | Get application |
| POST | `/api/v1/participants/{participant_id}/documents` | Upload document |
| GET | `/api/v1/participants/{participant_id}/documents` | List documents |
| POST | `/api/v1/participants/{participant_id}/approve` | Approve (admin) |
| POST | `/api/v1/participants/{participant_id}/reject` | Reject (admin) |

### Compliance — Screening (authenticated)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/screening/check` | Screen participant |
| GET | `/api/v1/screening/{screening_id}` | Get screening result |
| POST | `/api/v1/screening/batch` | Batch screen |
| POST | `/api/v1/screening/{screening_id}/resolve` | Resolve match |
| GET | `/api/v1/risk-scores/{participant_id}` | Get risk score |

### Compliance — Admin (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/compliance/alerts` | List alerts |
| POST | `/api/v1/compliance/alerts/{alert_id}/resolve` | Resolve alert |
| GET | `/api/v1/compliance/audit-trail` | Audit trail |
| POST | `/api/v1/compliance/sar` | File SAR |
| POST | `/api/v1/compliance/participants/{participant_id}/suspend` | Suspend |
| POST | `/api/v1/compliance/participants/{participant_id}/reinstate` | Reinstate |

### Admin — Exchange Operations (admin only)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/admin/instruments/{instrument_id}/halt` | Halt instrument |
| POST | `/api/v1/admin/instruments/{instrument_id}/resume` | Resume instrument |
| POST | `/api/v1/admin/trades/{trade_id}/bust` | Bust trade |
| PUT | `/api/v1/admin/instruments/{instrument_id}/circuit-breaker` | Set circuit breaker |
| POST | `/api/v1/admin/participants/{participant_id}/disable` | Disable participant |
| POST | `/api/v1/admin/mass-cancel` | Mass cancel orders |

### Health (public)
| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |
