# FIX Protocol Gateway Architecture Specification

**Document ID:** GARUDAX-FIX-GATEWAY-001
**Version:** 1.0
**Date:** 2026-04-24
**Status:** DRAFT
**Author:** Coder Agent (Phase 9)
**Authority:** GarudaX_Strategy_Directive.md, docs/platform-architecture.md

> **GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.**

---

## Table of Contents

1. [Overview](#1-overview)
2. [FIX Protocol](#2-fix-protocol)
3. [Message Types](#3-message-types)
4. [Session Management](#4-session-management)
5. [Broker Onboarding](#5-broker-onboarding)
6. [Order Flow](#6-order-flow)
7. [Market Data](#7-market-data)
8. [Tenant Isolation](#8-tenant-isolation)
9. [Service Architecture](#9-service-architecture)
10. [Database](#10-database)
11. [Appendix](#11-appendix)

---

## 1. Overview

The GarudaX FIX Protocol Gateway enables broker connectivity to GarudaX trading venues via the Financial Information eXchange (FIX) protocol. It provides a standard, industry-recognised interface for institutional participants to submit orders, receive execution reports, and subscribe to market data.

### Design Principles

- **Tenant-isolated FIX sessions.** Each FIX session is scoped to exactly one tenant (venue). A broker connecting to `mse-equities` cannot route messages to `ace-commodities` through that session, even if the broker has accounts on both venues.
- **Thin translation layer.** The FIX gateway translates FIX messages to internal GarudaX API calls. It contains no matching logic, no clearing logic, and no business rules beyond FIX-level validation.
- **Session-level security.** Each session authenticates via password or X.509 certificate at logon. Messages are encrypted in transit via TLS 1.3.
- **Zero-dependency Go module.** Following the established pattern (matching-engine, clearing-engine, gateway), the FIX gateway is a standalone Go service.

### Scope

This spec covers:
- FIX 4.4 session and application message handling
- FIX-to-internal field mapping for orders, executions, and market data
- Broker onboarding and session configuration API
- Tenant-scoped session management with sequence number persistence
- Database schema for sessions, brokers, and message logs

This spec does NOT cover:
- FIX 5.0/FIXT 1.1 (future enhancement)
- Drop-copy sessions (future enhancement)
- Allocation and post-trade messages (future enhancement)
- Backend service implementation (securities-service, matching-engine)

---

## 2. FIX Protocol

### 2.1 Protocol Version

**Primary:** FIX 4.4 (FPL specification, March 2003)

FIX 4.4 is the most widely deployed version across global exchanges and broker-dealers. The gateway implements the full FIX 4.4 session layer and a curated subset of the application layer relevant to order routing and market data.

**Future:** FIX 5.0 SP2 with FIXT 1.1 transport may be added as a separate listener on a different port. The session management and database schema are designed to accommodate both versions.

### 2.2 Transport

| Property | Value |
|---|---|
| Transport protocol | TCP |
| Encryption | TLS 1.3 (mandatory in production, optional in staging/dev) |
| TLS cipher suites | `TLS_AES_256_GCM_SHA384`, `TLS_CHACHA20_POLY1305_SHA256`, `TLS_AES_128_GCM_SHA256` |
| Certificate authority | Platform-operated CA or public CA (per broker config) |
| Client certificate | Optional (required if `auth_method = 'CERTIFICATE'`) |
| Listener port | 9878 (FIX over TLS) |
| Max message size | 8192 bytes (configurable per broker, max 65536) |
| BeginString | `FIX.4.4` |

### 2.3 Session-Level Messages

These messages manage the FIX session lifecycle. The gateway implements all required FIX 4.4 session-level messages:

| MsgType (35) | Name | Direction | Description |
|---|---|---|---|
| `A` | Logon | Bi-directional | Initiates session; carries credentials or cert reference |
| `5` | Logout | Bi-directional | Graceful session termination |
| `0` | Heartbeat | Bi-directional | Keepalive; sent at configured interval |
| `1` | TestRequest | Bi-directional | Forces counterparty to send Heartbeat with TestReqID |
| `2` | ResendRequest | Bi-directional | Requests retransmission of message range |
| `4` | SequenceReset | Bi-directional | Resets expected sequence number (GapFill or Reset mode) |
| `3` | Reject | Gateway -> Broker | Session-level rejection (malformed message, invalid tag) |

### 2.4 Application-Level Messages

| MsgType (35) | Name | Direction | Description |
|---|---|---|---|
| `D` | NewOrderSingle | Broker -> Gateway | Submit a new order |
| `8` | ExecutionReport | Gateway -> Broker | Order acknowledgement, fill, cancel confirm, reject |
| `F` | OrderCancelRequest | Broker -> Gateway | Request to cancel an existing order |
| `9` | OrderCancelReject | Gateway -> Broker | Rejection of a cancel request |
| `G` | OrderCancelReplaceRequest | Broker -> Gateway | Request to modify price/quantity |
| `V` | MarketDataRequest | Broker -> Gateway | Subscribe/unsubscribe to market data |
| `W` | MarketDataSnapshotFullRefresh | Gateway -> Broker | Full order book snapshot |
| `X` | MarketDataIncrementalRefresh | Gateway -> Broker | Incremental book update |
| `Y` | MarketDataRequestReject | Gateway -> Broker | Rejection of market data subscription |
| `j` | BusinessMessageReject | Gateway -> Broker | Application-level rejection |

---

## 3. Message Types

### 3.1 NewOrderSingle (35=D) -- Broker -> Gateway

| FIX Tag | Field Name | Required | Type | Internal Field (`securities.orders`) | Notes |
|---|---|---|---|---|---|
| 11 | ClOrdID | Y | String(64) | `client_order_id` | Unique per session per day |
| 55 | Symbol | Y | String(64) | `instrument_id` (resolved via lookup) | Ticker symbol, e.g., `APU.UB` |
| 48 | SecurityID | N | String(64) | `instrument_id` (resolved via ISIN lookup) | ISIN when SecurityIDSource=4 |
| 22 | SecurityIDSource | N | String(1) | -- | `4`=ISIN, `1`=CUSIP, `2`=SEDOL |
| 54 | Side | Y | Char(1) | `side` | `1`=Buy, `2`=Sell, `5`=Sell Short |
| 38 | OrderQty | Y | Int | `quantity` | In shares (must be lot-size multiple) |
| 40 | OrdType | Y | Char(1) | `order_type` | `1`=Market, `2`=Limit, `3`=Stop, `4`=Stop Limit |
| 44 | Price | C | Decimal(18,4) | `price` | Required for Limit and Stop Limit |
| 99 | StopPx | C | Decimal(18,4) | `stop_price` | Required for Stop and Stop Limit |
| 59 | TimeInForce | Y | Char(1) | `tif` | `0`=Day, `1`=GTC, `3`=IOC, `4`=FOK, `6`=GTD |
| 432 | ExpireDate | C | LocalMktDate | -- | Required when TimeInForce=GTD |
| 1 | Account | Y | String(64) | `account_id` | Broker's account identifier |
| 49 | SenderCompID | Y (header) | String(64) | -- (session-level) | Identifies the broker |
| 56 | TargetCompID | Y (header) | String(64) | -- (session-level) | Always `GARUDAX` |
| 60 | TransactTime | Y | UTCTimestamp | `created_at` | Broker-side timestamp |
| 58 | Text | N | String(256) | -- | Free text, logged but not processed |

**FIX Side (54) to Internal Mapping:**

| FIX Value | FIX Meaning | Internal `side` | Internal `is_short_sell` |
|---|---|---|---|
| `1` | Buy | `BUY` | `false` |
| `2` | Sell | `SELL` | `false` |
| `5` | Sell Short | `SELL` | `true` |
| `6` | Sell Short Exempt | `SELL` | `true` (exempt flag set) |

**FIX OrdType (40) to Internal Mapping:**

| FIX Value | FIX Meaning | Internal `order_type` |
|---|---|---|
| `1` | Market | `MARKET` |
| `2` | Limit | `LIMIT` |
| `3` | Stop | `STOP_MARKET` |
| `4` | Stop Limit | `STOP_LIMIT` |

**FIX TimeInForce (59) to Internal Mapping:**

| FIX Value | FIX Meaning | Internal `tif` |
|---|---|---|
| `0` | Day | `DAY` |
| `1` | Good Till Cancel | `GTC` |
| `3` | Immediate or Cancel | `IOC` |
| `4` | Fill or Kill | `FOK` |
| `6` | Good Till Date | `GTD` |

### 3.2 ExecutionReport (35=8) -- Gateway -> Broker

| FIX Tag | Field Name | Required | Type | Internal Source | Notes |
|---|---|---|---|---|---|
| 37 | OrderID | Y | String(64) | `securities.orders.id` | Exchange-assigned order ID |
| 11 | ClOrdID | Y | String(64) | `securities.orders.client_order_id` | Echo of broker's ClOrdID |
| 17 | ExecID | Y | String(64) | `securities.execution_reports.id` | Unique execution report ID |
| 150 | ExecType | Y | Char(1) | `securities.execution_reports.exec_type` | See mapping below |
| 39 | OrdStatus | Y | Char(1) | `securities.orders.status` | See mapping below |
| 55 | Symbol | Y | String(64) | Resolved from `instrument_id` | Ticker symbol |
| 54 | Side | Y | Char(1) | `securities.orders.side` | Same encoding as NewOrderSingle |
| 151 | LeavesQty | Y | Int | `securities.orders.remaining_qty` | Remaining quantity |
| 14 | CumQty | Y | Int | `securities.orders.filled_qty` | Cumulative filled quantity |
| 6 | AvgPx | Y | Decimal(18,4) | Computed | Volume-weighted average fill price |
| 31 | LastPx | C | Decimal(18,4) | `securities.trades.price` | Price of this fill (when ExecType=F) |
| 32 | LastQty | C | Int | `securities.trades.quantity` | Quantity of this fill (when ExecType=F) |
| 44 | Price | C | Decimal(18,4) | `securities.orders.price` | Order price (for limit orders) |
| 38 | OrderQty | Y | Int | `securities.orders.quantity` | Original order quantity |
| 60 | TransactTime | Y | UTCTimestamp | `securities.execution_reports.created_at` | Exchange timestamp |
| 1 | Account | Y | String(64) | `securities.orders.account_id` | Account echo |
| 58 | Text | N | String(256) | `securities.orders.reject_reason` | Rejection reason text |
| 103 | OrdRejReason | C | Int | -- | Standardised rejection code (when ExecType=8) |
| 30 | LastMkt | C | String(4) | Instrument `exchange_code` | MIC code, e.g., `MXUB` |
| 880 | TrdMatchID | C | String(64) | `securities.trades.id` | Trade ID (when ExecType=F) |

**FIX ExecType (150) Mapping:**

| FIX Value | FIX Meaning | Internal `exec_type` |
|---|---|---|
| `0` | New | `NEW` |
| `4` | Cancelled | `CANCELLED` |
| `5` | Replaced | `REPLACED` |
| `8` | Rejected | `REJECTED` |
| `F` | Trade (partial fill or fill) | `FILL` or `PARTIAL_FILL` |
| `C` | Expired | `EXPIRED` |

**FIX OrdStatus (39) Mapping:**

| FIX Value | FIX Meaning | Internal `status` |
|---|---|---|
| `0` | New | `NEW` |
| `1` | Partially Filled | `PARTIALLY_FILLED` |
| `2` | Filled | `FILLED` |
| `4` | Cancelled | `CANCELLED` |
| `8` | Rejected | `REJECTED` |
| `C` | Expired | `EXPIRED` |

**FIX OrdRejReason (103) Codes:**

| Code | Meaning | When Used |
|---|---|---|
| `0` | Broker/Exchange option | Generic rejection |
| `1` | Unknown symbol | Instrument not found in tenant |
| `2` | Exchange closed | Market phase is CLOSED or PRE_OPEN |
| `3` | Order exceeds limit | Position limit or concentration limit exceeded |
| `5` | Unknown order | Cancel/replace references non-existent order |
| `6` | Duplicate order | ClOrdID already used in this session today |
| `11` | Unsupported order characteristic | Invalid OrdType, TIF, or lot size |
| `13` | Incorrect quantity | Not a lot-size multiple |
| `99` | Other | See Text(58) for details |

### 3.3 OrderCancelRequest (35=F) -- Broker -> Gateway

| FIX Tag | Field Name | Required | Type | Internal Mapping | Notes |
|---|---|---|---|---|---|
| 41 | OrigClOrdID | Y | String(64) | Lookup `client_order_id` -> `id` | Original order's ClOrdID |
| 11 | ClOrdID | Y | String(64) | -- | Unique ID for this cancel request |
| 55 | Symbol | Y | String(64) | -- | Must match original order |
| 54 | Side | Y | Char(1) | -- | Must match original order |
| 60 | TransactTime | Y | UTCTimestamp | -- | Broker-side timestamp |
| 38 | OrderQty | Y | Int | -- | Original order quantity |

**Result:** The gateway responds with an ExecutionReport (35=8) with ExecType=4 (Cancelled) on success, or an OrderCancelReject (35=9) on failure.

### 3.4 OrderCancelReject (35=9) -- Gateway -> Broker

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 37 | OrderID | Y | String(64) | Exchange-assigned order ID |
| 11 | ClOrdID | Y | String(64) | ClOrdID from the cancel request |
| 41 | OrigClOrdID | Y | String(64) | Original order's ClOrdID |
| 39 | OrdStatus | Y | Char(1) | Current status of the order |
| 102 | CxlRejReason | Y | Int | `0`=Too late, `1`=Unknown order, `2`=Broker option, `99`=Other |
| 434 | CxlRejResponseTo | Y | Char(1) | `1`=Cancel request, `2`=Cancel/Replace request |
| 60 | TransactTime | Y | UTCTimestamp | Exchange timestamp |
| 58 | Text | N | String(256) | Human-readable reason |

### 3.5 OrderCancelReplaceRequest (35=G) -- Broker -> Gateway

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 41 | OrigClOrdID | Y | String(64) | Original order's ClOrdID |
| 11 | ClOrdID | Y | String(64) | New ClOrdID for the replacement |
| 55 | Symbol | Y | String(64) | Must match original order |
| 54 | Side | Y | Char(1) | Must match original order |
| 40 | OrdType | Y | Char(1) | Order type (may differ from original) |
| 44 | Price | C | Decimal(18,4) | New price (for limit orders) |
| 38 | OrderQty | Y | Int | New quantity (must be >= filled qty) |
| 59 | TimeInForce | Y | Char(1) | New time in force |
| 60 | TransactTime | Y | UTCTimestamp | Broker-side timestamp |

**Result:** ExecutionReport with ExecType=5 (Replaced) on success, or OrderCancelReject with CxlRejResponseTo=2 on failure.

### 3.6 MarketDataRequest (35=V) -- Broker -> Gateway

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 262 | MDReqID | Y | String(64) | Unique subscription ID |
| 263 | SubscriptionRequestType | Y | Char(1) | `0`=Snapshot, `1`=Snapshot+Updates, `2`=Unsubscribe |
| 264 | MarketDepth | Y | Int | `0`=Full book, `1`=Top of book, N=N levels |
| 267 | NoMDEntryTypes | Y | NumInGroup | Number of entry types requested |
| 269 | MDEntryType | Y | Char(1) | `0`=Bid, `1`=Offer, `2`=Trade, `4`=Opening, `5`=Closing, `7`=Session High, `8`=Session Low |
| 146 | NoRelatedSym | Y | NumInGroup | Number of instruments |
| 55 | Symbol | Y | String(64) | Instrument ticker |
| 265 | MDUpdateType | C | Char(1) | `0`=Full Refresh, `1`=Incremental (when SubscriptionRequestType=1) |

### 3.7 MarketDataSnapshotFullRefresh (35=W) -- Gateway -> Broker

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 262 | MDReqID | Y | String(64) | Echo of subscription ID |
| 55 | Symbol | Y | String(64) | Instrument ticker |
| 268 | NoMDEntries | Y | NumInGroup | Number of entries |
| 269 | MDEntryType | Y | Char(1) | `0`=Bid, `1`=Offer, `2`=Trade |
| 270 | MDEntryPx | Y | Decimal(18,4) | Price |
| 271 | MDEntrySize | Y | Int | Quantity |
| 272 | MDEntryDate | N | UTCDateOnly | Date of entry |
| 273 | MDEntryTime | N | UTCTimeOnly | Time of entry |
| 290 | MDEntryPositionNo | N | Int | Position in book (1-based) |
| 276 | QuoteCondition | N | String | `A`=Open, `B`=Closed, etc. |

### 3.8 MarketDataIncrementalRefresh (35=X) -- Gateway -> Broker

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 262 | MDReqID | Y | String(64) | Echo of subscription ID |
| 268 | NoMDEntries | Y | NumInGroup | Number of update entries |
| 279 | MDUpdateAction | Y | Char(1) | `0`=New, `1`=Change, `2`=Delete |
| 269 | MDEntryType | Y | Char(1) | `0`=Bid, `1`=Offer, `2`=Trade |
| 55 | Symbol | Y | String(64) | Instrument ticker |
| 270 | MDEntryPx | Y | Decimal(18,4) | Updated price |
| 271 | MDEntrySize | Y | Int | Updated quantity |
| 290 | MDEntryPositionNo | N | Int | Position in book |

### 3.9 MarketDataRequestReject (35=Y) -- Gateway -> Broker

| FIX Tag | Field Name | Required | Type | Notes |
|---|---|---|---|---|
| 262 | MDReqID | Y | String(64) | Echo of subscription ID |
| 281 | MDReqRejReason | Y | Char(1) | `0`=Unknown symbol, `1`=Duplicate MDReqID, `4`=Unsupported SubscriptionRequestType, `5`=Unsupported MarketDepth, `8`=Unsupported MDEntryType |
| 58 | Text | N | String(256) | Human-readable reason |

---

## 4. Session Management

### 4.1 Session Identity

Each FIX session is uniquely identified by a composite key:

```
SessionKey = SenderCompID + ":" + TargetCompID + ":" + TenantID
```

**Example session keys:**
- `BROKER_A:GARUDAX:ace-commodities` -- Broker A connected to ACE commodity venue
- `BROKER_A:GARUDAX:mse-equities` -- Broker A connected to MSE equity venue (separate session)
- `BROKER_B:GARUDAX:mse-equities` -- Broker B connected to MSE equity venue

The TargetCompID is always `GARUDAX` for inbound sessions. The SenderCompID is the broker's registered CompID.

### 4.2 Session Lifecycle

```
                           TCP Connect
                               |
                          TLS Handshake
                               |
                      +--------v--------+
                      |   DISCONNECTED  |
                      +--------+--------+
                               |
                    Logon (35=A) received
                               |
                    Validate credentials
                    Resolve tenant from CompID
                               |
                      +--------v--------+
                      |   LOGGED_ON     |
                      +--------+--------+
                         |           |
               Application      Heartbeat/TestRequest
               messages         (keepalive loop)
                         |           |
                    Logout (35=5)
                         or TCP disconnect
                         or error
                               |
                      +--------v--------+
                      |   LOGGED_OUT    |
                      +--------+--------+
                               |
                        Cleanup session
                        Persist seq nums
```

### 4.3 Sequence Number Management

Sequence numbers are persisted per session in the `fix_sessions` table. They survive service restarts and are critical for the FIX gap-fill and resend mechanism.

**Rules:**
- `seq_num_in`: Expected next incoming sequence number from broker. Incremented on each valid message received.
- `seq_num_out`: Next outgoing sequence number to broker. Incremented on each message sent.
- On Logon with `ResetSeqNumFlag(141)=Y`: both sequence numbers reset to 1.
- On incoming message with `MsgSeqNum(34) > seq_num_in`: send ResendRequest(2) for the gap.
- On incoming message with `MsgSeqNum(34) < seq_num_in`: check PossDupFlag(43). If Y, allow (duplicate); if N, disconnect with error.
- Sequence numbers persisted to database on every message (batched writes with 100ms flush interval for performance).

### 4.4 Daily Sequence Reset

Sequence numbers reset daily at a configurable time (default: 00:00 UTC, configurable per broker). The reset is implemented as:

1. At reset time, the gateway sends Logout(5) to all active sessions.
2. After logout confirmation (or 10s timeout), session state is archived.
3. `seq_num_in` and `seq_num_out` are reset to 1 in the database.
4. The gateway waits for the broker to reconnect and re-logon.

Alternatively, brokers can trigger a reset by sending Logon with `ResetSeqNumFlag(141)=Y`.

### 4.5 Logon Authentication

Two authentication methods are supported, configured per broker:

**Method 1: Password Authentication**
```
Logon (35=A):
  554=RawData (password)
  553=Username (optional, defaults to SenderCompID)
  98=EncryptMethod (0=None, required)
  108=HeartBtInt (heartbeat interval in seconds)
```

The gateway validates the password against the broker's stored credential hash (bcrypt). On failure, the gateway sends Logout with Text(58) explaining the failure and disconnects.

**Method 2: X.509 Certificate Authentication**
```
TLS handshake:
  Client presents X.509 certificate
  Gateway validates certificate chain against CA
  Gateway extracts CN (Common Name) as SenderCompID

Logon (35=A):
  98=EncryptMethod (0=None)
  108=HeartBtInt (heartbeat interval in seconds)
  (no password fields required)
```

The gateway verifies the certificate's CN matches a registered broker CompID and that the certificate is not revoked (CRL check).

### 4.6 Heartbeat Configuration

| Parameter | Default | Range | Description |
|---|---|---|---|
| `heartbeat_interval_sec` | 30 | 5-120 | Seconds between heartbeats |
| `heartbeat_tolerance_sec` | 5 | 1-30 | Grace period before declaring lost connection |
| `logon_timeout_sec` | 10 | 5-60 | Seconds to wait for Logon after TCP connect |
| `logout_timeout_sec` | 10 | 5-30 | Seconds to wait for Logout response |

If no message is received within `heartbeat_interval + heartbeat_tolerance` seconds, the gateway sends a TestRequest(1). If no Heartbeat response arrives within another `heartbeat_tolerance` seconds, the session is disconnected.

---

## 5. Broker Onboarding

### 5.1 Registration API

Brokers are registered via the platform control plane REST API. Only users with `platform-admin` or `exchange_admin` role (scoped to the target tenant) can register brokers.

**POST `/platform/v1/fix/brokers`**

Request:
```json
{
  "tenant_id": "mse-equities",
  "comp_id": "BROKER_MSE_001",
  "name": "Mongolia Securities LLC",
  "auth_method": "PASSWORD",
  "password": "initial-password-to-be-changed",
  "config": {
    "heartbeat_interval_sec": 30,
    "max_message_size": 8192,
    "encryption_required": true,
    "daily_reset_time_utc": "00:00",
    "allowed_instruments": ["*"],
    "max_orders_per_second": 100,
    "max_open_orders": 10000,
    "ip_whitelist": ["203.0.113.0/24", "198.51.100.10/32"],
    "allowed_order_types": ["LIMIT", "MARKET", "STOP_LIMIT", "STOP_MARKET"],
    "allowed_sides": ["BUY", "SELL", "SELL_SHORT"],
    "allowed_tifs": ["DAY", "GTC", "IOC", "FOK", "GTD"],
    "market_data_enabled": true,
    "market_data_depth": 10
  }
}
```

Response (201 Created):
```json
{
  "broker_id": "BRK-uuid-001",
  "tenant_id": "mse-equities",
  "comp_id": "BROKER_MSE_001",
  "name": "Mongolia Securities LLC",
  "status": "PENDING",
  "created_at": "2026-04-24T10:00:00Z"
}
```

### 5.2 Broker Lifecycle

```
PENDING ─────→ ACTIVE ─────→ SUSPENDED ─────→ ACTIVE (reactivation)
    |                |               |
    |                |               └──→ DECOMMISSIONED
    |                |
    |                └──→ DECOMMISSIONED
    |
    └──→ DECOMMISSIONED (abandoned)
```

| Status | FIX Sessions | Description |
|---|---|---|
| `PENDING` | Rejected at Logon | Awaiting activation by exchange admin |
| `ACTIVE` | Allowed | Normal operation |
| `SUSPENDED` | Rejected at Logon, existing sessions disconnected | Temporarily disabled |
| `DECOMMISSIONED` | Rejected at Logon | Permanently disabled, sessions archived |

### 5.3 Broker Management Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/platform/v1/fix/brokers` | platform-admin, exchange_admin | Register broker |
| `GET` | `/platform/v1/fix/brokers` | platform-admin, exchange_admin | List brokers (filtered by tenant) |
| `GET` | `/platform/v1/fix/brokers/{broker_id}` | platform-admin, exchange_admin | Get broker details |
| `PATCH` | `/platform/v1/fix/brokers/{broker_id}` | platform-admin, exchange_admin | Update broker config |
| `POST` | `/platform/v1/fix/brokers/{broker_id}/activate` | platform-admin, exchange_admin | PENDING/SUSPENDED -> ACTIVE |
| `POST` | `/platform/v1/fix/brokers/{broker_id}/suspend` | platform-admin, exchange_admin | ACTIVE -> SUSPENDED |
| `POST` | `/platform/v1/fix/brokers/{broker_id}/decommission` | platform-admin, exchange_admin | Any -> DECOMMISSIONED |
| `POST` | `/platform/v1/fix/brokers/{broker_id}/reset-password` | platform-admin | Reset broker password |
| `GET` | `/platform/v1/fix/brokers/{broker_id}/sessions` | platform-admin, exchange_admin | List active/recent sessions |
| `GET` | `/platform/v1/fix/sessions` | platform-admin, exchange_admin | List all sessions (filtered by tenant) |
| `DELETE` | `/platform/v1/fix/sessions/{session_id}` | platform-admin | Force-disconnect a session |

All endpoints require the `X-GarudaX-Tenant` header for tenant context (per `docs/platform-architecture.md` section 4.1).

---

## 6. Order Flow

### 6.1 End-to-End Flow

```
Broker FIX Client                  FIX Gateway                    Securities Service
       |                               |                               |
       |  NewOrderSingle (35=D)        |                               |
       |------------------------------>|                               |
       |                               |  1. Parse FIX message         |
       |                               |  2. Validate SenderCompID     |
       |                               |  3. Resolve tenant from       |
       |                               |     session context           |
       |                               |  4. Resolve instrument        |
       |                               |     (Symbol->instrument_id)   |
       |                               |  5. Map FIX fields to         |
       |                               |     SecurityOrder             |
       |                               |  6. Validate broker           |
       |                               |     permissions               |
       |                               |     (allowed instruments,     |
       |                               |      order types, sides)      |
       |                               |                               |
       |                               |  POST /api/v1/securities/     |
       |                               |       orders                  |
       |                               |------------------------------>|
       |                               |                               |
       |                               |  7. Securities service        |
       |                               |     validates order           |
       |                               |     (lot size, tick size,     |
       |                               |      position limits,         |
       |                               |      short-sell rules)        |
       |                               |                               |
       |                               |  ExecutionReport (NEW or      |
       |                               |  REJECTED)                    |
       |                               |<------------------------------|
       |                               |                               |
       |  ExecutionReport (35=8)       |                               |
       |<------------------------------|                               |
       |                               |                               |
       |  ... time passes ...          |                               |
       |                               |                               |
       |                               |  ExecutionReport (FILL)       |
       |                               |  (via Kafka event:            |
       |                               |   {tenant}.securities.        |
       |                               |   trade-executed)             |
       |                               |<------------------------------|
       |                               |                               |
       |  ExecutionReport (35=8,       |                               |
       |   ExecType=F, fill details)   |                               |
       |<------------------------------|                               |
```

### 6.2 FIX-to-Internal Order Conversion

```go
// ConvertFIXOrder maps a FIX NewOrderSingle to a securities.Order API request
func ConvertFIXOrder(msg *fix44.NewOrderSingle, session *FIXSession) (*SecurityOrderRequest, error) {
    req := &SecurityOrderRequest{
        TenantID:       session.TenantID,
        ClientOrderID:  msg.GetClOrdID(),
        AccountID:      msg.GetAccount(),
        ParticipantID:  session.BrokerParticipantID,
        Side:           mapFIXSide(msg.GetSide()),
        OrderType:      mapFIXOrdType(msg.GetOrdType()),
        TimeInForce:    mapFIXTIF(msg.GetTimeInForce()),
        Quantity:       msg.GetOrderQty(),
        IsShortSell:    msg.GetSide() == "5" || msg.GetSide() == "6",
    }

    // Resolve instrument
    symbol := msg.GetSymbol()
    instrument, err := resolveInstrument(session.TenantID, symbol)
    if err != nil {
        return nil, fmt.Errorf("unknown symbol %s: %w", symbol, err)
    }
    req.InstrumentID = instrument.InstrumentID

    // Price (required for limit orders)
    if msg.HasPrice() {
        req.Price = msg.GetPrice()
    }

    // Stop price
    if msg.HasStopPx() {
        req.StopPrice = msg.GetStopPx()
    }

    // Settlement date computed by securities service (T+2)
    return req, nil
}
```

### 6.3 Execution Report Delivery

The FIX gateway subscribes to Kafka topics for execution events:

| Kafka Topic | FIX Message Generated |
|---|---|
| `{tenant_id}.securities.order-created` | ExecutionReport with ExecType=0 (New) |
| `{tenant_id}.securities.trade-executed` | ExecutionReport with ExecType=F (Fill) |
| `{tenant_id}.securities.order-cancelled` | ExecutionReport with ExecType=4 (Cancelled) |
| `{tenant_id}.securities.order-rejected` | ExecutionReport with ExecType=8 (Rejected) |
| `{tenant_id}.securities.order-expired` | ExecutionReport with ExecType=C (Expired) |

The gateway routes each execution report to the correct FIX session by matching `participant_id` to the broker's registered `participant_id` in the session lookup table.

### 6.4 Order Rejection Scenarios

| Rejection Point | FIX Response | FIX Tag | Details |
|---|---|---|---|
| FIX gateway: malformed message | Reject (35=3) | SessionRejectReason(373) | Missing required tag, invalid tag value |
| FIX gateway: unknown symbol | ExecutionReport, ExecType=8 | OrdRejReason(103)=1 | Symbol not found in tenant |
| FIX gateway: broker not authorised | ExecutionReport, ExecType=8 | OrdRejReason(103)=0 | Instrument not in allowed_instruments |
| FIX gateway: order type not allowed | BusinessMessageReject (35=j) | BusinessRejectReason(380)=3 | OrdType not in allowed_order_types |
| Securities service: lot size | ExecutionReport, ExecType=8 | OrdRejReason(103)=13 | Quantity not a lot-size multiple |
| Securities service: tick size | ExecutionReport, ExecType=8 | OrdRejReason(103)=11 | Price not a tick-size multiple |
| Securities service: position limit | ExecutionReport, ExecType=8 | OrdRejReason(103)=3 | Would exceed position or concentration limit |
| Securities service: short-sell locate | ExecutionReport, ExecType=8 | OrdRejReason(103)=99 | No valid locate; Text(58) has details |
| Securities service: SSR/uptick rule | ExecutionReport, ExecType=8 | OrdRejReason(103)=99 | SSR active, price must be above best bid |
| Securities service: market closed | ExecutionReport, ExecType=8 | OrdRejReason(103)=2 | Market phase does not accept orders |

---

## 7. Market Data

### 7.1 Subscription Flow

```
Broker                         FIX Gateway                    Market Data Service
  |                                |                                |
  | MarketDataRequest (35=V)       |                                |
  | SubscriptionRequestType=1      |                                |
  | (Snapshot + Updates)           |                                |
  |------------------------------->|                                |
  |                                |  1. Validate symbol exists     |
  |                                |     in tenant                  |
  |                                |  2. Check broker has           |
  |                                |     market_data_enabled        |
  |                                |  3. Register subscription      |
  |                                |                                |
  |                                |  GET /api/v1/instruments/      |
  |                                |      {id}/book?depth=N         |
  |                                |------------------------------->|
  |                                |                                |
  |                                |  Order book snapshot           |
  |                                |<-------------------------------|
  |                                |                                |
  | MarketDataSnapshot-            |                                |
  | FullRefresh (35=W)             |                                |
  |<-------------------------------|                                |
  |                                |                                |
  |                                |  Subscribe to Kafka:           |
  |                                |  {tenant}.market-data.         |
  |                                |  trade-ingested                |
  |                                |  {tenant}.securities.          |
  |                                |  order-created                 |
  |                                |  (for book updates)            |
  |                                |                                |
  | MarketDataIncremental-         |  (on each book change)         |
  | Refresh (35=X)                 |                                |
  |<-------------------------------|                                |
  |                                |                                |
  | MarketDataRequest (35=V)       |                                |
  | SubscriptionRequestType=2      |                                |
  | (Unsubscribe)                  |                                |
  |------------------------------->|                                |
  |                                |  Remove subscription           |
  |                                |  Stop streaming                |
```

### 7.2 Subscription Limits

| Parameter | Default | Configurable | Description |
|---|---|---|---|
| `max_subscriptions_per_session` | 100 | Per broker config | Max concurrent market data subscriptions |
| `market_data_depth` | 10 | Per broker config | Default book depth |
| `market_data_max_depth` | 50 | Platform config | Maximum allowed book depth |
| `market_data_throttle_ms` | 100 | Per broker config | Minimum interval between incremental refreshes |

### 7.3 Market Data Entry Types

| MDEntryType (269) | Internal Source | Description |
|---|---|---|
| `0` (Bid) | Order book bid side | Best bid prices and sizes |
| `1` (Offer) | Order book ask side | Best offer prices and sizes |
| `2` (Trade) | Last trade | Most recent trade price and size |
| `4` (Opening Price) | Market data service | Opening auction price |
| `5` (Closing Price) | Market data service | Closing auction price |
| `7` (Session High) | Market data service | Highest trade price in session |
| `8` (Session Low) | Market data service | Lowest trade price in session |
| `B` (Trade Volume) | Market data service | Cumulative volume traded |

---

## 8. Tenant Isolation

### 8.1 Session-to-Tenant Binding

Every FIX session is bound to exactly one tenant at logon time. The binding is determined by the broker's registration:

```go
// Session resolution during Logon
func (gw *FIXGateway) OnLogon(msg *fix44.Logon) error {
    senderCompID := msg.Header.GetSenderCompID()

    // Look up broker by CompID
    broker, err := gw.brokerStore.GetByCompID(senderCompID)
    if err != nil {
        return fmt.Errorf("unknown SenderCompID: %s", senderCompID)
    }

    if broker.Status != "ACTIVE" {
        return fmt.Errorf("broker %s is %s", senderCompID, broker.Status)
    }

    // Session is now scoped to broker.TenantID
    session := &FIXSession{
        SessionID:    fmt.Sprintf("%s:GARUDAX:%s", senderCompID, broker.TenantID),
        SenderCompID: senderCompID,
        TargetCompID: "GARUDAX",
        TenantID:     broker.TenantID,
        BrokerID:     broker.ID,
        Status:       "LOGGED_ON",
    }

    gw.sessions.Store(session.SessionID, session)
    return nil
}
```

### 8.2 Cross-Tenant Rejection

The gateway enforces tenant isolation at multiple layers:

1. **Session layer:** A session's `TenantID` is immutable after logon. All messages on that session are processed in the context of that tenant.
2. **Instrument layer:** Symbol resolution (`Symbol(55)` -> `instrument_id`) only searches instruments within the session's tenant. A broker connected to `mse-equities` cannot reference an `ace-commodities` instrument.
3. **Kafka layer:** The gateway subscribes to execution events only on `{session.TenantID}.securities.*` topics. Events from other tenants are never consumed.
4. **API layer:** All calls to the securities-service API include the `X-GarudaX-Tenant` header with the session's tenant ID.

### 8.3 Multi-Tenant Broker Support

A broker that operates on multiple venues registers separately for each tenant and receives a separate CompID per tenant:

| Broker | Tenant | CompID | Session Key |
|---|---|---|---|
| Mongolia Securities LLC | `mse-equities` | `MONGOL_SEC_MSE` | `MONGOL_SEC_MSE:GARUDAX:mse-equities` |
| Mongolia Securities LLC | `ace-commodities` | `MONGOL_SEC_ACE` | `MONGOL_SEC_ACE:GARUDAX:ace-commodities` |

Each CompID is a separate FIX session with its own sequence numbers, heartbeat settings, and allowed instruments. There is no cross-session state sharing.

### 8.4 Observability

All FIX gateway metrics, logs, and traces carry `tenant_id` as a required label (per `docs/platform-architecture.md` section 4.5):

```go
// Prometheus metrics
fixMessageCounter.WithLabelValues(session.TenantID, msgType, direction).Inc()
fixSessionGauge.WithLabelValues(session.TenantID, status).Set(count)
fixOrderLatency.WithLabelValues(session.TenantID).Observe(duration.Seconds())

// Structured logging
logger.Info("FIX message received",
    "tenant_id", session.TenantID,
    "session_id", session.SessionID,
    "msg_type", msgType,
    "seq_num", seqNum,
)
```

---

## 9. Service Architecture

### 9.1 Service Overview

The FIX gateway is a standalone Go service following the zero-dependency module pattern established by matching-engine, clearing-engine, and other platform services.

| Property | Value |
|---|---|
| Service name | `fix-gateway` |
| Language | Go |
| Source path | `src/fix-gateway/` |
| FIX TCP port | 9878 |
| Admin HTTP port | 8091 |
| Health HTTP port | 9091 |
| gRPC port | N/A (FIX is TCP-native, not gRPC) |
| Docker base image | `golang:1.22-alpine` (build), `alpine:3.19` (runtime) |

### 9.2 Internal Architecture

```
src/fix-gateway/
  cmd/
    fix-gateway/
      main.go                    -- Entry point, config loading, server startup
  internal/
    config/
      config.go                  -- Configuration struct (env vars, defaults)
    session/
      manager.go                 -- Session lifecycle (create, logon, logout, cleanup)
      session.go                 -- Session state (seq nums, heartbeat timer)
      store.go                   -- Session persistence interface
    fix/
      parser.go                  -- FIX message parser (tag=value|SOH format)
      builder.go                 -- FIX message builder
      types.go                   -- FIX message type constants
      checksum.go                -- FIX checksum (tag 10) computation
      validate.go                -- Required tag validation per message type
    handler/
      logon.go                   -- Logon handler
      order.go                   -- NewOrderSingle, CancelRequest, CancelReplace
      execution.go               -- ExecutionReport generation
      marketdata.go              -- MarketDataRequest, snapshot, incremental
      admin.go                   -- Session-level handlers (heartbeat, test, resend)
    broker/
      store.go                   -- Broker registry (DB-backed)
      service.go                 -- Broker CRUD operations
    instrument/
      resolver.go                -- Symbol -> instrument_id resolution (tenant-scoped)
    tenant/
      context.go                 -- Tenant context from session
    server/
      tcp.go                     -- TCP listener with TLS
      http.go                    -- Admin HTTP server (broker management, health)
    kafka/
      consumer.go                -- Kafka consumer for execution events
      producer.go                -- Kafka producer for audit events
    store/
      postgres.go                -- PostgreSQL store implementation
  Dockerfile
  go.mod
  go.sum
```

### 9.3 Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `FIX_LISTEN_ADDR` | N | `:9878` | FIX TCP listen address |
| `FIX_ADMIN_ADDR` | N | `:8091` | Admin HTTP listen address |
| `FIX_HEALTH_ADDR` | N | `:9091` | Health check listen address |
| `FIX_TLS_CERT_FILE` | Y (prod) | -- | TLS certificate file path |
| `FIX_TLS_KEY_FILE` | Y (prod) | -- | TLS private key file path |
| `FIX_TLS_CA_FILE` | N | -- | CA certificate for client cert validation |
| `FIX_TLS_REQUIRED` | N | `true` | Whether TLS is mandatory |
| `DATABASE_URL` | Y | -- | PostgreSQL connection string |
| `KAFKA_BROKERS` | Y | -- | Kafka broker list |
| `GARUDAX_TENANT_HMAC_KEY` | Y | -- | HMAC key for tenant header signing |
| `SECURITIES_SERVICE_URL` | Y | -- | Securities service base URL |
| `MARKET_DATA_SERVICE_URL` | Y | -- | Market data service base URL |
| `FIX_TARGET_COMP_ID` | N | `GARUDAX` | TargetCompID for this gateway |
| `FIX_SEQ_FLUSH_INTERVAL_MS` | N | `100` | Sequence number DB flush interval |
| `FIX_MAX_SESSIONS` | N | `1000` | Maximum concurrent FIX sessions |
| `LOG_LEVEL` | N | `info` | Log level (debug, info, warn, error) |

### 9.4 Docker Configuration

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /fix-gateway ./cmd/fix-gateway

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /fix-gateway /usr/local/bin/fix-gateway
EXPOSE 9878 8091 9091
ENTRYPOINT ["/usr/local/bin/fix-gateway"]
```

### 9.5 Docker Compose Entry

```yaml
fix-gateway:
  build:
    context: ./src/fix-gateway
    dockerfile: Dockerfile
  ports:
    - "9878:9878"    # FIX TCP (TLS)
    - "8091:8091"    # Admin HTTP
    - "9091:9091"    # Health
  environment:
    FIX_LISTEN_ADDR: ":9878"
    FIX_ADMIN_ADDR: ":8091"
    FIX_HEALTH_ADDR: ":9091"
    FIX_TLS_REQUIRED: "false"
    DATABASE_URL: "postgres://ace_user:ace_pass@postgres:5432/ace_platform?sslmode=disable"
    KAFKA_BROKERS: "kafka:9092"
    SECURITIES_SERVICE_URL: "http://gateway:8080"
    MARKET_DATA_SERVICE_URL: "http://gateway:8080"
    FIX_TARGET_COMP_ID: "GARUDAX"
    LOG_LEVEL: "debug"
  depends_on:
    - postgres
    - kafka
    - gateway
  networks:
    - ace-network
```

---

## 10. Database

### 10.1 Migration: V31__fix_gateway.sql

```sql
-- V31: FIX Protocol Gateway tables
-- Supports multi-tenant FIX session management, broker registry, and message logging

-- ============================================================
-- fix_brokers: Registered FIX broker-dealers per tenant
-- ============================================================
CREATE TABLE platform.fix_brokers (
    id                  VARCHAR(64) PRIMARY KEY,           -- UUID v7
    tenant_id           VARCHAR(64) NOT NULL
                        REFERENCES platform.tenants(tenant_id),
    comp_id             VARCHAR(64) NOT NULL,              -- FIX SenderCompID
    name                VARCHAR(255) NOT NULL,             -- Human-readable broker name
    participant_id      VARCHAR(64),                       -- Linked exchange participant ID
    status              VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'DECOMMISSIONED')),
    auth_method         VARCHAR(20) NOT NULL DEFAULT 'PASSWORD'
                        CHECK (auth_method IN ('PASSWORD', 'CERTIFICATE')),
    password_hash       VARCHAR(255),                      -- bcrypt hash (for PASSWORD auth)
    cert_cn             VARCHAR(255),                      -- X.509 CN (for CERTIFICATE auth)
    cert_serial         VARCHAR(255),                      -- X.509 serial number
    config              JSONB NOT NULL DEFAULT '{}'::JSONB, -- Broker-specific configuration
    -- config schema:
    -- {
    --   "heartbeat_interval_sec": 30,
    --   "max_message_size": 8192,
    --   "encryption_required": true,
    --   "daily_reset_time_utc": "00:00",
    --   "allowed_instruments": ["*"],
    --   "max_orders_per_second": 100,
    --   "max_open_orders": 10000,
    --   "ip_whitelist": ["203.0.113.0/24"],
    --   "allowed_order_types": ["LIMIT", "MARKET", "STOP_LIMIT", "STOP_MARKET"],
    --   "allowed_sides": ["BUY", "SELL", "SELL_SHORT"],
    --   "allowed_tifs": ["DAY", "GTC", "IOC", "FOK", "GTD"],
    --   "market_data_enabled": true,
    --   "market_data_depth": 10,
    --   "max_subscriptions": 100,
    --   "market_data_throttle_ms": 100
    -- }
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at        TIMESTAMPTZ,
    suspended_at        TIMESTAMPTZ,
    decommissioned_at   TIMESTAMPTZ
);

-- CompID must be unique within a tenant
CREATE UNIQUE INDEX idx_fix_brokers_tenant_comp ON platform.fix_brokers (tenant_id, comp_id);
CREATE INDEX idx_fix_brokers_tenant ON platform.fix_brokers (tenant_id);
CREATE INDEX idx_fix_brokers_status ON platform.fix_brokers (status);
CREATE INDEX idx_fix_brokers_comp ON platform.fix_brokers (comp_id);

-- ============================================================
-- fix_sessions: Active and historical FIX sessions
-- ============================================================
CREATE TABLE platform.fix_sessions (
    id                  VARCHAR(64) PRIMARY KEY,           -- UUID v7
    tenant_id           VARCHAR(64) NOT NULL
                        REFERENCES platform.tenants(tenant_id),
    broker_id           VARCHAR(64) NOT NULL
                        REFERENCES platform.fix_brokers(id),
    sender_comp_id      VARCHAR(64) NOT NULL,              -- Broker's SenderCompID
    target_comp_id      VARCHAR(64) NOT NULL DEFAULT 'GARUDAX',
    session_key         VARCHAR(200) NOT NULL,             -- "{sender}:{target}:{tenant}" composite
    status              VARCHAR(20) NOT NULL DEFAULT 'DISCONNECTED'
                        CHECK (status IN ('DISCONNECTED', 'LOGGED_ON', 'LOGGED_OUT', 'ERROR')),
    seq_num_in          BIGINT NOT NULL DEFAULT 1,         -- Next expected inbound sequence number
    seq_num_out         BIGINT NOT NULL DEFAULT 1,         -- Next outbound sequence number
    last_heartbeat_at   TIMESTAMPTZ,                       -- Last heartbeat received
    logon_at            TIMESTAMPTZ,                       -- Most recent logon time
    logout_at           TIMESTAMPTZ,                       -- Most recent logout time
    disconnect_reason   TEXT,                              -- Reason for last disconnect
    remote_address      INET,                              -- Broker's IP address
    fix_version         VARCHAR(10) NOT NULL DEFAULT 'FIX.4.4',
    heartbeat_interval  INT NOT NULL DEFAULT 30,           -- Negotiated heartbeat interval (seconds)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_fix_sessions_key ON platform.fix_sessions (session_key)
    WHERE status = 'LOGGED_ON';  -- Only one active session per key
CREATE INDEX idx_fix_sessions_tenant ON platform.fix_sessions (tenant_id);
CREATE INDEX idx_fix_sessions_broker ON platform.fix_sessions (broker_id);
CREATE INDEX idx_fix_sessions_status ON platform.fix_sessions (status);
CREATE INDEX idx_fix_sessions_logon ON platform.fix_sessions (logon_at);

-- ============================================================
-- fix_message_log: Append-only log of all FIX messages
-- ============================================================
CREATE TABLE platform.fix_message_log (
    id                  BIGSERIAL PRIMARY KEY,             -- Auto-increment for fast inserts
    session_id          VARCHAR(64) NOT NULL
                        REFERENCES platform.fix_sessions(id),
    tenant_id           VARCHAR(64) NOT NULL,              -- Denormalised for query performance
    direction           VARCHAR(3) NOT NULL
                        CHECK (direction IN ('IN', 'OUT')),
    msg_type            VARCHAR(5) NOT NULL,               -- FIX MsgType(35) value
    msg_seq_num         BIGINT NOT NULL,                   -- FIX MsgSeqNum(34)
    sender_comp_id      VARCHAR(64) NOT NULL,
    target_comp_id      VARCHAR(64) NOT NULL,
    body                TEXT NOT NULL,                     -- Raw FIX message (SOH replaced with |)
    body_length         INT NOT NULL,                      -- Original message length in bytes
    checksum            VARCHAR(3) NOT NULL,               -- FIX checksum (tag 10)
    processing_time_us  INT,                               -- Processing latency in microseconds
    error_text          TEXT,                               -- Error details if message was rejected
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only protection
CREATE RULE no_update_fix_log AS ON UPDATE TO platform.fix_message_log DO INSTEAD NOTHING;
CREATE RULE no_delete_fix_log AS ON DELETE TO platform.fix_message_log DO INSTEAD NOTHING;

CREATE INDEX idx_fix_log_session ON platform.fix_message_log (session_id);
CREATE INDEX idx_fix_log_tenant ON platform.fix_message_log (tenant_id);
CREATE INDEX idx_fix_log_created ON platform.fix_message_log (created_at);
CREATE INDEX idx_fix_log_type ON platform.fix_message_log (msg_type);
CREATE INDEX idx_fix_log_direction ON platform.fix_message_log (direction);

-- Partitioning by month for message log (high-volume table)
-- Production should use declarative partitioning; this is the base table definition.
-- Example partition:
-- CREATE TABLE platform.fix_message_log_2026_04 PARTITION OF platform.fix_message_log
--     FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');

-- ============================================================
-- fix_sequence_archive: Daily sequence number archive
-- ============================================================
CREATE TABLE platform.fix_sequence_archive (
    id                  BIGSERIAL PRIMARY KEY,
    session_key         VARCHAR(200) NOT NULL,
    tenant_id           VARCHAR(64) NOT NULL,
    archive_date        DATE NOT NULL,
    final_seq_num_in    BIGINT NOT NULL,
    final_seq_num_out   BIGINT NOT NULL,
    total_messages_in   BIGINT NOT NULL DEFAULT 0,
    total_messages_out  BIGINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_fix_seq_archive_key_date
    ON platform.fix_sequence_archive (session_key, archive_date);
CREATE INDEX idx_fix_seq_archive_tenant ON platform.fix_sequence_archive (tenant_id);

-- ============================================================
-- Grants
-- ============================================================
DO $$
BEGIN
    -- FIX gateway service role
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_platform_svc') THEN
        GRANT SELECT, INSERT, UPDATE ON platform.fix_brokers TO garudax_platform_svc;
        GRANT SELECT, INSERT, UPDATE ON platform.fix_sessions TO garudax_platform_svc;
        GRANT SELECT, INSERT ON platform.fix_message_log TO garudax_platform_svc;
        GRANT SELECT, INSERT ON platform.fix_sequence_archive TO garudax_platform_svc;
        GRANT USAGE, SELECT ON SEQUENCE platform.fix_message_log_id_seq TO garudax_platform_svc;
        GRANT USAGE, SELECT ON SEQUENCE platform.fix_sequence_archive_id_seq TO garudax_platform_svc;
    END IF;

    -- Platform admin (read-only on FIX tables)
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'garudax_platform_admin') THEN
        GRANT SELECT ON platform.fix_brokers TO garudax_platform_admin;
        GRANT SELECT ON platform.fix_sessions TO garudax_platform_admin;
        GRANT SELECT ON platform.fix_message_log TO garudax_platform_admin;
        GRANT SELECT ON platform.fix_sequence_archive TO garudax_platform_admin;
    END IF;
END $$;
```

### 10.2 Message Log Retention

| Environment | Raw Log Retention | Sequence Archive Retention |
|---|---|---|
| Production | 90 days (then archived to S3) | 7 years (regulatory requirement) |
| Staging | 30 days | 1 year |
| Development | 7 days | 30 days |

Archival is handled by a scheduled job that exports `fix_message_log` rows older than the retention period to compressed Parquet files on S3:

```
s3://garudax-{tenant_id}-fix-archive/{year}/{month}/fix_messages_{date}.parquet
```

---

## 11. Appendix

### A.1 Sample FIX Messages

All examples use `|` as the SOH delimiter for readability. In production, the delimiter is ASCII 0x01 (SOH).

**Logon (35=A) -- Broker to Gateway:**
```
8=FIX.4.4|9=126|35=A|49=BROKER_MSE_001|56=GARUDAX|34=1|52=20260424-10:00:00.000|
98=0|108=30|553=BROKER_MSE_001|554=broker-password-123|141=Y|10=185|
```

| Tag | Value | Description |
|---|---|---|
| 8 | FIX.4.4 | BeginString |
| 9 | 126 | BodyLength |
| 35 | A | MsgType (Logon) |
| 49 | BROKER_MSE_001 | SenderCompID |
| 56 | GARUDAX | TargetCompID |
| 34 | 1 | MsgSeqNum |
| 52 | 20260424-10:00:00.000 | SendingTime |
| 98 | 0 | EncryptMethod (None) |
| 108 | 30 | HeartBtInt (30 seconds) |
| 553 | BROKER_MSE_001 | Username |
| 554 | broker-password-123 | Password |
| 141 | Y | ResetSeqNumFlag |
| 10 | 185 | CheckSum |

**Logon Response (35=A) -- Gateway to Broker:**
```
8=FIX.4.4|9=72|35=A|49=GARUDAX|56=BROKER_MSE_001|34=1|52=20260424-10:00:00.001|
98=0|108=30|141=Y|10=203|
```

**NewOrderSingle (35=D) -- Buy 500 shares of APU at limit 15000 MNT:**
```
8=FIX.4.4|9=168|35=D|49=BROKER_MSE_001|56=GARUDAX|34=2|52=20260424-10:01:00.000|
11=ORD-20260424-001|1=ACCT-001|55=APU.UB|54=1|38=500|40=2|44=15000.0000|
59=0|60=20260424-10:01:00.000|10=142|
```

| Tag | Value | Description |
|---|---|---|
| 11 | ORD-20260424-001 | ClOrdID (broker-assigned) |
| 1 | ACCT-001 | Account |
| 55 | APU.UB | Symbol (ticker) |
| 54 | 1 | Side (Buy) |
| 38 | 500 | OrderQty (500 shares) |
| 40 | 2 | OrdType (Limit) |
| 44 | 15000.0000 | Price (15,000 MNT) |
| 59 | 0 | TimeInForce (Day) |
| 60 | 20260424-10:01:00.000 | TransactTime |

**ExecutionReport (35=8) -- Order Acknowledged (New):**
```
8=FIX.4.4|9=210|35=8|49=GARUDAX|56=BROKER_MSE_001|34=2|52=20260424-10:01:00.005|
37=ORD-SEC-uuid-001|11=ORD-20260424-001|17=EXEC-uuid-001|150=0|39=0|
55=APU.UB|54=1|38=500|44=15000.0000|151=500|14=0|6=0|1=ACCT-001|
60=20260424-10:01:00.005|10=098|
```

| Tag | Value | Description |
|---|---|---|
| 37 | ORD-SEC-uuid-001 | OrderID (exchange-assigned) |
| 17 | EXEC-uuid-001 | ExecID |
| 150 | 0 | ExecType (New) |
| 39 | 0 | OrdStatus (New) |
| 151 | 500 | LeavesQty (500 remaining) |
| 14 | 0 | CumQty (0 filled) |
| 6 | 0 | AvgPx (no fills yet) |

**ExecutionReport (35=8) -- Partial Fill (200 of 500 shares at 14950):**
```
8=FIX.4.4|9=248|35=8|49=GARUDAX|56=BROKER_MSE_001|34=3|52=20260424-10:02:15.123|
37=ORD-SEC-uuid-001|11=ORD-20260424-001|17=EXEC-uuid-002|150=F|39=1|
55=APU.UB|54=1|38=500|44=15000.0000|31=14950.0000|32=200|151=300|14=200|
6=14950.0000|1=ACCT-001|30=MXUB|880=TRD-uuid-001|60=20260424-10:02:15.123|10=076|
```

| Tag | Value | Description |
|---|---|---|
| 150 | F | ExecType (Trade/Fill) |
| 39 | 1 | OrdStatus (Partially Filled) |
| 31 | 14950.0000 | LastPx (fill price) |
| 32 | 200 | LastQty (fill quantity) |
| 151 | 300 | LeavesQty (300 remaining) |
| 14 | 200 | CumQty (200 filled total) |
| 6 | 14950.0000 | AvgPx |
| 30 | MXUB | LastMkt (exchange MIC) |
| 880 | TRD-uuid-001 | TrdMatchID (trade ID) |

**ExecutionReport (35=8) -- Order Rejected:**
```
8=FIX.4.4|9=195|35=8|49=GARUDAX|56=BROKER_MSE_001|34=4|52=20260424-10:03:00.000|
37=NONE|11=ORD-20260424-002|17=EXEC-uuid-003|150=8|39=8|55=APU.UB|54=1|
38=99|44=15000.0000|151=0|14=0|6=0|1=ACCT-001|103=13|
58=INVALID_LOT_SIZE: quantity must be a multiple of 100|
60=20260424-10:03:00.000|10=145|
```

**OrderCancelRequest (35=F) -- Cancel the partially filled order:**
```
8=FIX.4.4|9=135|35=F|49=BROKER_MSE_001|56=GARUDAX|34=3|52=20260424-10:05:00.000|
41=ORD-20260424-001|11=CXLORD-20260424-001|55=APU.UB|54=1|38=500|
60=20260424-10:05:00.000|10=112|
```

**MarketDataRequest (35=V) -- Subscribe to APU.UB top 5 levels:**
```
8=FIX.4.4|9=105|35=V|49=BROKER_MSE_001|56=GARUDAX|34=4|52=20260424-10:00:00.000|
262=MDSUB-001|263=1|264=5|267=2|269=0|269=1|146=1|55=APU.UB|265=1|10=078|
```

| Tag | Value | Description |
|---|---|---|
| 262 | MDSUB-001 | MDReqID |
| 263 | 1 | SubscriptionRequestType (Snapshot + Updates) |
| 264 | 5 | MarketDepth (5 levels) |
| 267 | 2 | NoMDEntryTypes (bid and offer) |
| 269 | 0 | MDEntryType (Bid) |
| 269 | 1 | MDEntryType (Offer) |
| 146 | 1 | NoRelatedSym |
| 55 | APU.UB | Symbol |
| 265 | 1 | MDUpdateType (Incremental) |

### A.2 Error Handling Matrix

| Error Category | FIX Response | Recovery Action |
|---|---|---|
| **Session Errors** | | |
| Invalid BeginString | Disconnect TCP | Broker must correct FIX version |
| Invalid BodyLength | Reject (35=3), SessionRejectReason=1 | Broker resends |
| Invalid CheckSum | Reject (35=3), SessionRejectReason=1 | Broker resends |
| Missing required tag | Reject (35=3), SessionRejectReason=1 | Broker resends with required tags |
| Invalid tag value | Reject (35=3), SessionRejectReason=5 | Broker corrects value |
| Sequence number too high | ResendRequest (35=2) for gap | Broker sends gap-fill or resend |
| Sequence number too low (no PossDup) | Disconnect TCP | Broker must reset sequence |
| Logon failed (bad password) | Logout (35=5) with reason | Broker corrects credentials |
| Logon failed (broker suspended) | Logout (35=5) with reason | Contact exchange admin |
| **Application Errors** | | |
| Unknown message type | BusinessMessageReject (35=j) | Broker uses supported message type |
| Unknown symbol | ExecutionReport, OrdRejReason=1 | Broker corrects symbol |
| Market closed | ExecutionReport, OrdRejReason=2 | Broker waits for market open |
| Position limit exceeded | ExecutionReport, OrdRejReason=3 | Broker reduces quantity |
| Invalid lot size | ExecutionReport, OrdRejReason=13 | Broker adjusts to lot multiple |
| Invalid tick size | ExecutionReport, OrdRejReason=11 | Broker adjusts to tick multiple |
| Cancel unknown order | OrderCancelReject, CxlRejReason=1 | Broker verifies OrigClOrdID |
| Cancel too late (filled) | OrderCancelReject, CxlRejReason=0 | No recovery; order already filled |
| Market data: unknown symbol | MarketDataRequestReject, MDReqRejReason=0 | Broker corrects symbol |
| Market data: subscription limit | MarketDataRequestReject, MDReqRejReason=4 | Broker unsubscribes from other symbols |
| **Infrastructure Errors** | | |
| Securities service unavailable | BusinessMessageReject (35=j), reason=4 | Auto-retry after backoff |
| Database connection lost | Logout all sessions, reconnect | Sessions resume after reconnect |
| Kafka consumer lag | ExecutionReports delayed | Monitor lag; scale consumers |

### A.3 Reconnection Behavior

| Scenario | Gateway Behavior | Broker Expected Behavior |
|---|---|---|
| **Graceful logout** | Send Logout(5), wait for response, persist seq nums | Disconnect TCP after Logout response |
| **TCP disconnect (no logout)** | Detect via heartbeat timeout, persist seq nums, mark session DISCONNECTED | Reconnect and re-logon without ResetSeqNumFlag |
| **Gateway restart** | All sessions disconnected, seq nums loaded from DB | All brokers reconnect and re-logon; gateway sends ResendRequest if gap detected |
| **Daily reset** | Send Logout(5), reset seq nums to 1, wait for reconnect | Reconnect with MsgSeqNum=1 and ResetSeqNumFlag=Y |
| **Message gap detected** | Send ResendRequest(2) for missing range | Resend messages or send SequenceReset-GapFill |
| **Duplicate message** | Accept if PossDupFlag=Y, reject if N | Set PossDupFlag=Y on resends |
| **Network partition** | Heartbeat timeout -> TestRequest -> timeout -> disconnect | Reconnect after partition heals |

### A.4 Performance Targets

| Metric | Target | Notes |
|---|---|---|
| FIX message parse latency | < 50 us | Raw message to parsed struct |
| Order submission latency (FIX in to API call) | < 500 us | FIX parse + validation + API call |
| ExecutionReport latency (Kafka event to FIX out) | < 1 ms | Event consume + FIX build + TCP write |
| Max concurrent sessions | 1000 | Per gateway instance |
| Max messages per second (aggregate) | 50,000 | All sessions combined |
| Max message size | 65,536 bytes | Configurable per broker |
| Sequence number persistence | Batched, 100ms flush | Async with WAL for crash recovery |

### A.5 Port Allocation Update

| Service | gRPC Port | Health/Admin Port | Protocol |
|---|---|---|---|
| matching-engine | 50051 | 8081 | gRPC |
| clearing-engine | 50052 | 8082 | gRPC |
| margin-engine | 50053 | 8083 | gRPC |
| settlement-engine | 50054 | 8084 | gRPC |
| auth-service | 50055 | 8085 | gRPC |
| compliance-service | 50056 | 8086 | gRPC |
| market-data-service | 50057 | 8087 | gRPC |
| warehouse-service | 50058 | 8088 | gRPC |
| gateway (HTTP) | -- | 8080/8090 | HTTP |
| **fix-gateway** | -- | **8091/9091** | **FIX TCP: 9878** |

---

*This document is the authoritative specification for the GarudaX FIX Protocol Gateway. Builder agents implement directly from this spec. All FIX tag mappings, SQL DDL, and configuration structures are normative. GarudaX is the platform. Tenants are the venues. MSE is the flagship. Tenant ID is never optional.*
