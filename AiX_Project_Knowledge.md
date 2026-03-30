# GarudaX Platform — AI Powered Commodity Exchange
## Project Knowledge Base

---

## 1. PROJECT OVERVIEW

**Project name:** AI Powered Commodity Exchange (GarudaX)
**Platform type:** Full-scale commodity exchange — physical delivery + financial settlement
**Target market:** Mongolia (Ulaanbaatar, MNT currency)
**Delivery method:** AI agent-driven development, 9 phases, 18 calendar weeks
**Total tasks:** 49 tasks across 9 phases
**Agent types:** Architect · Builder · QA · DevOps

---

## 2. DEVELOPMENT PLAN — 9 PHASES

| Ph | Name | Weeks | Agents | Key modules |
|----|------|-------|--------|-------------|
| 0 | Foundation & infrastructure | 1–2 | 2 | Cloud arch, IaC, DB schema, Auth |
| 1 | Exchange engine | 2–6 | 4 | CLOB, matching engine, price discovery, trade ledger |
| 2 | Market participants | 3–6 | 2 | KYC/AML, farmer/coop accounts, broker management |
| 3 | Physical commodity layer | 5–8 | 3 | eWR system, grading, inspection, delivery scheduling |
| 4 | Financial infrastructure | 6–10 | 4 | Clearing engine, SPAN margin, daily settlement, payment gateway |
| 5 | Compliance & regulation | 8–11 | 2 | Trade reporting, position limits, audit trail, regulator portal |
| 6 | Market data & analytics | 9–12 | 2 | Real-time feed, tick store, commodity indices, analytics API |
| 7 | Frontend interfaces | 10–15 | 3 | Web terminal, mobile app, broker portal, admin dashboard |
| 8 | Integrations & launch | 14–18 | 3 | Bank APIs, FIX gateway, govt feeds, load test, go-live |

**Critical path (Stream A):** Ph0 → Ph1 → Ph4 → Ph5
**Parallel stream B:** Ph2 → Ph3 → Ph6
**Parallel stream C:** Ph7 → Ph8 (starts mid-build)

**Highest-risk seam:** T027→T028→T029 (clearing engine → SPAN margin → daily settlement)

---

## 3. COMPLETED TASKS

### T001 — Cloud Architecture Design ✅
**Status:** ACCEPTED (ADR-001)
**Deliverables:** ADR Word doc, variables.tf, prod.tfvars, staging.tfvars

**Decision:** AWS dual-region active/passive
- Primary: ap-northeast-1 (Tokyo) — lowest latency to Mongolia
- DR: ap-southeast-1 (Singapore) — warm standby
- RTO = 15 min, RPO = 1 min

**Infrastructure stack:**
- EKS (Kubernetes) with Istio service mesh
- RDS PostgreSQL 15 Multi-AZ (db.r6g.2xlarge, 1TB gp3)
- TimescaleDB on EC2 r6i.4xlarge (tick data)
- ElastiCache Redis 7 cluster mode (6 shards)
- Amazon MSK Kafka 3.5 (3 brokers)
- S3 with CRR to DR region

**5 EKS node groups:**
- `system` — m5.large × 3, fixed
- `exchange-core` — c5.2xlarge × 3–12, CPU-optimised, tainted
- `app-general` — m5.xlarge × 3–20
- `data-processing` — r5.xlarge × 2–8, memory-optimised, tainted
- `spot-burst` — m5.xlarge × 0–10, Spot instances

**Security:** IRSA (no long-lived credentials), KMS CMK for all stores, WAF on ALBs, IMDSv2 enforced, VPC Flow Logs, CloudTrail

**AWS cost estimate:** $9,210/month at launch scale (cap: $12,000/month)

---

### T001 — On-Premise Infrastructure Budget ✅
**Basis:** 1,000 orders/day capacity (hardware is 10–50× over-provisioned for growth)
**Deliverable:** 5-sheet Excel workbook (Hardware / Software / OpEx / TCO Summary / Sizing Notes)

**Hardware CapEx (Year 1, key items):**
- App servers: 2× Dell PowerEdge R650 (128GB RAM, NVMe) — $16,400
- DB servers: 2× Dell PowerEdge R750 (512GB RAM, 4×NVMe) — $29,000
- Matching engine: 2× Supermicro (Xeon Gold 6342, 256GB) — $23,600
- Kafka brokers: 3× Dell R650 (128GB, 6×2TB HDD) — $19,200
- NAS: 2× Synology RS3621RPxs + drives — $19,600
- Tape library: Quantum Scalar i3 LTO-9 — $12,000
- Generator: 20kVA diesel standby — $8,500

**Software (mostly open source):**
- PostgreSQL, TimescaleDB, Kafka, Kubernetes: $0
- RHEL 9 (22 socket pairs): $3,200/yr
- Veeam Backup: $2,800 (perpetual)
- Fortinet UTM subscription: $1,800/yr
- Qualys VMDR: $2,400/yr

**Annual OpEx (Year 1):** ~$87,000
- IT staff (2 FTE equivalent): $39,600
- Connectivity (1Gbps + failover + MPLS): $14,760
- Power (5.5kW avg × PUE 1.2): $4,557
- Vendor support contracts: $10,280

**3-Year TCO:** ~$195,000 total
**Crossover vs AWS cloud:** ~14–16 months

**Power sizing:** 4,565W IT load → 5,478W with PUE 1.2 cooling = 47,987 kWh/year

---

### T002 — Terraform IaC Modules ✅
**Status:** Complete (5 modules)
**Deliverable:** `infrastructure/terraform/` directory

**Modules produced:**
- `modules/vpc/` — VPC, 3-tier subnets (public/app/data) per AZ, NAT GWs, IGW, route tables, VPC endpoints (S3 gateway + 7 interface endpoints), VPC Flow Logs
- `modules/eks/` — EKS cluster, 5 node groups, OIDC provider for IRSA, IMDSv2 launch templates, Karpenter IRSA role, core add-ons (vpc-cni, coredns, ebs-csi)
- `modules/rds/` — RDS PostgreSQL, parameter group (tuned for exchange: idle_in_transaction_timeout=30s, log_lock_waits=on), enhanced monitoring, CloudWatch alarms
- `modules/msk/` — MSK Kafka, TLS+IAM auth, 12 topic definitions stored in SSM, CloudWatch alarms for consumer lag and disk
- `modules/security-groups/` — least-privilege SGs: data tier has no internet route; RDS/Redis/MSK only accept traffic from EKS node SG

**State backend:** S3 + DynamoDB lock in eu-west-1 (out-of-band)
**Workspaces:** dev / staging / prod (same modules, different tfvars)
**Required tags on every resource:** Project, Environment, Phase, TaskID, ManagedBy, CostCenter

---

### T004 — Core Database Schema ✅
**Status:** Complete (5 Flyway migrations)
**Deliverable:** `infrastructure/db/migrations/V1–V5`
**Database:** PostgreSQL 15 + TimescaleDB extension

**Schemas:**
- `reference` — commodity catalog, grades, delivery locations, trading calendar
- `participants` — accounts, KYC/AML, watchlist screening, trading limits
- `exchange` — orders, trades (append-only), market sessions, CLOB snapshots, circuit breakers
- `clearing` — positions, margin calls, settlement instructions
- `compliance` — audit trail (append-only + SHA-256 hash chain), position limits, breach tracking
- `warehouse` — electronic warehouse receipts (eWR), inspection orders
- `auth` — users, API keys, sessions
- `market_data` — TimescaleDB hypertables (ticks), continuous aggregates (1m/1h/1d OHLCV)

**Key schema decisions:**
- Trades table: DELETE/UPDATE blocked by Postgres RULES (append-only enforcement)
- Audit trail: append-only + SHA-256 hash chain for tamper detection
- eWR: hash_chain field for receipt integrity
- Orders: `remaining_quantity` as GENERATED ALWAYS AS computed column
- TimescaleDB: 1-day chunks, 90-day raw tick retention, 7-day compression, continuous aggregates auto-refresh
- Service roles scoped per domain: `ace_exchange_svc`, `ace_clearing_svc`, `ace_compliance_svc`, `ace_warehouse_svc`, `ace_analytics_ro`

**Seed data (V5):**
- 14 commodities: wheat (HRW/SRW), barley, oats, corn, rapeseed, sunflower, soybean, cattle, sheep, camel, cashmere, wool, flour
- 7 delivery locations across Mongolia provinces
- 2025–2026 trading calendar with Mongolian public holidays
- Circuit breaker configs: ±5% (15-min halt) and ±10% (60-min halt) per commodity
- Exchange and CCP system accounts

---

## 4. DOMAIN MODEL — KEY ENTITIES

```
Commodity → CommodityGrade → DeliveryLocation
Account (FARMER/BROKER/WAREHOUSE_OPERATOR/CLEARING_MEMBER)
  └── TradingLimit
  └── KYCDocument
  └── WatchlistScreening

MarketSession → Order → Trade → SettlementInstruction
                     ↓
              OrderBookSnapshot (periodic CLOB snapshots)

Trade → ClearingPosition → MarginCall
     → SettlementInstruction

WarehouseReceipt (eWR) → InspectionOrder → CommodityGrade
                       → ReceiptHistory (append-only log)

AuditEvent (append-only, hash-chained)
PositionLimit → PositionLimitBreach
```

---

## 5. NEXT TASKS (UNLOCKED)

**Week 2 — currently in progress:**
- **T003** — EKS cluster + Istio service mesh (DevOps, dep: T002)
- **T005** — Auth & IAM service (Builder, dep: T004)
- **T006** — CI/CD pipeline (DevOps, dep: T002)

**Week 3 onward:**
- **T007** — Exchange engine architecture spec (Architect, dep: T004) — CRITICAL PATH
- **T008** — Order matching engine (Builder, dep: T007) — HIGH RISK
- **T015** — KYC/AML architecture spec (Architect, no dep)

---

## 6. AGENT PROMPT STRATEGY

**Architect agents** run first. Input: business requirements + prior ADRs. Output: spec document + API contracts. This becomes the contract for all builder agents in the phase.

**Builder agents** receive: spec doc + relevant schemas + OpenAPI contract. Produce: single microservice Docker image. One service per agent invocation — never mix concerns.

**QA agents** receive: all service contracts + scenario library. Produce: test suite. Always the last task before a phase closes.

**DevOps agents** run first (bootstrap) and last (go-live). Own: IaC, CI/CD, deployment configs, runbooks.

---

## 7. TECH STACK SUMMARY

| Layer | Technology |
|-------|-----------|
| Container orchestration | Kubernetes (EKS) + Istio |
| Languages (expected) | Go (matching engine), Java/Kotlin (services), React (frontend) |
| Primary DB | PostgreSQL 15 |
| Tick data | TimescaleDB (PostgreSQL extension) |
| Cache | Redis 7 cluster mode |
| Message bus | Apache Kafka 3.5 |
| Object storage | S3 |
| IaC | Terraform 1.6+ (modular, workspace-based) |
| Migrations | Flyway |
| Monitoring | Prometheus + Grafana + ELK |
| CI/CD | GitHub Actions (T006) |
| Secrets | AWS Secrets Manager + SSM Parameter Store |
| Auth | JWT + OAuth2 PKCE + RBAC + IRSA |
| FIX protocol | FIX 4.4 (T053 external broker gateway) |

---

## 8. FILE DELIVERABLES (THIS SESSION)

| File | Task | Description |
|------|------|-------------|
| `T001_ADR-001_Cloud_Architecture.docx` | T001 | Full architecture decision record, 12 sections |
| `T001_variables.tf` | T001 | Terraform variable definitions (all environments) |
| `T001_prod.tfvars` | T001 | Production environment values |
| `T001_staging.tfvars` | T001 | Staging environment values |
| `T001_OnPrem_Infrastructure_Budget.xlsx` | T001 | 5-sheet on-premise budget model, 144 formulas |
| `T002_T004_deliverables.zip` | T002+T004 | Terraform modules + DB migration SQL files |
