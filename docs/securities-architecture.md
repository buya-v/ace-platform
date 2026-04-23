# Securities Module Architecture Specification

**Document ID:** SECURITIES-ARCH-001
**Version:** 1.0
**Date:** 2026-04-23
**Status:** DRAFT
**Author:** Coder Agent (Phase 7 Planning)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Asset Classes](#2-asset-classes)
3. [Instrument Model](#3-instrument-model)
4. [Order Types](#4-order-types)
5. [Position Limits](#5-position-limits)
6. [Securities Clearing](#6-securities-clearing)
7. [T+2 Settlement State Machine](#7-t2-settlement-state-machine)
8. [CSD Integration](#8-csd-integration)
9. [Database Schema](#9-database-schema)
10. [API Routes](#10-api-routes)
11. [Kafka Topics](#11-kafka-topics)
12. [Integration with Existing Platform](#12-integration-with-existing-platform)

---

## 1. Overview

The Securities module extends the GarudaX commodity exchange platform to support equities, bonds, and ETFs. It reuses the existing matching engine (CLOB with price-time priority), gateway, auth, and compliance infrastructure while adding securities-specific components:

- **Instrument reference data** with ISIN/CUSIP/SEDOL identifiers, lot sizes, tick sizes, and bond-specific attributes (coupon, maturity, day count convention)
- **Order validation** enforcing lot size multiples, tick size alignment, and short-sale rules (locate, uptick, restricted list)
- **Position limits** per security per participant with concentration caps and large trader reporting
- **CCP clearing with novation** and bilateral netting by instrument + settlement date
- **T+2 settlement state machine** with fail management, buy-in procedures, and CSDR-style penalty interest
- **CSD integration** for custody accounts, securities transfer (FoP/DvP), and corporate action processing

### What Changes vs Commodities

| Aspect | Commodities (current) | Securities (new) |
|---|---|---|
| Instrument identifiers | Internal symbol (`WHT-HRW-2026M07-UB`) | ISIN (12-char), CUSIP (9-char), SEDOL (7-char) |
| Settlement cycle | Daily MtM (same-day cash settlement) | T+2 rolling settlement (delivery of securities + cash) |
| Delivery | Physical via warehouse receipts (eWR) | Book-entry transfer via CSD |
| Lot size | `contract_size` (e.g., 5000 bushels) | Board lot (e.g., 100 shares) |
| Short selling | N/A (physical delivery) | Locate requirement, uptick rule, restricted list |
| Corporate actions | N/A | Dividends, stock splits, rights issues, mergers |
| Position limits | Per-instrument margin-based | Per-security + concentration (% of outstanding) |
| Clearing | Trade-by-trade novation + MtM | CCP novation + bilateral netting by settlement date |

### Scope

This spec covers the securities extension only. It does NOT modify existing commodity trading, clearing, or settlement flows. The two asset classes share the same matching engine, gateway, and auth/compliance services. The securities module adds new tables, new Kafka topics, and new API endpoints alongside the existing ones.

---

## 2. Asset Classes

### 2.1 Equities

**Common Stock (`COMMON`)**
- Ownership shares in a corporation with voting rights
- No maturity date, no coupon
- Dividends are discretionary (declared by board)
- Price quoted per share, traded in board lots (typically 100 shares)
- Settlement: T+2 DvP (delivery of shares against payment)

**Preferred Stock (`PREFERRED`)**
- Fixed dividend rate, priority over common in liquidation
- May be convertible to common stock
- No maturity unless callable/redeemable
- Treated as equity for trading purposes, bond-like for income

### 2.2 Bonds

**Government Bonds (`GOVT_BOND`)**
- Issued by sovereign governments (e.g., Mongolian Government Bond)
- Quoted as percentage of par value (e.g., 99.750 = 99.75% of par)
- Fixed or floating coupon, regular payment schedule
- Day count convention determines accrued interest calculation
- Settlement: T+2 DvP with accrued interest

**Corporate Bonds (`CORP_BOND`)**
- Issued by corporations
- Higher credit risk than government bonds, priced with credit spread
- May have call/put provisions
- Same quoting and settlement conventions as government bonds

**Zero-Coupon Bonds (`ZERO_COUPON`)**
- No periodic coupon payments
- Issued at a discount to par value
- Accrued interest calculated as straight-line amortization of discount
- `coupon_rate = 0`, `coupon_frequency = NONE`

### 2.3 ETFs (Exchange-Traded Funds)

**ETF (`ETF`)**
- Fund shares traded on exchange like equities
- NAV (Net Asset Value) calculated daily by fund administrator
- Creation/redemption by Authorized Participants (AP) in creation units
- Traded in board lots like equities
- Settlement: T+2 DvP
- No coupon, no maturity (open-ended)
- `nav_per_share` updated daily (informational, not used for settlement)

---

## 3. Instrument Model

### 3.1 Identifier Standards

| Identifier | Format | Standard | Example | Usage |
|---|---|---|---|---|
| `isin` | 12 alphanumeric chars | ISO 6166 | `MN0000012345` | Primary global identifier; country code (2) + NSIN (9) + check digit (1) |
| `cusip` | 9 alphanumeric chars | CUSIP (North America) | `12345A109` | Issuer (6) + issue (2) + check digit (1) |
| `sedol` | 7 alphanumeric chars | SEDOL (UK/Ireland) | `B0WNLY7` | 6-char identifier + check digit (1) |
| `exchange_code` | 4 alphanumeric chars | MIC (ISO 10383) | `MXUB` | Market Identifier Code for the listing exchange |
| `ticker` | Up to 12 chars | Exchange-specific | `APU.UB` | Human-readable trading symbol |

### 3.2 Instrument Attributes

```go
type AssetClass string

const (
    AssetClassEquity AssetClass = "EQUITY"
    AssetClassBond   AssetClass = "BOND"
    AssetClassETF    AssetClass = "ETF"
)

type SecurityType string

const (
    SecurityTypeCommon     SecurityType = "COMMON"
    SecurityTypePreferred  SecurityType = "PREFERRED"
    SecurityTypeGovtBond   SecurityType = "GOVT_BOND"
    SecurityTypeCorpBond   SecurityType = "CORP_BOND"
    SecurityTypeZeroCoupon SecurityType = "ZERO_COUPON"
    SecurityTypeETF        SecurityType = "ETF"
)

type TradingStatus string

const (
    TradingStatusActive    TradingStatus = "ACTIVE"
    TradingStatusHalted    TradingStatus = "HALTED"
    TradingStatusSuspended TradingStatus = "SUSPENDED"
    TradingStatusDelisted  TradingStatus = "DELISTED"
)

type CouponFrequency string

const (
    CouponFrequencyNone       CouponFrequency = "NONE"
    CouponFrequencyAnnual     CouponFrequency = "ANNUAL"
    CouponFrequencySemiAnnual CouponFrequency = "SEMI_ANNUAL"
    CouponFrequencyQuarterly  CouponFrequency = "QUARTERLY"
    CouponFrequencyMonthly    CouponFrequency = "MONTHLY"
)

type DayCountConvention string

const (
    DayCountACT360  DayCountConvention = "ACT/360"
    DayCountACT365  DayCountConvention = "ACT/365"
    DayCount30360   DayCountConvention = "30/360"
    DayCountACTACT  DayCountConvention = "ACT/ACT"
)

type SecurityInstrument struct {
    InstrumentID       string             // UUID v7, primary key
    ISIN               string             // 12-char ISO 6166 (unique, not null)
    CUSIP              string             // 9-char, nullable (not all securities have CUSIP)
    SEDOL              string             // 7-char, nullable
    Ticker             string             // Exchange-specific symbol, e.g., "APU.UB"
    ExchangeCode       string             // MIC code, e.g., "MXUB"
    Name               string             // Full instrument name, e.g., "APU JSC Common Stock"
    AssetClass         AssetClass         // EQUITY, BOND, ETF
    SecurityType       SecurityType       // COMMON, PREFERRED, GOVT_BOND, CORP_BOND, ZERO_COUPON, ETF
    Currency           string             // ISO 4217, e.g., "MNT", "USD"
    LotSize            int64              // Board lot size (e.g., 100 shares)
    TickSize           Decimal            // Minimum price increment (e.g., 1.00 MNT)
    ListingDate        time.Time          // Date instrument was listed on exchange
    TradingStatus      TradingStatus      // ACTIVE, HALTED, SUSPENDED, DELISTED

    // Issuer information
    IssuerName         string             // e.g., "APU JSC"
    IssuerCountry      string             // ISO 3166-1 alpha-2, e.g., "MN"
    Sector             string             // e.g., "Consumer Goods", "Banking"

    // Equity-specific
    SharesOutstanding  int64              // Total shares issued (for concentration limits)
    MarketCap          Decimal            // Informational, updated daily

    // Bond-specific (zero values for equities/ETFs)
    ParValue           Decimal            // Face value, e.g., 1000.00 MNT
    CouponRate         Decimal            // Annual coupon rate, e.g., 8.5000 (%)
    CouponFrequency    CouponFrequency    // NONE, ANNUAL, SEMI_ANNUAL, QUARTERLY, MONTHLY
    DayCountConvention DayCountConvention // ACT/360, ACT/365, 30/360, ACT/ACT
    MaturityDate       time.Time          // Bond maturity date (zero for equities/ETFs)
    IssueDate          time.Time          // Original issue date
    NextCouponDate     time.Time          // Next scheduled coupon payment date
    AccruedInterest    Decimal            // Current accrued interest per unit of par

    // ETF-specific
    NAVPerShare        Decimal            // Net Asset Value per share (updated daily)
    FundManager        string             // Fund management company name

    // Trading controls
    PriceBandPct       Decimal            // Max price deviation from reference (e.g., 10.00 = 10%)
    MaxOrderQty        int64              // Maximum order quantity in lots
    MaxOrderValue      Decimal            // Maximum order value in currency
    ShortSellAllowed   bool               // Whether short selling is permitted for this instrument
    MarginEligible     bool               // Whether the instrument can be used as margin collateral

    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

### 3.3 Accrued Interest Calculation

For bonds, the buyer pays the seller accrued interest from the last coupon date to the settlement date.

**ACT/360:** `accrued = par_value * coupon_rate/100 * actual_days / 360`
**ACT/365:** `accrued = par_value * coupon_rate/100 * actual_days / 365`
**30/360:** `accrued = par_value * coupon_rate/100 * day_count_30_360 / 360`
**ACT/ACT:** `accrued = par_value * coupon_rate/100 * actual_days / actual_days_in_period`

Where:
- `actual_days` = calendar days from last coupon date to settlement date
- `day_count_30_360` = `(Y2-Y1)*360 + (M2-M1)*30 + (D2-D1)` with day adjustments per ISDA 30/360

---

## 4. Order Types

### 4.1 Supported Order Types

The existing matching engine already supports all four order types. Securities orders reuse the same `OrderType` enum:

| Order Type | Behavior | Securities-Specific Notes |
|---|---|---|
| `LIMIT` | Execute at specified price or better | Most common for securities; tick size enforced |
| `MARKET` | Execute immediately at best available price | Rejected during auction phases for securities |
| `STOP_LIMIT` | Becomes limit order when stop price is triggered | Uses last trade price as trigger |
| `STOP_MARKET` | Becomes market order when stop price is triggered | Subject to uptick rule if short sell |

### 4.2 Lot Size Validation

All securities orders must be in whole lot multiples. An order is **rejected** if `quantity % lot_size != 0`.

```
Validation rule:
  IF order.quantity % instrument.lot_size != 0 THEN
    REJECT with reason "INVALID_LOT_SIZE: quantity must be a multiple of {lot_size}"
  END IF
```

**Odd lot handling:** Odd lots (quantities < 1 board lot) are NOT supported in this version. Future enhancement may add an odd-lot book.

### 4.3 Tick Size Enforcement

Order price must be a valid tick. An order is **rejected** if `price % tick_size != 0`.

```
Validation rule:
  IF order.order_type IN (LIMIT, STOP_LIMIT) THEN
    IF NOT price.IsMultipleOf(instrument.tick_size) THEN
      REJECT with reason "INVALID_TICK_SIZE: price must be a multiple of {tick_size}"
    END IF
  END IF
```

For bonds quoted as percentage of par: if `tick_size = 0.125` (1/8th), valid prices include 99.000, 99.125, 99.250, etc.

### 4.4 Short-Sale Rules

Short selling is selling securities the seller does not currently own, borrowing them for delivery.

**Locate Requirement:**
Before submitting a short-sell order, the participant must have a valid securities locate — confirmation from a lender that shares are available to borrow.

```
Validation rule:
  IF order.side == SELL AND order.is_short_sell == true THEN
    locate = LookupLocate(order.participant_id, order.instrument_id)
    IF locate == nil OR locate.status != "CONFIRMED" OR locate.available_qty < order.quantity THEN
      REJECT with reason "SHORT_SELL_LOCATE_REQUIRED: no valid locate for {quantity} shares"
    END IF
    IF instrument.short_sell_allowed == false THEN
      REJECT with reason "SHORT_SELL_RESTRICTED: instrument is on restricted list"
    END IF
  END IF
```

**Uptick Rule (SEC Rule 201 / SSR):**
When a security's price has declined 10% or more from the previous close, short-sell orders may only be executed at a price above the current best bid (uptick). The circuit breaker triggers the restriction for the remainder of the trading day and the next trading day.

```
Validation rule:
  IF instrument.ssr_active == true AND order.is_short_sell == true THEN
    IF order.price <= current_best_bid THEN
      REJECT with reason "UPTICK_RULE: SSR active, short-sell price must be above best bid {best_bid}"
    END IF
  END IF
```

**Short-Sell Restricted List:**
The exchange maintains a list of instruments where short selling is prohibited (e.g., during IPO lock-up, corporate events, or regulatory intervention). Stored in `securities.short_sell_restricted_list`.

### 4.5 Order Enrichment for Securities

Before matching, the gateway enriches securities orders with:

```json
{
  "is_short_sell": true,
  "locate_id": "LOC-20260423-000001",
  "settlement_date": "2026-04-25"
}
```

The `settlement_date` is computed as T+2 using the trading calendar (skipping weekends and holidays). The matching engine does NOT use `settlement_date` for matching — it is passed through to clearing for settlement processing.

---

## 5. Position Limits

### 5.1 Per-Security Per-Participant Limits

Each participant has configurable position limits per instrument:

```
Type: position_limits
Fields:
  participant_id  VARCHAR(64)
  instrument_id   VARCHAR(64)
  max_long_qty    BIGINT       -- Maximum net long position in shares
  max_short_qty   BIGINT       -- Maximum net short position in shares
  max_order_value DECIMAL(18,4) -- Maximum single order value
```

**Pre-trade check:**
```
projected_position = current_net_qty + (order.side == BUY ? order.quantity : -order.quantity)
IF projected_position > max_long_qty THEN REJECT "POSITION_LIMIT_EXCEEDED: long limit {max_long_qty}"
IF projected_position < -max_short_qty THEN REJECT "POSITION_LIMIT_EXCEEDED: short limit {max_short_qty}"
```

### 5.2 Concentration Limits

No single participant may hold more than a configured percentage of a security's total outstanding shares.

```
Default concentration_limit_pct = 5.00 (5% of shares outstanding)

Pre-trade check:
  projected_position_shares = current_net_qty + order.quantity
  concentration_pct = projected_position_shares / instrument.shares_outstanding * 100
  IF concentration_pct > concentration_limit_pct THEN
    REJECT "CONCENTRATION_LIMIT: would hold {concentration_pct}% of outstanding, limit is {concentration_limit_pct}%"
  END IF
```

### 5.3 Large Trader Reporting

When a participant's position in any single security exceeds the reporting threshold, a report event is generated for compliance:

```
Threshold: 100,000 shares OR 1% of shares outstanding (whichever is lower)

Post-trade check (non-blocking):
  IF abs(new_net_qty) >= large_trader_threshold THEN
    Produce event: ace.securities.large-trader-report
  END IF
```

The compliance service consumes this event and generates the regulatory report.

---

## 6. Securities Clearing

### 6.1 CCP Model (Central Counterparty)

Securities trades are cleared through the CCP using novation, identical to the existing commodity clearing model. Upon trade execution:

1. The CCP interposes itself between buyer and seller
2. The original trade becomes two obligations: Buyer-CCP and Seller-CCP
3. Each obligation specifies: instrument, quantity, price, settlement date

The existing `clearing-engine` CCP novation logic (`src/clearing-engine/internal/engine/engine.go`) is reused. The key difference is that securities obligations carry a `settlement_date` (T+2) rather than settling in the current daily cycle.

### 6.2 Bilateral Netting

At the end of each trading day, the clearing engine nets obligations for the same `instrument_id + settlement_date + participant_id` combination:

```
Netting key: (participant_id, instrument_id, settlement_date)

For each netting key:
  net_qty = SUM(buy_qty) - SUM(sell_qty)
  net_value = SUM(buy_value) - SUM(sell_value)

Output: one net settlement obligation per netting key
```

This reduces the number of settlement instructions and the gross exposure.

### 6.3 Exposure Calculation

The CCP calculates exposure per participant:

```
mark_to_market_exposure = SUM over all unsettled obligations:
  (current_market_price - trade_price) * quantity * (side == BUY ? 1 : -1)

replacement_cost = SUM over all unsettled obligations:
  abs(current_market_price - trade_price) * quantity

total_exposure = replacement_cost + potential_future_exposure (VaR-based)
```

### 6.4 Margin Requirements for Securities

Securities margin uses the existing SPAN-like framework from `margin-engine` with securities-specific parameters:

| Parameter | Equities | Bonds | ETFs |
|---|---|---|---|
| Initial margin | 20% of position value | 5% of position value | 15% of position value |
| Maintenance margin | 15% of position value | 3% of position value | 10% of position value |
| Concentration add-on | +5% if > 3% of outstanding | N/A | +3% if > 5% of fund AUM |
| Volatility add-on | SPAN 16-scenario scan | Duration-based | SPAN 16-scenario scan |

---

## 7. T+2 Settlement State Machine

### 7.1 State Diagram

```
                                  [trade executed]
                                        |
                                        v
                                   +---------+
                                   | PENDING |
                                   +----+----+
                                        |
                          [both parties affirm]
                                        |
                                        v
                                  +----------+
                                  | AFFIRMED |
                                  +----+-----+
                                        |
                            [netting run completes]
                                        |
                                        v
                                   +--------+
                                   | NETTED |
                                   +---+----+
                                       |
                         [settlement instruction sent to CSD]
                                       |
                                       v
                                 +------------+
                                 | INSTRUCTED |
                                 +-----+------+
                                       |
                          [CSD confirms transfer started]
                                       |
                                       v
                                  +----------+
                                  | SETTLING |
                                  +----+-----+
                                       |
                            +----------+----------+
                            |                     |
                   [CSD confirms               [CSD reports
                    transfer complete]           failure]
                            |                     |
                            v                     v
                      +---------+            +--------+
                      | SETTLED |            | FAILED |
                      +---------+            +---+----+
                                                 |
                                     [fail management
                                      procedures]
                                                 |
                                         +-------+-------+
                                         |               |
                                  [buy-in at          [resolved
                                   T+4]               manually]
                                         |               |
                                         v               v
                                    +---------+     +---------+
                                    | SETTLED |     | SETTLED |
                                    +---------+     +---------+
```

### 7.2 State Definitions

| State | Description | Entry Condition | Duration |
|---|---|---|---|
| `PENDING` | Trade executed, awaiting affirmation from both counterparties | Trade match event received | T to T+1 |
| `AFFIRMED` | Both buyer and seller (or their custodians) have confirmed trade details | Both parties send affirmation messages | T+1 |
| `NETTED` | Obligations have been netted for the settlement date | End-of-day netting run completes | T+1 evening |
| `INSTRUCTED` | Settlement instruction sent to the CSD for processing | Netting complete + pre-conditions validated | T+2 morning |
| `SETTLING` | CSD has accepted the instruction and is processing the transfer | CSD acknowledgment received | T+2 intraday |
| `SETTLED` | Securities and cash have been exchanged successfully | CSD final confirmation received | T+2 |
| `FAILED` | Settlement failed (insufficient securities, cash, or system error) | CSD failure notification received | T+2 onward |

### 7.3 State Transition Rules

**PENDING -> AFFIRMED:**
```
Conditions:
  - buyer_affirmed == true
  - seller_affirmed == true
  - affirmation received before affirmation_deadline (T+1 16:00 local)

If deadline passes without both affirmations:
  - Status remains PENDING
  - Alert sent to both parties and compliance
  - Auto-affirm if exchange rules permit (configurable per instrument)
```

**AFFIRMED -> NETTED:**
```
Conditions:
  - Netting run has been executed for the settlement_date
  - Net obligation calculated for (participant_id, instrument_id, settlement_date)

Trigger: End-of-day batch process on T+1
```

**NETTED -> INSTRUCTED:**
```
Conditions:
  - Pre-settlement checks passed:
    - Seller has sufficient securities in CSD custody account
    - Buyer has sufficient cash in settlement bank account
    - No regulatory hold on either party
  - Settlement instruction generated and sent to CSD

Trigger: Morning of settlement date (T+2), after pre-settlement validation
```

**INSTRUCTED -> SETTLING:**
```
Conditions:
  - CSD acknowledges receipt of settlement instruction
  - CSD confirms instruction is queued for processing

Trigger: CSD acknowledgment message received
```

**SETTLING -> SETTLED:**
```
Conditions:
  - CSD confirms both legs completed:
    - Securities transferred from seller's to buyer's custody account
    - Cash transferred from buyer's to seller's settlement bank account

Trigger: CSD final settlement confirmation
Post-action: Release margin/collateral held for this obligation
```

**SETTLING -> FAILED:**
```
Conditions (any one triggers failure):
  - Seller's custody account has insufficient securities
  - Buyer's settlement bank account has insufficient funds
  - CSD system error or timeout
  - Regulatory block on transfer

Trigger: CSD failure notification or settlement window timeout
```

### 7.4 Fail Management

**Penalty Interest (CSDR-style):**

When settlement fails, daily penalty interest accrues from the intended settlement date:

```
For securities delivery failure (seller fails):
  penalty_rate = 0.40 bps/day for liquid equities
  penalty_rate = 0.25 bps/day for government bonds
  penalty_rate = 0.50 bps/day for corporate bonds and ETFs

  daily_penalty = failed_value * penalty_rate / 10000

  Penalty accrues each business day the fail persists.
  Penalty paid by failing party to non-failing party.
```

**Buy-In Procedure (after T+4):**

If settlement has not occurred by T+4 (2 business days after intended settlement):

1. Non-failing party (buyer) initiates buy-in notification
2. Failing party (seller) has 2 business days to deliver (grace period to T+6)
3. If still not delivered by T+6, the exchange appoints a buy-in agent
4. Buy-in agent purchases the securities in the open market
5. Price difference between buy-in price and original trade price is charged to the failing seller
6. If buy-in price < original trade price, the difference is returned to the seller (cash compensation)

```
Buy-in cost allocation:
  buy_in_cost = (buy_in_price - original_price) * quantity
  IF buy_in_cost > 0 THEN
    Charge to seller (failed to deliver)
  ELSE
    Credit to seller (market moved in their favor, but cash compensation only)
  END IF
```

**Partial Settlement:**

If the seller can deliver a portion of the owed securities:

```
IF available_qty >= instrument.lot_size AND available_qty < obligation.quantity THEN
  Split obligation:
    settled_obligation: quantity = available_qty, status = SETTLED
    remaining_obligation: quantity = obligation.quantity - available_qty, status = FAILED
  Partial delivery via CSD for settled portion
END IF
```

---

## 8. CSD Integration

### 8.1 Custody Accounts

Each participant has a CSD custody account that holds their securities positions:

```
CSD Account Structure:
  account_id:      UUID
  participant_id:  VARCHAR(64)  -- Links to platform participant
  csd_account_ref: VARCHAR(30)  -- CSD's external account reference
  account_type:    ENUM('PROPRIETARY', 'CLIENT_SEGREGATED', 'COLLATERAL')
  status:          ENUM('ACTIVE', 'FROZEN', 'CLOSED')
  opened_at:       TIMESTAMPTZ
```

Balances per account per instrument:

```
CSD Balance:
  account_id:     UUID
  instrument_id:  VARCHAR(64)
  total_qty:      BIGINT       -- Total holdings
  available_qty:  BIGINT       -- Available for trading/settlement
  pledged_qty:    BIGINT       -- Pledged as collateral
  pending_in_qty: BIGINT       -- Pending receipt from settlement
  pending_out_qty: BIGINT      -- Pending delivery for settlement
```

`available_qty = total_qty - pledged_qty - pending_out_qty`

### 8.2 Securities Transfer Types

**Free of Payment (FoP):**
Securities are transferred without a corresponding cash payment. Used for:
- Collateral pledging/release
- Internal transfers between accounts of the same participant
- Gift/inheritance transfers

```
FoP Transfer:
  transfer_id:    UUID
  from_account_id: UUID
  to_account_id:   UUID
  instrument_id:   VARCHAR(64)
  quantity:         BIGINT
  transfer_type:   ENUM('FOP')
  reason:          VARCHAR(50)  -- COLLATERAL_PLEDGE, COLLATERAL_RELEASE, INTERNAL, OTHER
  status:          ENUM('PENDING', 'COMPLETED', 'REJECTED')
  instructed_at:   TIMESTAMPTZ
  completed_at:    TIMESTAMPTZ
```

**Delivery vs Payment (DvP):**
Securities and cash are exchanged simultaneously (atomic swap). This is the standard settlement mechanism. The CSD ensures that securities are only delivered if cash is received (and vice versa).

```
DvP Transfer:
  transfer_id:     UUID
  settlement_id:   VARCHAR(64)  -- Links to settlement obligation
  seller_account:  UUID          -- CSD account delivering securities
  buyer_account:   UUID          -- CSD account receiving securities
  instrument_id:   VARCHAR(64)
  quantity:         BIGINT
  settlement_value: DECIMAL(18,4) -- Cash amount (including accrued interest for bonds)
  transfer_type:    ENUM('DVP')
  status:           ENUM('PENDING', 'MATCHED', 'COMPLETED', 'FAILED')
  instructed_at:    TIMESTAMPTZ
  matched_at:       TIMESTAMPTZ   -- Both legs matched by CSD
  completed_at:     TIMESTAMPTZ
```

### 8.3 Corporate Actions

Corporate actions are events initiated by the issuer that affect the securities and/or their holders.

**Corporate Action Types:**

| Type | Code | Description | Effect on Holdings |
|---|---|---|---|
| Cash Dividend | `DIVIDEND` | Cash payment per share | No change to qty; cash credited to participant |
| Stock Dividend | `STOCK_DIVIDEND` | Additional shares per share held | Quantity increases by dividend ratio |
| Stock Split | `STOCK_SPLIT` | Shares multiplied by split ratio | Quantity multiplied; price divided by ratio |
| Reverse Split | `REVERSE_SPLIT` | Shares divided by split ratio | Quantity divided; price multiplied by ratio |
| Rights Issue | `RIGHTS_ISSUE` | Right to purchase new shares at discount | Rights instrument created; participant chooses to exercise |
| Merger | `MERGER` | Company acquired; shares converted | Old shares removed, new shares/cash credited |
| Tender Offer | `TENDER_OFFER` | Offer to buy shares at premium | Voluntary; participant elects to tender |
| Spin-Off | `SPIN_OFF` | New company shares distributed | New instrument shares credited proportionally |

**Date Fields:**

```
Corporate Action:
  action_id:         UUID
  instrument_id:     VARCHAR(64)
  action_type:       VARCHAR(30)     -- DIVIDEND, STOCK_SPLIT, etc.
  announcement_date: DATE            -- Date action was announced
  ex_date:           DATE            -- First date shares trade without entitlement
  record_date:       DATE            -- Date holdings are snapshot for entitlement
  payment_date:      DATE            -- Date cash/shares are distributed
  status:            ENUM('ANNOUNCED', 'EX_DATE_PASSED', 'RECORD_DATE_PASSED', 'PROCESSED', 'CANCELLED')
```

**Processing Logic:**

```
DIVIDEND processing:
  ON record_date:
    Snapshot all CSD balances for instrument_id
    FOR each holding:
      entitlement = holding.total_qty * dividend_per_share
      CREATE dividend_payment(participant_id, amount=entitlement, payment_date)
  ON payment_date:
    Execute all dividend_payments (credit cash to settlement accounts)

STOCK_SPLIT processing (e.g., 2:1 split):
  ON ex_date:
    Update instrument: price = price / split_ratio, shares_outstanding *= split_ratio
    Adjust all open orders: price /= split_ratio, quantity *= split_ratio
    Adjust all CSD balances: total_qty *= split_ratio, available_qty *= split_ratio, etc.
    Adjust all position limits: max_long_qty *= split_ratio, max_short_qty *= split_ratio
    Produce event: ace.securities.corporate-action.processed

RIGHTS_ISSUE processing:
  ON announcement_date:
    Create new instrument for the rights (tradeable until exercise deadline)
  ON record_date:
    Credit rights to holders: rights_qty = holding.total_qty * rights_ratio
  ON exercise_deadline:
    For exercised rights: create new shares, debit exercise price
    For unexercised rights: expire worthless
```

---

## 9. Database Schema

### 9.1 V26 — Securities Instruments

```sql
-- V26: Securities instrument reference data
-- Extends the exchange to support equities, bonds, and ETFs

CREATE SCHEMA IF NOT EXISTS securities;

-- Securities instrument master table
CREATE TABLE securities.instruments (
    instrument_id       VARCHAR(64) PRIMARY KEY,
    isin                CHAR(12) NOT NULL UNIQUE,
    cusip               CHAR(9),
    sedol               CHAR(7),
    ticker              VARCHAR(12) NOT NULL,
    exchange_code       CHAR(4) NOT NULL DEFAULT 'MXUB',
    name                VARCHAR(255) NOT NULL,
    asset_class         VARCHAR(10) NOT NULL CHECK (asset_class IN ('EQUITY', 'BOND', 'ETF')),
    security_type       VARCHAR(20) NOT NULL CHECK (security_type IN (
        'COMMON', 'PREFERRED', 'GOVT_BOND', 'CORP_BOND', 'ZERO_COUPON', 'ETF'
    )),
    currency            CHAR(3) NOT NULL DEFAULT 'MNT',
    lot_size            INT NOT NULL DEFAULT 100,
    tick_size           DECIMAL(18,8) NOT NULL DEFAULT 1.0,
    listing_date        DATE NOT NULL,
    trading_status      VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (trading_status IN (
        'ACTIVE', 'HALTED', 'SUSPENDED', 'DELISTED'
    )),

    -- Issuer
    issuer_name         VARCHAR(255),
    issuer_country      CHAR(2) DEFAULT 'MN',
    sector              VARCHAR(100),

    -- Equity-specific
    shares_outstanding  BIGINT DEFAULT 0,
    market_cap          DECIMAL(18,4) DEFAULT 0,

    -- Bond-specific
    par_value           DECIMAL(18,4) DEFAULT 0,
    coupon_rate         DECIMAL(8,4) DEFAULT 0,
    coupon_frequency    VARCHAR(15) DEFAULT 'NONE' CHECK (coupon_frequency IN (
        'NONE', 'ANNUAL', 'SEMI_ANNUAL', 'QUARTERLY', 'MONTHLY'
    )),
    day_count_convention VARCHAR(10) DEFAULT 'ACT/365' CHECK (day_count_convention IN (
        'ACT/360', 'ACT/365', '30/360', 'ACT/ACT'
    )),
    maturity_date       DATE,
    issue_date          DATE,
    next_coupon_date    DATE,

    -- ETF-specific
    nav_per_share       DECIMAL(18,4) DEFAULT 0,
    fund_manager        VARCHAR(255),

    -- Trading controls
    price_band_pct      DECIMAL(5,2) NOT NULL DEFAULT 10.00,
    max_order_qty       BIGINT NOT NULL DEFAULT 10000,
    max_order_value     DECIMAL(18,4) NOT NULL DEFAULT 5000000000,
    short_sell_allowed  BOOLEAN NOT NULL DEFAULT TRUE,
    margin_eligible     BOOLEAN NOT NULL DEFAULT TRUE,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_instruments_isin ON securities.instruments(isin);
CREATE INDEX idx_sec_instruments_ticker ON securities.instruments(ticker);
CREATE INDEX idx_sec_instruments_asset_class ON securities.instruments(asset_class);
CREATE INDEX idx_sec_instruments_security_type ON securities.instruments(security_type);
CREATE INDEX idx_sec_instruments_trading_status ON securities.instruments(trading_status);
CREATE INDEX idx_sec_instruments_exchange_code ON securities.instruments(exchange_code);
CREATE INDEX idx_sec_instruments_maturity ON securities.instruments(maturity_date) WHERE maturity_date IS NOT NULL;

-- Short-sell restricted list
CREATE TABLE securities.short_sell_restricted_list (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    reason              VARCHAR(100) NOT NULL,
    restricted_from     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    restricted_until    TIMESTAMPTZ,
    added_by            VARCHAR(64) NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id, restricted_from)
);

-- Short-sell locate records
CREATE TABLE securities.locates (
    locate_id           VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    requested_qty       BIGINT NOT NULL,
    confirmed_qty       BIGINT NOT NULL DEFAULT 0,
    lender_id           VARCHAR(64),
    status              VARCHAR(15) NOT NULL DEFAULT 'REQUESTED' CHECK (status IN (
        'REQUESTED', 'CONFIRMED', 'DECLINED', 'EXPIRED', 'USED'
    )),
    valid_until         TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at        TIMESTAMPTZ
);

CREATE INDEX idx_locates_participant ON securities.locates(participant_id);
CREATE INDEX idx_locates_instrument ON securities.locates(instrument_id);
CREATE INDEX idx_locates_status ON securities.locates(status);

-- Position limits per security per participant
CREATE TABLE securities.position_limits (
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    max_long_qty        BIGINT NOT NULL DEFAULT 1000000,
    max_short_qty       BIGINT NOT NULL DEFAULT 500000,
    concentration_limit_pct DECIMAL(5,2) NOT NULL DEFAULT 5.00,
    max_order_value     DECIMAL(18,4) NOT NULL DEFAULT 1000000000,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, instrument_id)
);

-- Large trader reporting threshold
CREATE TABLE securities.large_trader_thresholds (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    threshold_qty       BIGINT NOT NULL DEFAULT 100000,
    threshold_pct       DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id)
);

-- SSR (Short-Sale Restriction) trigger tracking
CREATE TABLE securities.ssr_triggers (
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    trigger_date        DATE NOT NULL,
    previous_close      DECIMAL(18,4) NOT NULL,
    trigger_price       DECIMAL(18,4) NOT NULL,
    decline_pct         DECIMAL(5,2) NOT NULL,
    active_until        DATE NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instrument_id, trigger_date)
);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA securities TO garudax_exchange_svc;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT ON securities.instruments TO garudax_clearing_svc;
        GRANT SELECT ON securities.position_limits TO garudax_clearing_svc;
    END IF;
END $$;
```

### 9.2 V27 — Securities Trading

```sql
-- V27: Securities trading tables
-- Orders and trades for securities (extends exchange schema)

-- Securities orders table (parallels exchange.orders for commodities)
CREATE TABLE securities.orders (
    id                  VARCHAR(64) PRIMARY KEY,
    client_order_id     VARCHAR(64),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    account_id          VARCHAR(64) NOT NULL,
    participant_id      VARCHAR(64) NOT NULL,
    side                VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    order_type          VARCHAR(20) NOT NULL CHECK (order_type IN (
        'LIMIT', 'MARKET', 'STOP_LIMIT', 'STOP_MARKET'
    )),
    tif                 VARCHAR(10) NOT NULL CHECK (tif IN ('DAY', 'GTC', 'GTD', 'IOC', 'FOK')),
    price               DECIMAL(18,4),
    stop_price          DECIMAL(18,4),
    quantity            BIGINT NOT NULL,
    filled_qty          BIGINT NOT NULL DEFAULT 0,
    remaining_qty       BIGINT NOT NULL,
    status              VARCHAR(20) NOT NULL CHECK (status IN (
        'NEW', 'PARTIALLY_FILLED', 'FILLED', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    is_short_sell       BOOLEAN NOT NULL DEFAULT FALSE,
    locate_id           VARCHAR(64),
    settlement_date     DATE NOT NULL,
    reject_reason       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_orders_instrument ON securities.orders(instrument_id);
CREATE INDEX idx_sec_orders_account ON securities.orders(account_id);
CREATE INDEX idx_sec_orders_participant ON securities.orders(participant_id);
CREATE INDEX idx_sec_orders_status ON securities.orders(status);
CREATE INDEX idx_sec_orders_settlement ON securities.orders(settlement_date);
CREATE INDEX idx_sec_orders_created ON securities.orders(created_at);

-- Securities trades table (append-only)
CREATE TABLE securities.trades (
    id                  VARCHAR(64) PRIMARY KEY,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    buy_order_id        VARCHAR(64) NOT NULL,
    sell_order_id       VARCHAR(64) NOT NULL,
    buyer_participant_id  VARCHAR(64) NOT NULL,
    seller_participant_id VARCHAR(64) NOT NULL,
    price               DECIMAL(18,4) NOT NULL,
    quantity            BIGINT NOT NULL,
    trade_value         DECIMAL(18,4) NOT NULL,
    accrued_interest    DECIMAL(18,4) NOT NULL DEFAULT 0,
    settlement_value    DECIMAL(18,4) NOT NULL,
    aggressor_side      VARCHAR(4) NOT NULL CHECK (aggressor_side IN ('BUY', 'SELL')),
    is_short_sell       BOOLEAN NOT NULL DEFAULT FALSE,
    settlement_date     DATE NOT NULL,
    traded_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only protection
CREATE RULE no_update_sec_trades AS ON UPDATE TO securities.trades DO INSTEAD NOTHING;
CREATE RULE no_delete_sec_trades AS ON DELETE TO securities.trades DO INSTEAD NOTHING;

CREATE INDEX idx_sec_trades_instrument ON securities.trades(instrument_id);
CREATE INDEX idx_sec_trades_traded_at ON securities.trades(traded_at);
CREATE INDEX idx_sec_trades_buyer ON securities.trades(buyer_participant_id);
CREATE INDEX idx_sec_trades_seller ON securities.trades(seller_participant_id);
CREATE INDEX idx_sec_trades_settlement_date ON securities.trades(settlement_date);

-- Securities execution reports (append-only)
CREATE TABLE securities.execution_reports (
    id                  VARCHAR(64) PRIMARY KEY,
    order_id            VARCHAR(64) NOT NULL,
    trade_id            VARCHAR(64),
    exec_type           VARCHAR(20) NOT NULL CHECK (exec_type IN (
        'NEW', 'PARTIAL_FILL', 'FILL', 'CANCELLED', 'REJECTED', 'EXPIRED'
    )),
    status              VARCHAR(20) NOT NULL,
    side                VARCHAR(4) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    price               DECIMAL(18,4),
    quantity            BIGINT NOT NULL,
    last_qty            BIGINT NOT NULL DEFAULT 0,
    last_price          DECIMAL(18,4),
    cum_qty             BIGINT NOT NULL DEFAULT 0,
    leaves_qty          BIGINT NOT NULL DEFAULT 0,
    reject_reason       TEXT,
    account_id          VARCHAR(64) NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE RULE no_update_sec_exec_reports AS ON UPDATE TO securities.execution_reports DO INSTEAD NOTHING;
CREATE RULE no_delete_sec_exec_reports AS ON DELETE TO securities.execution_reports DO INSTEAD NOTHING;

CREATE INDEX idx_sec_exec_order ON securities.execution_reports(order_id);
CREATE INDEX idx_sec_exec_trade ON securities.execution_reports(trade_id) WHERE trade_id IS NOT NULL;
CREATE INDEX idx_sec_exec_created ON securities.execution_reports(created_at);

-- Securities positions (net position per participant per instrument)
CREATE TABLE securities.positions (
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    net_qty             BIGINT NOT NULL DEFAULT 0,
    avg_price           DECIMAL(18,4) NOT NULL DEFAULT 0,
    market_value        DECIMAL(18,4) NOT NULL DEFAULT 0,
    unrealized_pnl      DECIMAL(18,4) NOT NULL DEFAULT 0,
    realized_pnl        DECIMAL(18,4) NOT NULL DEFAULT 0,
    total_buy_qty       BIGINT NOT NULL DEFAULT 0,
    total_sell_qty      BIGINT NOT NULL DEFAULT 0,
    short_qty           BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, instrument_id)
);

CREATE INDEX idx_sec_positions_instrument ON securities.positions(instrument_id);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN
        GRANT SELECT, INSERT ON securities.orders TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON securities.trades TO garudax_exchange_svc;
        GRANT SELECT, INSERT ON securities.execution_reports TO garudax_exchange_svc;
        GRANT UPDATE (filled_qty, remaining_qty, status, updated_at) ON securities.orders TO garudax_exchange_svc;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON securities.positions TO garudax_clearing_svc;
    END IF;
END $$;
```

### 9.3 V28 — Securities Settlement and CSD

```sql
-- V28: Securities settlement and CSD integration tables

-- Settlement obligations (novated trades pending settlement)
CREATE TABLE securities.settlement_obligations (
    obligation_id       VARCHAR(64) PRIMARY KEY,
    trade_id            VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    participant_id      VARCHAR(64) NOT NULL,
    counterparty_id     VARCHAR(64) NOT NULL DEFAULT 'GARUDAX-CCP',
    side                VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    quantity            BIGINT NOT NULL,
    price               DECIMAL(18,4) NOT NULL,
    settlement_value    DECIMAL(18,4) NOT NULL,
    accrued_interest    DECIMAL(18,4) NOT NULL DEFAULT 0,
    settlement_date     DATE NOT NULL,
    status              VARCHAR(15) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'AFFIRMED', 'NETTED', 'INSTRUCTED', 'SETTLING', 'SETTLED', 'FAILED'
    )),
    netting_run_id      VARCHAR(64),
    csd_instruction_id  VARCHAR(64),
    fail_reason         TEXT,
    penalty_accrued     DECIMAL(18,4) NOT NULL DEFAULT 0,
    buyer_affirmed      BOOLEAN NOT NULL DEFAULT FALSE,
    seller_affirmed     BOOLEAN NOT NULL DEFAULT FALSE,
    affirmed_at         TIMESTAMPTZ,
    netted_at           TIMESTAMPTZ,
    instructed_at       TIMESTAMPTZ,
    settled_at          TIMESTAMPTZ,
    failed_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_obligations_trade ON securities.settlement_obligations(trade_id);
CREATE INDEX idx_sec_obligations_instrument ON securities.settlement_obligations(instrument_id);
CREATE INDEX idx_sec_obligations_participant ON securities.settlement_obligations(participant_id);
CREATE INDEX idx_sec_obligations_settlement_date ON securities.settlement_obligations(settlement_date);
CREATE INDEX idx_sec_obligations_status ON securities.settlement_obligations(status);
CREATE INDEX idx_sec_obligations_netting_run ON securities.settlement_obligations(netting_run_id) WHERE netting_run_id IS NOT NULL;

-- Netting results for securities
CREATE TABLE securities.netting_results (
    id                  VARCHAR(64) PRIMARY KEY,
    run_id              VARCHAR(64) NOT NULL,
    participant_id      VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    settlement_date     DATE NOT NULL,
    net_qty             BIGINT NOT NULL DEFAULT 0,
    net_value           DECIMAL(18,4) NOT NULL DEFAULT 0,
    net_accrued_interest DECIMAL(18,4) NOT NULL DEFAULT 0,
    gross_buy_qty       BIGINT NOT NULL DEFAULT 0,
    gross_sell_qty      BIGINT NOT NULL DEFAULT 0,
    obligations_count   INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_netting_run ON securities.netting_results(run_id);
CREATE INDEX idx_sec_netting_participant ON securities.netting_results(participant_id);
CREATE INDEX idx_sec_netting_settlement_date ON securities.netting_results(settlement_date);

-- CSD custody accounts
CREATE TABLE securities.csd_accounts (
    account_id          VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    csd_account_ref     VARCHAR(30) NOT NULL UNIQUE,
    account_type        VARCHAR(25) NOT NULL DEFAULT 'PROPRIETARY' CHECK (account_type IN (
        'PROPRIETARY', 'CLIENT_SEGREGATED', 'COLLATERAL'
    )),
    status              VARCHAR(10) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN (
        'ACTIVE', 'FROZEN', 'CLOSED'
    )),
    opened_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at           TIMESTAMPTZ
);

CREATE INDEX idx_csd_accounts_participant ON securities.csd_accounts(participant_id);
CREATE INDEX idx_csd_accounts_status ON securities.csd_accounts(status);

-- CSD holdings (balances per account per instrument)
CREATE TABLE securities.csd_balances (
    account_id          VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    total_qty           BIGINT NOT NULL DEFAULT 0,
    available_qty       BIGINT NOT NULL DEFAULT 0,
    pledged_qty         BIGINT NOT NULL DEFAULT 0,
    pending_in_qty      BIGINT NOT NULL DEFAULT 0,
    pending_out_qty     BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, instrument_id),
    CONSTRAINT chk_available CHECK (available_qty >= 0),
    CONSTRAINT chk_total CHECK (total_qty >= 0),
    CONSTRAINT chk_balance CHECK (available_qty = total_qty - pledged_qty - pending_out_qty)
);

CREATE INDEX idx_csd_balances_instrument ON securities.csd_balances(instrument_id);

-- CSD transfers (FoP and DvP)
CREATE TABLE securities.csd_transfers (
    transfer_id         VARCHAR(64) PRIMARY KEY,
    settlement_obligation_id VARCHAR(64),
    from_account_id     VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    to_account_id       VARCHAR(64) NOT NULL REFERENCES securities.csd_accounts(account_id),
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    quantity            BIGINT NOT NULL,
    transfer_type       VARCHAR(5) NOT NULL CHECK (transfer_type IN ('FOP', 'DVP')),
    settlement_value    DECIMAL(18,4) DEFAULT 0,
    reason              VARCHAR(50),
    status              VARCHAR(10) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'MATCHED', 'COMPLETED', 'FAILED', 'REJECTED'
    )),
    fail_reason         TEXT,
    instructed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    matched_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX idx_csd_transfers_obligation ON securities.csd_transfers(settlement_obligation_id) WHERE settlement_obligation_id IS NOT NULL;
CREATE INDEX idx_csd_transfers_from ON securities.csd_transfers(from_account_id);
CREATE INDEX idx_csd_transfers_to ON securities.csd_transfers(to_account_id);
CREATE INDEX idx_csd_transfers_instrument ON securities.csd_transfers(instrument_id);
CREATE INDEX idx_csd_transfers_status ON securities.csd_transfers(status);

-- Settlement fail penalties
CREATE TABLE securities.settlement_penalties (
    penalty_id          VARCHAR(64) PRIMARY KEY,
    obligation_id       VARCHAR(64) NOT NULL REFERENCES securities.settlement_obligations(obligation_id),
    failing_participant_id VARCHAR(64) NOT NULL,
    penalty_date        DATE NOT NULL,
    failed_value        DECIMAL(18,4) NOT NULL,
    penalty_rate_bps    DECIMAL(5,2) NOT NULL,
    penalty_amount      DECIMAL(18,4) NOT NULL,
    status              VARCHAR(10) NOT NULL DEFAULT 'ACCRUED' CHECK (status IN (
        'ACCRUED', 'INVOICED', 'PAID', 'WAIVED'
    )),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sec_penalties_obligation ON securities.settlement_penalties(obligation_id);
CREATE INDEX idx_sec_penalties_participant ON securities.settlement_penalties(failing_participant_id);
CREATE INDEX idx_sec_penalties_status ON securities.settlement_penalties(status);

-- Buy-in records
CREATE TABLE securities.buy_ins (
    buy_in_id           VARCHAR(64) PRIMARY KEY,
    obligation_id       VARCHAR(64) NOT NULL REFERENCES securities.settlement_obligations(obligation_id),
    initiated_by        VARCHAR(64) NOT NULL,
    failing_participant_id VARCHAR(64) NOT NULL,
    instrument_id       VARCHAR(64) NOT NULL,
    original_qty        BIGINT NOT NULL,
    original_price      DECIMAL(18,4) NOT NULL,
    buy_in_qty          BIGINT,
    buy_in_price        DECIMAL(18,4),
    cost_difference     DECIMAL(18,4),
    status              VARCHAR(15) NOT NULL DEFAULT 'NOTIFIED' CHECK (status IN (
        'NOTIFIED', 'GRACE_PERIOD', 'EXECUTING', 'COMPLETED', 'CANCELLED'
    )),
    notification_date   DATE NOT NULL,
    grace_deadline      DATE NOT NULL,
    execution_date      DATE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX idx_buy_ins_obligation ON securities.buy_ins(obligation_id);
CREATE INDEX idx_buy_ins_failing ON securities.buy_ins(failing_participant_id);
CREATE INDEX idx_buy_ins_status ON securities.buy_ins(status);

-- Corporate actions
CREATE TABLE securities.corporate_actions (
    action_id           VARCHAR(64) PRIMARY KEY,
    instrument_id       VARCHAR(64) NOT NULL REFERENCES securities.instruments(instrument_id),
    action_type         VARCHAR(20) NOT NULL CHECK (action_type IN (
        'DIVIDEND', 'STOCK_DIVIDEND', 'STOCK_SPLIT', 'REVERSE_SPLIT',
        'RIGHTS_ISSUE', 'MERGER', 'TENDER_OFFER', 'SPIN_OFF'
    )),
    announcement_date   DATE NOT NULL,
    ex_date             DATE NOT NULL,
    record_date         DATE NOT NULL,
    payment_date        DATE,
    status              VARCHAR(25) NOT NULL DEFAULT 'ANNOUNCED' CHECK (status IN (
        'ANNOUNCED', 'EX_DATE_PASSED', 'RECORD_DATE_PASSED', 'PROCESSED', 'CANCELLED'
    )),

    -- Cash dividend fields
    dividend_per_share  DECIMAL(18,4) DEFAULT 0,
    dividend_currency   CHAR(3) DEFAULT 'MNT',

    -- Split fields
    split_ratio_from    INT DEFAULT 1,
    split_ratio_to      INT DEFAULT 1,

    -- Rights issue fields
    rights_ratio        DECIMAL(8,4) DEFAULT 0,
    exercise_price      DECIMAL(18,4) DEFAULT 0,
    rights_instrument_id VARCHAR(64),

    -- Merger fields
    target_instrument_id VARCHAR(64),
    conversion_ratio    DECIMAL(8,4) DEFAULT 0,
    cash_component      DECIMAL(18,4) DEFAULT 0,

    description         TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_corp_actions_instrument ON securities.corporate_actions(instrument_id);
CREATE INDEX idx_corp_actions_type ON securities.corporate_actions(action_type);
CREATE INDEX idx_corp_actions_ex_date ON securities.corporate_actions(ex_date);
CREATE INDEX idx_corp_actions_record_date ON securities.corporate_actions(record_date);
CREATE INDEX idx_corp_actions_status ON securities.corporate_actions(status);

-- Corporate action entitlements (per participant)
CREATE TABLE securities.corporate_action_entitlements (
    entitlement_id      VARCHAR(64) PRIMARY KEY,
    action_id           VARCHAR(64) NOT NULL REFERENCES securities.corporate_actions(action_id),
    participant_id      VARCHAR(64) NOT NULL,
    csd_account_id      VARCHAR(64) NOT NULL,
    holding_qty         BIGINT NOT NULL,
    entitlement_type    VARCHAR(10) NOT NULL CHECK (entitlement_type IN ('CASH', 'SHARES', 'RIGHTS')),
    cash_amount         DECIMAL(18,4) DEFAULT 0,
    shares_qty          BIGINT DEFAULT 0,
    status              VARCHAR(15) NOT NULL DEFAULT 'PENDING' CHECK (status IN (
        'PENDING', 'PROCESSED', 'PAID', 'FAILED'
    )),
    payment_date        DATE,
    processed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entitlements_action ON securities.corporate_action_entitlements(action_id);
CREATE INDEX idx_entitlements_participant ON securities.corporate_action_entitlements(participant_id);
CREATE INDEX idx_entitlements_status ON securities.corporate_action_entitlements(status);

-- Grant access
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON securities.settlement_obligations TO garudax_clearing_svc;
        GRANT SELECT, INSERT ON securities.netting_results TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.csd_balances TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.csd_transfers TO garudax_clearing_svc;
        GRANT SELECT, INSERT ON securities.settlement_penalties TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.buy_ins TO garudax_clearing_svc;
        GRANT SELECT ON securities.corporate_actions TO garudax_clearing_svc;
        GRANT SELECT, INSERT, UPDATE ON securities.corporate_action_entitlements TO garudax_clearing_svc;
        GRANT SELECT ON securities.csd_accounts TO garudax_clearing_svc;
    END IF;
END $$;
```

---

## 10. API Routes

All securities endpoints use the existing gateway at `/api/v1/` prefix. Authentication and rate limiting are handled by the existing gateway middleware.

### 10.1 Instrument Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/securities/instruments` | public | List securities instruments |
| `GET` | `/api/v1/securities/instruments/{instrument_id}` | public | Get instrument details |
| `POST` | `/api/v1/securities/instruments` | exchange_admin | Create a new instrument |
| `PATCH` | `/api/v1/securities/instruments/{instrument_id}` | exchange_admin | Update instrument attributes |
| `POST` | `/api/v1/securities/instruments/{instrument_id}/halt` | exchange_admin | Halt trading |
| `POST` | `/api/v1/securities/instruments/{instrument_id}/resume` | exchange_admin | Resume trading |

**GET /api/v1/securities/instruments**

Query parameters:
- `asset_class` (optional): `EQUITY`, `BOND`, `ETF`
- `security_type` (optional): `COMMON`, `PREFERRED`, `GOVT_BOND`, `CORP_BOND`, `ZERO_COUPON`, `ETF`
- `trading_status` (optional): `ACTIVE`, `HALTED`, `SUSPENDED`, `DELISTED`
- `exchange_code` (optional): `MXUB`
- `limit` (optional, default 50, max 200): page size
- `cursor` (optional): pagination cursor

Response (200):
```json
{
  "data": [
    {
      "instrument_id": "SEC-EQ-001",
      "isin": "MN0000012345",
      "cusip": null,
      "sedol": null,
      "ticker": "APU.UB",
      "exchange_code": "MXUB",
      "name": "APU JSC Common Stock",
      "asset_class": "EQUITY",
      "security_type": "COMMON",
      "currency": "MNT",
      "lot_size": 100,
      "tick_size": "1.0000",
      "listing_date": "2011-03-15",
      "trading_status": "ACTIVE",
      "issuer_name": "APU JSC",
      "issuer_country": "MN",
      "sector": "Consumer Goods",
      "shares_outstanding": 83542000,
      "market_cap": "1254000000.0000",
      "short_sell_allowed": true,
      "margin_eligible": true,
      "price_band_pct": "10.00",
      "created_at": "2026-04-01T00:00:00Z"
    }
  ],
  "pagination": {
    "next_cursor": "eyJpZCI6IlNFQy1FUS0wMDIifQ",
    "has_more": true
  }
}
```

**GET /api/v1/securities/instruments/{instrument_id}** (for a bond):

Response (200):
```json
{
  "instrument_id": "SEC-BD-001",
  "isin": "MN0000054321",
  "ticker": "MONGOV-2028",
  "exchange_code": "MXUB",
  "name": "Mongolia Government Bond 8.5% 2028",
  "asset_class": "BOND",
  "security_type": "GOVT_BOND",
  "currency": "MNT",
  "lot_size": 10,
  "tick_size": "0.12500000",
  "listing_date": "2023-06-01",
  "trading_status": "ACTIVE",
  "issuer_name": "Government of Mongolia",
  "issuer_country": "MN",
  "par_value": "1000.0000",
  "coupon_rate": "8.5000",
  "coupon_frequency": "SEMI_ANNUAL",
  "day_count_convention": "ACT/365",
  "maturity_date": "2028-06-01",
  "issue_date": "2023-06-01",
  "next_coupon_date": "2026-06-01",
  "accrued_interest": "23.4247",
  "short_sell_allowed": false,
  "margin_eligible": true,
  "price_band_pct": "5.00",
  "created_at": "2023-06-01T00:00:00Z"
}
```

### 10.2 Securities Order Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/v1/securities/orders` | trader | Submit a securities order |
| `GET` | `/api/v1/securities/orders/{order_id}` | trader | Get order details |
| `GET` | `/api/v1/securities/orders` | trader | List open orders |
| `DELETE` | `/api/v1/securities/orders/{order_id}` | trader | Cancel an order |
| `PATCH` | `/api/v1/securities/orders/{order_id}` | trader | Modify price/qty |

**POST /api/v1/securities/orders**

Request:
```json
{
  "client_order_id": "my-sec-order-001",
  "instrument_id": "SEC-EQ-001",
  "side": "buy",
  "order_type": "limit",
  "time_in_force": "day",
  "price": "15000.0000",
  "quantity": 500,
  "is_short_sell": false
}
```

Response (201):
```json
{
  "exec_id": "EXEC-SEC-001",
  "order_id": "ORD-SEC-001",
  "client_order_id": "my-sec-order-001",
  "exec_type": "new",
  "order_status": "new",
  "side": "buy",
  "instrument_id": "SEC-EQ-001",
  "price": "15000.0000",
  "quantity": 500,
  "last_qty": 0,
  "last_price": "0",
  "cumulative_qty": 0,
  "leaves_qty": 500,
  "settlement_date": "2026-04-25",
  "transact_time": "2026-04-23T09:15:00.123Z",
  "account_id": "participant-uuid"
}
```

**Short-sell order request:**
```json
{
  "client_order_id": "my-short-001",
  "instrument_id": "SEC-EQ-001",
  "side": "sell",
  "order_type": "limit",
  "time_in_force": "day",
  "price": "15200.0000",
  "quantity": 200,
  "is_short_sell": true,
  "locate_id": "LOC-20260423-000001"
}
```

### 10.3 Position Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/securities/positions` | trader | List all positions for the authenticated participant |
| `GET` | `/api/v1/securities/positions/{instrument_id}` | trader | Get position for a specific instrument |

**GET /api/v1/securities/positions**

Response (200):
```json
{
  "data": [
    {
      "participant_id": "PART-001",
      "instrument_id": "SEC-EQ-001",
      "ticker": "APU.UB",
      "net_qty": 1500,
      "avg_price": "14800.0000",
      "market_value": "22500000.0000",
      "unrealized_pnl": "300000.0000",
      "realized_pnl": "150000.0000",
      "short_qty": 0,
      "updated_at": "2026-04-23T09:30:00Z"
    }
  ]
}
```

### 10.4 Settlement Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/securities/settlements` | clearing_admin | List settlement obligations |
| `GET` | `/api/v1/securities/settlements/{obligation_id}` | clearing_admin | Get obligation details |
| `POST` | `/api/v1/securities/settlements/{obligation_id}/affirm` | trader | Affirm a settlement obligation |
| `POST` | `/api/v1/securities/settlements/netting` | clearing_admin | Trigger netting run for a settlement date |
| `GET` | `/api/v1/securities/settlements/netting/{run_id}` | clearing_admin | Get netting results |
| `GET` | `/api/v1/securities/settlements/fails` | clearing_admin | List failed settlements |
| `POST` | `/api/v1/securities/settlements/{obligation_id}/buy-in` | clearing_admin | Initiate buy-in |

**GET /api/v1/securities/settlements**

Query parameters:
- `settlement_date` (optional): `2026-04-25`
- `status` (optional): `PENDING`, `AFFIRMED`, `NETTED`, `INSTRUCTED`, `SETTLING`, `SETTLED`, `FAILED`
- `participant_id` (optional)
- `instrument_id` (optional)
- `limit` (optional, default 50)
- `cursor` (optional)

Response (200):
```json
{
  "data": [
    {
      "obligation_id": "OBL-SEC-001",
      "trade_id": "TRD-SEC-001",
      "instrument_id": "SEC-EQ-001",
      "participant_id": "PART-001",
      "counterparty_id": "GARUDAX-CCP",
      "side": "BUY",
      "quantity": 500,
      "price": "15000.0000",
      "settlement_value": "7500000.0000",
      "accrued_interest": "0.0000",
      "settlement_date": "2026-04-25",
      "status": "AFFIRMED",
      "buyer_affirmed": true,
      "seller_affirmed": true,
      "affirmed_at": "2026-04-24T10:00:00Z",
      "created_at": "2026-04-23T09:15:00Z"
    }
  ],
  "pagination": {
    "next_cursor": null,
    "has_more": false
  }
}
```

**POST /api/v1/securities/settlements/{obligation_id}/affirm**

Request:
```json
{
  "affirm": true,
  "notes": "Confirmed by back office"
}
```

Response (200):
```json
{
  "obligation_id": "OBL-SEC-001",
  "status": "AFFIRMED",
  "buyer_affirmed": true,
  "seller_affirmed": true,
  "affirmed_at": "2026-04-24T10:00:00Z"
}
```

### 10.5 Corporate Action Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/securities/corporate-actions` | trader | List upcoming corporate actions |
| `GET` | `/api/v1/securities/corporate-actions/{action_id}` | trader | Get corporate action details |
| `POST` | `/api/v1/securities/corporate-actions` | exchange_admin | Announce a corporate action |
| `PATCH` | `/api/v1/securities/corporate-actions/{action_id}` | exchange_admin | Update corporate action |
| `POST` | `/api/v1/securities/corporate-actions/{action_id}/process` | exchange_admin | Trigger processing |
| `GET` | `/api/v1/securities/corporate-actions/{action_id}/entitlements` | clearing_admin | List entitlements |

**POST /api/v1/securities/corporate-actions**

Request (cash dividend):
```json
{
  "instrument_id": "SEC-EQ-001",
  "action_type": "DIVIDEND",
  "announcement_date": "2026-04-20",
  "ex_date": "2026-05-01",
  "record_date": "2026-05-02",
  "payment_date": "2026-05-15",
  "dividend_per_share": "150.0000",
  "dividend_currency": "MNT",
  "description": "Q1 2026 cash dividend of 150 MNT per share"
}
```

Response (201):
```json
{
  "action_id": "CA-001",
  "instrument_id": "SEC-EQ-001",
  "action_type": "DIVIDEND",
  "status": "ANNOUNCED",
  "announcement_date": "2026-04-20",
  "ex_date": "2026-05-01",
  "record_date": "2026-05-02",
  "payment_date": "2026-05-15",
  "dividend_per_share": "150.0000",
  "created_at": "2026-04-23T10:00:00Z"
}
```

Request (stock split):
```json
{
  "instrument_id": "SEC-EQ-001",
  "action_type": "STOCK_SPLIT",
  "announcement_date": "2026-04-20",
  "ex_date": "2026-05-15",
  "record_date": "2026-05-16",
  "split_ratio_from": 1,
  "split_ratio_to": 2,
  "description": "2-for-1 stock split effective May 15, 2026"
}
```

### 10.6 CSD Account Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/v1/securities/csd/accounts` | trader | List CSD accounts for participant |
| `GET` | `/api/v1/securities/csd/accounts/{account_id}/balances` | trader | Get balances for account |
| `POST` | `/api/v1/securities/csd/transfers` | clearing_admin | Initiate a securities transfer |
| `GET` | `/api/v1/securities/csd/transfers/{transfer_id}` | clearing_admin | Get transfer status |

**GET /api/v1/securities/csd/accounts/{account_id}/balances**

Response (200):
```json
{
  "account_id": "CSD-ACCT-001",
  "csd_account_ref": "MN-CSD-12345-PROP",
  "balances": [
    {
      "instrument_id": "SEC-EQ-001",
      "ticker": "APU.UB",
      "total_qty": 5000,
      "available_qty": 4500,
      "pledged_qty": 300,
      "pending_in_qty": 500,
      "pending_out_qty": 200,
      "updated_at": "2026-04-23T09:30:00Z"
    }
  ]
}
```

---

## 11. Kafka Topics

### 11.1 Topic Catalog

Following the existing naming convention: `ace.{domain}.{event-type}`

| Topic | Partitions | Replication Factor | Retention | Partition Key |
|---|---|---|---|---|
| `ace.securities.order-created` | 16 | 3 | 7 days | `instrument_id` |
| `ace.securities.trade-executed` | 16 | 3 | 7 days | `instrument_id` |
| `ace.securities.settlement-instructed` | 8 | 3 | 7 days | `instrument_id` |
| `ace.securities.settlement-completed` | 8 | 3 | 7 days | `instrument_id` |
| `ace.securities.settlement-failed` | 4 | 3 | 7 days | `participant_id` |
| `ace.securities.corporate-action-announced` | 4 | 3 | 7 days | `instrument_id` |
| `ace.securities.corporate-action-processed` | 4 | 3 | 7 days | `instrument_id` |
| `ace.securities.large-trader-report` | 4 | 3 | 7 days | `participant_id` |
| `ace.securities.ssr-triggered` | 4 | 3 | 7 days | `instrument_id` |
| `ace.dlq.securities.trade-executed` | 4 | 3 | 30 days | (original key) |
| `ace.dlq.securities.settlement-instructed` | 4 | 3 | 30 days | (original key) |
| `ace.dlq.securities.settlement-completed` | 4 | 3 | 30 days | (original key) |

### 11.2 Event Schemas

All events use the existing `ace-event-envelope-v1` envelope from `T051_kafka_event_spec.md`.

**`ace.securities.order-created`**

```json
{
  "id": "evt-uuid-001",
  "type": "ace.securities.order-created",
  "timestamp": "2026-04-23T09:15:00.123Z",
  "source": "matching-engine",
  "correlation_id": "corr-uuid-001",
  "schema_version": 1,
  "payload": {
    "order_id": "ORD-SEC-001",
    "client_order_id": "my-sec-order-001",
    "instrument_id": "SEC-EQ-001",
    "isin": "MN0000012345",
    "participant_id": "PART-001",
    "side": "BUY",
    "order_type": "LIMIT",
    "price": "15000.0000",
    "quantity": 500,
    "time_in_force": "DAY",
    "is_short_sell": false,
    "settlement_date": "2026-04-25",
    "created_at": "2026-04-23T09:15:00.123Z"
  }
}
```

**`ace.securities.trade-executed`**

```json
{
  "id": "evt-uuid-002",
  "type": "ace.securities.trade-executed",
  "timestamp": "2026-04-23T09:15:00.456Z",
  "source": "matching-engine",
  "correlation_id": "corr-uuid-001",
  "schema_version": 1,
  "payload": {
    "trade_id": "TRD-SEC-001",
    "instrument_id": "SEC-EQ-001",
    "isin": "MN0000012345",
    "buy_order_id": "ORD-SEC-001",
    "sell_order_id": "ORD-SEC-002",
    "buyer_participant_id": "PART-001",
    "seller_participant_id": "PART-002",
    "price": "15000.0000",
    "quantity": 500,
    "trade_value": "7500000.0000",
    "accrued_interest": "0.0000",
    "settlement_value": "7500000.0000",
    "aggressor_side": "BUY",
    "is_short_sell": false,
    "settlement_date": "2026-04-25",
    "executed_at": "2026-04-23T09:15:00.456Z"
  }
}
```

**`ace.securities.settlement-instructed`**

```json
{
  "id": "evt-uuid-003",
  "type": "ace.securities.settlement-instructed",
  "timestamp": "2026-04-25T08:00:00.000Z",
  "source": "settlement-engine",
  "correlation_id": "corr-uuid-001",
  "schema_version": 1,
  "payload": {
    "obligation_id": "OBL-SEC-001",
    "trade_id": "TRD-SEC-001",
    "instrument_id": "SEC-EQ-001",
    "participant_id": "PART-001",
    "counterparty_id": "GARUDAX-CCP",
    "side": "BUY",
    "quantity": 500,
    "settlement_value": "7500000.0000",
    "settlement_date": "2026-04-25",
    "csd_instruction_id": "CSD-INSTR-001",
    "netting_run_id": "NET-20260424-001",
    "instructed_at": "2026-04-25T08:00:00.000Z"
  }
}
```

**`ace.securities.settlement-completed`**

```json
{
  "id": "evt-uuid-004",
  "type": "ace.securities.settlement-completed",
  "timestamp": "2026-04-25T14:00:00.000Z",
  "source": "settlement-engine",
  "correlation_id": "corr-uuid-001",
  "schema_version": 1,
  "payload": {
    "obligation_id": "OBL-SEC-001",
    "instrument_id": "SEC-EQ-001",
    "participant_id": "PART-001",
    "side": "BUY",
    "quantity": 500,
    "settlement_value": "7500000.0000",
    "settlement_date": "2026-04-25",
    "transfer_id": "XFER-001",
    "settled_at": "2026-04-25T14:00:00.000Z"
  }
}
```

**`ace.securities.corporate-action-announced`**

```json
{
  "id": "evt-uuid-005",
  "type": "ace.securities.corporate-action-announced",
  "timestamp": "2026-04-23T10:00:00.000Z",
  "source": "settlement-engine",
  "correlation_id": "corr-uuid-005",
  "schema_version": 1,
  "payload": {
    "action_id": "CA-001",
    "instrument_id": "SEC-EQ-001",
    "isin": "MN0000012345",
    "action_type": "DIVIDEND",
    "announcement_date": "2026-04-20",
    "ex_date": "2026-05-01",
    "record_date": "2026-05-02",
    "payment_date": "2026-05-15",
    "dividend_per_share": "150.0000",
    "dividend_currency": "MNT",
    "description": "Q1 2026 cash dividend of 150 MNT per share"
  }
}
```

### 11.3 Event Flow

```
                        +------------------+
                        | matching-engine  |
                        +--------+---------+
                                 |
              ace.securities.order-created (→ compliance for surveillance)
              ace.securities.trade-executed (→ clearing-engine)
                                 |
                                 v
                        +------------------+
              +---------| clearing-engine  |---------+
              |         +------------------+         |
              |                                      |
    ace.clearing.novated                ace.securities.settlement-instructed
    (existing topic, reused)                         |
              |                                      v
              v                             +------------------+
     +----------------+                     | settlement-engine |
     | margin-engine  |                     +--------+---------+
     +----------------+                              |
                                    +----------------+----------------+
                                    |                                 |
                     ace.securities.settlement-completed    ace.securities.settlement-failed
                                    |                                 |
                                    v                                 v
                           +------------------+              +------------------+
                           | clearing-engine  |              | compliance       |
                           | (release margin) |              | (fail monitoring)|
                           +------------------+              +------------------+
```

### 11.4 Consumer Group Mapping

| Consumer Group ID | Topic | Service |
|---|---|---|
| `clearing-engine-ace.securities.trade-executed` | `ace.securities.trade-executed` | clearing-engine |
| `settlement-engine-ace.securities.settlement-instructed` | `ace.securities.settlement-instructed` | settlement-engine |
| `clearing-engine-ace.securities.settlement-completed` | `ace.securities.settlement-completed` | clearing-engine |
| `compliance-ace.securities.settlement-failed` | `ace.securities.settlement-failed` | compliance-service |
| `gateway-ace.securities.corporate-action-announced` | `ace.securities.corporate-action-announced` | gateway |
| `compliance-ace.securities.large-trader-report` | `ace.securities.large-trader-report` | compliance-service |
| `market-data-ace.securities.trade-executed` | `ace.securities.trade-executed` | market-data-service |
| `gateway-ace.securities.trade-executed` | `ace.securities.trade-executed` | gateway (WebSocket push) |

### 11.5 Kafka ACLs

| Principal | Topic Pattern | Operation |
|---|---|---|
| `matching-engine` | `ace.securities.order-created` | WRITE |
| `matching-engine` | `ace.securities.trade-executed` | WRITE |
| `clearing-engine` | `ace.securities.trade-executed` | READ |
| `clearing-engine` | `ace.securities.settlement-completed` | READ |
| `settlement-engine` | `ace.securities.settlement-instructed` | WRITE |
| `settlement-engine` | `ace.securities.settlement-completed` | WRITE |
| `settlement-engine` | `ace.securities.settlement-failed` | WRITE |
| `settlement-engine` | `ace.securities.corporate-action-announced` | WRITE |
| `settlement-engine` | `ace.securities.corporate-action-processed` | WRITE |
| `compliance-service` | `ace.securities.settlement-failed` | READ |
| `compliance-service` | `ace.securities.large-trader-report` | READ |
| `matching-engine` | `ace.securities.ssr-triggered` | WRITE |
| `gateway` | `ace.securities.trade-executed` | READ |
| `gateway` | `ace.securities.corporate-action-announced` | READ |
| `market-data-service` | `ace.securities.trade-executed` | READ |

---

## 12. Integration with Existing Platform

### 12.1 Reused Components (No Changes)

| Component | What's Reused | Notes |
|---|---|---|
| **Gateway** (`src/gateway/`) | HTTP routing, JWT auth, rate limiting, WebSocket, gRPC forwarding | New routes added to existing router for `/api/v1/securities/*` |
| **Auth Service** (`src/auth-service/`) | JWT issuance, RBAC, session management | No new roles needed; existing `trader`, `clearing_admin`, `exchange_admin` cover all securities endpoints |
| **Compliance Service** (`src/compliance-service/`) | KYC/AML, participant screening, risk scoring | Consumes new Kafka topics (`settlement-failed`, `large-trader-report`) for surveillance |
| **Market Data Service** (`src/market-data-service/`) | Candle aggregation, OHLCV, market data distribution | Consumes `ace.securities.trade-executed` for candle building (same pattern as commodity trades) |
| **Admin Bot** (`src/admin-bot/`) | MCP tools, Telegram integration, natural language admin | Extended with new MCP tools for securities instrument management and corporate actions |
| **Admin Dashboard** (`src/admin-ui/`) | React SPA for exchange operators | Extended with securities instrument list, settlement monitoring, corporate action management pages |
| **Trading UI** (`src/web-ui/`) | React SPA for traders | Extended with securities order entry, position view, and settlement status |
| **PostgreSQL** | Database engine, existing schemas | New `securities` schema added (V26-V28); existing `clearing`, `exchange`, `margin` schemas untouched |
| **Kafka** | Event bus, existing broker cluster | New topics added alongside existing commodity topics |
| **Redis** | Rate limiting, session cache | No changes |
| **Docker Compose** | Container orchestration | New service containers for securities-specific workers if needed; mostly reuses existing services |

### 12.2 Extended Components (Modifications Required)

| Component | Change | Scope |
|---|---|---|
| **Matching Engine** (`src/matching-engine/`) | Add securities-aware order validation (lot size, tick size, short-sell checks). The CLOB matching algorithm itself is unchanged — securities orders go through the same price-time priority matching. New order book instances are created per securities instrument. | Validation layer + instrument registry |
| **Clearing Engine** (`src/clearing-engine/`) | Add securities novation (same CCP model) with `settlement_date` tracking. New netting logic groups by `(instrument_id, settlement_date)` instead of just `instrument_id`. | Netting + obligation management |
| **Settlement Engine** (`src/settlement-engine/`) | Add T+2 state machine (PENDING -> AFFIRMED -> ... -> SETTLED). Existing MtM daily settlement for commodities is unaffected — runs on separate settlement cycles. | New state machine + CSD integration |
| **Margin Engine** (`src/margin-engine/`) | Add securities margin parameters (initial/maintenance by asset class). Reuses SPAN scan framework with securities-specific risk arrays. | Parameter tables + risk array extension |

### 12.3 New Components

| Component | Description |
|---|---|
| **CSD Adapter** | Interface to the Central Securities Depository. Abstracts CSD communication (initially in-memory/stub, production adapter via ISO 20022 messages). Handles FoP/DvP transfers, balance queries, and corporate action notifications. |
| **Corporate Action Processor** | Batch processor that executes corporate actions on record dates. Snapshots CSD balances, calculates entitlements, and generates payment/share distribution instructions. Runs as a scheduled job. |
| **Securities Locate Service** | Manages short-sell locate requests and confirmations. In production, integrates with prime brokers or stock lending desks. Initially a simple table-backed service with admin API for manual locate entry. |

### 12.4 Data Flow: Securities Trade Lifecycle

```
1. Trader submits order via Gateway REST API
   POST /api/v1/securities/orders → Gateway validates JWT → forwards to Matching Engine

2. Matching Engine validates order
   - Lot size check: qty % lot_size == 0
   - Tick size check: price % tick_size == 0
   - Short-sell checks: locate exists, SSR not active, not on restricted list
   - Position limit check: projected position within limits
   - Concentration limit check: projected % of outstanding within cap

3. Matching Engine matches order (same CLOB algorithm)
   - Price-time priority matching
   - Generates Trade + ExecutionReports
   - Publishes ace.securities.trade-executed to Kafka

4. Clearing Engine consumes trade
   - Novation: interposes CCP between buyer and seller
   - Creates two settlement obligations with settlement_date = T+2
   - Publishes ace.clearing.novated (existing topic, reused)

5. Margin Engine consumes novation
   - Recalculates margin requirements using securities parameters
   - Issues margin calls if deficit exists
   - Publishes ace.margin.call-issued (existing topic, reused)

6. T+1: Affirmation
   - Both parties affirm trade details via API
   - Obligation status: PENDING → AFFIRMED

7. T+1 EOD: Netting
   - Clearing engine runs netting for settlement_date = T+2
   - Groups by (participant_id, instrument_id, settlement_date)
   - Obligation status: AFFIRMED → NETTED

8. T+2 Morning: Pre-settlement validation
   - Check seller has securities in CSD account
   - Check buyer has cash in settlement bank
   - Generate settlement instructions
   - Obligation status: NETTED → INSTRUCTED
   - Publish ace.securities.settlement-instructed

9. T+2 Intraday: CSD settlement
   - CSD processes DvP transfer
   - Securities and cash exchanged atomically
   - Obligation status: INSTRUCTED → SETTLING → SETTLED
   - Publish ace.securities.settlement-completed

10. Post-settlement: Margin release
    - Clearing engine releases margin held for settled obligations
    - CSD balances updated (pending_in/out → available/total)

11. Fail management (if settlement fails):
    - Obligation status: SETTLING → FAILED
    - Daily penalty interest accrues
    - After T+4: buy-in procedure initiated
    - Publish ace.securities.settlement-failed
```

---

## Appendix A: Seed Data for Development

```sql
-- Sample equity instruments
INSERT INTO securities.instruments (instrument_id, isin, ticker, exchange_code, name, asset_class, security_type, currency, lot_size, tick_size, listing_date, issuer_name, issuer_country, sector, shares_outstanding) VALUES
    ('SEC-EQ-001', 'MN0000012345', 'APU.UB', 'MXUB', 'APU JSC Common Stock', 'EQUITY', 'COMMON', 'MNT', 100, 1.0, '2011-03-15', 'APU JSC', 'MN', 'Consumer Goods', 83542000),
    ('SEC-EQ-002', 'MN0000012346', 'GOV.UB', 'MXUB', 'Govisumber JSC Common Stock', 'EQUITY', 'COMMON', 'MNT', 100, 1.0, '2015-06-01', 'Govisumber JSC', 'MN', 'Mining', 45000000),
    ('SEC-EQ-003', 'MN0000012347', 'TDB.UB', 'MXUB', 'TDB JSC Common Stock', 'EQUITY', 'COMMON', 'MNT', 100, 1.0, '2006-09-20', 'Trade and Development Bank', 'MN', 'Banking', 120000000);

-- Sample bond instruments
INSERT INTO securities.instruments (instrument_id, isin, ticker, exchange_code, name, asset_class, security_type, currency, lot_size, tick_size, listing_date, issuer_name, issuer_country, par_value, coupon_rate, coupon_frequency, day_count_convention, maturity_date, issue_date, next_coupon_date) VALUES
    ('SEC-BD-001', 'MN0000054321', 'MONGOV-2028', 'MXUB', 'Mongolia Government Bond 8.5% 2028', 'BOND', 'GOVT_BOND', 'MNT', 10, 0.125, '2023-06-01', 'Government of Mongolia', 'MN', 1000.0, 8.5, 'SEMI_ANNUAL', 'ACT/365', '2028-06-01', '2023-06-01', '2026-06-01'),
    ('SEC-BD-002', 'MN0000054322', 'DBMN-2027', 'MXUB', 'Development Bank of Mongolia 9.0% 2027', 'BOND', 'CORP_BOND', 'MNT', 10, 0.125, '2024-01-15', 'Development Bank of Mongolia', 'MN', 1000.0, 9.0, 'QUARTERLY', 'ACT/365', '2027-01-15', '2024-01-15', '2026-07-15');

-- Sample ETF instrument
INSERT INTO securities.instruments (instrument_id, isin, ticker, exchange_code, name, asset_class, security_type, currency, lot_size, tick_size, listing_date, issuer_name, issuer_country, nav_per_share, fund_manager) VALUES
    ('SEC-ETF-001', 'MN0000098765', 'MSE20.UB', 'MXUB', 'MSE Top 20 Index ETF', 'ETF', 'ETF', 'MNT', 10, 1.0, '2025-01-02', 'MSE Top 20 ETF Fund', 'MN', 25000.0, 'Ard Financial Group');
```

## Appendix B: Related Documents

- T033: API Gateway Architecture Specification
- T051: Kafka Event Wiring Specification
- V6: Exchange Engine Tables (commodity instruments)
- V11: Matching Engine Tables (commodity orders/trades)
- V12: Clearing Engine Tables (obligations/positions/netting)
- V14: Settlement Engine Tables (cycles/instructions/prices)
- V17: Reference Data Tables (commodity instruments)
- V24: Margin Scenarios (SPAN risk arrays)
