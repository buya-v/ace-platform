# KYC/AML Architecture Specification

**Document ID:** T015-SPEC-001
**Version:** 1.0
**Date:** 2026-03-27
**Status:** DRAFT
**Author:** Coder Agent (Phase 1)

---

## Table of Contents

1. [Overview](#1-overview)
2. [System Context](#2-system-context)
3. [Participant Onboarding Workflow](#3-participant-onboarding-workflow)
4. [Document Verification](#4-document-verification)
5. [Watchlist Screening](#5-watchlist-screening)
6. [Risk Scoring Model](#6-risk-scoring-model)
7. [Ongoing Monitoring](#7-ongoing-monitoring)
8. [API Contracts](#8-api-contracts)
9. [Data Model](#9-data-model)
10. [Integration Points](#10-integration-points)
11. [Regulatory Requirements](#11-regulatory-requirements)
12. [Security & Privacy](#12-security--privacy)
13. [Failure Modes & Recovery](#13-failure-modes--recovery)

---

## 1. Overview

The ACE Compliance Service implements KYC (Know Your Customer) and AML (Anti-Money Laundering) workflows for the Agriculture Commodity Exchange of Mongolia. Every participant must pass identity verification, document validation, watchlist screening, and risk assessment before they can trade.

### Design Principles

- **Immutable audit trail**: All compliance decisions are append-only in `compliance.audit_events` with SHA-256 hash chaining (per T004).
- **Risk-based approach**: Onboarding depth and ongoing monitoring frequency scale with participant risk score.
- **Separation of duties**: Automated checks produce recommendations; final approval requires human compliance officer sign-off for elevated-risk participants.
- **Pluggable screening providers**: Watchlist and document verification use adapter interfaces so providers can be swapped without changing core logic.

### Scope

This spec covers:
- Participant onboarding lifecycle (application → verification → approval)
- Document upload, classification, and verification workflows
- Sanctions/PEP/adverse media watchlist screening
- Quantitative risk scoring model
- Ongoing transaction monitoring and periodic re-screening
- gRPC API contracts for builder agents

This spec does NOT cover:
- Authentication and authorization (T005)
- Transaction matching or order submission (T007/T008)
- Clearing and settlement (T027)

---

## 2. System Context

```
                    +------------------+
                    |   API Gateway    |
                    +--------+---------+
                             |
                   gRPC / REST (onboarding, document upload, screening)
                             |
                    +--------v---------+
                    | Compliance       |
                    | Service          |
                    +--+-----+------+--+
                       |     |      |
          +------------+     |      +-------------+
          |                  |                    |
  +-------v-------+  +------v--------+  +--------v--------+
  | PostgreSQL     |  | Document      |  | Watchlist        |
  | (compliance    |  | Store (S3)    |  | Provider         |
  |  schema)       |  |               |  | (sanctions API)  |
  +----------------+  +---------------+  +-----------------+
                             |
                    +--------v---------+
                    | Kafka/NATS       |
                    | (compliance      |
                    |  events)         |
                    +------------------+
```

### Service Dependencies

| Dependency | Purpose |
|---|---|
| PostgreSQL (`compliance` schema) | Participant records, KYC cases, screening results, audit trail |
| S3-compatible object store | Encrypted document storage (ID scans, certificates, financials) |
| Watchlist screening provider | Sanctions, PEP, adverse media checks (Dow Jones, Refinitiv, or equivalent) |
| Kafka/NATS | Publish compliance events consumed by exchange engine (trading gate) and clearing |
| Auth service (T005) | JWT validation for API access; role-based access control |

---

## 3. Participant Onboarding Workflow

### 3.1 Participant Types

| Type | Description | KYC Tier |
|---|---|---|
| `INDIVIDUAL` | Natural person trading on own account | Standard |
| `CORPORATE` | Mongolian-registered legal entity | Enhanced |
| `COOPERATIVE` | Agricultural cooperative (herder groups) | Standard |
| `BROKER` | Licensed intermediary | Enhanced |
| `INSTITUTIONAL` | Banks, funds, foreign institutions | Enhanced |

### 3.2 Onboarding State Machine

```
  APPLICATION_SUBMITTED
         │
         ▼
  DOCUMENTS_PENDING  ──────────────┐
         │                         │ (documents rejected)
         ▼                         │
  DOCUMENTS_UPLOADED               │
         │                         │
         ▼                         │
  VERIFICATION_IN_PROGRESS ◄───────┘
         │
         ├── automated checks pass ──► SCREENING_IN_PROGRESS
         │                                    │
         │                                    ├── clear ──► RISK_SCORING
         │                                    │                  │
         │                                    │                  ├── low/medium ──► APPROVED
         │                                    │                  │
         │                                    │                  └── high ──► MANUAL_REVIEW
         │                                    │                                    │
         │                                    │                                    ├── approve ──► APPROVED
         │                                    │                                    │
         │                                    │                                    └── reject ──► REJECTED
         │                                    │
         │                                    └── match found ──► MANUAL_REVIEW
         │
         └── verification fails ──► DOCUMENTS_PENDING (re-upload required)
```

### 3.3 Status Definitions

| Status | Description |
|---|---|
| `APPLICATION_SUBMITTED` | Initial application received; awaiting documents |
| `DOCUMENTS_PENDING` | Compliance service awaits required document uploads |
| `DOCUMENTS_UPLOADED` | All required documents received; queued for verification |
| `VERIFICATION_IN_PROGRESS` | Document authenticity and data extraction running |
| `SCREENING_IN_PROGRESS` | Watchlist/sanctions screening running |
| `RISK_SCORING` | Automated risk score computation |
| `MANUAL_REVIEW` | Escalated to compliance officer for human decision |
| `APPROVED` | Participant cleared to trade |
| `REJECTED` | Application denied; participant cannot trade |
| `SUSPENDED` | Previously approved participant suspended (triggered by monitoring) |
| `EXPIRED` | KYC approval expired; re-verification required |

### 3.4 SLA Targets

| Step | Target | Measurement |
|---|---|---|
| Document verification | < 2 minutes (automated) | 95th percentile |
| Watchlist screening | < 30 seconds | 95th percentile |
| Risk scoring | < 5 seconds | 95th percentile |
| End-to-end (no manual review) | < 5 minutes | 95th percentile |
| Manual review queue | < 24 hours | Business hours |

---

## 4. Document Verification

### 4.1 Required Documents by Participant Type

| Document | Individual | Corporate | Cooperative | Broker | Institutional |
|---|---|---|---|---|---|
| National ID / Passport | ✓ | - | - | - | - |
| Proof of address (< 3 months) | ✓ | - | - | - | - |
| Company registration certificate | - | ✓ | ✓ | ✓ | ✓ |
| Beneficial ownership declaration | - | ✓ | - | ✓ | ✓ |
| Financial statements (latest FY) | - | ✓ | - | ✓ | ✓ |
| Broker license (FRC-issued) | - | - | - | ✓ | - |
| Board resolution / POA | - | ✓ | ✓ | ✓ | ✓ |
| Tax registration certificate | - | ✓ | ✓ | ✓ | ✓ |
| Cooperative membership list | - | - | ✓ | - | - |

### 4.2 Verification Pipeline

```
Document Upload
     │
     ▼
  File Validation          (mime type, size ≤ 20MB, virus scan)
     │
     ▼
  Classification           (ML model or manual tag: ID, proof-of-address, etc.)
     │
     ▼
  Data Extraction (OCR)    (name, DOB, ID number, address, expiry)
     │
     ▼
  Cross-Reference Check    (extracted data vs. application data)
     │
     ▼
  Authenticity Check       (MRZ validation for passports, hologram detection)
     │
     ▼
  Expiry Validation        (document not expired)
     │
     ▼
  Result: VERIFIED / REJECTED / NEEDS_REVIEW
```

### 4.3 Document Storage

- Documents stored in S3 with AES-256 server-side encryption
- Bucket policy: no public access, compliance service role only
- Object key format: `kyc/{participant_id}/{document_type}/{upload_timestamp}_{hash}.{ext}`
- Retention: 7 years after account closure (Mongolian regulatory requirement)
- Access logged to `compliance.audit_events`

---

## 5. Watchlist Screening

### 5.1 Screening Scope

| List Category | Source | Check Frequency |
|---|---|---|
| UN Sanctions | UN Security Council consolidated list | Every onboarding + daily delta |
| OFAC SDN | US Treasury SDN/SSI lists | Every onboarding + daily delta |
| EU Sanctions | EU consolidated sanctions list | Every onboarding + daily delta |
| Mongolia FIU | Mongolian Financial Intelligence Unit | Every onboarding + daily delta |
| PEP Lists | Politically Exposed Persons databases | Every onboarding + quarterly |
| Adverse Media | Structured news screening | Every onboarding + quarterly |

### 5.2 Screening Algorithm

```
Input: participant name, aliases, DOB, nationality, ID numbers

For each watchlist:
  1. Exact match on ID numbers (national ID, passport)
  2. Fuzzy name match (Jaro-Winkler distance ≥ 0.85)
  3. Phonetic match (Double Metaphone for Cyrillic transliteration)
  4. DOB proximity (± 2 years for fuzzy name matches)

Output per match:
  - match_score: 0.0 – 1.0
  - match_type: EXACT_ID | EXACT_NAME | FUZZY_NAME | PHONETIC
  - list_source: which sanctions list
  - listed_entity: the matched entry from the list
```

### 5.3 Match Disposition

| Match Score | Action |
|---|---|
| ≥ 0.95 | Auto-reject; escalate to compliance officer and FIU |
| 0.85 – 0.94 | Escalate to manual review |
| < 0.85 | Auto-clear (log result) |

### 5.4 Screening Provider Interface

The watchlist screening uses an adapter pattern to decouple from any specific provider:

```go
type ScreeningProvider interface {
    // Screen checks a subject against all configured watchlists.
    Screen(ctx context.Context, req ScreeningRequest) (*ScreeningResult, error)

    // GetListVersions returns current list version metadata for audit.
    GetListVersions(ctx context.Context) ([]ListVersion, error)
}

type ScreeningRequest struct {
    SubjectID    string
    FullName     string
    Aliases      []string
    DateOfBirth  *time.Time
    Nationality  string
    IDNumbers    []IDNumber
}

type ScreeningResult struct {
    ScreeningID  string
    Timestamp    time.Time
    Matches      []ScreeningMatch
    ListVersions []ListVersion
    Provider     string
}
```

---

## 6. Risk Scoring Model

### 6.1 Risk Factors

Risk score is a weighted composite of multiple factors, normalized to 0–100.

| Factor | Weight | Low (0–30) | Medium (31–60) | High (61–100) |
|---|---|---|---|---|
| **Participant type** | 15% | Individual, Cooperative | Corporate | Broker, Institutional |
| **Country risk** | 20% | Mongolia domestic | FATF grey-list countries | FATF blacklist, sanctioned |
| **Screening result** | 25% | No matches | Cleared after review | PEP match, adverse media |
| **Transaction profile** | 15% | Low volume, standard commodities | Moderate volume | High volume, unusual patterns |
| **Source of funds** | 15% | Salary, farming income | Business revenue | Complex structures, offshore |
| **Document quality** | 10% | All verified, consistent | Minor discrepancies | Expired, inconsistent data |

### 6.2 Score Calculation

```
risk_score = Σ (factor_score × factor_weight)

Where factor_score ∈ [0, 100] for each factor
and   Σ factor_weight = 1.0
```

### 6.3 Risk Tiers and Consequences

| Tier | Score Range | Onboarding | Monitoring | Re-screening |
|---|---|---|---|---|
| **Low** | 0 – 30 | Auto-approve | Annual | Annual |
| **Medium** | 31 – 60 | Auto-approve | Quarterly | Semi-annual |
| **High** | 61 – 80 | Manual review required | Monthly | Quarterly |
| **Prohibited** | 81 – 100 | Auto-reject | N/A | N/A |

### 6.4 Risk Score Versioning

- Risk model version stored with every score computation
- Scores are never overwritten; new scores are appended as new records
- Model version changes trigger batch re-scoring of all active participants

---

## 7. Ongoing Monitoring

### 7.1 Transaction Monitoring Rules

| Rule ID | Description | Trigger | Action |
|---|---|---|---|
| `TXN-001` | Single trade exceeds participant's daily limit | trade_value > daily_limit | Alert + hold if > 2× limit |
| `TXN-002` | Cumulative daily volume spike | daily_volume > 3× rolling 30-day avg | Alert |
| `TXN-003` | Structuring detection | Multiple trades just below reporting threshold within 24h | Alert + SAR filing |
| `TXN-004` | Wash trading pattern | Participant on both sides of a trade (via related accounts) | Alert + trade review |
| `TXN-005` | Dormant account activation | First trade after > 90 days inactivity | Re-screen |
| `TXN-006` | Cross-border payment | Settlement to non-Mongolian bank account | Enhanced review |

### 7.2 Periodic Re-screening

- **Daily**: Delta screening against updated sanctions lists
- **Quarterly**: Full re-screening of medium-risk participants + PEP refresh
- **Annually**: Full KYC refresh for low-risk participants (document re-validation)
- **On-event**: Any material change (ownership, address, beneficial owners) triggers immediate re-screening

### 7.3 Suspicious Activity Reporting

When monitoring rules trigger above threshold:

1. Generate internal alert → compliance officer queue
2. If confirmed suspicious → file SAR (Suspicious Activity Report) with Mongolia FIU
3. Optionally suspend participant trading (compliance officer decision)
4. All SAR filings logged in `compliance.audit_events` with hash chain

---

## 8. API Contracts

### 8.1 gRPC Services

The compliance service exposes three gRPC services defined in `src/compliance-service/proto/compliance.proto`:

1. **OnboardingService** — participant application lifecycle
2. **ScreeningService** — watchlist screening operations
3. **ComplianceAdminService** — compliance officer tools and reporting

### 8.2 REST-to-gRPC Mapping

| HTTP Method | Path | gRPC Method |
|---|---|---|
| POST | `/api/v1/participants` | `OnboardingService/SubmitApplication` |
| GET | `/api/v1/participants/{id}` | `OnboardingService/GetApplication` |
| GET | `/api/v1/participants` | `OnboardingService/ListApplications` |
| POST | `/api/v1/participants/{id}/documents` | `OnboardingService/UploadDocument` |
| GET | `/api/v1/participants/{id}/documents` | `OnboardingService/ListDocuments` |
| POST | `/api/v1/participants/{id}/approve` | `OnboardingService/ApproveApplication` |
| POST | `/api/v1/participants/{id}/reject` | `OnboardingService/RejectApplication` |
| POST | `/api/v1/screening/check` | `ScreeningService/ScreenParticipant` |
| GET | `/api/v1/screening/{id}` | `ScreeningService/GetScreeningResult` |
| POST | `/api/v1/screening/batch` | `ScreeningService/BatchScreen` |
| POST | `/api/v1/screening/{id}/resolve` | `ScreeningService/ResolveMatch` |
| GET | `/api/v1/risk-scores/{participant_id}` | `ScreeningService/GetRiskScore` |
| GET | `/api/v1/compliance/alerts` | `ComplianceAdminService/ListAlerts` |
| POST | `/api/v1/compliance/alerts/{id}/resolve` | `ComplianceAdminService/ResolveAlert` |
| GET | `/api/v1/compliance/audit-trail` | `ComplianceAdminService/GetAuditTrail` |
| POST | `/api/v1/compliance/sar` | `ComplianceAdminService/FileSAR` |

---

## 9. Data Model

### 9.1 Core Tables

The compliance service uses the `compliance` schema (created in T004 V3 migration). This spec adds the following tables via migration V7:

- `compliance.kyc_applications` — onboarding application records
- `compliance.kyc_documents` — document metadata (content in S3)
- `compliance.screening_results` — watchlist screening outcomes
- `compliance.screening_matches` — individual matches within a screening
- `compliance.risk_scores` — versioned risk score records
- `compliance.monitoring_alerts` — transaction monitoring alerts
- `compliance.sar_filings` — suspicious activity report records

### 9.2 Entity Relationships

```
participants.participants (T004)
       │
       ├──── 1:N ──── compliance.kyc_applications
       │                    │
       │                    ├──── 1:N ──── compliance.kyc_documents
       │                    │
       │                    ├──── 1:N ──── compliance.screening_results
       │                    │                    │
       │                    │                    └──── 1:N ──── compliance.screening_matches
       │                    │
       │                    └──── 1:N ──── compliance.risk_scores
       │
       └──── 1:N ──── compliance.monitoring_alerts
                            │
                            └──── 0:1 ──── compliance.sar_filings
```

---

## 10. Integration Points

### 10.1 Kafka Topics

| Topic | Producer | Consumers | Payload |
|---|---|---|---|
| `compliance.participant.status` | Compliance service | Exchange engine, Clearing | Participant approval/suspension events |
| `compliance.screening.completed` | Compliance service | Internal (alerting) | Screening outcome for audit |
| `compliance.alert.created` | Compliance service | Compliance dashboard | New monitoring alert |

### 10.2 Exchange Engine Integration

The exchange engine (T007/T008) must gate order submission on participant compliance status:

```
SubmitOrder flow:
  1. Gateway authenticates request (T005)
  2. Gateway calls ComplianceAdminService/CheckParticipantStatus
     → returns APPROVED / SUSPENDED / EXPIRED
  3. If not APPROVED → reject order with reason "COMPLIANCE_HOLD"
  4. If APPROVED → forward to matching engine
```

The compliance check should be cached in Redis with a TTL of 60 seconds to avoid per-order latency.

### 10.3 Clearing Integration

The clearing service (T027) consumes `compliance.participant.status` events to:
- Block settlement for suspended participants
- Trigger position liquidation if participant is rejected after having open positions

---

## 11. Regulatory Requirements

### 11.1 Mongolian Regulatory Framework

| Regulation | Requirement | Implementation |
|---|---|---|
| AML/CFT Law of Mongolia (2013, amended 2019) | Customer due diligence for all exchange participants | Full KYC workflow |
| FRC Regulation on Securities Market | Broker licensing and participant registration | Broker license verification in document pipeline |
| Bank of Mongolia AML Guidelines | Transaction monitoring and SAR filing | Monitoring rules + SAR workflow |
| FATF Recommendations | Risk-based approach to CDD | Tiered risk scoring model |

### 11.2 Data Retention

| Data Type | Retention Period | Legal Basis |
|---|---|---|
| KYC application records | 7 years post-account closure | AML/CFT Law Art. 6 |
| Document images | 7 years post-account closure | AML/CFT Law Art. 6 |
| Screening results | 7 years post-account closure | AML/CFT Law Art. 6 |
| Audit trail | 10 years | FRC Regulation |
| SAR filings | 10 years | AML/CFT Law Art. 7 |

---

## 12. Security & Privacy

### 12.1 Data Classification

| Field | Classification | Encryption |
|---|---|---|
| National ID number | PII – Restricted | AES-256 at rest, column-level encryption |
| Passport scan | PII – Restricted | AES-256 at rest (S3 SSE) |
| Name, address, DOB | PII – Confidential | AES-256 at rest |
| Risk score | Internal | Standard DB encryption |
| Screening result | Internal | Standard DB encryption |
| Audit trail hash | Public integrity | No encryption needed |

### 12.2 Access Control

| Role | Permissions |
|---|---|
| `compliance_officer` | Full read/write on KYC cases, approve/reject, file SARs |
| `compliance_viewer` | Read-only on KYC cases and screening results |
| `ace_compliance_svc` | Service account — DB read/write via application |
| `exchange_svc` | Read-only: participant compliance status check |
| `participant` | Self-service: own application status, document upload |

### 12.3 Audit Requirements

Every compliance action produces an append-only audit event:

```json
{
  "event_id": "uuid",
  "event_type": "KYC_APPLICATION_SUBMITTED | DOCUMENT_VERIFIED | SCREENING_COMPLETED | ...",
  "actor_id": "uuid (user or service)",
  "participant_id": "uuid",
  "details": { ... },
  "prev_hash": "sha256 of previous event",
  "hash": "sha256(event_id + event_type + actor_id + details + prev_hash)",
  "timestamp": "ISO 8601"
}
```

---

## 13. Failure Modes & Recovery

| Failure | Impact | Recovery |
|---|---|---|
| Screening provider unavailable | New onboarding blocked | Retry with exponential backoff; queue applications; alert compliance team if > 15 minutes |
| Document store (S3) unavailable | Document upload/retrieval fails | Return 503; retry; documents already verified remain valid |
| Database unavailable | All operations blocked | Standard PostgreSQL HA failover; application retries |
| Kafka unavailable | Status events not published | Buffer events locally; replay on reconnect; exchange engine falls back to direct DB check |
| Risk model computation error | Risk score not assigned | Default to MANUAL_REVIEW; alert engineering |

### Recovery Ordering

1. Restore database connectivity (all state lives here)
2. Process buffered Kafka events (eventual consistency)
3. Retry failed screening checks (idempotent by screening_id)
4. Resume document verification pipeline
