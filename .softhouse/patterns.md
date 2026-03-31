# Softhouse Learned Patterns

<!-- LEARNED PATTERNS START -->

### Run 20260331 — GarudaX Production-Ready Commodity Exchange Refinement (2026-03-31)

- **What worked**:
  - 30 tasks across 8 phases, all completed on first attempt (zero rejections)
  - Massive parallelism: up to 9 agents running simultaneously in iteration 2
  - Worktree isolation prevented all merge conflicts between parallel tasks
  - Shared DB package (T101) as foundation → all 8 service DB tasks consumed it cleanly
  - `database/sql` + `pgx/v5/stdlib` pattern: every service uses the same driver approach, no shared module dependency needed
  - Pure function extraction for testability: SPAN scanner, DVP coordinator, fee calculator, surveillance detector, report generators — all easily testable without DB
  - In-memory fallback pattern (DB_HOST check) preserved all existing tests across all services

- **What failed**:
  - T119 (auction) overwrote T120 (iceberg) changes in `orderbook.go` — worktree isolation means parallel tasks editing the same file don't see each other's changes. Required manual reconciliation after merge.
  - T116 (Kafka) go.mod overwrote service go.mods that had pgx dependency from T102-T109. Required `go mod tidy` across 6 services after merge.
  - Planning should have serialized T119 and T120 via dependencies since both modify `orderbook.go`.

- **New knowledge**:
  - Go services use `database/sql` + `pgx/v5/stdlib` (not pgxpool) — simpler, no shared module needed
  - Redis client: `github.com/redis/go-redis/v9` works well for sessions and rate limiting
  - Kafka client: `github.com/segmentio/kafka-go` Writer is simple and sufficient for all services
  - SPAN margin model: 16 scenarios (4 price shifts × 2 vol shifts × 2 directions) is the standard
  - DVP settlement: 4-step sequence (validate→lock→confirm→release) with rollback
  - Performance baselines: 230K+ orders/sec matching, 754K+ novations/sec, <10ms p99 latency

- **Planning advice**:
  - Tasks modifying the same file MUST be serialized — `files_hint` overlap check is critical
  - When multiple tasks add go.mod dependencies, the last merge wins — run `go mod tidy` after merging
  - DB persistence tasks are highly parallelizable since each service is an independent Go module
  - Frontend tasks (React) parallelize well since they touch different page files
  - Estimate 5-8 min per task for this codebase (30 tasks completed in ~2.5 hours wall-clock)

<!-- LEARNED PATTERNS END -->
