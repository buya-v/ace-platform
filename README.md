# ACE Platform — Agriculture Commodity Exchange

Full-scale commodity exchange for Mongolia — physical delivery + financial settlement.

**Target:** Ulaanbaatar, MNT currency  
**Delivery:** AI agent-driven development, 9 phases, 18 calendar weeks  
**Stack:** Go (matching engine) · Java/Kotlin (services) · React (frontend) · PostgreSQL · TimescaleDB · Kafka · Redis · EKS/Istio · Terraform

---

## Architecture

- **Cloud:** AWS dual-region active/passive (Tokyo primary, Singapore DR)
- **Orchestration:** EKS + Istio service mesh with mTLS
- **Database:** PostgreSQL 15 (OLTP) + TimescaleDB (tick data) + Redis 7 (cache)
- **Messaging:** Apache Kafka 3.5 via MSK
- **IaC:** Terraform 1.6+ with workspace-based environments (dev/staging/prod)

See [`docs/adr/`](docs/adr/) for Architecture Decision Records.

---

## Repository Structure

```
ace-platform/
├── CLAUDE.md                    # AI pipeline memory + learned patterns
├── AiX_Project_Knowledge.md     # Project knowledge base (all phases/tasks)
├── tasks.json                   # Current task graph (machine-readable)
├── handoff/                     # Agent-to-agent communication
│   ├── <task-id>.md             # Worker completion summaries
│   └── <task-id>-review.md      # Reviewer verdicts
├── docs/
│   ├── adr/                     # Architecture Decision Records
│   └── specs/                   # Phase architecture specs
├── infrastructure/
│   ├── terraform/
│   │   ├── modules/             # vpc, eks, rds, msk, etc.
│   │   └── environments/        # tfvars per environment
│   └── db/
│       └── migrations/          # Flyway SQL migrations (V1–V5+)
├── src/                         # Application source code
│   ├── matching-engine/         # Go — CLOB + order matching
│   ├── auth-service/            # Auth & IAM microservice
│   ├── clearing-service/        # Clearing engine
│   └── ...                      # Additional microservices
├── tests/                       # Test suites
└── deploy/                      # K8s manifests, Helm charts
```

---

## Development Phases

| Phase | Name                       | Weeks | Status      |
|-------|----------------------------|-------|-------------|
| 0     | Foundation & Infrastructure | 1–2   | In progress |
| 1     | Exchange Engine            | 2–6   | Pending     |
| 2     | Market Participants        | 3–6   | Pending     |
| 3     | Physical Commodity Layer   | 5–8   | Pending     |
| 4     | Financial Infrastructure   | 6–10  | Pending     |
| 5     | Compliance & Regulation    | 8–11  | Pending     |
| 6     | Market Data & Analytics    | 9–12  | Pending     |
| 7     | Frontend Interfaces        | 10–15 | Pending     |
| 8     | Integrations & Launch      | 14–18 | Pending     |

**Critical path:** Ph0 → Ph1 → Ph4 → Ph5

---

## Completed (Phase 0)

- **T001** — Cloud Architecture Design (ADR-001) ✅
- **T002** — Terraform IaC Modules (5 modules) ✅
- **T004** — Core Database Schema (5 Flyway migrations) ✅

## Next Up

- **T003** — EKS Cluster + Istio Service Mesh
- **T005** — Auth & IAM Service
- **T006** — CI/CD Pipeline
- **T007** — Exchange Engine Architecture Spec *(critical path)*

---

## Agent Workflow

This project uses the **Self-Learning Softhouse** pattern (see `CLAUDE.md`):

1. **Planner** reads requirements + learned patterns → produces `tasks.json`
2. **Orchestrator** spawns worker agents in isolated git worktrees
3. **Workers** (Coder/QA/Docs/Security) write code + `handoff/` summaries
4. **Reviewer** approves or rejects with notes
5. **PostMortem** extracts patterns → appends to `CLAUDE.md`

The learning loop means plans improve as the codebase grows.

---

## Getting Started

```bash
# Clone
git clone <repo-url> && cd ace-platform

# Check current task state
cat tasks.json | jq '.tasks[] | select(.status != "done") | {id, title, status}'

# Resume from where agents left off
# 1. Read CLAUDE.md
# 2. Check tasks.json for incomplete tasks
# 3. Read handoff/ for completed context
```

---

## License

Proprietary — Agriculture Commodity Exchange Platform
