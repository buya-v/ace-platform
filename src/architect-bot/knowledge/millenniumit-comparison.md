# GarudaX vs MillenniumIT — Head-to-Head Comparison

## Overview

MillenniumIT (MIT) is a London Stock Exchange Group subsidiary that provides exchange technology to 40+ venues globally, including MSE's current production system. GarudaX is a modern multi-tenant platform being built as an alternative.

This comparison is based on analysis of 600+ pages of MIT documentation (13 PDFs) cross-referenced against actual GarudaX source code, covering 86 discrete features.

## Feature Scoring Summary

| Score | Count | Percentage | Definition |
|-------|-------|------------|------------|
| FULL (>80%) | 3 | 3% | Covers most of MIT's described functionality |
| SUBSTANTIAL (50-80%) | 28 | 33% | Core logic works, missing significant sub-features |
| BASIC (20-50%) | 35 | 41% | Types and basic CRUD, missing business logic depth |
| STUB (<20%) | 8 | 9% | Type definition or minimal handler only |
| MISSING (0%) | 12 | 14% | Nothing implemented |
| **TOTAL** | **86** | **100%** | |

**GarudaX overall depth: ~47% of MillenniumIT functionality.**

This is up from ~22% at the start of the sprint cycle (8 sprints ago). The jump came from PostgreSQL persistence, privilege hierarchy, trading parameters, surveillance engine, FIX gateway, trading cycles, day lifecycle, and indices/permissions/folders/warnings.

## What GarudaX Does Well (31 FULL + SUBSTANTIAL Features)

**FULL (3):**
- Tiered tick tables — multi-band price validation with tests
- Session phases — per-instrument PRE_OPEN/CONTINUOUS/CLOSING_AUCTION/CLOSED with extend/shorten API
- Circuit breakers — static + dynamic price bands, TradingParameterSet integration, configurable per instrument

**SUBSTANTIAL (28):**
- Market/Segment CRUD, opening/closing auctions, equity instrument fields (22+ fields)
- Bond instruments (maturity, coupon, 3 day-count conventions)
- Participant/Firm hierarchy with PostgreSQL persistence
- Settlement obligations (full T+2 lifecycle with accrued interest)
- Trade corrections (bust/correct/reinstate with audit trail)
- Off-book trades (confirm/reject workflow)
- Maker-checker pending changes (submit/approve/reject with four-eyes)
- Surveillance engine (12 alert patterns, dashboard, investigation linking)
- FIX TCP gateway (session management, heartbeat, 95.3% tag coverage, 89 tests)
- Day lifecycle (DayManager state machine: CLOSED -> PRE_OPEN -> TRADING -> POST_CLOSE)
- Trading parameter sets, entity permissions, privilege hierarchy
- Order management, trade management, watch lists, announcements, password policy

## What Needs Depth (35 BASIC Features)

These have types, CRUD, and some business logic but lack MIT's depth:

- Settlement processing (lifecycle exists, needs actual netting/novation depth)
- Auction parameters (no random end, no configurable surplus modes)
- Activity logs (audit store exists, needs rich filtering and export)
- Order/trade queries (some filters, not MIT's 15-20 field query system)
- User management (auth service not integrated with securities-service user model)
- Market data distribution (SSE exists, no L2/L3 depth feed protocol)
- Corporate actions, CSD integration, position management (basic CRUD, needs workflows)
- Reports, replay, strategy orders, RFQ, give-up trades (handlers + tests, need depth)

## Still Missing (20 STUB + MISSING Features)

**STUB (8):** Dual auth for deletion, forced logout, lock/unlock user, surveillance roles, event descriptions, benchmark upload, bid-offer graph, relationship manager.

**MISSING (12):** Closed Order Books Window, Admin Console/Salvage Mode, Partitions (load distribution), Definition/template system, Rule Builder, Surveillance graphs, Graph Manager, Instrument Group Privileges, 4 MIT admin categories, Multi-market prerequisites, Configurable surplus modes, Price update workflow.

## 7 GarudaX Differentiators (Capabilities MIT Does Not Have)

1. **Multi-tenant architecture** — GarudaX hosts multiple independent exchanges on one platform. MIT is deployed once per venue. GarudaX onboards a new exchange without new infrastructure.

2. **AI-native operations** — 9 MCP tools, admin-bot service, natural language admin operations. MIT has zero AI integration.

3. **Modern open stack** — Go 1.25, React 19, PostgreSQL, Kafka, Docker, Kubernetes. MIT runs proprietary C++ with custom protocols. Any team that knows Docker can deploy GarudaX.

4. **FIX 4.4 protocol gateway** — Industry-standard broker connectivity. MIT uses proprietary OUCH/ITCH derivatives that require custom client libraries.

5. **Native binary protocol codec** — 92.5% field coverage alongside FIX, giving brokers connectivity options MIT charges separately for.

6. **Comprehensive test suite** — 2,540+ Go tests, 313 frontend tests, 58 e2e tests, 13 Playwright tests. Exceeds typical exchange system test coverage at any stage.

7. **Open infrastructure** — Docker Compose for dev, Kubernetes for production, standard PostgreSQL and Kafka. Zero vendor lock-in to proprietary hardware or middleware.

## What MIT Has That GarudaX Doesn't (And Why Some Don't Matter)

| MIT Feature | Relevance to MSE |
|-------------|-----------------|
| Partition-based load distribution | MIT's horizontal scaling for 100K+ msg/sec. MSE processes ~2,000 trades/day — Kubernetes pod replication is sufficient |
| Definition/template system | Runtime schema modification. Useful for rapid customization but adds complexity. GarudaX uses compiled Go structs — safer, faster, tested |
| Admin Console / Salvage Mode | Emergency recovery. Important for production — GarudaX needs this before go-live |
| Rule Builder | Visual trading control composition. Nice-to-have; rules can be configured via API |
| 25+ general privilege types | MIT has deeper permission granularity. GarudaX has ~10 but the EntityPermission system can expand without architecture changes |

## Key Message

GarudaX is at 47% functional depth vs MIT and growing fast (22% to 47% in 8 sprints). The remaining gap is concentrated in configuration depth and operational tooling, not core trading functionality. GarudaX can reach MIT parity in production-critical areas (matching, settlement, surveillance) while offering multi-tenancy, AI integration, and open infrastructure that MIT will never have.

## Sprint-by-Sprint Progress

| Sprint | Focus | Key Result |
|--------|-------|------------|
| 1 | PostgreSQL persistence | 6 Pg stores implemented |
| 2 | Privilege hierarchy | PrivilegeEngine + EntityPermission |
| 3 | Trading parameters | Unified TradingParameterSet per instrument |
| 4 | Surveillance engine | 12 alert patterns (up from 4) |
| 5 | FIX TCP gateway | TCP listener, session mgmt, 89 tests |
| 6 | Trading config | TradingCycle, PostTradeParams, ConfigTables |
| 7 | Day lifecycle | DayManager state machine, session extension API |
| 8 | Indices + permissions + folders + warnings | Index calculation, EntityPermission CRUD, Folder hierarchy |
