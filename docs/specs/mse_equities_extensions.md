# MSE Equities Extensions — Architecture Specification

**Document ID:** MSE-EXT-ARCH-001
**Version:** 1.0
**Date:** 2026-06-19
**Status:** DRAFT
**Task:** ARCH-1
**Authority:** `GarudaX_Strategy_Directive.md`, `docs/platform-architecture.md` §10

> GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.

---

## Table of Contents

1. [Purpose & Scope](#1-purpose--scope)
2. [Relationship to Existing Specs & Code](#2-relationship-to-existing-specs--code)
3. [Domain Difference Summary](#3-domain-difference-summary)
4. [Settlement Cycles — T+1 / T+2 Configurable](#4-settlement-cycles--t1--t2-configurable)
5. [Corporate Actions](#5-corporate-actions)
6. [Short Selling](#6-short-selling)
7. [Call Auctions (Opening / Closing)](#7-call-auctions-opening--closing)
8. [Tenant Configuration Keys](#8-tenant-configuration-keys)
9. [Data Model Additions](#9-data-model-additions)
10. [Kafka Topics & Events](#10-kafka-topics--events)
11. [API Surface](#11-api-surface)
12. [Sequencing & Phasing](#12-sequencing--phasing)
13. [Open Questions](#13-open-questions)
14. [Acceptance Criteria](#14-acceptance-criteria)

---

## 1. Purpose & Scope

This document specifies the **technical architecture for the MSE-equities-specific domain differences** that distinguish the Mongolian Stock Exchange (flagship `mse-equities` tenant) from the existing `ace-commodities` venue. It covers four domains called out in the strategy directive (lines 80–83):

1. **T+1 / T+2 settlement cycles** — configurable rolling DvP settlement against a central securities depository, versus ACE's same-day cash mark-to-market.
2. **Corporate actions** — dividends, splits, rights issues, mergers; no commodity equivalent.
3. **Short selling** — locate requirement, uptick/SSR rule, restricted list; per-tenant regulatory regime.
4. **Call auctions** — single-price uncrossing at session open/close, versus ACE's continuous CLOB.

### In scope
- The MSE-specific behaviours, configuration, and wiring for the four domains above.
- How those behaviours are selected by **tenant context** so ACE is unaffected.
- Gaps between the *current* code (`src/securities-service`, `src/matching-engine`, `src/settlement-engine`, `src/clearing-engine`) and the MSE flagship requirement, and the design to close them.

### Out of scope
- General multi-tenant plumbing (tenant header, schema prefixing, JWT claims) — owned by `docs/platform-architecture.md` §§2–9.
- The base securities instrument/order/trade model and the T+2 state machine internals — owned by `docs/securities-architecture.md`. This spec **references and configures**, it does not re-define them.
- MCSD wire-protocol (ISO 20022) message schemas — a separate integration spec; this document only fixes the adapter interface boundary.
- FRC report field-level layouts — a separate reporting spec; referenced here only where settlement/corporate-action events feed those reports.

---

## 2. Relationship to Existing Specs & Code

This spec is an **extension layer**. It does not duplicate the foundational designs; it points to them and specifies only the deltas.

| Concern | Authoritative source | This spec's role |
|---|---|---|
| Multi-tenant isolation, tenant header, schema/topic naming | `docs/platform-architecture.md` | Consumes; assumes `tenant_id` is always present |
| Securities instrument/order/trade model, V26–V28 schema | `docs/securities-architecture.md` §§3–9 | Configures per-MSE; adds settlement-cycle parameterisation |
| Settlement profiles concept | `docs/platform-architecture.md` §10.1 | Specifies T+1/T+2 profile mechanics |
| Corporate actions service | `docs/platform-architecture.md` §10.2 | Specifies the domain logic and data flow |
| Call auction concept | `docs/platform-architecture.md` §10.3 | Specifies the algorithm wiring and session scheduling |
| Short selling concept | `docs/securities-architecture.md` §4.4, `platform-architecture.md` §10.4 | Specifies MSE config and enforcement points |

### Existing code already present (must be reused, not rewritten)

| Capability | Location | Status |
|---|---|---|
| Call auction uncrossing (single clearing price, max volume) | `src/securities-service/internal/engine/auction.go` (`AuctionEngine.RunAuction`) | Implemented; tenant-aware (`RunAuction(instrumentID, tenantID)`) |
| Equilibrium-price auction (CLOB) | `src/matching-engine/internal/orderbook/auction.go` (`AuctionEngine.RunAuction`) | Implemented |
| T+2 settlement lifecycle engine | `src/securities-service/internal/settlement/engine.go` (`SettlementEngine`) | Implemented for T+2; needs cycle parameterisation |
| Corporate actions CRUD + `/process` | `src/securities-service/internal/server/handlers_corporate_actions.go` | Implemented for announce/list/get/process |
| Settlement obligation netting | `src/clearing-engine/internal/netting/` | Implemented (commodity); reused by key `(participant, instrument, settlement_date)` |
| Locate handlers | `src/securities-service/internal/server/handlers_locate.go` | Implemented |
| Day / session management | `src/securities-service/internal/engine/day_manager.go`, `session.go` | Implemented |

**Design rule:** every item below is expressed as either *configuration of existing code* or a *named, scoped new component*. No domain that already has working code is re-implemented from scratch.

---

## 3. Domain Difference Summary

| Aspect | ACE (`ace-commodities`) | MSE (`mse-equities`) | Selection mechanism |
|---|---|---|---|
| Settlement | T+0 daily cash MtM | T+1 or T+2 rolling DvP | `settlement.default_cycle` tenant config |
| Settlement venue | Internal cash | MCSD (book-entry DvP) | `settlement.csd` tenant config + CSD adapter |
| Corporate actions | None | Dividends, splits, rights, mergers | `features.corporate_actions` flag |
| Short selling | Disabled | Locate + SSR + restricted list | `features.short_selling` + `short_selling.*` config |
| Sessions | Continuous CLOB | Opening + closing call auctions around continuous | `auction.*` config + market-phase state machine |
| Price band | 5% | 15% (per `venues/mse-equities/config.json`) | `circuit_breaker.*` tenant config |

The selection mechanism is always **tenant configuration read at request/cycle time**, never a compile-time branch. A single binary serves both tenants; behaviour diverges on `TenantFromContext(ctx).TenantID` → config lookup.

---

## 4. Settlement Cycles — T+1 / T+2 Configurable

### 4.1 Requirement

Per the directive (line 80): *"the clearing engine must support both models as separate settlement profiles per tenant."* MSE settles equities and ETFs at T+2 and may settle some bonds at T+1; ACE settles same-day. The settlement cycle must be **data-driven**, not hard-coded.

### 4.2 Settlement Profile

A settlement profile is the unit of per-tenant settlement behaviour. It is resolved by `(tenant_id, asset_class)` and supplied to the settlement and clearing engines.

```go
type SettlementProfile struct {
    ProfileID        string   // "SECURITIES_T2_DVP", "SECURITIES_T1_DVP", "COMMODITY_DAILY_MTM"
    TenantID         string   // "mse-equities"
    AssetClasses     []string // ["EQUITY", "ETF"] or ["BOND"]
    Cycle            string   // "T+0" | "T+1" | "T+2"
    Mechanism        string   // "DVP" | "DAILY_MTM"
    NettingKey       string   // "PARTICIPANT_INSTRUMENT_DATE" | "INSTRUMENT"
    AffirmationDeadline string // "T+1 16:00 local" — empty for T+0
    FailPenaltyBps   map[string]float64 // by asset class, bps/day
    BuyInTriggerDay  int      // e.g. 4 → T+4; 0 = disabled
    CSDRequired      bool     // true for DVP
}
```

| Profile | Tenant | Asset classes | Cycle | Netting key |
|---|---|---|---|---|
| `COMMODITY_DAILY_MTM` | ace-commodities | COMMODITY | T+0 | instrument |
| `SECURITIES_T2_DVP` | mse-equities | EQUITY, ETF | T+2 | (participant, instrument, settlement_date) |
| `SECURITIES_T1_DVP` | mse-equities | BOND (configurable) | T+1 | (participant, instrument, settlement_date) |

### 4.3 Settlement Date Computation

`settlement_date` is computed at order-enrichment time (gateway/securities-service) and carried, immutable, on the order → trade → obligation chain. The matching engine never uses it.

```
settlement_date = addTradingDays(trade_date, cycleDays, tenant_trading_calendar)

where:
  cycleDays = 1 for T+1, 2 for T+2, 0 for T+0
  addTradingDays skips weekends and tenant holidays
  trading_calendar = platform.tenant_config[tenant_id]["trading_calendar"]
```

The trading calendar already exists for MSE in `venues/mse-equities/config.json` (`trading_calendar.holidays`). The settlement-date helper must read holidays from tenant config, **not** a hard-coded list.

### 4.4 Code Changes (parameterising the existing T+2 engine)

The current `src/securities-service/internal/settlement/engine.go` is documented as "the T+2 settlement engine." It must be generalised:

1. **Inject the profile.** `NewSettlementEngine` (or a new `WithProfile` setter, mirroring the existing `SetBondStore` pattern) accepts a `SettlementProfile`. `cycleDays` comes from the profile, not a constant.
2. **Calendar-aware date math.** Replace any `AddDate(0,0,2)` style arithmetic with `addTradingDays(tradeDate, profile.cycleDays, calendar)`.
3. **Profile-driven fail management.** Penalty bps and buy-in trigger day come from `profile.FailPenaltyBps` / `profile.BuyInTriggerDay`. The state machine itself (PENDING→AFFIRMED→NETTED→INSTRUCTED→SETTLING→SETTLED/FAILED) is unchanged — see `securities-architecture.md` §7.
4. **No behavioural change for ACE.** ACE keeps `COMMODITY_DAILY_MTM` (T+0); the securities settlement engine is simply not on ACE's path.

The T+2 state machine, netting, fail/penalty/buy-in mechanics are **already specified** in `docs/securities-architecture.md` §7. This spec only makes the cycle length and penalty/buy-in parameters a function of the profile.

### 4.5 Profile Resolution

```go
func (s *SettlementEngine) profileFor(ctx context.Context, assetClass string) SettlementProfile {
    tid := middleware.TenantFromContext(ctx).TenantID
    raw := s.configStore.Get(tid, "settlement.profiles") // JSONB from platform.tenant_config
    return raw.ProfileFor(assetClass)                     // matches AssetClasses, falls back to tenant default_cycle
}
```

Default fallback: if no asset-class-specific profile exists, build one from `platform.tenants.default_settlement_cycle` (`T+2` for MSE per `config.json`).

---

## 5. Corporate Actions

### 5.1 Requirement

Directive line 81: dividends, splits, rights issues, mergers — *"No commodity equivalent. New domain service required."* The domain logic, date model, and processing rules are specified in `docs/securities-architecture.md` §8.3. Existing handler code lives in `src/securities-service/internal/server/handlers_corporate_actions.go` (announce / list / get / process). This section specifies the **MSE wiring and lifecycle orchestration** around that code.

### 5.2 Action Types & Effects (reference)

See `securities-architecture.md` §8.3 for the full table (`DIVIDEND`, `STOCK_DIVIDEND`, `STOCK_SPLIT`, `REVERSE_SPLIT`, `RIGHTS_ISSUE`, `MERGER`, `TENDER_OFFER`, `SPIN_OFF`) and their effect on holdings. MSE supports all of them; the FRC-mandated minimum for the flagship launch is `DIVIDEND`, `STOCK_SPLIT`, `REVERSE_SPLIT`, and `RIGHTS_ISSUE`.

### 5.3 Lifecycle Orchestration

Corporate actions are date-driven. A scheduled job ("corporate action processor") advances each action through its key dates. This is the orchestration gap to close: handlers exist for manual `/process`, but date-triggered automation must be specified.

```
ANNOUNCED ──(announcement_date)──▶ holders notified, rights instrument created (if RIGHTS_ISSUE)
   │
   ├──(ex_date)──▶ EX_DATE_PASSED
   │                 • price/qty adjustment for splits applied to instrument, open orders, positions, CSD balances
   │                 • SSR reference (previous_close) recomputed post-adjustment
   │
   ├──(record_date)──▶ RECORD_DATE_PASSED
   │                 • snapshot CSD balances (securities.csd_balances)
   │                 • compute entitlements → securities.corporate_action_entitlements
   │
   └──(payment_date)──▶ PROCESSED
                     • DIVIDEND: cash credit instructions to settlement accounts (DvP-free cash leg)
                     • STOCK_DIVIDEND / SPLIT: FoP credit of new shares via CSD adapter
                     • RIGHTS_ISSUE: process exercises; expire unexercised rights
```

**Ordering invariant:** `announcement_date ≤ ex_date ≤ record_date ≤ payment_date`. Validated on announce (HTTP 400 on violation).

**Idempotency:** each (action_id, date-stage) transition is guarded by the action `status` column — re-running the processor for an already-`PROCESSED` action is a no-op. This protects against the channel-based event redelivery noted in the learned patterns.

### 5.4 Interaction With Other Domains

- **Open orders on ex_date:** a split adjusts resting order price and quantity atomically with the instrument adjustment. The matching engine must be quiesced (instrument `HALTED`) during the adjustment window, then resumed. The adjustment runs in `POST_CLOSE`/pre-open, never mid-continuous-session.
- **Settlement in flight:** obligations with `settlement_date ≥ ex_date` for a split-affected instrument are adjusted by the same ratio. Obligations already `SETTLED` are untouched.
- **Short positions on record_date:** a short holder owes the dividend/entitlement to the lender (manufactured dividend). The entitlement snapshot uses **net** CSD position; manufactured-dividend obligations are emitted as a compliance event for the securities-lending desk (out-of-band for v1, flagged via `large-trader-report`-style event).

### 5.5 New Component

`corporate-actions` capability lives inside `securities-service` (not a separate microservice for v1, to avoid the cross-process event-bridge problem documented in the learned patterns — Go-channel Kafka stubs don't cross processes). It comprises:

- existing handlers (`handlers_corporate_actions.go`)
- a new in-process scheduler (`internal/corpactions/scheduler.go`) that polls `securities.corporate_actions` for due date-stages each trading day
- a new entitlement calculator (`internal/corpactions/entitlement.go`)
- CSD adapter calls for share/cash distribution (§5.3)

If/when extracted to `src/corporate-actions-service/` (per `platform-architecture.md` §10.2), it must use a real Kafka adapter, not the channel stub.

---

## 6. Short Selling

### 6.1 Requirement

Directive line 82: *"must be implementable per tenant's regulatory regime."* MSE config (`venues/mse-equities/config.json`) has `features.short_selling: true`. The enforcement rules (locate, uptick/SSR, restricted list) and V26 schema (`securities.locates`, `securities.short_sell_restricted_list`, `securities.ssr_triggers`) are specified in `docs/securities-architecture.md` §4.4 and §9.1. Locate handlers exist (`handlers_locate.go`). This section specifies the **enforcement wiring and MSE configuration**.

### 6.2 MSE Configuration

| Config key | MSE value | Effect |
|---|---|---|
| `short_selling.enabled` | `true` | Short-sell orders accepted |
| `short_selling.locate_required` | `true` | Reject short sell without confirmed locate |
| `short_selling.uptick_rule_enabled` | `true` | SSR price test active after 10% decline |
| `short_selling.ssr_decline_pct` | `10.0` | Decline from prev close that triggers SSR |
| `short_selling.ssr_duration` | `rest_of_day + next_day` | SSR active window |
| `short_selling.restricted_list_update_frequency` | `daily` | Restricted list refresh cadence |

ACE leaves `short_selling.enabled` = `false`; the enforcement code is short-circuited when the flag is off, so ACE order flow is untouched.

### 6.3 Enforcement Points (pre-trade, in order path)

The order is marked `is_short_sell` at submission. Enforcement runs in the securities order-validation path **before** the order reaches the book:

```
1. If !config.short_selling.enabled and order.is_short_sell → REJECT SHORT_SELL_DISABLED
2. If instrument in securities.short_sell_restricted_list (active window) → REJECT SHORT_SELL_RESTRICTED
3. If config.locate_required:
     locate = lookupLocate(participant, instrument)   // handlers_locate.go store
     if locate == nil || status != CONFIRMED || available_qty < order.qty → REJECT SHORT_SELL_LOCATE_REQUIRED
4. If instrument.ssr_active (securities.ssr_triggers, active_until >= today):
     if order.price <= current_best_bid → REJECT UPTICK_RULE
5. Mark locate USED on fill (decrement available_qty)
```

Rejection reason codes mirror `securities-architecture.md` §4.4. All rejections produce an execution report with `exec_type = REJECTED` and the reason string (no raw errors).

### 6.4 SSR Trigger Detection

A market-data-driven monitor compares intraday last price against `previous_close`. On a ≥10% decline it inserts a `securities.ssr_triggers` row with `active_until = next_trading_day` and emits `mse-equities.securities.ssr-triggered`. The monitor runs in the securities-service market-data consumer; threshold comes from `short_selling.ssr_decline_pct`.

### 6.5 Locate Lifecycle (reference + MSE note)

Locate states (`REQUESTED → CONFIRMED → USED | DECLINED | EXPIRED`) and the `securities.locates` table are in `securities-architecture.md` §9.1. MSE note: locates expire at end of trading day (`valid_until = today 13:00 UB`); an unused confirmed locate is not carried overnight.

---

## 7. Call Auctions (Opening / Closing)

### 7.1 Requirement

Directive line 83: *"equities use call auctions at session boundaries; commodity CLOB is continuous. Matching engine must support both session types."* The uncrossing algorithm exists twice — `securities-service` `AuctionEngine.RunAuction` and `matching-engine` `AuctionEngine.RunAuction`. This section specifies the **session schedule, phase wiring, and which engine owns MSE auctions**.

### 7.2 Engine Ownership

MSE securities order flow is served by `securities-service`, whose `internal/engine/auction.go` already implements a tenant-aware `RunAuction(instrumentID, tenantID)` that collects `PENDING` orders and uncrosses at the max-volume clearing price, creating settlement obligations via the injected `SettlementEngine`. **MSE call auctions use this engine.** The `matching-engine` auction engine remains the path for any commodity/CLOB auction use and is not modified by this spec.

### 7.3 MSE Daily Session Schedule

Derived from `venues/mse-equities/config.json` (`trading_hours`, `pre_open_auction`, `closing_auction`). Note the config's continuous window is 10:00–13:00 with a pre-open auction 09:30–10:00 and closing auction 12:50–13:00. This spec adopts the config values as authoritative:

```
09:30–10:00  OPENING_AUCTION   (call auction; orders collected, no matching)
10:00        OPENING UNCROSS   (RunAuction → equilibrium price; carryover to continuous)
10:00–12:50  CONTINUOUS        (standard CLOB, price-time priority)
12:50–13:00  CLOSING_AUCTION   (call auction; orders collected, no matching)
13:00        CLOSING UNCROSS   (RunAuction → closing price = official close)
13:00–13:30  POST_CLOSE        (no trading; settlement, corp-action adjustments)
```

> Note: `platform-architecture.md` §10.3 shows an illustrative 09:00–13:30 schedule. Where it differs from `venues/mse-equities/config.json`, **the tenant config file is authoritative** and the times above govern. Flagged in [Open Questions](#13-open-questions).

### 7.4 Market-Phase State Machine (per tenant)

```
PRE_OPEN → OPENING_AUCTION → CONTINUOUS → CLOSING_AUCTION → CLOSED → POST_CLOSE
```

- The phase machine is per-tenant (`{tenant_id}:market-phase` Redis key, `platform-architecture.md` §7.2). MSE can be in `OPENING_AUCTION` while ACE is `CONTINUOUS`.
- Phase transitions are driven by the tenant trading calendar + clock (day_manager/session — `src/securities-service/internal/engine/day_manager.go`, `session.go`).
- Order acceptance rules by phase:

| Phase | LIMIT | MARKET | Cancel | Matching |
|---|---|---|---|---|
| OPENING_AUCTION | accept (collect) | reject | accept | none until uncross |
| CONTINUOUS | accept | accept | accept | immediate |
| CLOSING_AUCTION | accept (collect) | reject | accept | none until uncross |
| POST_CLOSE / CLOSED | reject | reject | reject | none |

MARKET orders are rejected during auction collection (no continuous reference price); this matches `securities-architecture.md` §4.1.

### 7.5 Uncrossing Algorithm (reference)

The max-executable-volume / reference-price-tiebreak algorithm is implemented and documented in both `auction.go` files. Summary: collect unique prices, for each candidate compute matchable `min(cumBid, cumAsk)`, pick max volume, tiebreak nearest to reference (previous close / last trade) then higher price, fill at the single clearing price in time priority. No change required; this spec only fixes *when* it is invoked (uncross at phase boundary) and *which reference price* to pass (previous close for opening, last continuous trade for closing).

### 7.6 Indicative Auction Data

During `OPENING_AUCTION`/`CLOSING_AUCTION`, the venue publishes an **indicative equilibrium price and volume** (IEP/IEV) without matching, so participants can react. Specified as a periodic (e.g. every 5s) dry-run of `RunAuction` over collected orders, published to `mse-equities.market-data.trade-ingested` as an `INDICATIVE` tick (non-binding, flagged). This is a small addition over the existing engine (the uncross calc is reused read-only).

---

## 8. Tenant Configuration Keys

All MSE behaviour is driven by `platform.tenant_config` (`platform-architecture.md` §8). Keys consumed by this spec:

| Config key | MSE value | Domain |
|---|---|---|
| `settlement.default_cycle` | `T+2` | §4 |
| `settlement.profiles` | JSON array of `SettlementProfile` | §4 |
| `settlement.csd` | `MCSD` | §4 |
| `settlement.affirmation_deadline` | `T+1 16:00 local` | §4 |
| `settlement.fail_penalty_rates` | `{"EQUITY":0.40,"GOVT_BOND":0.25,"CORP_BOND":0.50,"ETF":0.50}` bps/day | §4 |
| `settlement.buy_in_trigger_day` | `4` (T+4) | §4 |
| `features.corporate_actions` | `true` | §5 |
| `features.short_selling` | `true` | §6 |
| `short_selling.locate_required` | `true` | §6 |
| `short_selling.uptick_rule_enabled` | `true` | §6 |
| `short_selling.ssr_decline_pct` | `10.0` | §6 |
| `auction.opening_window` | `09:30–10:00` | §7 |
| `auction.closing_window` | `12:50–13:00` | §7 |
| `circuit_breaker.price_band_default` | `15.0` | §7 (ref) |
| `trading_calendar` | from `venues/mse-equities/config.json` | §4, §7 |

Seeding: these are inserted during MSE provisioning (`platform-architecture.md` §6.2 step 8) from `venues/mse-equities/config.json`. The config file is the single source of truth; `tenant_config` rows are derived from it.

---

## 9. Data Model Additions

The V26–V28 securities schema (`securities-architecture.md` §9) already provides: `instruments`, `orders`, `trades`, `positions`, `settlement_obligations`, `netting_results`, `csd_accounts`, `csd_balances`, `csd_transfers`, `locates`, `short_sell_restricted_list`, `ssr_triggers`, `position_limits`, `large_trader_thresholds`. Under multi-tenancy these live in `mse_securities.*`.

This spec adds, in `mse_securities` (migration V32+, per `platform-architecture.md` §A.5):

```sql
-- Corporate actions master (if not already present from V26-V28 backfill)
CREATE TABLE mse_securities.corporate_actions (
    action_id          VARCHAR(64) PRIMARY KEY,
    tenant_id          VARCHAR(64) NOT NULL DEFAULT 'mse-equities'
                       CHECK (tenant_id = 'mse-equities'),
    instrument_id      VARCHAR(64) NOT NULL,
    action_type        VARCHAR(30) NOT NULL CHECK (action_type IN (
        'DIVIDEND','STOCK_DIVIDEND','STOCK_SPLIT','REVERSE_SPLIT',
        'RIGHTS_ISSUE','MERGER','TENDER_OFFER','SPIN_OFF')),
    ratio              DECIMAL(18,8),            -- split/dividend ratio
    cash_per_share     DECIMAL(18,4),            -- for DIVIDEND
    announcement_date  DATE NOT NULL,
    ex_date            DATE NOT NULL,
    record_date        DATE NOT NULL,
    payment_date       DATE NOT NULL,
    status             VARCHAR(25) NOT NULL DEFAULT 'ANNOUNCED' CHECK (status IN (
        'ANNOUNCED','EX_DATE_PASSED','RECORD_DATE_PASSED','PROCESSED','CANCELLED')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ca_dates CHECK (
        announcement_date <= ex_date AND ex_date <= record_date AND record_date <= payment_date)
);
CREATE INDEX idx_mse_ca_instrument ON mse_securities.corporate_actions(instrument_id);
CREATE INDEX idx_mse_ca_status ON mse_securities.corporate_actions(status);
CREATE INDEX idx_mse_ca_dates ON mse_securities.corporate_actions(ex_date, record_date, payment_date);

-- Entitlement snapshot (record-date holdings × ratio)
CREATE TABLE mse_securities.corporate_action_entitlements (
    entitlement_id     VARCHAR(64) PRIMARY KEY,
    tenant_id          VARCHAR(64) NOT NULL DEFAULT 'mse-equities'
                       CHECK (tenant_id = 'mse-equities'),
    action_id          VARCHAR(64) NOT NULL REFERENCES mse_securities.corporate_actions(action_id),
    participant_id     VARCHAR(64) NOT NULL,
    held_qty           BIGINT NOT NULL,          -- net position at record date
    cash_entitlement   DECIMAL(18,4) NOT NULL DEFAULT 0,
    share_entitlement  BIGINT NOT NULL DEFAULT 0,
    distributed        BOOLEAN NOT NULL DEFAULT FALSE,
    distributed_at     TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (action_id, participant_id)
);
CREATE INDEX idx_mse_ca_ent_participant ON mse_securities.corporate_action_entitlements(participant_id);
```

`settlement_obligations.status` already covers the T+1/T+2 lifecycle (`securities-architecture.md` §9.3); no enum change needed — cycle length is carried by `settlement_date`, not status. Every new table carries the `tenant_id` defense-in-depth column + CHECK per `platform-architecture.md` §3.3.

---

## 10. Kafka Topics & Events

All under the `mse-equities.*` namespace (`platform-architecture.md` §4.3). Topics already enumerated there; this spec confirms the producers/consumers for the four domains:

| Topic | Producer | Consumer(s) | Domain |
|---|---|---|---|
| `mse-equities.securities.settlement-instructed` | settlement engine | CSD adapter, market-data | §4 |
| `mse-equities.securities.settlement-completed` | CSD adapter callback | clearing, margin release, FRC report | §4 |
| `mse-equities.securities.settlement-failed` | settlement engine | fail-mgmt, FRC fails report | §4 |
| `mse-equities.securities.corporate-action-announced` | corp-actions handler | market-data, participant notify | §5 |
| `mse-equities.securities.corporate-action-processed` | corp-actions scheduler | positions, CSD, market-data | §5 |
| `mse-equities.securities.ssr-triggered` | SSR monitor | order-validation, surveillance | §6 |
| `mse-equities.securities.large-trader-report` | position monitor | compliance/FRC | §6 (manufactured div, large trader) |
| `mse-equities.market-data.trade-ingested` | auction/CLOB | market-data | §7 (incl. INDICATIVE ticks) |

Event envelope carries mandatory `tenant_id` and `schema_version: 2` (`platform-architecture.md` §4.3). Each consumer is idempotent (keyed by event `id`) to tolerate redelivery — critical given the channel-based stub limitation noted in learned patterns; production cross-service flows require a real broker.

---

## 11. API Surface

MSE reuses the securities-service HTTP surface (`docs/securities-openapi.yaml`). Endpoints relevant to the four domains (all tenant-scoped via `X-GarudaX-Tenant`):

| Method | Path | Domain | Status |
|---|---|---|---|
| POST | `/api/v1/securities/orders` (with `is_short_sell`, `locate_id`) | §6 | exists |
| POST | `/api/v1/securities/locates` | §6 | exists (`handlers_locate.go`) |
| GET | `/api/v1/securities/short-sell/restricted` | §6 | add |
| POST | `/api/v1/securities/corporate-actions` | §5 | exists |
| GET | `/api/v1/securities/corporate-actions` | §5 | exists |
| POST | `/api/v1/securities/corporate-actions/{id}/process` | §5 | exists |
| GET | `/api/v1/securities/corporate-actions/{id}/entitlements` | §5 | add |
| GET | `/api/v1/securities/settlement/obligations` | §4 | exists |
| POST | `/api/v1/securities/settlement/cycle` | §4 | add (gap noted in e2e learned pattern) |
| POST | `/api/v1/securities/auctions/{instrument}/uncross` | §7 | add (admin/scheduler-triggered) |
| GET | `/api/v1/securities/auctions/{instrument}/indicative` | §7 | add (IEP/IEV) |

"add" items are the concrete build backlog this spec authorises. The `POST /v1/settlement/cycle` route closes a gap repeatedly surfaced by e2e tests (per CLAUDE.md learned patterns).

---

## 12. Sequencing & Phasing

This spec is consumed during **Phase 0.8 — mse-equities flagship build** (`platform-architecture.md` §11). Recommended task ordering (dependencies first):

1. **Settlement profile parameterisation** (§4) — generalise existing T+2 engine; prerequisite for auction-created obligations.
2. **Call auction wiring** (§7) — schedule + phase machine + uncross invocation; reuses existing engine.
3. **Short selling enforcement** (§6) — config + pre-trade checks + SSR monitor; reuses locate handlers.
4. **Corporate actions orchestration** (§5) — scheduler + entitlement calc + CSD distribution.
5. **MCSD adapter** (separate spec) — stub first (`platform-architecture.md` §10.6), real ISO 20022 later.

Each is a spec-first implementation pair (per the learned pattern that spec-first yields zero-rejection runs). Settlement and corporate actions are business-logic-heavy → spec task before builder task. Estimates: ~10m spec, ~15m implementation per domain (calibrated per CLAUDE.md timing patterns).

---

## 13. Open Questions

1. **Session schedule conflict.** `platform-architecture.md` §10.3 (09:00–13:30) vs `venues/mse-equities/config.json` (09:30–13:00 with 12:50 close auction). This spec takes the config file as authoritative; needs MSE/FRC confirmation of official hours.
2. **Bond settlement cycle.** Is T+1 actually used for MSE government bonds, or is everything T+2? `SECURITIES_T1_DVP` profile is specified but gated on confirmation. Default is T+2 if unconfirmed.
3. **Manufactured dividends.** v1 emits an event for the securities-lending desk but does not auto-settle short-holder dividend obligations. Confirm whether MSE requires automated manufactured-dividend settlement at launch.
4. **MCSD affirmation model.** Does MCSD provide trade affirmation messages (driving PENDING→AFFIRMED), or is auto-affirmation required? Affects §4.4 deadline handling.
5. **Corporate-action service extraction.** Kept in-process for v1 to avoid the cross-process channel-stub limitation. Extraction to `src/corporate-actions-service/` requires a real Kafka adapter — track as tech debt.

---

## 14. Acceptance Criteria

This spec is "done" / consumable when:

- [x] Each of the four directive domains (T+1/T+2, corporate actions, short selling, call auctions) has a concrete design grounded in existing code paths.
- [x] Every behaviour is selected by **tenant config**, with ACE explicitly unaffected.
- [x] Settlement is parameterised by profile (T+1/T+2/T+0), not hard-coded.
- [x] New data tables carry `tenant_id` defense-in-depth + CHECK constraint.
- [x] Kafka topics/events use the `mse-equities.*` namespace with `tenant_id` in the envelope.
- [x] The API build backlog ("add" rows in §11) is enumerated.
- [x] Conflicts with prior specs are surfaced (Open Questions §13) rather than silently resolved.
- [x] No domain with working code is re-implemented; the spec configures and extends.

Downstream (implementation) acceptance is owned by the builder tasks this spec spawns; not gated here.

---

*This document extends `docs/platform-architecture.md` and `docs/securities-architecture.md` for the MSE flagship. When in doubt: tenant config selects behaviour, ACE is never regressed, and MSE wins design ties.*
</content>
</invoke>
