# GarudaX Platform Architecture Overview

## What is GarudaX

GarudaX is a multi-tenant, AI-native operating platform that hosts regulated trading venues. Each tenant is an independent exchange with its own trading rules, participants, instruments, and regulatory requirements. GarudaX is the platform; tenants are the venues.

## Multi-Tenant Design

Every request carries an `X-GarudaX-Tenant` header. Tenant middleware in the gateway resolves the header to a tenant context before any business logic executes. Tenant ID is never optional — services reject unscoped requests.

**Current tenants:**

| Tenant ID | Venue | Status | Description |
|-----------|-------|--------|-------------|
| mse-equities | Mongolia Stock Exchange | ONBOARDING (flagship) | Equities, bonds, ETFs, T+2 settlement, CSD integration |
| ace-commodities | ACE Commodity Exchange | ACTIVE | Wheat, barley, cattle, cashmere, wool, physical delivery via eWR |

A platform control plane (Phase 0.7) will manage tenant lifecycle: onboarding, suspension, configuration, and decommissioning.

## Services (11 Go Microservices)

| Service | Role |
|---------|------|
| **matching-engine** | Central Limit Order Book (CLOB), price-time priority matching, iceberg orders, auction clearing |
| **clearing-engine** | Trade novation, netting, CCP functions |
| **margin-engine** | Real-time margin calculation, collateral management, margin calls |
| **settlement-engine** | T+2 settlement lifecycle, DVP, obligation tracking, fail management |
| **auth-service** | JWT authentication, PKCE, RBAC, bcrypt password hashing, session management |
| **compliance-service** | KYC/AML onboarding, regulatory screening, sanctions checks |
| **gateway** | HTTP API gateway, 101 registered routes, WebSocket, rate limiting, tenant middleware |
| **market-data-service** | Real-time market data distribution, SSE feeds, OHLCV candles, TimescaleDB |
| **warehouse-service** | Historical data archival, analytics, reporting |
| **securities-service** | Instrument CRUD, reference data, surveillance, order/trade management, FRC reporting |
| **admin-bot** | AI-powered admin operations via natural language, MCP tool integration |

## Frontend Applications (3 React SPAs)

| App | Port | Role |
|-----|------|------|
| **web-ui** | 3000 | Trading terminal for brokers and participants |
| **admin-ui** | 3001 | Exchange operator dashboard, surveillance, instrument management |
| **demo-runner** | 3002 | Interactive demo with guided trading scenarios |

## Protocol Gateways

- **FIX 4.4 Gateway** — TCP listener with session management, heartbeat, sequence tracking, order routing. 95.3% FIX tag parser coverage. Broker connectivity via industry-standard FIX protocol.
- **Native Binary Protocol** — Binary codec with 92.5% field coverage for low-latency connectivity.

## Technology Stack

| Layer | Technology |
|-------|------------|
| Language | Go 1.25 |
| Frontend | React 19, TypeScript, Vite |
| Database | PostgreSQL 16, TimescaleDB |
| Messaging | Kafka (channel-based stubs with wire-protocol interface) |
| Cache | Redis 7 |
| Container | Docker, Kubernetes |
| Auth | JWT + PKCE + RBAC + bcrypt |
| Testing | Go testing, Vitest, Playwright |

## Codebase Metrics

| Metric | Count |
|--------|-------|
| Go lines of code | ~136,000 |
| Domain types (types.go) | 65 structs |
| Store interfaces | 48 |
| Handler files | 94 |
| Go test functions | 2,540+ |
| Frontend tests | 313 |
| E2E API tests | 58 |
| Playwright tests | 13 |
| Gateway routes | 101 |
| PostgreSQL stores | 6 (Market, Segment, Firm, Participant, Settlement, Audit) |
| Go services | 11 |
| React SPAs | 3 |

## Securities Module Capabilities

The securities-service is the largest service (~48,000 Go lines) and covers:

- **Instruments**: Equity, Bond, ETF with ISIN/CUSIP/SEDOL identifiers, 22+ fields per instrument
- **Orders**: Limit, Market, Stop, Stop-Limit with 5 time-in-force types (GTC, IOC, FOK, DAY, GTD)
- **Matching**: Price-time priority, iceberg orders (visible + hidden quantity), self-trade prevention (3 modes)
- **Auctions**: Opening and closing auctions with price-ladder clearing price algorithm
- **Sessions**: Per-instrument session management (PRE_OPEN, CONTINUOUS, CLOSING_AUCTION, CLOSED)
- **Circuit Breakers**: Static + dynamic price bands with configurable cooldown
- **Settlement**: Full T+2 lifecycle (PENDING -> AFFIRMED -> NETTED -> INSTRUCTED -> SETTLING -> SETTLED)
- **Surveillance**: 12 alert patterns, investigation workflow, dashboard aggregation
- **Corporate Actions**: Dividend, stock split, rights issue, merger with entitlement processing
- **Market Structure**: Market -> Segment -> Instrument hierarchy with trading cycles
- **Participants**: Firm -> Participant -> Node hierarchy with RBAC permissions (35+ permission constants)

## Development Model

GarudaX is built by an AI-driven development pipeline (Softhouse) that uses task graphs, isolated worktrees, automated review, and a learning loop. 60 tasks have been completed across 7 phases with 887+ tests and 66% business-logic coverage.
