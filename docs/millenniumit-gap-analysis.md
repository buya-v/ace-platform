# MillenniumIT vs GarudaX — Gap Analysis

**Date:** 2026-04-25 (revised — reflects actual implementation depth)
**Source:** MillenniumIT platform documents from /home/vcp/MilleniumIT/
**Purpose:** Identify feature gaps for MSE flagship tenant migration
**Method:** Each feature scored by comparing MillenniumIT document descriptions against actual GarudaX code

## Coverage Summary

| Category | Features | Covered | Partial | Missing |
|----------|---------|---------|---------|---------|
| Trading Engine | 10 | 7 | 2 | 1 |
| Market Management | 6 | 4 | 2 | 0 |
| Instrument Management | 7 | 5 | 2 | 0 |
| Trading Parameters | 4 | 4 | 0 | 0 |
| Post-Trade | 6 | 4 | 1 | 1 |
| Corporate Actions | 1 | 1 | 0 | 0 |
| Surveillance | 5 | 2 | 2 | 1 |
| Investigation | 5 | 2 | 1 | 2 |
| Market Replay | 3 | 1 | 1 | 1 |
| User & Access Control | 8 | 4 | 2 | 2 |
| Reference Data Mgmt | 3 | 2 | 1 | 0 |
| Service Desk | 4 | 2 | 1 | 1 |
| CSD | 3 | 1 | 2 | 0 |
| Connectivity | 4 | 2 | 1 | 1 |
| Reporting | 2 | 2 | 0 | 0 |
| **TOTAL** | **71** | **43 (61%)** | **18 (25%)** | **10 (14%)** |

## Scoring Criteria

- **Covered**: Feature has types, store, handlers, tests, and gateway routes. Functionally complete for demo and MVP.
- **Partial**: Types and stores exist but missing some of: proper validation, edge case handling, production-grade error handling, UI integration, or MillenniumIT-equivalent depth (e.g. we have basic alerts but not the full 15-pattern surveillance workstation).
- **Missing**: Not implemented or only a placeholder.

## Detailed Feature Assessment

### Trading Engine (7 Covered, 2 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 1 | LIMIT/MARKET orders | **Covered** | Price-time priority matching, engine.go |
| 2 | STOP/STOP_LIMIT orders | **Covered** | Stop price validation in handlers_order.go |
| 3 | Iceberg (hidden quantity) | **Covered** | Visible/hidden qty matching + replenish in engine.go |
| 4 | IOC/FOK time-in-force | **Covered** | IOC post-cancel, FOK pre-check in engine.go |
| 5 | Self-trade prevention | **Covered** | Cancel newest/oldest/both modes in engine.go |
| 6 | Quote entry (market makers) | **Covered** | Two-sided quotes creating paired orders, handlers_quote.go |
| 7 | Mass cancel | **Covered** | By instrument/participant/side, handlers_order.go |
| 8 | Order throttling | **Partial** | Per-participant rate limiting exists but not configurable per-firm via admin API — hardcoded 100/sec |
| 9 | RFQ system | **Partial** | Submit/respond/cancel workflow exists but no real-time notification to market makers — polling only |
| 10 | Off-book trades | **Missing** | Types + CRUD handlers exist but no counterparty confirmation workflow, no regulatory trade report generation |

### Market Management (4 Covered, 2 Partial)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 11 | Market/Segment hierarchy | **Covered** | Market + Segment structs, CRUD, MSE seeded |
| 12 | Start/End Day workflow | **Covered** | DayManager with PRE_OPEN→TRADING→POST_CLOSE→CLOSED |
| 13 | Session phases per instrument | **Covered** | SessionManager with PRE_OPEN/CONTINUOUS/CLOSING_AUCTION/CLOSED |
| 14 | Opening/closing auctions | **Covered** | AuctionEngine with price-ladder clearing price |
| 15 | Extend/shorten sessions | **Partial** | No timer-based session duration — transitions are manual only |
| 16 | Halt/resume (market/segment/instrument) | **Partial** | Instrument-level halt exists but no market-wide or segment-wide halt cascade |

### Instrument Management (5 Covered, 2 Partial)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 17 | Equity instruments CRUD | **Covered** | Full lifecycle with ISIN, ticker, lot_size, tick_size |
| 18 | Bond instruments | **Covered** | BondInstrument with maturity, coupon, accrued interest (3 day-count conventions) |
| 19 | Multi-leg strategies | **Covered** | Strategy types (SPREAD/STRADDLE/STRANGLE/BUTTERFLY/CALENDAR) with legs |
| 20 | Instrument groups | **Covered** | Expression/manual grouping, CRUD |
| 21 | Instrument deletion lifecycle | **Covered** | Soft delete with flag-for-deletion + 30-day grace |
| 22 | Reference data bulk upload | **Partial** | JSON array upload exists but no CSV/XML import, no template download |
| 23 | ETF instruments | **Partial** | EQUITY asset_class covers basic ETFs but no NAV calculation, no creation/redemption basket |

### Trading Parameters (4 Covered)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 24 | Price tick tables | **Covered** | Tiered tick tables with multi-price-band validation |
| 25 | Circuit breakers (static + dynamic) | **Covered** | Static/dynamic price bands, auto-halt on breach |
| 26 | Reference price management | **Covered** | Set/get with stale detection, circuit breaker sync |
| 27 | Position limits | **Covered** | Per-instrument per-participant (in Instrument struct) |

### Post-Trade (4 Covered, 1 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 28 | T+2 settlement state machine | **Covered** | PENDING→AFFIRMED→NETTED→SETTLED/FAILED |
| 29 | Trade correction (bust/correct/reinstate) | **Covered** | With audit trail, handlers_trade.go |
| 30 | Give-up/give-in | **Covered** | Trade transfer with accept/reject workflow |
| 31 | Drop copy service | **Covered** | Firm subscription, execution report logging |
| 32 | Clearing firm/mnemonic | **Partial** | Firm struct has ClearingFirmID but no clearing account management or fee netting |
| 33 | Accrued interest on settlement | **Missing** | Bond accrued interest calculation exists standalone but not wired into settlement price adjustment |

### Corporate Actions (1 Covered)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 34 | Dividend, stock split, rights, merger | **Covered** | Announce + process entitlements, handlers_corporate_actions.go |

### Surveillance (2 Covered, 2 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 35 | Alert generation | **Covered** | LARGE_TRADE + WASH_TRADE auto-detection in matching engine |
| 36 | Alert threshold config | **Covered** | Per-instrument thresholds, set/get endpoints |
| 37 | Real-time monitoring views | **Partial** | Alert list endpoint exists but no instrument/firm/client split views as in MillenniumIT workstation |
| 38 | Alert patterns (15+ types) | **Partial** | Only 2 patterns (LARGE_TRADE, WASH_TRADE) vs MillenniumIT's 15 (front running, spoofing, layering, etc.) |
| 39 | Watch lists | **Missing** | Not implemented |

### Investigation (2 Covered, 1 Partial, 2 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 40 | Case management | **Covered** | Create from alert, assign, close with findings |
| 41 | Evidence attachment | **Covered** | Add evidence_id to case |
| 42 | Pattern manager (rule editor) | **Partial** | AlertThreshold struct exists but no complex rule logic (IF/AND/OR conditions) |
| 43 | Relationship manager | **Missing** | No insider/trader/client relationship mapping |
| 44 | Graph manager (visualization) | **Missing** | No chart/graph generation for investigations |

### Market Replay (1 Covered, 1 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 45 | Replay session creation | **Covered** | Create session, collect events from stores |
| 46 | Event stream + order book reconstruction | **Partial** | Events stored but no real-time playback with pause/forward/reverse controls |
| 47 | Conditional pause points | **Missing** | No breakpoint/filter system on replayed events |

### User & Access Control (4 Covered, 2 Partial, 2 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 48 | Participant/Firm hierarchy | **Covered** | Firm + ExchangeParticipant with status |
| 49 | Permission system | **Covered** | PERM_* constants, CheckPermission before order submit |
| 50 | Force logout + user suspension | **Covered** | Cascade to mass-cancel orders + audit |
| 51 | Dual authorization (maker-checker) | **Covered** | PendingChange with four-eyes enforcement (reviewer≠submitter) |
| 52 | Instrument-level permissions | **Partial** | Instrument groups exist but no per-group positive/negative permission grants |
| 53 | Node hierarchy (Participant→Node→User) | **Partial** | Flat firm→participant model, no intermediate node level with privilege inheritance |
| 54 | IP address restrictions | **Missing** | Not implemented |
| 55 | Password policy management | **Missing** | Not implemented |

### Reference Data Management (2 Covered, 1 Partial)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 56 | Pending changes workflow | **Covered** | Submit → approve/reject by different user |
| 57 | Activity log / audit trail | **Covered** | Append-only AuditEntry with filters |
| 58 | Mass amend (bulk modify) | **Partial** | Bulk create exists (handlers_bulk.go) but no bulk update/modify |

### Service Desk (2 Covered, 1 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 59 | Submit orders on behalf | **Covered** | Service desk order submission + cancel |
| 60 | Announcements | **Covered** | Public/custom audience, create/list |
| 61 | Market data display | **Partial** | Order book snapshot + ticker exist but no Time & Sales, no Dashboard, no Ticker tape |
| 62 | Trade capture reports | **Missing** | No per-firm formatted trade capture report generation |

### CSD (1 Covered, 2 Partial)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 63 | Custody accounts | **Covered** | CRUD with trading/settlement/safekeeping types |
| 64 | DvP/FoP transfers | **Partial** | Transfer struct + complete/fail workflow but no actual balance movement validation |
| 65 | Corporate action distribution via CSD | **Partial** | Corporate actions exist and CSD exists but not wired together — dividends don't flow through CSD accounts |

### Connectivity (2 Covered, 1 Partial, 1 Missing)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 66 | FIX 4.4 protocol gateway | **Covered** | Parser (95.3%), mapper, session manager (100%), broker registry — complete service |
| 67 | Native binary protocol | **Covered** | Codec library (92.5%), 7 message types, fixed-point price encoding |
| 68 | Market data feeds (FAST/MITCH) | **Partial** | Market data endpoints exist but no push-based feed protocol — HTTP polling only |
| 69 | Drop copy gateway | **Missing** | Drop copy store exists but no dedicated FIX/native gateway for execution report delivery |

### Reporting (2 Covered)

| # | Feature | Status | Detail |
|---|---------|--------|--------|
| 70 | FRC regulatory reports | **Covered** | Daily summary, large trader, suspicious activity |
| 71 | Trade reporting | **Covered** | Trade list + correction history endpoints |

## Build Artifacts

| Artifact | Count | Detail |
|----------|-------|--------|
| Handler files | 55 | src/securities-service/internal/server/handlers_*.go |
| In-memory store implementations | 32 | All store interfaces have InMemory implementations |
| Struct types | 48 | Domain models in types.go |
| Engine files | 7 | matching, auction, session, circuit breaker, day manager, permissions, tick table |
| Test files | 41 | Across securities-service + fix-gateway |
| Protocol codec | 4 | message.go, messages.go, codec.go, codec_test.go |
| Gateway proxy routes | 90+ | All securities + platform + FIX routes |
| Total API endpoints | 120+ | HTTP endpoints across securities-service |

## Key Gaps for MSE Production

### Must Fix Before Go-Live

1. **Market-wide halt cascade** — halting MSE market should halt all instruments, not just one
2. **Full surveillance patterns** — need at least 8-10 patterns (add: front running, spoofing, layering, price manipulation, wash trading enhancement)
3. **CSD ↔ corporate actions wiring** — dividends must flow through CSD custody accounts
4. **Settlement ↔ bond accrued interest** — settlement price must include accrued interest for bond trades
5. **Throttle configurability** — admin API to set per-firm order rate limits

### Nice to Have

6. Watch lists for surveillance
7. Relationship manager for investigations
8. CSV/XML bulk import (currently JSON only)
9. Market data push feeds (WebSocket or FAST)
10. IP address restrictions per user
