# API Gateway Architecture Specification

**Document ID:** T033-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 3)

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Context](#2-system-context)
3. [REST API Design](#3-rest-api-design)
4. [Authentication & Authorization](#4-authentication--authorization)
5. [Rate Limiting](#5-rate-limiting)
6. [REST-to-gRPC Translation](#6-rest-to-grpc-translation)
7. [WebSocket Support](#7-websocket-support)
8. [Error Handling](#8-error-handling)
9. [API Versioning](#9-api-versioning)
10. [Endpoint Mapping Table](#10-endpoint-mapping-table)
11. [Request/Response Examples](#11-requestresponse-examples)
12. [Performance Requirements](#12-performance-requirements)
13. [Deployment Architecture](#13-deployment-architecture)
14. [Failure Modes & Recovery](#14-failure-modes--recovery)

---

## 1. Overview

The GarudaX API Gateway is the single public-facing HTTP entry point for the AI Powered Commodity Exchange platform. It translates REST/JSON requests into gRPC calls to backend services and exposes WebSocket streams for real-time market data.

### Design Principles

- **Thin translation layer**: The gateway contains no business logic. It validates JWTs, enforces rate limits, translates REST↔gRPC, and forwards. All domain logic lives in the backend services.
- **Unified API surface**: Clients interact with one base URL (`https://api.garudax.mn/api/v1/`) regardless of which backend service handles the request.
- **Zero-dependency Go module**: Following the established pattern (matching-engine, clearing-engine, etc.), the gateway is a standalone Go service with no shared library dependencies.
- **Connection pooling**: Maintains persistent gRPC connections to all backend services for low-latency forwarding.

### Scope

This spec covers:
- REST endpoint design for all 6 backend gRPC services
- JWT-based authentication middleware
- Rate limiting (per-user, per-endpoint)
- REST JSON ↔ gRPC protobuf mapping conventions
- WebSocket endpoints for real-time streams
- API versioning strategy
- OpenAPI 3.0 contract
- gRPC-to-HTTP error code mapping

This spec does NOT cover:
- Backend service implementation (T007, T008, T015, T027, T028, T029)
- TLS termination (handled by Istio ingress gateway / load balancer)
- CDN or static asset serving
- GraphQL (future consideration)

---

## 2. System Context

```
                  Internet / Client Apps
                          |
                    TLS termination
                    (Istio Ingress)
                          |
                 +--------v---------+
                 |   API Gateway    |
                 |   HTTP :8080     |
                 +--+--+--+--+--+--+
                    |  |  |  |  |  |
       gRPC calls   |  |  |  |  |  |
       to backends  |  |  |  |  |  |
                    v  v  v  v  v  v
    +-----------+ +--+ +--+ +--+ +--+ +--+
    | matching  | |cl| |ma| |se| |au| |co|
    | engine    | |ea| |rg| |tt| |th| |mp|
    | :50051    | |ri| |in| |le| |  | |li|
    |           | |ng| |  | |  | |:5| |an|
    |           | |:5| |:5| |:5| |00| |ce|
    |           | |00| |00| |00| |55| |:5|
    |           | |52| |53| |54| |  | |00|
    |           | |  | |  | |  | |  | |56|
    +-----------+ +--+ +--+ +--+ +--+ +--+
```

### Backend Services

| Service | gRPC Port | Health Port | Proto Package |
|---|---|---|---|
| matching-engine | 50051 | 8081 | `ace.exchange.v1` |
| clearing-engine | 50052 | 8082 | (Go types, no proto yet) |
| margin-engine | 50053 | 8083 | (Go types, no proto yet) |
| settlement-engine | 50054 | 8084 | (Go types, no proto yet) |
| auth-service | 50055 | 8085 | (to be defined) |
| compliance-service | 50056 | 8086 | `ace.compliance.v1` |

### Gateway Service

| Property | Value |
|---|---|
| HTTP Port | 8080 |
| Health Port | 8090 |
| Protocol | HTTP/1.1 + WebSocket |
| Base URL | `/api/v1/` |

---

## 3. REST API Design

All endpoints use the `/api/v1/` prefix. Request and response bodies are JSON. Resource naming follows REST conventions: plural nouns for collections, specific IDs for individual resources.

### 3.1 Order Endpoints (→ matching-engine :50051)

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/orders` | `OrderService/SubmitOrder` | trader | Submit a new order |
| `GET` | `/api/v1/orders/{order_id}` | `OrderService/GetOrder` | trader | Get order details |
| `GET` | `/api/v1/orders` | `OrderService/GetOpenOrders` | trader | List open orders |
| `DELETE` | `/api/v1/orders/{order_id}` | `OrderService/CancelOrder` | trader | Cancel an order |
| `DELETE` | `/api/v1/orders` | `OrderService/CancelAllOrders` | trader | Cancel all orders |
| `PATCH` | `/api/v1/orders/{order_id}` | `OrderService/ModifyOrder` | trader | Modify price/qty |

### 3.2 Market Data Endpoints (→ matching-engine :50051)

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/instruments/{instrument_id}/book` | `MarketDataService/GetOrderBook` | public | L2 order book snapshot |
| `GET` | `/api/v1/instruments/{instrument_id}/book/l3` | `MarketDataService/GetOrderBookL3` | public | L3 order book (individual orders) |
| `GET` | `/api/v1/instruments/{instrument_id}/trades/latest` | `MarketDataService/GetLastTrade` | public | Most recent trade |

### 3.3 Admin Endpoints (→ matching-engine :50051)

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/admin/instruments/{instrument_id}/halt` | `AdminService/HaltInstrument` | exchange_admin | Halt trading |
| `POST` | `/api/v1/admin/instruments/{instrument_id}/resume` | `AdminService/ResumeInstrument` | exchange_admin | Resume trading |
| `POST` | `/api/v1/admin/trades/{trade_id}/bust` | `AdminService/BustTrade` | exchange_admin | Bust a trade |
| `PUT` | `/api/v1/admin/instruments/{instrument_id}/circuit-breaker` | `AdminService/SetCircuitBreaker` | exchange_admin | Update circuit breaker config |
| `POST` | `/api/v1/admin/participants/{participant_id}/disable` | `AdminService/DisableParticipant` | exchange_admin | Disable a participant |
| `POST` | `/api/v1/admin/mass-cancel` | `AdminService/MassCancel` | exchange_admin | Mass cancel orders |

### 3.4 Clearing Endpoints (→ clearing-engine :50052)

| Method | Path | Server Method | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/clearing/positions` | `GetPositions` | trader | List positions for a participant |
| `GET` | `/api/v1/clearing/positions/{instrument_id}` | `GetPosition` | trader | Get position for specific instrument |
| `GET` | `/api/v1/clearing/netting` | `NetObligations` | clearing_admin | Get netting results |

### 3.5 Margin Endpoints (→ margin-engine :50053)

| Method | Path | Server Method | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/margin` | `GetPortfolioMargin` | trader | Get portfolio margin for participant |
| `POST` | `/api/v1/margin/calculate` | `CalculateMargin` | clearing_admin | Trigger margin calculation |
| `GET` | `/api/v1/margin/calls` | `GetAllActiveMarginCalls` | clearing_admin | List active margin calls |
| `GET` | `/api/v1/margin/calls/stats` | `GetMarginCallStats` | clearing_admin | Margin call statistics |

### 3.6 Settlement Endpoints (→ settlement-engine :50054)

| Method | Path | Server Method | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/settlement/cycles` | `GetAllCycles` | clearing_admin | List settlement cycles |
| `GET` | `/api/v1/settlement/cycles/{cycle_id}` | `GetCycle` | clearing_admin | Get specific cycle details |

### 3.7 Auth Endpoints (→ auth-service :50055)

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/auth/login` | `AuthService/Login` | none | Authenticate and get JWT |
| `POST` | `/api/v1/auth/refresh` | `AuthService/RefreshToken` | none | Refresh an expired JWT |
| `POST` | `/api/v1/auth/logout` | `AuthService/Logout` | any | Invalidate session |
| `POST` | `/api/v1/auth/register` | `AuthService/Register` | none | Create a new account |
| `GET` | `/api/v1/auth/me` | `AuthService/GetProfile` | any | Get current user profile |
| `POST` | `/api/v1/auth/password/change` | `AuthService/ChangePassword` | any | Change password |
| `POST` | `/api/v1/auth/password/reset` | `AuthService/RequestPasswordReset` | none | Request password reset |

### 3.8 Compliance Endpoints (→ compliance-service :50056)

#### Onboarding

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/participants` | `OnboardingService/SubmitApplication` | any | Submit KYC application |
| `GET` | `/api/v1/participants/{participant_id}` | `OnboardingService/GetApplication` | trader, compliance | Get application details |
| `GET` | `/api/v1/participants` | `OnboardingService/ListApplications` | compliance | List applications |
| `POST` | `/api/v1/participants/{participant_id}/documents` | `OnboardingService/UploadDocument` | trader | Upload KYC document |
| `GET` | `/api/v1/participants/{participant_id}/documents` | `OnboardingService/ListDocuments` | trader, compliance | List documents |
| `POST` | `/api/v1/participants/{participant_id}/approve` | `OnboardingService/ApproveApplication` | compliance | Approve application |
| `POST` | `/api/v1/participants/{participant_id}/reject` | `OnboardingService/RejectApplication` | compliance | Reject application |

#### Screening

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `POST` | `/api/v1/screening/check` | `ScreeningService/ScreenParticipant` | compliance | Run watchlist screening |
| `GET` | `/api/v1/screening/{screening_id}` | `ScreeningService/GetScreeningResult` | compliance | Get screening result |
| `POST` | `/api/v1/screening/batch` | `ScreeningService/BatchScreen` | compliance | Batch screening |
| `POST` | `/api/v1/screening/{screening_id}/resolve` | `ScreeningService/ResolveMatch` | compliance | Resolve a match |
| `GET` | `/api/v1/risk-scores/{participant_id}` | `ScreeningService/GetRiskScore` | compliance | Get risk score |

#### Compliance Admin

| Method | Path | gRPC RPC | Auth | Description |
|---|---|---|---|---|
| `GET` | `/api/v1/compliance/alerts` | `ComplianceAdminService/ListAlerts` | compliance | List monitoring alerts |
| `POST` | `/api/v1/compliance/alerts/{alert_id}/resolve` | `ComplianceAdminService/ResolveAlert` | compliance | Resolve an alert |
| `GET` | `/api/v1/compliance/audit-trail` | `ComplianceAdminService/GetAuditTrail` | compliance | Get audit trail |
| `POST` | `/api/v1/compliance/sar` | `ComplianceAdminService/FileSAR` | compliance | File a SAR |
| `POST` | `/api/v1/compliance/participants/{participant_id}/suspend` | `ComplianceAdminService/SuspendParticipant` | compliance | Suspend participant |
| `POST` | `/api/v1/compliance/participants/{participant_id}/reinstate` | `ComplianceAdminService/ReinstateParticipant` | compliance | Reinstate participant |

---

## 4. Authentication & Authorization

### 4.1 JWT Validation

The gateway validates JWTs on every request (except explicitly public endpoints). It does NOT issue JWTs — that is the auth-service's responsibility.

**Flow:**

```
Client → Gateway:  Authorization: Bearer <JWT>
Gateway:
  1. Extract token from Authorization header
  2. Validate signature using auth-service's public key (JWKS endpoint, cached)
  3. Check expiration (exp claim)
  4. Extract claims: sub (user_id), participant_id, roles[], iss, aud
  5. Attach claims to request context
  6. Forward to backend with claims in gRPC metadata
Gateway → Backend:  gRPC metadata: x-user-id, x-participant-id, x-roles
```

**JWT Claims:**

```json
{
  "sub": "uuid-user-id",
  "participant_id": "uuid-participant-id",
  "roles": ["trader", "compliance_viewer"],
  "iss": "ace-auth-service",
  "aud": "ace-api-gateway",
  "exp": 1711612800,
  "iat": 1711609200,
  "jti": "unique-token-id"
}
```

### 4.2 Role-Based Access Control

| Role | Description | Endpoint Access |
|---|---|---|
| `trader` | Exchange participant | Orders, own positions, own margin, own documents |
| `compliance_officer` | Compliance team | All compliance endpoints, read-only on trading data |
| `compliance_viewer` | Read-only compliance | Compliance read endpoints only |
| `clearing_admin` | Clearing house operator | Clearing, margin, settlement endpoints |
| `exchange_admin` | Exchange operator | Admin endpoints (halt, resume, bust, circuit breaker) |
| `system` | Service-to-service | Internal endpoints (compliance status checks) |

### 4.3 Public Endpoints (no auth required)

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/password/reset`
- `POST /api/v1/auth/refresh`
- `GET /api/v1/instruments/{id}/book`
- `GET /api/v1/instruments/{id}/book/l3`
- `GET /api/v1/instruments/{id}/trades/latest`
- `GET /healthz`
- `GET /readyz`
- `WS /api/v1/ws/trades/{instrument_id}` (read-only market data)
- `WS /api/v1/ws/book/{instrument_id}` (read-only market data)

### 4.4 Owner-Scoped Access

For endpoints that return participant-specific data (orders, positions, margin), the gateway enforces that the authenticated user's `participant_id` matches the requested resource's participant. Admin roles bypass this check.

```
trader requests GET /api/v1/orders?account_id=X
  → gateway checks JWT.participant_id == X (or has admin role)
  → rejects with 403 if mismatch
```

---

## 5. Rate Limiting

### 5.1 Strategy

Rate limiting uses a **token bucket** algorithm implemented in Redis. Limits are applied per authenticated user (by `sub` claim) and per endpoint group.

### 5.2 Rate Limit Tiers

| Endpoint Group | Limit (per user) | Burst | Window |
|---|---|---|---|
| Order submission (`POST /orders`) | 50 req/s | 100 | 1 second |
| Order cancellation (`DELETE /orders/*`) | 100 req/s | 200 | 1 second |
| Order query (`GET /orders*`) | 100 req/s | 200 | 1 second |
| Market data (authenticated) | 200 req/s | 400 | 1 second |
| Market data (unauthenticated) | 20 req/s | 40 | 1 second (by IP) |
| Compliance operations | 30 req/s | 60 | 1 second |
| Admin operations | 10 req/s | 20 | 1 second |
| Auth operations | 5 req/s | 10 | 1 second |
| WebSocket connections (per user) | 5 concurrent | - | - |
| WebSocket connections (per IP, unauth) | 2 concurrent | - | - |

### 5.3 Rate Limit Headers

Every response includes rate limit headers:

```http
X-RateLimit-Limit: 50
X-RateLimit-Remaining: 47
X-RateLimit-Reset: 1711612801
Retry-After: 1          (only on 429 responses)
```

### 5.4 Rate Limit Exceeded Response

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 1

{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Rate limit exceeded. Try again in 1 second.",
    "retry_after_seconds": 1
  }
}
```

---

## 6. REST-to-gRPC Translation

### 6.1 Request Mapping

| REST Concept | gRPC Concept |
|---|---|
| URL path parameters | Request message fields |
| Query parameters | Request message fields (GET requests) |
| JSON request body | Protobuf request message (POST/PUT/PATCH) |
| `Authorization` header | gRPC metadata `authorization` |
| Request ID header `X-Request-ID` | gRPC metadata `x-request-id` |

### 6.2 JSON ↔ Protobuf Conventions

| Protobuf Type | JSON Representation |
|---|---|
| `string` (UUID fields) | `"550e8400-e29b-41d4-a716-446655440000"` |
| `string` (decimal fields: price, value) | `"1234.5678"` |
| `uint64` | JSON number |
| `int64` | JSON number |
| `google.protobuf.Timestamp` | ISO 8601 string `"2026-03-28T09:05:00Z"` |
| `enum` (e.g., `Side`) | Lowercase string `"buy"`, `"sell"` |
| `repeated` | JSON array |
| `bytes` | Base64-encoded string |

### 6.3 Enum Mapping

Enums are exposed in REST as lowercase snake_case strings (not integer values):

| Proto Enum | REST JSON Value |
|---|---|
| `SIDE_BUY` | `"buy"` |
| `SIDE_SELL` | `"sell"` |
| `ORDER_TYPE_LIMIT` | `"limit"` |
| `ORDER_TYPE_MARKET` | `"market"` |
| `ORDER_TYPE_STOP_LIMIT` | `"stop_limit"` |
| `ORDER_TYPE_STOP_MARKET` | `"stop_market"` |
| `TIME_IN_FORCE_DAY` | `"day"` |
| `TIME_IN_FORCE_GTC` | `"gtc"` |
| `TIME_IN_FORCE_GTD` | `"gtd"` |
| `TIME_IN_FORCE_IOC` | `"ioc"` |
| `TIME_IN_FORCE_FOK` | `"fok"` |
| `ORDER_STATUS_NEW` | `"new"` |
| `ORDER_STATUS_PARTIALLY_FILLED` | `"partially_filled"` |
| `ORDER_STATUS_FILLED` | `"filled"` |
| `ORDER_STATUS_CANCELLED` | `"cancelled"` |
| `ORDER_STATUS_REJECTED` | `"rejected"` |
| `ORDER_STATUS_EXPIRED` | `"expired"` |

### 6.4 Pagination

List endpoints use cursor-based pagination:

**Request:**
```
GET /api/v1/orders?account_id=X&limit=50&cursor=eyJpZCI6ImFiYyJ9
```

**Response:**
```json
{
  "data": [...],
  "pagination": {
    "next_cursor": "eyJpZCI6ImRlZiJ9",
    "has_more": true
  }
}
```

### 6.5 Path Parameter → gRPC Field Mapping

The gateway extracts path parameters and injects them into the protobuf request message:

```
DELETE /api/v1/orders/abc-123
  → CancelOrderRequest { order_id: "abc-123", account_id: <from JWT> }

GET /api/v1/instruments/xyz-456/book?depth=20
  → GetOrderBookRequest { instrument_id: "xyz-456", depth: 20 }
```

---

## 7. WebSocket Support

### 7.1 Endpoints

| Path | Backend RPC | Auth | Description |
|---|---|---|---|
| `WS /api/v1/ws/trades/{instrument_id}` | `MarketDataService/StreamTrades` | public | Real-time trade stream |
| `WS /api/v1/ws/book/{instrument_id}` | `MarketDataService/StreamOrderBook` | public | Incremental order book updates |
| `WS /api/v1/ws/executions` | (internal) | trader | User's execution reports |

### 7.2 Connection Lifecycle

```
Client → Gateway: HTTP Upgrade to WebSocket
  1. Validate auth token (if provided in query param or first message)
  2. Apply rate limits on connection count
  3. Open server-streaming gRPC call to backend
  4. Forward gRPC stream messages → WebSocket frames (JSON)
  5. On client disconnect → cancel gRPC stream
  6. On backend stream end → close WebSocket with reason
```

### 7.3 WebSocket Message Format

All WebSocket messages are JSON. Server→client messages have a consistent envelope:

```json
{
  "type": "trade",
  "instrument_id": "WHT-HRW-2026M07-UB",
  "sequence": 12345,
  "timestamp": "2026-03-28T09:15:30.123Z",
  "data": {
    "trade_id": "uuid",
    "price": "45000.0000",
    "quantity": 10,
    "aggressor_side": "buy"
  }
}
```

**Message types:**
- `trade` — individual trade execution
- `book_snapshot` — full order book snapshot (sent on initial connect if `snapshot_first=true`)
- `book_update` — incremental price level update
- `execution_report` — order execution report (authenticated streams only)
- `heartbeat` — sent every 30 seconds to keep connection alive
- `error` — error notification

### 7.4 WebSocket Query Parameters

```
ws://api.garudax.mn/api/v1/ws/book/WHT-HRW-2026M07-UB?snapshot_first=true&depth=10
ws://api.garudax.mn/api/v1/ws/trades/WHT-HRW-2026M07-UB?since_sequence=12000
ws://api.garudax.mn/api/v1/ws/executions?token=<JWT>
```

### 7.5 Reconnection

Clients should implement exponential backoff reconnection. The `since_sequence` parameter allows replay from a known point, preventing message gaps.

---

## 8. Error Handling

### 8.1 gRPC → HTTP Status Code Mapping

| gRPC Status | HTTP Status | Error Code |
|---|---|---|
| `OK` | 200 | - |
| `INVALID_ARGUMENT` | 400 | `INVALID_ARGUMENT` |
| `FAILED_PRECONDITION` | 400 | `FAILED_PRECONDITION` |
| `UNAUTHENTICATED` | 401 | `UNAUTHENTICATED` |
| `PERMISSION_DENIED` | 403 | `PERMISSION_DENIED` |
| `NOT_FOUND` | 404 | `NOT_FOUND` |
| `ALREADY_EXISTS` | 409 | `ALREADY_EXISTS` |
| `RESOURCE_EXHAUSTED` | 429 | `RATE_LIMIT_EXCEEDED` |
| `ABORTED` | 409 | `CONFLICT` |
| `UNIMPLEMENTED` | 501 | `NOT_IMPLEMENTED` |
| `INTERNAL` | 500 | `INTERNAL_ERROR` |
| `UNAVAILABLE` | 503 | `SERVICE_UNAVAILABLE` |
| `DEADLINE_EXCEEDED` | 504 | `TIMEOUT` |

### 8.2 Error Response Format

All error responses use a consistent JSON envelope:

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "Price must be a positive decimal value",
    "details": [
      {
        "field": "price",
        "reason": "must be > 0"
      }
    ],
    "request_id": "req-abc-123"
  }
}
```

### 8.3 Gateway-Level Errors

Errors generated by the gateway itself (not forwarded from backends):

| Scenario | HTTP Status | Error Code |
|---|---|---|
| Missing/invalid JWT | 401 | `UNAUTHENTICATED` |
| Expired JWT | 401 | `TOKEN_EXPIRED` |
| Insufficient role | 403 | `PERMISSION_DENIED` |
| Rate limit exceeded | 429 | `RATE_LIMIT_EXCEEDED` |
| Request body too large (>1MB) | 413 | `PAYLOAD_TOO_LARGE` |
| Malformed JSON | 400 | `INVALID_JSON` |
| Backend unavailable | 503 | `SERVICE_UNAVAILABLE` |
| Request timeout (>30s) | 504 | `TIMEOUT` |

---

## 9. API Versioning

### 9.1 Strategy

URL-path versioning with a `v1` prefix:

```
/api/v1/orders
/api/v1/instruments/{id}/book
```

### 9.2 Version Lifecycle

| Phase | Duration | Behavior |
|---|---|---|
| **Current** | Indefinite | Active development, new features added |
| **Deprecated** | 6 months minimum | `Sunset` header added, deprecation notice in docs |
| **Removed** | After sunset | Returns 410 Gone |

### 9.3 Breaking vs Non-Breaking Changes

**Non-breaking** (allowed within a version):
- Adding new optional fields to response bodies
- Adding new endpoints
- Adding new optional query parameters
- Adding new enum values

**Breaking** (requires new version):
- Removing or renaming fields
- Changing field types
- Changing URL paths
- Changing authentication requirements
- Removing endpoints

### 9.4 Version Header

Responses include the API version:

```http
X-API-Version: v1
Sunset: Sat, 28 Mar 2027 00:00:00 GMT   (only if deprecated)
```

---

## 10. Endpoint Mapping Table

Complete mapping from REST to backend service:

| # | HTTP Method | REST Path | Backend Service | gRPC/Method | Auth Level |
|---|---|---|---|---|---|
| 1 | POST | `/api/v1/orders` | matching-engine:50051 | `OrderService/SubmitOrder` | trader |
| 2 | GET | `/api/v1/orders/{order_id}` | matching-engine:50051 | `OrderService/GetOrder` | trader |
| 3 | GET | `/api/v1/orders` | matching-engine:50051 | `OrderService/GetOpenOrders` | trader |
| 4 | DELETE | `/api/v1/orders/{order_id}` | matching-engine:50051 | `OrderService/CancelOrder` | trader |
| 5 | DELETE | `/api/v1/orders` | matching-engine:50051 | `OrderService/CancelAllOrders` | trader |
| 6 | PATCH | `/api/v1/orders/{order_id}` | matching-engine:50051 | `OrderService/ModifyOrder` | trader |
| 7 | GET | `/api/v1/instruments/{id}/book` | matching-engine:50051 | `MarketDataService/GetOrderBook` | public |
| 8 | GET | `/api/v1/instruments/{id}/book/l3` | matching-engine:50051 | `MarketDataService/GetOrderBookL3` | public |
| 9 | GET | `/api/v1/instruments/{id}/trades/latest` | matching-engine:50051 | `MarketDataService/GetLastTrade` | public |
| 10 | POST | `/api/v1/admin/instruments/{id}/halt` | matching-engine:50051 | `AdminService/HaltInstrument` | exchange_admin |
| 11 | POST | `/api/v1/admin/instruments/{id}/resume` | matching-engine:50051 | `AdminService/ResumeInstrument` | exchange_admin |
| 12 | POST | `/api/v1/admin/trades/{id}/bust` | matching-engine:50051 | `AdminService/BustTrade` | exchange_admin |
| 13 | PUT | `/api/v1/admin/instruments/{id}/circuit-breaker` | matching-engine:50051 | `AdminService/SetCircuitBreaker` | exchange_admin |
| 14 | POST | `/api/v1/admin/participants/{id}/disable` | matching-engine:50051 | `AdminService/DisableParticipant` | exchange_admin |
| 15 | POST | `/api/v1/admin/mass-cancel` | matching-engine:50051 | `AdminService/MassCancel` | exchange_admin |
| 16 | GET | `/api/v1/clearing/positions` | clearing-engine:50052 | `GetPositions` | trader |
| 17 | GET | `/api/v1/clearing/positions/{instrument_id}` | clearing-engine:50052 | `GetPosition` | trader |
| 18 | GET | `/api/v1/clearing/netting` | clearing-engine:50052 | `NetObligations` | clearing_admin |
| 19 | GET | `/api/v1/margin` | margin-engine:50053 | `GetPortfolioMargin` | trader |
| 20 | POST | `/api/v1/margin/calculate` | margin-engine:50053 | `CalculateMargin` | clearing_admin |
| 21 | GET | `/api/v1/margin/calls` | margin-engine:50053 | `GetAllActiveMarginCalls` | clearing_admin |
| 22 | GET | `/api/v1/margin/calls/stats` | margin-engine:50053 | `GetMarginCallStats` | clearing_admin |
| 23 | GET | `/api/v1/settlement/cycles` | settlement-engine:50054 | `GetAllCycles` | clearing_admin |
| 24 | GET | `/api/v1/settlement/cycles/{cycle_id}` | settlement-engine:50054 | `GetCycle` | clearing_admin |
| 25 | POST | `/api/v1/auth/login` | auth-service:50055 | `AuthService/Login` | none |
| 26 | POST | `/api/v1/auth/refresh` | auth-service:50055 | `AuthService/RefreshToken` | none |
| 27 | POST | `/api/v1/auth/logout` | auth-service:50055 | `AuthService/Logout` | any |
| 28 | POST | `/api/v1/auth/register` | auth-service:50055 | `AuthService/Register` | none |
| 29 | GET | `/api/v1/auth/me` | auth-service:50055 | `AuthService/GetProfile` | any |
| 30 | POST | `/api/v1/auth/password/change` | auth-service:50055 | `AuthService/ChangePassword` | any |
| 31 | POST | `/api/v1/auth/password/reset` | auth-service:50055 | `AuthService/RequestPasswordReset` | none |
| 32 | POST | `/api/v1/participants` | compliance-service:50056 | `OnboardingService/SubmitApplication` | any |
| 33 | GET | `/api/v1/participants/{id}` | compliance-service:50056 | `OnboardingService/GetApplication` | trader, compliance |
| 34 | GET | `/api/v1/participants` | compliance-service:50056 | `OnboardingService/ListApplications` | compliance |
| 35 | POST | `/api/v1/participants/{id}/documents` | compliance-service:50056 | `OnboardingService/UploadDocument` | trader |
| 36 | GET | `/api/v1/participants/{id}/documents` | compliance-service:50056 | `OnboardingService/ListDocuments` | trader, compliance |
| 37 | POST | `/api/v1/participants/{id}/approve` | compliance-service:50056 | `OnboardingService/ApproveApplication` | compliance |
| 38 | POST | `/api/v1/participants/{id}/reject` | compliance-service:50056 | `OnboardingService/RejectApplication` | compliance |
| 39 | POST | `/api/v1/screening/check` | compliance-service:50056 | `ScreeningService/ScreenParticipant` | compliance |
| 40 | GET | `/api/v1/screening/{id}` | compliance-service:50056 | `ScreeningService/GetScreeningResult` | compliance |
| 41 | POST | `/api/v1/screening/batch` | compliance-service:50056 | `ScreeningService/BatchScreen` | compliance |
| 42 | POST | `/api/v1/screening/{id}/resolve` | compliance-service:50056 | `ScreeningService/ResolveMatch` | compliance |
| 43 | GET | `/api/v1/risk-scores/{participant_id}` | compliance-service:50056 | `ScreeningService/GetRiskScore` | compliance |
| 44 | GET | `/api/v1/compliance/alerts` | compliance-service:50056 | `ComplianceAdminService/ListAlerts` | compliance |
| 45 | POST | `/api/v1/compliance/alerts/{id}/resolve` | compliance-service:50056 | `ComplianceAdminService/ResolveAlert` | compliance |
| 46 | GET | `/api/v1/compliance/audit-trail` | compliance-service:50056 | `ComplianceAdminService/GetAuditTrail` | compliance |
| 47 | POST | `/api/v1/compliance/sar` | compliance-service:50056 | `ComplianceAdminService/FileSAR` | compliance |
| 48 | POST | `/api/v1/compliance/participants/{id}/suspend` | compliance-service:50056 | `ComplianceAdminService/SuspendParticipant` | compliance |
| 49 | POST | `/api/v1/compliance/participants/{id}/reinstate` | compliance-service:50056 | `ComplianceAdminService/ReinstateParticipant` | compliance |
| 50 | WS | `/api/v1/ws/trades/{instrument_id}` | matching-engine:50051 | `MarketDataService/StreamTrades` | public |
| 51 | WS | `/api/v1/ws/book/{instrument_id}` | matching-engine:50051 | `MarketDataService/StreamOrderBook` | public |
| 52 | WS | `/api/v1/ws/executions` | matching-engine:50051 | (execution report stream) | trader |

---

## 11. Request/Response Examples

### 11.1 Submit Order

**Request:**
```http
POST /api/v1/orders HTTP/1.1
Authorization: Bearer eyJhbGci...
Content-Type: application/json
X-Request-ID: req-001

{
  "client_order_id": "my-order-001",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440000",
  "side": "buy",
  "order_type": "limit",
  "time_in_force": "day",
  "price": "45000.0000",
  "quantity": 10
}
```

**Response (201 Created):**
```json
{
  "exec_id": "a1b2c3d4-...",
  "order_id": "e5f6g7h8-...",
  "client_order_id": "my-order-001",
  "exec_type": "new",
  "order_status": "new",
  "side": "buy",
  "instrument_id": "550e8400-e29b-41d4-a716-446655440000",
  "price": "45000.0000",
  "quantity": 10,
  "last_qty": 0,
  "last_price": "0",
  "cumulative_qty": 0,
  "leaves_qty": 10,
  "transact_time": "2026-03-28T09:05:00.123Z",
  "account_id": "participant-uuid"
}
```

### 11.2 Get Order Book

**Request:**
```http
GET /api/v1/instruments/550e8400-.../book?depth=5 HTTP/1.1
```

**Response (200 OK):**
```json
{
  "instrument_id": "550e8400-...",
  "bids": [
    {"price": "45000.0000", "quantity": 50, "order_count": 3},
    {"price": "44950.0000", "quantity": 30, "order_count": 2}
  ],
  "asks": [
    {"price": "45050.0000", "quantity": 40, "order_count": 2},
    {"price": "45100.0000", "quantity": 25, "order_count": 1}
  ],
  "last_trade_price": "45000.0000",
  "sequence_number": 12345,
  "state": "continuous",
  "timestamp": "2026-03-28T09:15:30Z"
}
```

### 11.3 Get Portfolio Margin

**Request:**
```http
GET /api/v1/margin HTTP/1.1
Authorization: Bearer eyJhbGci...
```

**Response (200 OK):**
```json
{
  "participant_id": "uuid",
  "requirements": [
    {
      "instrument_id": "uuid",
      "net_quantity": 10,
      "scan_risk": "500000.0000",
      "initial_margin": "525000.0000",
      "mark_to_market": "-25000.0000",
      "total_required": "550000.0000"
    }
  ],
  "total_initial": "525000.0000",
  "total_mtm": "-25000.0000",
  "total_required": "550000.0000",
  "collateral_on_hand": "1000000.0000",
  "excess_deficit": "450000.0000",
  "calculated_at": "2026-03-28T09:00:00Z"
}
```

---

## 12. Performance Requirements

| Metric | Target | Rationale |
|---|---|---|
| Gateway latency overhead (p50) | < 2 ms | Translation + auth should be negligible |
| Gateway latency overhead (p99) | < 10 ms | Includes JWT validation and rate limit check |
| Throughput | 5,000 req/s | Sufficient headroom above matching engine's 10K orders/s |
| WebSocket connections | 10,000 concurrent | For market data subscribers |
| Max request body size | 1 MB | Sufficient for all API payloads; document upload is chunked |
| Connection pool per backend | 10 gRPC connections | Multiplexed HTTP/2, sufficient for throughput target |
| JWT JWKS cache TTL | 5 minutes | Balance freshness vs. auth-service load |
| Health check interval | 5 seconds | For gRPC backend health monitoring |

### Scaling Strategy

- Horizontally scaled behind Istio ingress gateway / load balancer
- Stateless — all state is in Redis (rate limits) or backends
- Multiple gateway replicas behind round-robin DNS
- Pod autoscaler based on CPU and active connection count

---

## 13. Deployment Architecture

```
gateway-service pod (2 replicas minimum)
  +------------------------------------------+
  | api-gateway process                      |
  |   - HTTP server :8080                    |
  |   - WebSocket upgrade handler            |
  |   - gRPC client connections (pooled)     |
  |   - JWT validation (JWKS cached)         |
  |   - Redis client (rate limiting)         |
  |   - sidecar: Istio proxy (mTLS)          |
  +------------------------------------------+
  | Resources:                               |
  |   CPU: 1 core (request), 2 cores (limit) |
  |   Memory: 512 Mi (request), 1 Gi (limit) |
  +------------------------------------------+
```

### Dependencies

| Dependency | Purpose | Failure Impact |
|---|---|---|
| Redis | Rate limiting, compliance status cache | Degrade: allow requests without rate limiting |
| Auth-service JWKS | JWT public key for validation | Cached locally; stale keys used until refresh |
| Backend gRPC services | Request forwarding | 503 for affected endpoint group |

---

## 14. Failure Modes & Recovery

| Failure | Impact | Recovery |
|---|---|---|
| Backend service unavailable | 503 for that service's endpoints | gRPC health checks detect; retry with circuit breaker (3 failures → open for 30s) |
| Redis unavailable | Rate limiting disabled | Fail-open: allow requests, log warning |
| Auth-service unavailable | Cannot validate new JWTs | Use cached JWKS; existing tokens still validated |
| Gateway pod crash | Brief connection drop | Kubernetes restarts pod; load balancer routes to healthy replicas |
| WebSocket backend stream ends | Client disconnected | Client reconnects with `since_sequence` for seamless replay |
| JWT key rotation | Tokens signed with old key fail | JWKS cache refreshes within TTL; brief window of rejected tokens |

### Circuit Breaker Pattern

Each backend connection uses a circuit breaker:

```
CLOSED (normal) → 3 consecutive failures → OPEN (reject fast, return 503)
OPEN → 30 seconds → HALF-OPEN (allow 1 probe request)
HALF-OPEN → success → CLOSED
HALF-OPEN → failure → OPEN
```

---

## Appendix A: OpenAPI Specification

See `docs/T033_openapi.yaml` for the complete OpenAPI 3.0 specification.

## Appendix B: Related Documents

- T007: Exchange Engine Architecture (matching-engine proto contracts)
- T015: KYC/AML Architecture (compliance-service proto contracts)
- T005: Auth Service (JWT issuance, RBAC)
- T001: Cloud Architecture Design (ADR-001, networking, Istio)
- T008: Matching Engine Implementation
- T027: Clearing Engine Implementation
- T028: Margin Engine Implementation
- T029: Settlement Engine Implementation
