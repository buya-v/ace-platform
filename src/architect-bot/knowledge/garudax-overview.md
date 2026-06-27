# GarudaX Platform Architecture Overview

## What is GarudaX

GarudaX is a multi-tenant, AI-native operating platform that hosts regulated trading venues. Each tenant is an independent exchange with its own trading rules, participants, instruments, and regulatory requirements. GarudaX is the platform; tenants are the venues.

## Multi-Tenant Design

Every tenant-scoped request carries an `X-GarudaX-Tenant` header. Tenant middleware in the gateway resolves and validates the header against the tenant registry **before routing and before any business logic executes** — tenant resolution sits ahead of the router (with a route-existence guard so unknown paths still return 404). Requests to tenant-scoped routes without a valid tenant header are rejected with 401/403; only genuinely platform-level paths (`/api/v1/platform/`, `/api/v1/auth/`, health) bypass tenant enforcement. Tenant ID is never optional — this is enforced in code at the edge, not just asserted in policy. The validated tenant is forwarded to backend services as the canonical `X-GarudaX-Tenant` header.

**Current tenants:**

| Tenant ID | Venue | Status | Description |
|-----------|-------|--------|-------------|
| ace-commodities | ACE Commodity Exchange | ACTIVE (live tenant) | Wheat, barley, cattle, cashmere, wool, physical delivery via eWR |
| mse-equities | Mongolia Stock Exchange | ONBOARDING (flagship) | Equities, bonds, ETFs, T+2 settlement, CSD integration |

**Today:** one live tenant (ace-commodities), with mse-equities onboarding. The platform validates and forwards tenant context at the edge. Per-tenant schema namespacing (`ace_*`) isolates data; full backend-side cross-tenant authorization (binding the authenticated user to a specific tenant) is on the roadmap, not yet enforced end-to-end. A platform control plane (platform-service, Phase 0.7) manages tenant lifecycle: onboarding, suspension, configuration, and decommissioning.

## Services (14 Go Modules)

The backend is 14 independent Go modules under `src/*/go.mod`, plus 2 shared zero-dependency sub-modules (`shared/pkg/types/decimal`, `shared/pkg/tenant`) consumed via filesystem `replace`. Two AI bots (admin-bot, architect-bot) run as Node.js services.

| Module | Role |
|---------|------|
| **matching-engine** | Central Limit Order Book (CLOB), price-time priority matching, iceberg orders, auction clearing |
| **clearing-engine** | Trade novation, netting, CCP functions |
| **margin-engine** | Real-time margin calculation, collateral management, margin calls |
| **settlement-engine** | T+2 settlement lifecycle, DVP, obligation tracking, fail management |
| **auth-service** | JWT authentication, PKCE, RBAC, bcrypt password hashing, session management |
| **compliance-service** | KYC/AML onboarding, regulatory screening, sanctions checks, MCSD/FRC settlement |
| **gateway** | HTTP API gateway, ~94 registered routes, WebSocket, rate limiting, tenant middleware |
| **market-data-service** | Real-time market data distribution, SSE feeds, OHLCV candles, TimescaleDB |
| **warehouse-service** | Warehouse receipts (eWR), historical data archival, analytics, reporting |
| **securities-service** | Instrument CRUD, reference data, surveillance, order/trade management, FRC reporting |
| **platform-service** | Platform control plane — tenant registry and lifecycle |
| **fix-gateway** | FIX 4.4 TCP gateway for broker connectivity |
| **corporate-actions** | Dividend, split, rights-issue, merger entitlement engine (library) |
| **shared** | Shared platform primitives — tenant context, observability, gRPC interceptors (library) |

The four real-time trading engines (matching, clearing, margin, settlement) are race-tested: `go test -race` passes green across all of them.

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
| Messaging | Apache Kafka — real wire-protocol event bus (segmentio/kafka-go), live cross-service event propagation |
| Cache | Redis 7 |
| Container | Docker, Kubernetes |
| Auth | JWT + PKCE + RBAC + bcrypt |
| Testing | Go testing, Vitest, Playwright |

## Codebase Metrics

| Metric | Count |
|--------|-------|
| Go modules | 14 (+ 2 shared zero-dep sub-modules) |
| Go test functions | ~2,795 (+ ~520 sub-tests) |
| Frontend tests | 800+ |
| E2E API tests | 32 top-level / 141 sub-tests |
| Live cross-service (Kafka) e2e | seeded gateway-to-settlement, verified PASS |
| Gateway routes | ~94 registered |
| Go test coverage | ~65% statement-weighted / ~70% business-logic |
| React SPAs | 3 (web-ui, admin-ui, demo-runner) |
| AI bots (Node.js) | 2 (admin-bot, architect-bot) |

All figures are derived from the current source tree; counts drift as the platform grows.

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

## Financial Correctness

For a regulated venue, money math has to be exact. GarudaX does not compute money in `float64` anywhere on the platform — this was audited and remediated across every service, and re-audited to confirm zero residual float money paths.

- **Single shared fixed-point Decimal type.** All monetary values use one shared, zero-dependency `Decimal` (4 decimal places, integer-scaled) rather than per-service copies that could drift.
- **No silent overflow.** Every multiply and divide checks a 128-bit intermediate (via `math/bits`) and reports overflow rather than wrapping an int64.
- **Banker's rounding.** Half-even rounding is used throughout, eliminating the systematic upward bias of truncation or round-half-up.
- **Error on divide-by-zero.** Division returns an explicit error instead of silently yielding zero — a correctness guarantee that matters for margin, fees, and settlement amounts.
- **Verified cross-service.** Cash legs reconcile across matching, clearing, margin, and settlement using the real shared type.

## Platform Hardening

Beyond financial correctness, the platform has been hardened for production operation:

- **Live event bus.** Cross-service events flow over real wire-protocol Kafka, wired into each engine's process startup. A real authenticated, tenant-scoped, gateway-submitted trade has been verified end-to-end on a live Kafka broker: it propagates matching → clearing (novated) → margin (call issued) → settlement (completed). Kafka wiring is fail-fast — a misconfigured deployment with no broker is rejected at startup rather than silently dropping events.
- **Race-tested engines.** The four trading engines pass `go test -race` clean — handler callbacks fire outside engine locks, eliminating the data-race and deadlock classes.
- **Reproducible deploys.** The full stack builds from committed Dockerfiles and initializes a clean database from committed migrations on a fresh volume — no manual workarounds — so a fresh bring-up is hands-off reproducible.
- **Enforced tenancy.** Tenant resolution is mandatory at the gateway for every tenant-scoped route (see Multi-Tenant Design).

## Development Model

GarudaX is built by an AI-driven development pipeline (Softhouse) that uses task graphs, isolated worktrees, automated review, and a learning loop. The platform has moved through its initial feature build and a multi-month correctness/hardening remediation arc (financial-correctness, concurrency, multi-tenancy enforcement, and a reproducible deploy pipeline), with a fresh full-stack integration run passing against a live broker.
