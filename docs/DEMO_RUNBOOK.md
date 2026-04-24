# GarudaX — Multi-Tenant AI-Native Trading Platform Demo Runbook

Step-by-step guide for demonstrating the full multi-tenant GarudaX platform, including commodity exchange, securities exchange, platform administration, and AI bot operations.

> **Base URL:** All `curl` commands assume the gateway is on `https://garudax.asla.mn`.
> For local development, replace with `http://localhost:8080`.
>
> **Tenant Header:** Securities and platform APIs require the `X-GarudaX-Tenant` header.
> Use `ace-commodities` for commodity exchange, `mse-equities` for MSE equities.
>
> **Demo Runner:** Available at `https://demo.garudax.asla.mn` (or `http://localhost:3002` locally).

### DNS Setup

To use the production domains, configure DNS A records:

| Domain | Target |
|---|---|
| `garudax.asla.mn` | Your server/load balancer IP |
| `demo.garudax.asla.mn` | Your server/load balancer IP |

TLS certificates are automatically provisioned via Let's Encrypt (cert-manager) in Kubernetes,
or can be configured manually in `infrastructure/nginx/nginx.conf` for Docker Compose deployments.

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
10. [Platform Admin Demo](#10-platform-admin-demo)
11. [Securities Exchange Demo](#11-securities-exchange-demo)
12. [Securities Lifecycle Demo](#12-securities-lifecycle-demo)
13. [Admin UI Walkthrough](#13-admin-ui-walkthrough)
14. [Bot Commands](#14-bot-commands)

---

## 1. Environment Setup

### 1.1 Start the Stack

```bash
# From the project root
docker compose up -d --build
```

Docker Compose brings up 16 containers: postgres (TimescaleDB), redis, zookeeper, kafka, and 12 application services.

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
| securities-service | 9089 | 50059 |
| platform-service | 9090 | N/A |
| gateway | 8080 (HTTP API) / 8090 (health) | N/A |

**Check all health endpoints:**

```bash
# Gateway (serves API on 8080, health on 8090 inside container, proxied on 8080)
curl -s https://garudax.asla.mn/healthz | jq .
# Expected: {"status":"ok","service":"garudax-gateway"}

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

# Securities & Platform services
curl -s http://localhost:9089/healthz
# Expected: ok

curl -s http://localhost:9090/healthz
# Expected: ok
```

> **Note:** The gateway's `/healthz` returns JSON (`{"status":"ok","service":"garudax-gateway"}`).
> The individual services return plain text `ok`.

**One-liner to check all services:**

```bash
for port in 8080 8081 8082 8083 8084 8085 8086 8087 8088 9089 9090; do
  printf "port %s: " "$port"
  curl -s -o /dev/null -w "%{http_code}" "http://localhost:${port}/healthz"
  echo
done
# All should print: 200
```

### 1.3 Verify Frontend Apps

If built (requires Node.js):

- **Trading Web UI:** https://garudax.asla.mn (or http://localhost:3000 locally)
- **Admin Dashboard:** https://garudax.asla.mn/admin (or http://localhost:3001 locally)
- **Demo Runner:** https://demo.garudax.asla.mn (or http://localhost:3002 locally)

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
curl -s -X POST https://garudax.asla.mn/api/v1/auth/register \
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
curl -s -X POST https://garudax.asla.mn/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader2@example.com",
    "password": "SecurePass123!",
    "role": "trader"
  }' | jq .
```

### 2.3 Register an Admin User

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/auth/register \
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
TRADER1_RESPONSE=$(curl -s -X POST https://garudax.asla.mn/api/v1/auth/login \
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
TRADER2_RESPONSE=$(curl -s -X POST https://garudax.asla.mn/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "trader2@example.com",
    "password": "SecurePass123!"
  }')
TRADER2_TOKEN=$(echo "$TRADER2_RESPONSE" | jq -r '.AccessToken // .access_token')
```

**Login as admin:**

```bash
ADMIN_RESPONSE=$(curl -s -X POST https://garudax.asla.mn/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "AdminPass123!"
  }')
ADMIN_TOKEN=$(echo "$ADMIN_RESPONSE" | jq -r '.AccessToken // .access_token')
```

### 2.5 Submit KYC / Participant Onboarding Application

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/participants \
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
curl -s -X POST "https://garudax.asla.mn/api/v1/participants/${PARTICIPANT_ID}/approve" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{}' | jq .
```

**Expected:** Participant status changes to `APPROVED`.

**Verify:**

```bash
curl -s "https://garudax.asla.mn/api/v1/participants/${PARTICIPANT_ID}" \
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
curl -s -X POST https://garudax.asla.mn/api/v1/orders \
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
curl -s -X POST https://garudax.asla.mn/api/v1/orders \
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
curl -s "https://garudax.asla.mn/api/v1/instruments/WHT-HRW-2026M07-UB/book" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .

# Level 3 order book (individual orders)
curl -s "https://garudax.asla.mn/api/v1/instruments/WHT-HRW-2026M07-UB/book/l3" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** After the full fill above, the book should be empty (both orders fully matched). Submit a new order without a counterparty to see it resting in the book.

### 3.5 View Last Trade

```bash
curl -s "https://garudax.asla.mn/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** The trade from step 3.3 with price 550.0000, quantity 10.

### 3.6 List Open Orders

```bash
curl -s "https://garudax.asla.mn/api/v1/orders" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Empty list (both orders filled) or any resting orders.

### 3.7 Cancel an Order

```bash
# First, place a new order that won't match
curl -s -X POST https://garudax.asla.mn/api/v1/orders \
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
curl -s -X DELETE "https://garudax.asla.mn/api/v1/orders/${ORDER_ID}" \
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
curl -s "https://garudax.asla.mn/api/v1/clearing/positions" \
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
curl -s "https://garudax.asla.mn/api/v1/clearing/positions/WHT-HRW-2026M07-UB" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 4.3 View Netting Obligations

```bash
curl -s "https://garudax.asla.mn/api/v1/clearing/netting" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**What this proves:** Trade flows from matching-engine to clearing-engine. Novation creates CCP-intermediated positions.

### 4.4 View Margin Requirements

```bash
curl -s "https://garudax.asla.mn/api/v1/margin" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Portfolio margin showing initial margin, maintenance margin, and available balance.

### 4.5 Calculate Margin (explicit)

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/margin/calculate \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TRADER1_TOKEN" \
  -d '{}' | jq .
```

### 4.6 View Margin Calls

```bash
curl -s "https://garudax.asla.mn/api/v1/margin/calls" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 4.7 Trigger End-of-Day Settlement

```bash
# View existing settlement cycles
curl -s "https://garudax.asla.mn/api/v1/settlement/cycles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .

# View a specific cycle
curl -s "https://garudax.asla.mn/api/v1/settlement/cycles/{cycle_id}" \
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
  "pledged_to": "GarudaX-CCP"
}' localhost:50058 WarehouseService/PledgeReceipt
```

**Expected:** Receipt status changes to `PLEDGED`, `pledged_to: "GarudaX-CCP"`.

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
curl -s "https://garudax.asla.mn/api/v1/instruments/WHT-HRW-2026M07-UB/book" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

**Expected:** Bid/ask levels with price and quantity.

### 6.2 Get Last Trade

```bash
curl -s "https://garudax.asla.mn/api/v1/instruments/WHT-HRW-2026M07-UB/trades/latest" \
  -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
```

### 6.3 WebSocket: Live Trade Feed

```bash
# Using websocat (or wscat):
websocat "wss://garudax.asla.mn/api/v1/ws/trades/WHT-HRW-2026M07-UB"
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
websocat "wss://garudax.asla.mn/api/v1/ws/book/WHT-HRW-2026M07-UB"
```

### 6.5 WebSocket: Execution Reports (authenticated)

```bash
websocat "wss://garudax.asla.mn/api/v1/ws/executions"
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
curl -s -X POST https://garudax.asla.mn/api/v1/screening/check \
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
curl -s -X POST https://garudax.asla.mn/api/v1/screening/batch \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "participant_ids": ["'"$PARTICIPANT_ID"'"],
    "screening_type": "PEP"
  }' | jq .
```

### 7.3 Get Risk Score

```bash
curl -s "https://garudax.asla.mn/api/v1/risk-scores/${PARTICIPANT_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

**Expected:** Risk score object with `score`, `factors`, `last_updated`.

### 7.4 View Compliance Alerts

```bash
curl -s "https://garudax.asla.mn/api/v1/compliance/alerts" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 7.5 View Audit Trail

```bash
curl -s "https://garudax.asla.mn/api/v1/compliance/audit-trail" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 7.6 File a Suspicious Activity Report (SAR)

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/compliance/sar \
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
curl -s -X POST "https://garudax.asla.mn/api/v1/compliance/participants/${PARTICIPANT_ID}/suspend" \
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
   curl -s "https://garudax.asla.mn/api/v1/margin/calls" \
     -H "Authorization: Bearer $TRADER1_TOKEN" | jq .
   ```
4. When mark-to-market loss exceeds maintenance margin, a margin call is generated.

### 7.9 Circuit Breaker

The matching-engine supports circuit breakers to halt trading when prices move beyond configured limits.

```bash
# Set circuit breaker for an instrument (admin only)
curl -s -X PUT "https://garudax.asla.mn/api/v1/admin/instruments/WHT-HRW-2026M07-UB/circuit-breaker" \
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
curl -s -X POST "https://garudax.asla.mn/api/v1/admin/instruments/WHT-HRW-2026M07-UB/halt" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .

# Resume
curl -s -X POST "https://garudax.asla.mn/api/v1/admin/instruments/WHT-HRW-2026M07-UB/resume" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.3 Bust a Trade (error trade correction)

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/admin/trades/{trade_id}/bust" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.4 Disable a Participant from Trading

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/admin/participants/{participant_id}/disable" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 8.5 Mass Cancel All Orders

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/admin/mass-cancel" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "instrument_id": "WHT-HRW-2026M07-UB"
  }' | jq .
```

### 8.6 Margin Call Statistics

```bash
curl -s "https://garudax.asla.mn/api/v1/margin/calls/stats" \
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
| securities-service | :9089/healthz | 100+ | ~80% | Multi-tenant, equities/bonds/ETF |
| platform-service | :9090/healthz | 50+ | ~75% | Tenant lifecycle, provisioning |
| gateway | :8080/healthz | ~60 | 56.7%-93% | WebSocket 0% coverage |
| web-ui | :3000 | 84 | 60-100% biz logic | Component coverage 0% (needs React Testing Library) |
| admin-ui | :3001 | 144 | 50-100% biz logic | Component coverage 0% |

**Totals:** 828 Go unit tests + 313 frontend tests + 58 e2e tests = **1199 tests**

**Average Go coverage:** 66.5% (statement-weighted)

### 9.2 Infrastructure Readiness

| Component | Status | Notes |
|---|---|---|
| DB Migrations | Partial | V8 conflict: duplicate `V8__market_data_timescaledb.sql` and `V8__warehouse_tables.sql` — renumber one |
| Kafka Topics | Channel-based | In-memory Go channels; no wire-protocol Kafka client yet. Real adapters needed for production |
| K8s Manifests | Reworked (T054) | Namespace, ConfigMap, and Secret cross-references fixed |
| Docker Compose | Working (T053) | All 12 services + infra; V8 migration conflict noted |
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

## 10. Platform Admin Demo

The platform-service manages tenants (trading venues) on the GarudaX platform. Each tenant is an independent exchange with its own instruments, orders, and regulatory requirements.

> **Tenant concept:** GarudaX is the platform. Tenants are the venues. Current tenants:
> - `ace-commodities` (ACTIVE) — Mongolian commodity exchange
> - `mse-equities` (ONBOARDING) — Mongolian Stock Exchange (flagship)

### 10.1 List All Tenants

```bash
curl -s https://garudax.asla.mn/platform/v1/tenants | jq .
```

**Expected response (HTTP 200):**

```json
[
  {
    "id": "ace-commodities",
    "name": "ACE Commodity Exchange",
    "status": "ACTIVE",
    "flagship": false,
    "governance_tier": "STANDARD",
    "onboarding_metadata": {},
    "created_at": "2026-04-01T00:00:00Z",
    "updated_at": "2026-04-01T00:00:00Z"
  },
  {
    "id": "mse-equities",
    "name": "Mongolian Stock Exchange",
    "status": "ONBOARDING",
    "flagship": true,
    "governance_tier": "FLAGSHIP",
    "onboarding_metadata": {},
    "created_at": "2026-04-01T00:00:00Z",
    "updated_at": "2026-04-01T00:00:00Z"
  }
]
```

**What this proves:** Platform service is running and the tenant registry is populated.

### 10.2 Get a Single Tenant

```bash
curl -s https://garudax.asla.mn/platform/v1/tenants/mse-equities | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "mse-equities",
  "name": "Mongolian Stock Exchange",
  "status": "ONBOARDING",
  "flagship": true,
  "governance_tier": "FLAGSHIP",
  "onboarding_metadata": {},
  "created_at": "2026-04-01T00:00:00Z",
  "updated_at": "2026-04-01T00:00:00Z"
}
```

### 10.3 Create a New Tenant

```bash
curl -s -X POST https://garudax.asla.mn/platform/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "id": "test-exchange",
    "name": "Test Exchange Sandbox",
    "governance_tier": "STANDARD"
  }' | jq .
```

**Expected response (HTTP 201):**

```json
{
  "tenant": {
    "id": "test-exchange",
    "name": "Test Exchange Sandbox",
    "status": "ONBOARDING",
    "flagship": false,
    "governance_tier": "STANDARD",
    "onboarding_metadata": {},
    "created_at": "2026-04-24T...",
    "updated_at": "2026-04-24T..."
  },
  "provisioning": {
    "tenant_id": "test-exchange",
    "schemas_created": ["test_exchange_trading", "test_exchange_clearing"],
    "topic_prefixes": ["test-exchange.orders", "test-exchange.trades"],
    "status": "COMPLETED"
  }
}
```

**What this proves:** Tenant creation triggers automatic provisioning of database schemas and Kafka topic prefixes.

**Validation rules:**
- `id` is required, must match `[a-z0-9-]+` (lowercase slug)
- `name` is required
- `governance_tier` defaults to `STANDARD` if omitted
- Status defaults to `ONBOARDING`
- Duplicate `id` returns `409 TENANT_ALREADY_EXISTS`

### 10.4 Get Tenant Configuration

```bash
curl -s https://garudax.asla.mn/platform/v1/tenants/mse-equities/config | jq .
```

**Expected response (HTTP 200):** The venue configuration from `venues/mse-equities/config.json`:

```json
{
  "tenant_id": "mse-equities",
  "name": "Mongolian Stock Exchange",
  "trading_calendar": {
    "timezone": "Asia/Ulaanbaatar",
    "trading_hours": {"open": "10:00", "close": "13:00"},
    "pre_open_auction": {"start": "09:30", "end": "10:00"},
    "closing_auction": {"start": "12:50", "end": "13:00"},
    "holidays": ["2026-01-01", "2026-02-10", "2026-02-11", "2026-07-11", "2026-07-12", "2026-07-13"]
  },
  "settlement": {"default_cycle": "T+2", "csd": "MCSD", "delivery": "book-entry"},
  "circuit_breakers": {"upper_limit_pct": 15.0, "lower_limit_pct": 15.0, "cooldown_minutes": 10},
  "compliance": {"kyc_required": true, "aml_screening": true, "frc_reporting": true},
  "features": {"short_selling": true, "auctions": true, "corporate_actions": true, "frc_reporting": true}
}
```

### 10.5 Update Tenant Status

```bash
# Activate the MSE tenant
curl -s -X PUT https://garudax.asla.mn/platform/v1/tenants/mse-equities/status \
  -H "Content-Type: application/json" \
  -d '{"status": "ACTIVE"}' | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "mse-equities",
  "name": "Mongolian Stock Exchange",
  "status": "ACTIVE",
  "flagship": true,
  "governance_tier": "FLAGSHIP",
  "onboarding_metadata": {},
  "created_at": "2026-04-01T00:00:00Z",
  "updated_at": "2026-04-24T..."
}
```

**Valid status values:** `ACTIVE`, `SUSPENDED`, `ONBOARDING`, `DECOMMISSIONED`

### 10.6 Update Tenant Metadata

```bash
curl -s -X PATCH https://garudax.asla.mn/platform/v1/tenants/mse-equities \
  -H "Content-Type: application/json" \
  -d '{
    "name": "MSE - Mongolian Stock Exchange",
    "governance_tier": "FLAGSHIP"
  }' | jq .
```

**Expected response (HTTP 200):** Updated tenant object with new name and governance tier.

**Updatable fields:** `name`, `governance_tier` only. Other fields are immutable or managed via dedicated endpoints.

### 10.7 Suspend a Tenant

```bash
curl -s -X PUT https://garudax.asla.mn/platform/v1/tenants/test-exchange/status \
  -H "Content-Type: application/json" \
  -d '{"status": "SUSPENDED"}' | jq .
```

**What this proves:** Full tenant lifecycle management — create, configure, activate, suspend, decommission.

---

## 11. Securities Exchange Demo

This section demonstrates the MSE (Mongolian Stock Exchange) equities trading workflow using the securities-service. All requests require the `X-GarudaX-Tenant: mse-equities` header.

> **MSE Trading Hours:** 10:00-13:00 UB time (Asia/Ulaanbaatar)
> **Pre-open auction:** 09:30-10:00 | **Closing auction:** 12:50-13:00
> **Settlement:** T+2 via MCSD (Mongolian Central Securities Depository)
> **Currency:** MNT (Mongolian Tugrik)

### 11.1 Create Equity Instruments

**APU JSC** — Mongolia's largest food and beverages company:

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/instruments \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "ticker": "APU",
    "name": "APU JSC",
    "asset_class": "EQUITY",
    "exchange_code": "MNSE",
    "lot_size": 1,
    "tick_size": 1.0,
    "currency": "MNT",
    "isin": "MN0000APU001",
    "outstanding_shares": 45000000
  }' | jq .
```

**Expected response (HTTP 201):**

```json
{
  "id": "a1b2c3d4-...",
  "ticker": "APU",
  "name": "APU JSC",
  "asset_class": "EQUITY",
  "exchange_code": "MNSE",
  "lot_size": 1,
  "tick_size": 1.0,
  "currency": "MNT",
  "isin": "MN0000APU001",
  "outstanding_shares": 45000000,
  "trading_status": "ACTIVE",
  "created_at": "2026-04-24T...",
  "updated_at": "2026-04-24T..."
}
```

Save the instrument ID:

```bash
APU_ID="<id from response>"
```

**Govisumber Mining JSC** — mining sector:

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/instruments \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "ticker": "GOV",
    "name": "Govisumber Mining JSC",
    "asset_class": "EQUITY",
    "exchange_code": "MNSE",
    "lot_size": 10,
    "tick_size": 50.0,
    "currency": "MNT",
    "isin": "MN0000GOV001",
    "outstanding_shares": 12000000
  }' | jq .
```

```bash
GOV_ID="<id from response>"
```

### 11.2 List Instruments

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/instruments?asset_class=EQUITY" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "data": [
    {
      "id": "...",
      "ticker": "APU",
      "name": "APU JSC",
      "asset_class": "EQUITY",
      "trading_status": "ACTIVE",
      "lot_size": 1,
      "tick_size": 1.0,
      "currency": "MNT"
    },
    {
      "id": "...",
      "ticker": "GOV",
      "name": "Govisumber Mining JSC",
      "asset_class": "EQUITY",
      "trading_status": "ACTIVE",
      "lot_size": 10,
      "tick_size": 50.0,
      "currency": "MNT"
    }
  ],
  "total": 2,
  "limit": 50,
  "offset": 0
}
```

**Query parameters:** `asset_class`, `trading_status`, `exchange_code`, `search`, `limit`, `offset`

### 11.3 Submit a Buy Order for APU at 850 MNT

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$APU_ID"'",
    "side": "BUY",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 850,
    "time_in_force": "GTC"
  }' | jq .
```

**Expected response (HTTP 201):**

```json
{
  "tenant_id": "mse-equities",
  "order": {
    "id": "e5f6g7h8-...",
    "instrument_id": "a1b2c3d4-...",
    "side": "BUY",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 850,
    "time_in_force": "GTC",
    "status": "PENDING",
    "filled_quantity": 0,
    "avg_fill_price": 0,
    "created_at": "2026-04-24T...",
    "updated_at": "2026-04-24T..."
  },
  "trades": []
}
```

Save the order ID:

```bash
BUY_ORDER_ID="<order.id from response>"
```

**Field reference (securities orders use string enums, not integers):**
- `side`: `BUY`, `SELL`, `SHORT_SELL` (disabled)
- `order_type`: `LIMIT`, `MARKET`, `STOP`, `STOP_LIMIT`
- `time_in_force`: `GTC`, `IOC`, `FOK`, `DAY`, `GTD`
- `price`: Numeric (must be a multiple of tick_size)
- `quantity`: Integer (must be a multiple of lot_size)

### 11.4 Submit a Matching Sell Order at 850 MNT

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$APU_ID"'",
    "side": "SELL",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 850,
    "time_in_force": "GTC"
  }' | jq .
```

**Expected response (HTTP 201):** Order created with a trade match:

```json
{
  "tenant_id": "mse-equities",
  "order": {
    "id": "i9j0k1l2-...",
    "instrument_id": "a1b2c3d4-...",
    "side": "SELL",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 850,
    "status": "FILLED",
    "filled_quantity": 100,
    "avg_fill_price": 850
  },
  "trades": [
    {
      "id": "trd_m3n4o5p6-...",
      "buy_order_id": "e5f6g7h8-...",
      "sell_order_id": "i9j0k1l2-...",
      "instrument_id": "a1b2c3d4-...",
      "price": 850,
      "quantity": 100,
      "trade_date": "2026-04-24T...",
      "settlement_date": "2026-04-28T...",
      "status": "TRADE_PENDING"
    }
  ]
}
```

**What this proves:** Price-time priority matching works in the securities-service. The sell order crosses the resting buy at 850 MNT.

### 11.5 View Trades and Positions

**List orders:**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/orders?instrument_id=${APU_ID}&status=FILLED" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response:**

```json
{
  "data": [
    {"id": "...", "side": "BUY", "status": "FILLED", "quantity": 100, "price": 850, "filled_quantity": 100},
    {"id": "...", "side": "SELL", "status": "FILLED", "quantity": 100, "price": 850, "filled_quantity": 100}
  ],
  "total": 2,
  "limit": 50,
  "offset": 0
}
```

**View positions:**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/positions" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

### 11.6 Submit a Market Order

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$APU_ID"'",
    "side": "BUY",
    "order_type": "MARKET",
    "quantity": 50
  }' | jq .
```

**Expected:** Market orders do not require a price. They match against the best available resting order. If no resting orders exist, the order remains PENDING.

### 11.7 Trade Govisumber Mining at 12500 MNT

```bash
# Place a buy order (note: lot_size=10, so quantity must be a multiple of 10)
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$GOV_ID"'",
    "side": "BUY",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 12500,
    "time_in_force": "GTC"
  }' | jq .

# Place a matching sell
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$GOV_ID"'",
    "side": "SELL",
    "order_type": "LIMIT",
    "quantity": 100,
    "price": 12500,
    "time_in_force": "GTC"
  }' | jq .
```

**Expected:** The sell order response includes a trade at 12500 MNT for 100 shares of Govisumber Mining.

**Validation notes:**
- Quantity 50 would fail: `"quantity must be a multiple of lot_size (10)"`
- Price 12510 would fail: `"price must be a multiple of tick_size (50)"`

### 11.8 Cancel an Order

```bash
# Place a far-from-market order
curl -s -X POST https://garudax.asla.mn/api/v1/securities/orders \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$APU_ID"'",
    "side": "BUY",
    "order_type": "LIMIT",
    "quantity": 10,
    "price": 700,
    "time_in_force": "GTC"
  }' | jq .

# Save the order ID, then cancel it
CANCEL_ID="<order.id from response>"
curl -s -X DELETE "https://garudax.asla.mn/api/v1/securities/orders/${CANCEL_ID}" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "...",
  "instrument_id": "...",
  "side": "BUY",
  "order_type": "LIMIT",
  "quantity": 10,
  "price": 700,
  "status": "CANCELLED",
  "filled_quantity": 0
}
```

**Cancellation rules:** Only `PENDING` and `PARTIALLY_FILLED` orders can be cancelled. Attempting to cancel a `FILLED` order returns `409 INVALID_STATE`.

### 11.9 Get a Single Instrument

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/instruments/${APU_ID}" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

### 11.10 Update Instrument Trading Status

```bash
# Halt APU trading
curl -s -X PUT "https://garudax.asla.mn/api/v1/securities/instruments/${APU_ID}/status" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "status": "HALTED",
    "reason": "Pending corporate announcement"
  }' | jq .
```

**Expected response (HTTP 200):**

```json
{
  "instrument_id": "a1b2c3d4-...",
  "previous_status": "ACTIVE",
  "current_status": "HALTED",
  "reason": "Pending corporate announcement",
  "changed_at": "2026-04-24T..."
}
```

Orders submitted for a halted instrument return `422 INSTRUMENT_NOT_ACTIVE`.

```bash
# Resume trading
curl -s -X PUT "https://garudax.asla.mn/api/v1/securities/instruments/${APU_ID}/status" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "status": "ACTIVE",
    "reason": "Announcement released",
    "resume_with_auction": true
  }' | jq .
```

**Valid statuses:** `ACTIVE`, `HALTED`, `SUSPENDED`, `DELISTED`

**What this proves:** Full equities trading lifecycle — instrument creation, order submission, matching, cancellation, and trading halts — all with MSE-specific parameters (MNT currency, MNSE exchange code, realistic tick/lot sizes).

---

## 12. Securities Lifecycle Demo

This section covers advanced securities features: market sessions (auction flow), corporate actions, settlement cycles, and FRC regulatory reports.

### 12.1 Market Sessions — Auction Flow

MSE uses a call auction model: PRE_OPEN -> CONTINUOUS -> CLOSING_AUCTION -> CLOSED.

**View current sessions:**

```bash
curl -s https://garudax.asla.mn/api/v1/securities/sessions \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "sessions": {
    "a1b2c3d4-...": "CLOSED"
  }
}
```

**Transition to PRE_OPEN (09:30 — collect orders for opening auction):**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/sessions/${APU_ID}/transition" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{"session": "PRE_OPEN"}' | jq .
```

**Expected response (HTTP 200):**

```json
{
  "instrument_id": "a1b2c3d4-...",
  "current_session": "PRE_OPEN"
}
```

**Transition to CONTINUOUS (10:00 — opening auction executes, continuous trading begins):**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/sessions/${APU_ID}/transition" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{"session": "CONTINUOUS"}' | jq .
```

**Expected response (HTTP 200):** If orders were collected during PRE_OPEN, the auction executes:

```json
{
  "instrument_id": "a1b2c3d4-...",
  "current_session": "CONTINUOUS",
  "auction_result": {
    "instrument_id": "a1b2c3d4-...",
    "clearing_price": 855,
    "matched_volume": 200,
    "unmatched_buy_volume": 50,
    "unmatched_sell_volume": 0,
    "trade_count": 3
  }
}
```

**Transition to CLOSING_AUCTION (12:50):**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/sessions/${APU_ID}/transition" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{"session": "CLOSING_AUCTION"}' | jq .
```

**Transition to CLOSED (13:00 — closing auction executes, market closes):**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/sessions/${APU_ID}/transition" \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{"session": "CLOSED"}' | jq .
```

**Valid session values:** `PRE_OPEN`, `CONTINUOUS`, `CLOSING_AUCTION`, `CLOSED`

**What this proves:** Full MSE trading day lifecycle with opening/closing call auctions matching the real MSE schedule (09:30-10:00 pre-open, 10:00-12:50 continuous, 12:50-13:00 closing auction).

### 12.2 Corporate Actions — Dividend

**Announce a cash dividend for APU JSC:**

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/corporate-actions \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$APU_ID"'",
    "action_type": "CA_DIVIDEND",
    "announcement_date": "2026-04-24",
    "ex_date": "2026-05-10",
    "record_date": "2026-05-12",
    "payment_date": "2026-06-01",
    "details": {
      "dividend_amount": 50.0,
      "dividend_type": "CASH",
      "currency": "MNT"
    }
  }' | jq .
```

**Expected response (HTTP 201):**

```json
{
  "id": "ca_q1r2s3t4-...",
  "instrument_id": "a1b2c3d4-...",
  "action_type": "CA_DIVIDEND",
  "status": "ANNOUNCED",
  "announcement_date": "2026-04-24",
  "ex_date": "2026-05-10",
  "record_date": "2026-05-12",
  "payment_date": "2026-06-01",
  "details": {
    "dividend_amount": 50.0,
    "dividend_type": "CASH",
    "currency": "MNT"
  },
  "created_at": "2026-04-24T...",
  "updated_at": "2026-04-24T..."
}
```

Save the corporate action ID:

```bash
DIVIDEND_ID="<id from response>"
```

**Process the dividend (creates entitlements for all position holders):**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/corporate-actions/${DIVIDEND_ID}/process" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "corporate_action_id": "ca_q1r2s3t4-...",
  "processed_at": "2026-04-24T...",
  "action_type": "CA_DIVIDEND",
  "entitlements_created": 2
}
```

**What this proves:** Dividend entitlements are calculated as `quantity * dividend_amount` for each position holder. A holder of 100 APU shares receives an entitlement of 5,000 MNT (100 x 50 MNT).

### 12.3 Corporate Actions — Stock Split

**Announce a 2-for-1 stock split for Govisumber Mining:**

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/corporate-actions \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{
    "instrument_id": "'"$GOV_ID"'",
    "action_type": "CA_STOCK_SPLIT",
    "announcement_date": "2026-04-24",
    "ex_date": "2026-05-15",
    "record_date": "2026-05-17",
    "details": {
      "split_ratio": 2.0,
      "description": "2-for-1 stock split"
    }
  }' | jq .
```

```bash
SPLIT_ID="<id from response>"
```

**Process the stock split:**

```bash
curl -s -X POST "https://garudax.asla.mn/api/v1/securities/corporate-actions/${SPLIT_ID}/process" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "corporate_action_id": "...",
  "processed_at": "2026-04-24T...",
  "action_type": "CA_STOCK_SPLIT",
  "positions_adjusted": 2
}
```

**What this proves:** Stock split adjusts all position quantities by the split ratio. A holder of 100 GOV shares now holds 200 shares.

### 12.4 List Corporate Actions

```bash
# All corporate actions
curl -s "https://garudax.asla.mn/api/v1/securities/corporate-actions" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .

# Filter by instrument
curl -s "https://garudax.asla.mn/api/v1/securities/corporate-actions?instrument_id=${APU_ID}" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .

# Filter by type
curl -s "https://garudax.asla.mn/api/v1/securities/corporate-actions?action_type=CA_DIVIDEND" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "corporate_actions": [
    {
      "id": "...",
      "instrument_id": "...",
      "action_type": "CA_DIVIDEND",
      "status": "COMPLETED",
      "record_date": "2026-05-12"
    },
    {
      "id": "...",
      "instrument_id": "...",
      "action_type": "CA_STOCK_SPLIT",
      "status": "COMPLETED",
      "record_date": "2026-05-17"
    }
  ],
  "count": 2
}
```

### 12.5 Settlement Cycle

**List settlement obligations:**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/settlements?date=2026-04-28" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):** Array of settlement obligations from trades with T+2 settlement dates:

```json
[
  {
    "id": "...",
    "trade_id": "trd_...",
    "instrument_id": "...",
    "buyer_participant_id": "...",
    "seller_participant_id": "...",
    "quantity": 100,
    "price": 850,
    "net_amount": 85000,
    "settlement_date": "2026-04-28",
    "status": "SETTLE_PENDING"
  }
]
```

**Query parameters:** `date` (YYYY-MM-DD) and/or `status` (SETTLE_PENDING, SETTLE_AFFIRMED, SETTLE_NETTED, SETTLE_SETTLED, SETTLE_FAILED). At least one is required.

**Trigger a settlement cycle:**

```bash
curl -s -X POST https://garudax.asla.mn/api/v1/securities/settlements/cycle \
  -H "Content-Type: application/json" \
  -H "X-GarudaX-Tenant: mse-equities" \
  -d '{"date": "2026-04-28"}' | jq .
```

**Expected response (HTTP 200):**

```json
{
  "date": "2026-04-28",
  "processed": 3,
  "affirmed": 3,
  "netted": 2,
  "settled": 2,
  "failed": 0
}
```

**What this proves:** T+2 settlement cycle processes all pending obligations through the affirm -> net -> settle pipeline.

### 12.6 FRC Reports

The FRC (Financial Regulatory Commission of Mongolia) requires periodic reports from exchanges. Three report types are available:

**Daily Summary Report:**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/reports/frc?type=DAILY_SUMMARY&date=2026-04-24" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "rpt_...",
  "tenant_id": "mse-equities",
  "report_type": "DAILY_SUMMARY",
  "report_date": "2026-04-24",
  "data": {
    "date": "2026-04-24",
    "trade_count": 5,
    "total_volume": 500,
    "total_value": 1625000.0
  },
  "generated_at": "2026-04-24T..."
}
```

**Large Trader Report (positions > 1000 shares):**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/reports/frc?type=LARGE_TRADER&date=2026-04-24" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "rpt_...",
  "tenant_id": "mse-equities",
  "report_type": "LARGE_TRADER",
  "report_date": "2026-04-24",
  "data": {
    "date": "2026-04-24",
    "threshold_shares": 1000,
    "large_trader_positions": [
      {"participant_id": "...", "instrument_id": "...", "quantity": 1500}
    ],
    "count": 1
  },
  "generated_at": "2026-04-24T..."
}
```

**Suspicious Activity Report:**

```bash
curl -s "https://garudax.asla.mn/api/v1/securities/reports/frc?type=SUSPICIOUS_ACTIVITY&date=2026-04-24" \
  -H "X-GarudaX-Tenant: mse-equities" | jq .
```

**Expected response (HTTP 200):**

```json
{
  "id": "rpt_...",
  "tenant_id": "mse-equities",
  "report_type": "SUSPICIOUS_ACTIVITY",
  "report_date": "2026-04-24",
  "data": {
    "date": "2026-04-24",
    "suspicious_activity": [],
    "count": 0,
    "note": "automated surveillance pending integration"
  },
  "generated_at": "2026-04-24T..."
}
```

**What this proves:** FRC regulatory reporting is functional with three report types. Daily summary aggregates trade data, large trader report flags positions above the 1000-share threshold, and suspicious activity report is ready for surveillance integration.

---

## 13. Admin UI Walkthrough

The Admin Dashboard (port 3001) provides visual management of the multi-tenant platform.

### 13.1 Tenant Selector

The top navigation bar includes a tenant selector dropdown showing all registered tenants:
- **ace-commodities** — ACE Commodity Exchange (ACTIVE)
- **mse-equities** — Mongolian Stock Exchange (ONBOARDING/ACTIVE)

Switching tenants reloads all dashboard panels with tenant-scoped data. The `X-GarudaX-Tenant` header is set automatically on all API requests.

### 13.2 Securities Pages

**Instruments Page** (`/admin/securities/instruments`):
- Table listing all instruments with columns: Ticker, Name, Asset Class, Trading Status, Lot Size, Tick Size, Currency
- Search filter by ticker or name
- Status badge colors: ACTIVE (green), HALTED (yellow), SUSPENDED (orange), DELISTED (red)
- Click to view instrument details, update status, or modify properties

**Orders Page** (`/admin/securities/orders`):
- Filterable table: instrument, side, status, date range
- Columns: Order ID, Instrument, Side, Type, Quantity, Price, Filled Qty, Status, Created At
- Side badges: BUY (green), SELL (red)
- Status badges: PENDING (blue), FILLED (green), CANCELLED (gray), PARTIALLY_FILLED (yellow)

**Positions Page** (`/admin/securities/positions`):
- Portfolio view grouped by instrument
- Columns: Instrument, Quantity, Avg Cost, Market Value, Unrealized P&L, Participant
- P&L coloring: positive (green), negative (red)

**Corporate Actions Page** (`/admin/securities/corporate-actions`):
- Timeline of announced, processing, and completed corporate actions
- Columns: ID, Instrument, Type, Status, Record Date
- Process button for ANNOUNCED actions

### 13.3 Platform Admin

**Tenants Page** (`/admin/platform/tenants`):
- Table of all registered tenants with status badges
- Create new tenant button (opens form modal)
- Status management: Activate, Suspend, Decommission
- Click to view tenant config (trading hours, settlement, circuit breakers, features)

**System Health Page** (`/admin/health`):
- Service health cards for all 11 services
- Each card shows: service name, health port, status (green/red), last check timestamp
- Kafka and PostgreSQL connectivity indicators

**Settlement Dashboard** (`/admin/securities/settlements`):
- T+2 settlement calendar view
- Obligation counts by status: PENDING, AFFIRMED, NETTED, SETTLED, FAILED
- Trigger settlement cycle button (POST to settlements/cycle)

**FRC Reports Page** (`/admin/securities/reports`):
- Date picker and report type selector
- Generate button produces the selected report
- Downloadable as JSON or formatted table

---

## 14. Bot Commands

The admin-bot MCP server exposes 17 tools for AI-assisted platform management. These tools are invoked by the admin-bot agent (via Model Context Protocol) and can also be called programmatically.

### 14.1 Platform Tools (5 tools)

**list_tenants** — List all tenants registered on the GarudaX platform

```
> list_tenants
ID                      NAME                    STATUS        FLAGSHIP  TIER
ace-commodities         ACE Commodity Exchange  ACTIVE        false     STANDARD
mse-equities            Mongolian Stock Exchang ONBOARDING    true      FLAGSHIP
```

**get_tenant** — Get details of a specific tenant

```
> get_tenant tenant_id="mse-equities"
ID               : mse-equities
Name             : Mongolian Stock Exchange
Status           : ONBOARDING
Flagship         : true
Governance Tier  : FLAGSHIP
```

**create_tenant** — Create a new tenant and provision infrastructure

```
> create_tenant id="test-sandbox" name="Test Sandbox Exchange" governance_tier="STANDARD"
Tenant created successfully.
ID               : test-sandbox
Name             : Test Sandbox Exchange
Status           : ONBOARDING
Governance Tier  : STANDARD
```

**update_tenant_status** — Update tenant lifecycle status

```
> update_tenant_status tenant_id="mse-equities" status="ACTIVE"
Tenant status updated.
ID         : mse-equities
New Status : ACTIVE
```

**get_tenant_config** — Get runtime configuration for a tenant

```
> get_tenant_config tenant_id="mse-equities"
Tenant Config: mse-equities
──────────────────────────────────────────────────
Trading Hours:
  Open  : 10:00
  Close : 13:00
  Zone  : Asia/Ulaanbaatar
Settlement:
  Cycle    : T+2
  Currency : MNT
  Cutoff   : —
```

### 14.2 Securities Tools (12 tools)

**list_securities_instruments** — List instruments with optional filters

```
> list_securities_instruments tenant_id="mse-equities" asset_class="EQUITY"
TICKER       NAME                 ASSET CLASS  STATUS      LOT   TICK   CCY
APU          APU JSC              EQUITY       ACTIVE      1     1      MNT
GOV          Govisumber Mining JS EQUITY       ACTIVE      10    50     MNT
```

**get_securities_instrument** — Get instrument details by ID

```
> get_securities_instrument tenant_id="mse-equities" instrument_id="a1b2c3d4-..."
Instrument ID : a1b2c3d4-...
Ticker        : APU
Name          : APU JSC
Asset Class   : EQUITY
Status        : ACTIVE
Lot Size      : 1
Tick Size     : 1
Currency      : MNT
```

**create_securities_instrument** — Create a new instrument

```
> create_securities_instrument tenant_id="mse-equities" ticker="SHV" name="Shivee Ovoo JSC" asset_class="EQUITY" lot_size=10 tick_size=5 currency="MNT"
Instrument created successfully.
ID          : u7v8w9x0-...
Ticker      : SHV
Name        : Shivee Ovoo JSC
Asset Class : EQUITY
Status      : ACTIVE
```

**submit_securities_order** — Submit a buy or sell order

```
> submit_securities_order tenant_id="mse-equities" instrument_id="a1b2c3d4-..." side="BUY" order_type="LIMIT" quantity=100 price=860
Order submitted.
Order ID    : y1z2a3b4-...
Instrument  : a1b2c3d4-...
Side        : BUY
Type        : LIMIT
Quantity    : 100
Price       : 860
Status      : PENDING
```

**list_securities_orders** — List orders with filters

```
> list_securities_orders tenant_id="mse-equities" status="PENDING"
ORDER ID              INSTRUMENT   SIDE   TYPE     QTY      PRICE      STATUS
y1z2a3b4-...          a1b2c3d4-... BUY    LIMIT       100       860    PENDING
```

**list_securities_positions** — List positions

```
> list_securities_positions tenant_id="mse-equities"
INSTRUMENT        QTY         AVG COST       P&L            PARTICIPANT
a1b2c3d4-...          100        850.00          0.00       trader1
```

**cancel_securities_order** — Cancel an open order

```
> cancel_securities_order tenant_id="mse-equities" order_id="y1z2a3b4-..."
Order cancelled.
Order ID : y1z2a3b4-...
Status   : CANCELLED
```

**list_settlement_obligations** — List settlement obligations

```
> list_settlement_obligations tenant_id="mse-equities" date="2026-04-28"
Trade ID                              Instrument    Qty    Amount        Status    Date
trd_m3n4o5p6-...                      a1b2c3d4-...    100      85000.00  PENDING   2026-04-28
```

**trigger_settlement_cycle** — Trigger settlement for a date

```
> trigger_settlement_cycle tenant_id="mse-equities" date="2026-04-28"
Settlement Cycle -- 2026-04-28
Processed : 3
Affirmed  : 3
Netted    : 2
Settled   : 2
Failed    : 0
```

**list_corporate_actions** — List corporate actions

```
> list_corporate_actions tenant_id="mse-equities" action_type="DIVIDEND"
ID                                    Instrument    Type          Status      Record Date
ca_q1r2s3t4-...                       a1b2c3d4-...  CA_DIVIDEND   COMPLETED   2026-05-12
```

**announce_corporate_action** — Announce a new corporate action

```
> announce_corporate_action tenant_id="mse-equities" instrument_id="a1b2c3d4-..." action_type="DIVIDEND" details='{"dividend_amount": 75, "record_date": "2026-07-01"}'
Corporate action announced.
ID          : ca_c5d6e7f8-...
Instrument  : a1b2c3d4-...
Type        : DIVIDEND
Status      : ANNOUNCED
Record Date : 2026-07-01
```

**generate_frc_report** — Generate FRC regulatory report

```
> generate_frc_report tenant_id="mse-equities" report_type="DAILY_SUMMARY" date="2026-04-24"
FRC Report -- DAILY_SUMMARY
Date    : 2026-04-24
Tenant  : mse-equities
Total Trades    : 5
Total Volume    : 500
Total Turnover  : 1625000
```

**provision_tenant** — Provision a new tenant (creates schemas and topics)

```
> provision_tenant id="new-exchange" name="New Exchange"
Tenant provisioning initiated.
Tenant:
  ID     : new-exchange
  Name   : New Exchange
  Status : ONBOARDING
Provisioning Result:
  Status   : COMPLETED
  Schemas created:
    - new_exchange_trading
    - new_exchange_clearing
  Kafka topics created:
    - new-exchange.orders
    - new-exchange.trades
```

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

### Compliance -- Onboarding (authenticated)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/participants` | Submit application |
| GET | `/api/v1/participants` | List applications |
| GET | `/api/v1/participants/{participant_id}` | Get application |
| POST | `/api/v1/participants/{participant_id}/documents` | Upload document |
| GET | `/api/v1/participants/{participant_id}/documents` | List documents |
| POST | `/api/v1/participants/{participant_id}/approve` | Approve (admin) |
| POST | `/api/v1/participants/{participant_id}/reject` | Reject (admin) |

### Compliance -- Screening (authenticated)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/screening/check` | Screen participant |
| GET | `/api/v1/screening/{screening_id}` | Get screening result |
| POST | `/api/v1/screening/batch` | Batch screen |
| POST | `/api/v1/screening/{screening_id}/resolve` | Resolve match |
| GET | `/api/v1/risk-scores/{participant_id}` | Get risk score |

### Compliance -- Admin (authenticated)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/compliance/alerts` | List alerts |
| POST | `/api/v1/compliance/alerts/{alert_id}/resolve` | Resolve alert |
| GET | `/api/v1/compliance/audit-trail` | Audit trail |
| POST | `/api/v1/compliance/sar` | File SAR |
| POST | `/api/v1/compliance/participants/{participant_id}/suspend` | Suspend |
| POST | `/api/v1/compliance/participants/{participant_id}/reinstate` | Reinstate |

### Admin -- Exchange Operations (admin only)
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/admin/instruments/{instrument_id}/halt` | Halt instrument |
| POST | `/api/v1/admin/instruments/{instrument_id}/resume` | Resume instrument |
| POST | `/api/v1/admin/trades/{trade_id}/bust` | Bust trade |
| PUT | `/api/v1/admin/instruments/{instrument_id}/circuit-breaker` | Set circuit breaker |
| POST | `/api/v1/admin/participants/{participant_id}/disable` | Disable participant |
| POST | `/api/v1/admin/mass-cancel` | Mass cancel orders |

### Securities Service (tenant-scoped, requires X-GarudaX-Tenant header)
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/securities/instruments` | List instruments |
| POST | `/api/v1/securities/instruments` | Create instrument |
| GET | `/api/v1/securities/instruments/{id}` | Get instrument |
| PATCH | `/api/v1/securities/instruments/{id}` | Update instrument |
| PUT | `/api/v1/securities/instruments/{id}/status` | Update trading status |
| GET | `/api/v1/securities/orders` | List orders |
| POST | `/api/v1/securities/orders` | Submit order |
| GET | `/api/v1/securities/orders/{id}` | Get order |
| DELETE | `/api/v1/securities/orders/{id}` | Cancel order |
| GET | `/api/v1/securities/positions` | List positions |
| GET | `/api/v1/securities/sessions` | List market sessions |
| POST | `/api/v1/securities/sessions/{id}/transition` | Transition session |
| GET | `/api/v1/securities/corporate-actions` | List corporate actions |
| POST | `/api/v1/securities/corporate-actions` | Announce corporate action |
| GET | `/api/v1/securities/corporate-actions/{id}` | Get corporate action |
| POST | `/api/v1/securities/corporate-actions/{id}/process` | Process corporate action |
| GET | `/api/v1/securities/settlements` | List settlements |
| POST | `/api/v1/securities/settlements/cycle` | Trigger settlement cycle |
| GET | `/api/v1/securities/reports/frc` | Generate FRC report |

### Platform Service (platform admin)
| Method | Path | Description |
|---|---|---|
| GET | `/platform/v1/tenants` | List tenants |
| POST | `/platform/v1/tenants` | Create tenant |
| GET | `/platform/v1/tenants/{id}` | Get tenant |
| PATCH | `/platform/v1/tenants/{id}` | Update tenant |
| PUT | `/platform/v1/tenants/{id}/status` | Update tenant status |
| GET | `/platform/v1/tenants/{id}/config` | Get tenant config |

### Health (public)
| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |
