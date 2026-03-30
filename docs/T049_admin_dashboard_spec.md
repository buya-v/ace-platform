# Admin Dashboard Interface Specification

**Document ID:** T049-SPEC-001
**Version:** 1.0
**Date:** 2026-03-28
**Status:** DRAFT
**Author:** Coder Agent (Phase 4)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Technology Stack](#2-technology-stack)
3. [Authentication & Role-Based Access](#3-authentication--role-based-access)
4. [Page Routes & Navigation](#4-page-routes--navigation)
5. [Component Hierarchy](#5-component-hierarchy)
6. [State Management](#6-state-management)
7. [Feature Specifications](#7-feature-specifications)
   - 7.1 [System Monitoring](#71-system-monitoring)
   - 7.2 [Participant Management](#72-participant-management)
   - 7.3 [Margin Call Overview](#73-margin-call-overview)
   - 7.4 [Settlement Cycle Status](#74-settlement-cycle-status)
   - 7.5 [Circuit Breaker Controls](#75-circuit-breaker-controls)
   - 7.6 [Warehouse Receipt Overview](#76-warehouse-receipt-overview)
   - 7.7 [Compliance Alerts](#77-compliance-alerts)
   - 7.8 [Audit Log](#78-audit-log)
8. [API Endpoint Mapping](#8-api-endpoint-mapping)
9. [Role-Permission Matrix](#9-role-permission-matrix)
10. [Polling & Real-Time Strategy](#10-polling--real-time-strategy)
11. [Error Handling](#11-error-handling)
12. [Deployment & Build](#12-deployment--build)

---

## 1. Overview

The GarudaX Admin Dashboard is a single-page application for exchange operators and compliance officers to monitor, manage, and control the AI Powered Commodity Exchange platform. It consumes the gateway's REST API (`/api/v1/`) and provides real-time visibility into system health, participant status, margin calls, settlements, circuit breakers, warehouse receipts, and compliance alerts.

### Design Principles

- **Role-scoped views**: The UI renders only the sections the authenticated user's role permits. No hidden-but-present DOM elements for unauthorized features.
- **Polling-first, WebSocket-enhanced**: Health checks and dashboards use polling at defined intervals. Market data and trade feeds use WebSocket streams where available.
- **Minimal client-side state**: Server is the source of truth. Client state is limited to UI concerns (filters, pagination cursors, modal open/closed).
- **Accessible and responsive**: WCAG 2.1 AA compliance. Responsive layout for desktop (primary) and tablet.

### Scope

This spec covers:
- Component hierarchy and page structure
- State management architecture
- API endpoint mapping for every dashboard feature
- Role-permission matrix
- Polling intervals and real-time data strategy
- Route definitions

This spec does NOT cover:
- Trading UI (separate SPA for traders)
- Backend service implementation
- Visual design / CSS / design system (assumes a component library like Radix UI or shadcn/ui)

---

## 2. Technology Stack

| Layer | Technology | Rationale |
|---|---|---|
| Framework | React 18 | Same stack as trading UI; concurrent features for data-heavy dashboards |
| Language | TypeScript 5.x | Type safety across API contracts |
| Build | Vite 5.x | Fast HMR, ESBuild-based bundling |
| Routing | React Router v6 | Nested routes, layout routes, loader pattern |
| State | React Context + `useReducer` | Sufficient for admin dashboard scope; no Redux overhead |
| HTTP | `fetch` + custom hooks | No axios dependency; typed request/response wrappers |
| Real-time | Native WebSocket | For market data streams via gateway WS endpoints |
| Testing | Vitest + React Testing Library | Aligned with Vite ecosystem |
| Linting | ESLint + Prettier | Standard config |

---

## 3. Authentication & Role-Based Access

### 3.1 Login Flow

```
User → /login → POST /api/v1/auth/login { email, password }
         ← { access_token, refresh_token, expires_in }
         → Store access_token in memory (not localStorage)
         → Store refresh_token in httpOnly cookie (set by gateway)
         → Decode JWT claims → extract roles[]
         → Redirect to /dashboard
```

### 3.2 Token Management

- **Access token**: Stored in-memory (`AuthContext`). Attached to every API request via `Authorization: Bearer <token>`.
- **Refresh token**: Stored in httpOnly cookie. On 401 response, call `POST /api/v1/auth/refresh` transparently.
- **Token expiry**: Access tokens expire in 15 minutes. Refresh tokens expire in 7 days.
- **Logout**: `POST /api/v1/auth/logout` to invalidate server-side, then clear in-memory token and redirect to `/login`.

### 3.3 Roles for Admin Dashboard

The admin dashboard recognizes two primary roles from the auth-service JWT:

| Role | JWT Value | Dashboard Access |
|---|---|---|
| Admin | `admin` or `exchange_admin` | Full access to all dashboard sections |
| Compliance Officer | `compliance_officer` | Participant Management (read + approve/reject), Compliance Alerts, Audit Log |

Users without either role are redirected to the trading UI or shown an "unauthorized" page.

### 3.4 Route Guards

```tsx
// ProtectedRoute checks AuthContext for valid token
// RoleGuard checks roles[] from JWT claims against required roles

<Route element={<ProtectedRoute />}>
  <Route element={<RoleGuard roles={['admin', 'compliance_officer']} />}>
    <Route path="/dashboard" element={<DashboardLayout />}>
      {/* admin-only routes */}
      <Route element={<RoleGuard roles={['admin']} />}>
        <Route path="monitoring" element={<SystemMonitoring />} />
        <Route path="margin" element={<MarginOverview />} />
        <Route path="settlement" element={<SettlementStatus />} />
        <Route path="circuit-breakers" element={<CircuitBreakers />} />
        <Route path="warehouse" element={<WarehouseOverview />} />
      </Route>
      {/* admin + compliance routes */}
      <Route path="participants" element={<ParticipantManagement />} />
      <Route path="participants/:id" element={<ParticipantDetail />} />
      <Route path="compliance" element={<ComplianceAlerts />} />
      <Route path="audit" element={<AuditLog />} />
    </Route>
  </Route>
</Route>
```

---

## 4. Page Routes & Navigation

| Route | Page Component | Required Role | Nav Label |
|---|---|---|---|
| `/login` | `LoginPage` | none | — |
| `/dashboard` | `DashboardHome` | admin, compliance_officer | Overview |
| `/dashboard/monitoring` | `SystemMonitoring` | admin | System Health |
| `/dashboard/participants` | `ParticipantManagement` | admin, compliance_officer | Participants |
| `/dashboard/participants/:id` | `ParticipantDetail` | admin, compliance_officer | — (detail) |
| `/dashboard/margin` | `MarginOverview` | admin | Margin Calls |
| `/dashboard/settlement` | `SettlementStatus` | admin | Settlement |
| `/dashboard/circuit-breakers` | `CircuitBreakers` | admin | Circuit Breakers |
| `/dashboard/warehouse` | `WarehouseOverview` | admin | Warehouse |
| `/dashboard/compliance` | `ComplianceAlerts` | admin, compliance_officer | Compliance |
| `/dashboard/audit` | `AuditLog` | admin, compliance_officer | Audit Log |

### Navigation Sidebar

The sidebar renders only links the user's role permits. Structure:

```
[GarudaX Admin Logo]

Overview              /dashboard
──────────────────────
OPERATIONS (admin only)
  System Health       /dashboard/monitoring
  Margin Calls        /dashboard/margin
  Settlement          /dashboard/settlement
  Circuit Breakers    /dashboard/circuit-breakers
  Warehouse           /dashboard/warehouse
──────────────────────
COMPLIANCE (admin + compliance_officer)
  Participants        /dashboard/participants
  Compliance Alerts   /dashboard/compliance
  Audit Log           /dashboard/audit
──────────────────────
[User avatar + name]
  Logout
```

---

## 5. Component Hierarchy

```
App
├── AuthProvider (Context)
│   ├── LoginPage
│   └── ProtectedRoute
│       └── DashboardLayout
│           ├── Sidebar
│           │   ├── NavSection ("Operations")
│           │   │   └── NavLink[]
│           │   ├── NavSection ("Compliance")
│           │   │   └── NavLink[]
│           │   └── UserMenu
│           ├── TopBar
│           │   ├── Breadcrumb
│           │   └── NotificationBell
│           └── <Outlet /> (page content)
│               ├── DashboardHome
│               │   ├── SystemHealthSummary (mini cards)
│               │   ├── ActiveMarginCallsBadge
│               │   ├── PendingKYCBadge
│               │   └── SettlementCycleBadge
│               ├── SystemMonitoring
│               │   └── ServiceHealthGrid
│               │       └── ServiceHealthCard[] (×9)
│               ├── ParticipantManagement
│               │   ├── ParticipantFilters
│               │   ├── ParticipantTable
│               │   │   └── ParticipantRow[]
│               │   └── KYCActionModal
│               ├── ParticipantDetail
│               │   ├── ParticipantInfo
│               │   ├── DocumentList
│               │   │   └── DocumentItem[]
│               │   └── KYCTimeline
│               ├── MarginOverview
│               │   ├── MarginCallStats
│               │   ├── ActiveMarginCallTable
│               │   │   └── MarginCallRow[]
│               │   ├── MarginUtilizationChart
│               │   └── TriggerMarginCallModal
│               ├── SettlementStatus
│               │   ├── CurrentCyclePanel
│               │   ├── PendingSettlementTable
│               │   └── SettlementHistoryTable
│               ├── CircuitBreakers
│               │   ├── InstrumentControlTable
│               │   │   └── InstrumentControlRow[]
│               │   └── PriceLimitEditModal
│               ├── WarehouseOverview
│               │   ├── ReceiptInventoryTable
│               │   │   └── ReceiptRow[]
│               │   ├── PendingDeliveryTable
│               │   └── CapacityUtilizationChart
│               ├── ComplianceAlerts
│               │   ├── SARQueue
│               │   │   └── SARCard[]
│               │   ├── ScreeningResults
│               │   │   └── ScreeningRow[]
│               │   └── RiskScoreDistribution (chart)
│               └── AuditLog
│                   ├── AuditFilters
│                   └── AuditEventTable
│                       └── AuditEventRow[]
```

### Shared Components

| Component | Purpose |
|---|---|
| `DataTable` | Sortable, paginated table with column definitions |
| `StatusBadge` | Colored badge for status values (ACTIVE, PENDING, HALTED, etc.) |
| `ConfirmDialog` | Modal for destructive actions (halt trading, reject KYC) |
| `LoadingSpinner` | Skeleton/spinner for async data |
| `ErrorBoundary` | Catches render errors, shows fallback |
| `EmptyState` | Placeholder when a table/list has no data |
| `DateRangePicker` | Filter for audit log and history views |
| `PollingIndicator` | Small dot showing last refresh time and polling status |

---

## 6. State Management

### 6.1 Architecture

```
AuthContext           — token, user, roles, login/logout actions
├── useAuth()         — hook for consuming auth state
│
DashboardContext      — shared dashboard state (selected filters, notifications)
├── useDashboard()    — hook for dashboard-wide state
```

Each page manages its own data-fetching state via custom hooks. No global data store.

### 6.2 AuthContext

```ts
interface AuthState {
  token: string | null;
  user: {
    id: string;
    email: string;
    name: string;
    roles: string[];
    participant_id: string | null;
  } | null;
  isAuthenticated: boolean;
  isLoading: boolean;
}

type AuthAction =
  | { type: 'LOGIN_SUCCESS'; payload: { token: string; user: User } }
  | { type: 'LOGIN_FAILURE'; payload: { error: string } }
  | { type: 'LOGOUT' }
  | { type: 'TOKEN_REFRESHED'; payload: { token: string } };
```

### 6.3 Data-Fetching Hooks Pattern

Each feature page uses a custom hook that encapsulates fetching, polling, and error state:

```ts
// Example: useServiceHealth()
function useServiceHealth(pollInterval: number = 15000) {
  const [state, dispatch] = useReducer(serviceHealthReducer, initialState);
  const { token } = useAuth();

  useEffect(() => {
    const controller = new AbortController();
    const poll = async () => {
      dispatch({ type: 'FETCH_START' });
      try {
        const data = await fetchServiceHealth(token, controller.signal);
        dispatch({ type: 'FETCH_SUCCESS', payload: data });
      } catch (err) {
        if (!controller.signal.aborted) {
          dispatch({ type: 'FETCH_ERROR', payload: err });
        }
      }
    };
    poll();
    const id = setInterval(poll, pollInterval);
    return () => { controller.abort(); clearInterval(id); };
  }, [token, pollInterval]);

  return state;
}
```

### 6.4 Hook Inventory

| Hook | Page | API Calls |
|---|---|---|
| `useAuth` | Global | `/auth/login`, `/auth/refresh`, `/auth/logout`, `/auth/me` |
| `useServiceHealth` | SystemMonitoring | `GET /healthz` on 9 services |
| `useParticipants` | ParticipantManagement | `GET /participants`, `POST .../approve`, `POST .../reject` |
| `useParticipantDetail` | ParticipantDetail | `GET /participants/:id`, `GET /participants/:id/documents` |
| `useMarginCalls` | MarginOverview | `GET /margin/calls`, `GET /margin/calls/stats`, `POST /margin/calculate` |
| `useSettlement` | SettlementStatus | `GET /settlement/cycles` |
| `useCircuitBreakers` | CircuitBreakers | `PUT /admin/instruments/:id/circuit-breaker`, `POST .../halt`, `POST .../resume` |
| `useWarehouse` | WarehouseOverview | `GET /warehouse/receipts`, `GET /warehouse/deliveries`, `GET /warehouse/facilities` |
| `useComplianceAlerts` | ComplianceAlerts | `GET /compliance/alerts`, `POST /compliance/sar`, `GET /screening/:id`, `GET /risk-scores/:id` |
| `useAuditLog` | AuditLog | `GET /compliance/audit-trail` |

---

## 7. Feature Specifications

### 7.1 System Monitoring

**Purpose:** Real-time health dashboard for all 9 GarudaX platform services.

**Data Source:** Each service exposes a `/healthz` endpoint on its health HTTP port.

| Service | Health URL | Poll Target |
|---|---|---|
| matching-engine | `http://matching-engine:8081/healthz` | Gateway proxied |
| clearing-engine | `http://clearing-engine:8082/healthz` | Gateway proxied |
| margin-engine | `http://margin-engine:8083/healthz` | Gateway proxied |
| settlement-engine | `http://settlement-engine:8084/healthz` | Gateway proxied |
| auth-service | `http://auth-service:8085/healthz` | Gateway proxied |
| compliance-service | `http://compliance-service:8086/healthz` | Gateway proxied |
| market-data-service | `http://market-data-service:8087/healthz` | Gateway proxied |
| warehouse-service | `http://warehouse-service:8088/healthz` | Gateway proxied |
| gateway | `http://gateway:8080/healthz` | Direct |

**Gateway Admin Endpoint:** `GET /api/v1/admin/health` — aggregated health check that proxies to all backend services and returns a combined response:

```json
{
  "services": [
    {
      "name": "matching-engine",
      "status": "healthy",
      "latency_ms": 2,
      "last_check": "2026-03-28T10:00:15Z",
      "uptime_seconds": 86400,
      "version": "1.0.0"
    }
  ],
  "overall_status": "healthy"
}
```

**UI Components:**

- `ServiceHealthGrid`: 3×3 grid of `ServiceHealthCard` components
- Each card shows: service name, status indicator (green/yellow/red), latency, uptime, last check time
- Overall status banner at top: "All Systems Operational" / "Degraded" / "Outage"

**Polling interval:** 15 seconds

**Status logic:**
- `healthy`: 200 response within 5s
- `degraded`: 200 response but latency > 1s, or non-200 within retry window
- `unhealthy`: No response or 5xx within 3 consecutive checks

---

### 7.2 Participant Management

**Purpose:** View, search, and manage KYC applications and participant accounts.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| List participants | GET | `/api/v1/participants?status={status}&page={p}&limit={l}` | compliance-service |
| Get participant detail | GET | `/api/v1/participants/{participant_id}` | compliance-service |
| Approve KYC | POST | `/api/v1/participants/{participant_id}/approve` | compliance-service |
| Reject KYC | POST | `/api/v1/participants/{participant_id}/reject` | compliance-service |
| List documents | GET | `/api/v1/participants/{participant_id}/documents` | compliance-service |

**UI Components:**

- **ParticipantTable**: Columns — Name, Email, KYC Status (PENDING/APPROVED/REJECTED/UNDER_REVIEW), Submission Date, Risk Score, Actions
- **ParticipantFilters**: Status dropdown, search by name/email, date range
- **KYCActionModal**: Confirmation dialog for approve/reject with required reason field for rejections
- **ParticipantDetail** (sub-page):
  - Info panel: Name, email, organization, submission date, current status
  - Document list: File name, type (ID, proof of address, etc.), upload date, download link
  - KYC timeline: Chronological list of status changes with timestamps and actor

**Polling interval:** 30 seconds (participant list); no polling on detail page (fetch on mount)

---

### 7.3 Margin Call Overview

**Purpose:** Monitor active margin calls and margin utilization across participants.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| List active margin calls | GET | `/api/v1/margin/calls` | margin-engine |
| Margin call statistics | GET | `/api/v1/margin/calls/stats` | margin-engine |
| Get portfolio margin | GET | `/api/v1/margin?participant_id={id}` | margin-engine |
| Trigger margin calculation | POST | `/api/v1/margin/calculate` | margin-engine |

**UI Components:**

- **MarginCallStats**: Summary cards — Total Active Calls, Total Shortfall Amount, Participants in Call, Average Utilization %
- **ActiveMarginCallTable**: Columns — Participant, Instrument, Required Margin, Current Margin, Shortfall, Issued At, Deadline, Status (PENDING/MET/BREACHED)
- **MarginUtilizationChart**: Bar chart showing margin utilization % per participant (top 20), with threshold line at 80% (warning) and 100% (call)
- **TriggerMarginCallModal**: Select participant + instrument, confirm manual margin recalculation

**Polling interval:** 10 seconds (margin calls are time-sensitive)

---

### 7.4 Settlement Cycle Status

**Purpose:** Monitor settlement cycle phases and pending settlements.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| List settlement cycles | GET | `/api/v1/settlement/cycles?status={status}` | settlement-engine |
| Get cycle detail | GET | `/api/v1/settlement/cycles/{cycle_id}` | settlement-engine |

**UI Components:**

- **CurrentCyclePanel**: Shows the active cycle — ID, phase (OPEN/NETTING/SETTLING/COMPLETED/FAILED), start time, expected completion, progress indicator
- **PendingSettlementTable**: Columns — Settlement ID, Buyer, Seller, Instrument, Quantity, Price, Status (PENDING/SETTLED/FAILED), Due Date
- **SettlementHistoryTable**: Paginated table of completed cycles with date, total settlements, total value, pass/fail count

**Polling interval:** 15 seconds

**Phase visualization:** Horizontal stepper showing cycle phases: `OPEN → NETTING → SETTLING → COMPLETED`, with current phase highlighted.

---

### 7.5 Circuit Breaker Controls

**Purpose:** View and manage price limits and trading halts per instrument.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| Get instrument list | GET | `/api/v1/instruments` | matching-engine (via gateway) |
| Set circuit breaker | PUT | `/api/v1/admin/instruments/{instrument_id}/circuit-breaker` | matching-engine |
| Halt trading | POST | `/api/v1/admin/instruments/{instrument_id}/halt` | matching-engine |
| Resume trading | POST | `/api/v1/admin/instruments/{instrument_id}/resume` | matching-engine |

**UI Components:**

- **InstrumentControlTable**: Columns — Instrument, Last Price, Upper Limit, Lower Limit, Status (TRADING/HALTED/PRE_OPEN), Daily Volume, Actions (Halt/Resume, Edit Limits)
- **PriceLimitEditModal**: Form fields — Upper Limit (absolute or %), Lower Limit (absolute or %), Cooldown Period (minutes). Shows current values and requires confirmation.
- **HaltConfirmDialog**: "Are you sure you want to halt trading for {instrument}? This will cancel all open orders." Requires typed confirmation (instrument ticker).

**Polling interval:** 10 seconds

**Circuit breaker request body:**

```json
{
  "upper_limit_pct": 10.0,
  "lower_limit_pct": 10.0,
  "cooldown_minutes": 5,
  "reference_price": "1250.00"
}
```

---

### 7.6 Warehouse Receipt Overview

**Purpose:** Monitor warehouse receipt inventory, pending deliveries, and facility capacity.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| List receipts | GET | `/api/v1/warehouse/receipts?status={status}&page={p}` | warehouse-service |
| List deliveries | GET | `/api/v1/warehouse/deliveries?status={status}` | warehouse-service |
| List facilities | GET | `/api/v1/warehouse/facilities` | warehouse-service |

**UI Components:**

- **ReceiptInventoryTable**: Columns — Receipt ID, Commodity, Grade, Quantity, Unit, Warehouse, Status (ACTIVE/PLEDGED/IN_TRANSIT/DELIVERED/CANCELLED), Issue Date, Holder
- **PendingDeliveryTable**: Columns — Delivery ID, Receipt, From Warehouse, To Warehouse/Buyer, Commodity, Quantity, Status (PENDING/IN_TRANSIT/DELIVERED), Requested Date
- **CapacityUtilizationChart**: Stacked bar chart per facility showing used/available capacity by commodity type. Threshold line at 90% capacity.

**Polling interval:** 60 seconds (warehouse data changes infrequently)

---

### 7.7 Compliance Alerts

**Purpose:** Manage SAR queue, screening results, and risk score monitoring.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| List alerts | GET | `/api/v1/compliance/alerts?status={status}` | compliance-service |
| Resolve alert | POST | `/api/v1/compliance/alerts/{alert_id}/resolve` | compliance-service |
| File SAR | POST | `/api/v1/compliance/sar` | compliance-service |
| Get screening result | GET | `/api/v1/screening/{screening_id}` | compliance-service |
| Run screening | POST | `/api/v1/screening/check` | compliance-service |
| Get risk score | GET | `/api/v1/risk-scores/{participant_id}` | compliance-service |

**UI Components:**

- **SARQueue**: Card list of pending SARs — Participant, Alert Type (UNUSUAL_VOLUME, WATCHLIST_MATCH, PATTERN_DETECTED), Severity (LOW/MEDIUM/HIGH/CRITICAL), Created At, Actions (Review, File SAR, Dismiss)
- **ScreeningResults**: Table — Participant, Screening Type, Status (CLEAR/MATCH_FOUND/PENDING_REVIEW), Match Details, Screened At, Resolution
- **RiskScoreDistribution**: Histogram chart showing distribution of risk scores across all participants (0-100 scale), with bands: Low (0-30), Medium (31-60), High (61-80), Critical (81-100)

**Polling interval:** 30 seconds

**Alert severity styling:**
- CRITICAL: Red background, auto-sort to top
- HIGH: Orange left border
- MEDIUM: Yellow left border
- LOW: Default styling

---

### 7.8 Audit Log

**Purpose:** Searchable log of system events for compliance and operational review.

**API Endpoints:**

| Action | Method | Gateway Endpoint | Backend |
|---|---|---|---|
| Get audit trail | GET | `/api/v1/compliance/audit-trail?actor={actor}&action={action}&from={ts}&to={ts}&page={p}` | compliance-service |

**UI Components:**

- **AuditFilters**: Actor (user/system), Action type dropdown (LOGIN, KYC_APPROVED, TRADE_HALTED, MARGIN_CALL_ISSUED, etc.), Date range picker, Free-text search
- **AuditEventTable**: Columns — Timestamp, Actor (user email or "system"), Action, Target (resource type + ID), Details (expandable), IP Address

**Polling interval:** None (on-demand fetch with pagination). Manual refresh button.

**Audit event types:**

| Category | Events |
|---|---|
| Auth | `LOGIN`, `LOGOUT`, `PASSWORD_CHANGED`, `TOKEN_REFRESHED` |
| KYC | `KYC_SUBMITTED`, `KYC_APPROVED`, `KYC_REJECTED`, `DOCUMENT_UPLOADED` |
| Trading | `TRADE_HALTED`, `TRADE_RESUMED`, `CIRCUIT_BREAKER_SET`, `TRADE_BUSTED`, `MASS_CANCEL` |
| Margin | `MARGIN_CALL_ISSUED`, `MARGIN_CALL_MET`, `MARGIN_CALL_BREACHED`, `MANUAL_MARGIN_CALC` |
| Settlement | `CYCLE_STARTED`, `CYCLE_COMPLETED`, `CYCLE_FAILED`, `SETTLEMENT_PROCESSED` |
| Compliance | `SAR_FILED`, `ALERT_RESOLVED`, `PARTICIPANT_SUSPENDED`, `PARTICIPANT_REINSTATED`, `SCREENING_RUN` |
| Warehouse | `RECEIPT_ISSUED`, `RECEIPT_TRANSFERRED`, `DELIVERY_REQUESTED`, `DELIVERY_COMPLETED` |

---

## 8. API Endpoint Mapping

Complete mapping of dashboard features to gateway REST endpoints. All paths prefixed with `/api/v1/`.

| Dashboard Feature | HTTP Method | Endpoint | Role Required |
|---|---|---|---|
| **System Health** | | | |
| Aggregated health | GET | `/admin/health` | admin |
| **Participants** | | | |
| List participants | GET | `/participants` | admin, compliance_officer |
| Get participant | GET | `/participants/{id}` | admin, compliance_officer |
| Approve KYC | POST | `/participants/{id}/approve` | admin, compliance_officer |
| Reject KYC | POST | `/participants/{id}/reject` | admin, compliance_officer |
| List documents | GET | `/participants/{id}/documents` | admin, compliance_officer |
| **Margin** | | | |
| Active margin calls | GET | `/margin/calls` | admin |
| Margin call stats | GET | `/margin/calls/stats` | admin |
| Portfolio margin | GET | `/margin?participant_id={id}` | admin |
| Trigger calculation | POST | `/margin/calculate` | admin |
| **Settlement** | | | |
| List cycles | GET | `/settlement/cycles` | admin |
| Get cycle | GET | `/settlement/cycles/{id}` | admin |
| **Circuit Breakers** | | | |
| Set limits | PUT | `/admin/instruments/{id}/circuit-breaker` | admin |
| Halt trading | POST | `/admin/instruments/{id}/halt` | admin |
| Resume trading | POST | `/admin/instruments/{id}/resume` | admin |
| **Warehouse** | | | |
| List receipts | GET | `/warehouse/receipts` | admin |
| List deliveries | GET | `/warehouse/deliveries` | admin |
| List facilities | GET | `/warehouse/facilities` | admin |
| **Compliance** | | | |
| List alerts | GET | `/compliance/alerts` | admin, compliance_officer |
| Resolve alert | POST | `/compliance/alerts/{id}/resolve` | admin, compliance_officer |
| File SAR | POST | `/compliance/sar` | admin, compliance_officer |
| Get screening | GET | `/screening/{id}` | admin, compliance_officer |
| Run screening | POST | `/screening/check` | admin, compliance_officer |
| Get risk score | GET | `/risk-scores/{id}` | admin, compliance_officer |
| **Audit** | | | |
| Get audit trail | GET | `/compliance/audit-trail` | admin, compliance_officer |

---

## 9. Role-Permission Matrix

Granular permission matrix mapping roles to UI features and actions.

| Feature / Action | `admin` | `compliance_officer` |
|---|---|---|
| **Dashboard Home** | | |
| View overview | Yes | Yes (compliance sections only) |
| **System Monitoring** | | |
| View service health | Yes | No |
| **Participant Management** | | |
| View participant list | Yes | Yes |
| View participant detail | Yes | Yes |
| Approve KYC | Yes | Yes |
| Reject KYC | Yes | Yes |
| View documents | Yes | Yes |
| **Margin Call Overview** | | |
| View margin calls | Yes | No |
| View margin stats | Yes | No |
| Trigger manual calculation | Yes | No |
| **Settlement Status** | | |
| View settlement cycles | Yes | No |
| View settlement history | Yes | No |
| **Circuit Breaker Controls** | | |
| View instrument status | Yes | No |
| Halt/resume trading | Yes | No |
| Edit price limits | Yes | No |
| **Warehouse Overview** | | |
| View receipts | Yes | No |
| View deliveries | Yes | No |
| View capacity | Yes | No |
| **Compliance Alerts** | | |
| View alerts | Yes | Yes |
| Resolve alerts | Yes | Yes |
| File SAR | Yes | Yes |
| Run screening | Yes | Yes |
| View risk scores | Yes | Yes |
| **Audit Log** | | |
| View audit trail | Yes | Yes |
| Filter/search | Yes | Yes |

---

## 10. Polling & Real-Time Strategy

### 10.1 Polling Intervals

| Feature | Interval | Rationale |
|---|---|---|
| System Health | 15s | Timely detection of outages |
| Margin Calls | 10s | Margin calls are time-critical |
| Circuit Breakers | 10s | Trading halts need immediate visibility |
| Settlement Cycles | 15s | Cycle phases change infrequently within a session |
| Participant List | 30s | KYC status changes are not urgent |
| Compliance Alerts | 30s | Alerts need attention but not sub-second |
| Warehouse Receipts | 60s | Physical warehouse changes are slow |
| Audit Log | None | On-demand only; manual refresh |

### 10.2 Polling Implementation

All polling uses `setInterval` inside `useEffect` with `AbortController` cleanup. Polling pauses when:
- The browser tab is not visible (`document.visibilityState === 'hidden'`)
- The user has an active modal/dialog open (to prevent table re-renders)
- The network is offline (`navigator.onLine === false`)

### 10.3 WebSocket Streams (Future Enhancement)

For the initial release, all data is fetched via polling. Future iterations may add WebSocket connections for:
- Real-time trade feed on Circuit Breaker page (via `WS /api/v1/ws/trades/{instrument_id}`)
- Live margin call notifications
- Health check push notifications

---

## 11. Error Handling

### 11.1 HTTP Error Responses

The dashboard handles gateway error responses per the standard error format:

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Token expired",
    "details": {}
  }
}
```

| HTTP Status | Dashboard Behavior |
|---|---|
| 401 | Attempt token refresh; if refresh fails, redirect to `/login` |
| 403 | Show "insufficient permissions" inline message |
| 404 | Show "resource not found" state in component |
| 422 | Show validation errors on form fields |
| 429 | Show "rate limited, retrying..." with exponential backoff |
| 500+ | Show error banner, continue polling with backoff |

### 11.2 Error Boundary

A top-level `ErrorBoundary` catches render crashes and shows a "Something went wrong" page with a reload button. Per-section error boundaries wrap each dashboard page so a single failing section doesn't crash the whole app.

### 11.3 Network Resilience

- Polling continues through transient failures (single failures don't change displayed status)
- After 3 consecutive failures on a polling endpoint, show a warning banner
- `AbortController` used on all fetches to prevent stale response handling on navigation

---

## 12. Deployment & Build

### 12.1 Build Output

```bash
npm run build  # → dist/ directory with hashed assets
```

Vite produces a static SPA bundle served by:
- NGINX container in production (lightweight static file server)
- Vite dev server in development

### 12.2 Environment Configuration

```ts
// src/config.ts — loaded at runtime from window.__GARUDAX_CONFIG__
interface AppConfig {
  API_BASE_URL: string;        // e.g., "https://api.garudax.mn/api/v1"
  WS_BASE_URL: string;         // e.g., "wss://api.garudax.mn/api/v1/ws"
  HEALTH_POLL_INTERVAL: number; // default: 15000
  AUTH_TOKEN_REFRESH_BUFFER: number; // seconds before expiry to refresh, default: 60
}
```

Runtime config is injected via a `config.js` script tag in `index.html`, allowing the same build artifact to run in different environments (dev, staging, production) without rebuilding.

### 12.3 Docker

```dockerfile
# Build stage
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Serve stage
FROM nginx:1.25-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
```

### 12.4 Port Allocation

| Environment | Port |
|---|---|
| Development (Vite) | 5173 |
| Production (NGINX) | 3000 |

---

## Appendix A: TypeScript Type Definitions

Key types shared across dashboard features:

```ts
// API response wrapper
interface ApiResponse<T> {
  data: T;
  pagination?: {
    page: number;
    limit: number;
    total: number;
    total_pages: number;
  };
}

// Service health
interface ServiceHealth {
  name: string;
  status: 'healthy' | 'degraded' | 'unhealthy';
  latency_ms: number;
  last_check: string; // ISO 8601
  uptime_seconds: number;
  version: string;
}

// Participant
interface Participant {
  id: string;
  name: string;
  email: string;
  organization: string;
  kyc_status: 'PENDING' | 'APPROVED' | 'REJECTED' | 'UNDER_REVIEW';
  risk_score: number;
  submitted_at: string;
  updated_at: string;
}

// Margin call
interface MarginCall {
  id: string;
  participant_id: string;
  participant_name: string;
  instrument_id: string;
  required_margin: string; // Decimal string
  current_margin: string;
  shortfall: string;
  status: 'PENDING' | 'MET' | 'BREACHED';
  issued_at: string;
  deadline: string;
}

// Settlement cycle
interface SettlementCycle {
  id: string;
  phase: 'OPEN' | 'NETTING' | 'SETTLING' | 'COMPLETED' | 'FAILED';
  started_at: string;
  expected_completion: string;
  total_settlements: number;
  total_value: string;
  completed_at?: string;
}

// Instrument control
interface InstrumentControl {
  instrument_id: string;
  ticker: string;
  last_price: string;
  upper_limit: string;
  lower_limit: string;
  status: 'TRADING' | 'HALTED' | 'PRE_OPEN';
  daily_volume: number;
}

// Warehouse receipt
interface WarehouseReceipt {
  id: string;
  commodity: string;
  grade: string;
  quantity: string;
  unit: string;
  warehouse_name: string;
  status: 'ACTIVE' | 'PLEDGED' | 'IN_TRANSIT' | 'DELIVERED' | 'CANCELLED';
  issued_at: string;
  holder_name: string;
}

// Compliance alert
interface ComplianceAlert {
  id: string;
  participant_id: string;
  participant_name: string;
  alert_type: 'UNUSUAL_VOLUME' | 'WATCHLIST_MATCH' | 'PATTERN_DETECTED';
  severity: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  description: string;
  status: 'OPEN' | 'UNDER_REVIEW' | 'RESOLVED' | 'DISMISSED';
  created_at: string;
}

// Audit event
interface AuditEvent {
  id: string;
  timestamp: string;
  actor: string; // email or "system"
  action: string;
  target_type: string;
  target_id: string;
  details: Record<string, unknown>;
  ip_address: string;
}
```
