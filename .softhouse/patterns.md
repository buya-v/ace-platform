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

### Run 20260331-bot — GarudaX Admin Bot with MCP + Ticket System (2026-03-31)

- **What worked**:
  - 9/9 tasks completed on first attempt, zero rejections
  - Clean 4-iteration dependency chain: foundation (T201+T202) → services (T203+T204+T206+T207) → UI (T205+T208) → tests (T209)
  - MCP server + orchestrator as separate TypeScript packages worked well — clean separation of concerns
  - Gateway bot fallback pattern (keyword-based responses when orchestrator unavailable) makes the bot usable immediately without external AI services
  - Page-aware suggestions pattern is simple and effective — static data, no ML needed
  - BotContext + useReducer pattern integrates cleanly with existing React architecture
  - Test writer agent (T209) produced 73 tests covering all three layers (orchestrator, gateway, admin-ui)

- **What failed**:
  - Worktree cleanup race: some worktrees were cleaned up before merge, requiring files to already be staged from prior commits
  - T207 (MCP extension) worktree was cleaned up before explicit merge, but files were already in git from sharing the worktree with T203

- **New knowledge**:
  - MCP SDK: `@modelcontextprotocol/sdk` — use `server.tool()` for tools, `server.resource()` for resources, `StdioServerTransport` for CLI integration
  - GPT-nano routing: keyword-based fallback is sufficient for 90% of admin queries — nano classification is a nice-to-have enhancement
  - Claude CLI as orchestrator: `claude -p "prompt" --bare --max-budget-usd 0.10` works for headless bot usage
  - Bot chat panels: 380x500px fixed-position is the sweet spot for chat overlays
  - Ticket system: simple schema (tickets + comments) covers bug reports, feature requests, and support

- **Planning advice**:
  - TypeScript tasks (MCP, orchestrator) are faster than Go tasks (~3-5 min vs ~5-8 min)
  - Frontend component tasks (BotButton, BotChatPanel, BotTicketForm) can be in one task if they share a context provider
  - Test writer tasks should wait for ALL upstream code tasks, not just the direct dependency — they need the full picture
  - MCP server tools are highly parallel by domain — each tool file is independent

### Run 20260331-uat — Softhouse UAT Playwright Sub-Agents (2026-03-31)

- **What worked**:
  - 7/7 tasks completed, zero rejections
  - UAT framework (helpers.ts, page-checks.ts, api-checks.ts) reused across all 4 test suites
  - Sonnet model sufficient for all Playwright test-writing tasks
  - SPA-safe navigation pattern (sidebar clicks, not page.goto) prevents React auth token loss
  - Soft assertions (expect.soft) allow collecting all failures without early abort
  - Graceful skip pattern (isPortalAvailable) keeps CI green when portal is down

- **What failed**:
  - T607 (skill creation) couldn't write outside worktree sandbox (/home/vcp/.claude/skills/) — had to be completed by orchestrator directly
  - T602 agent found skill file at SKILL.md not softhouse.md — naming convention varies across skills

- **New knowledge**:
  - Playwright toHaveScreenshot() auto-creates baselines on first run — no manual setup needed
  - UAT test count: 89 total (18 pages + 12 bot + 47 API contracts + 12 visual regression)
  - Skills directory: /home/vcp/.claude/skills/<name>/SKILL.md is the convention
  - Worktree agents cannot write to paths outside the repo — skill files must be created by orchestrator

- **Planning advice**:
  - Tasks writing to /home/vcp/.claude/ should NOT use worktree isolation — orchestrator must handle them directly
  - Playwright UAT tests are fast to write (~2-3 min per spec file) because helpers abstract the complexity
  - UAT step 5.5 in the softhouse pipeline is the key quality gate — unit tests alone are insufficient
  - Visual regression baselines need an initial --update-snapshots run before they become useful

- **UAT pass rate**: 7/7 tasks completed on first try
- **UAT failure categories**: N/A (framework creation, not execution)

<!-- LEARNED PATTERNS END -->
