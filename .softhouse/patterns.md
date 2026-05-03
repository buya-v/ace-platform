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

### Run 20260423-cx-polish — Admin Dashboard CX Polish (2026-04-23)

- **What worked**:
  - 6/6 tasks completed on first attempt, zero rejections
  - CX score 9/10 — only minor styling inconsistency deducted
  - Level 0 parallelism (4 agents) with zero file conflicts due to careful task scoping
  - T3 (loading + toast) combined into single task to avoid page file conflicts — correct decision
  - T3 agent proactively added form validation to CircuitBreakers, going beyond the task spec
  - DataGrid already had `loading` prop with skeleton — task was just wiring, not building infrastructure
  - Toast system already existed (ToastProvider, useToast) — task was adoption, not creation

- **What failed**:
  - T1 and T2 (haiku model) worktrees were auto-cleaned before merge. Changes were committed directly to main branch by the agents, bypassing the worktree merge step. This worked but was uncontrolled.
  - T1 config fix broke 9 tests (window.location undefined in test env) — required a try/catch fallback fix after merge
  - T4 worktree had uncommitted changes that had to be manually copied

- **New knowledge**:
  - `usePolling` hook returns `{ data, isLoading, error, lastUpdated, refresh }` — pages only destructured `data` and `refresh`
  - Toast system: `useToast()` → `showToast(message, 'success'|'error'|'info')` — already provided by ToastContext wrapping DashboardLayout
  - DataGrid empty state: use `role="status" aria-live="polite"` for screen reader announcement
  - Responsive breakpoints: 768px (tablet/sidebar collapse), 480px (small mobile)
  - Sidebar collapse: hide text with `display: none` on `.navLabel`, keep icons centered at 56px width
  - `window.location` is undefined in Vitest jsdom environment — any code accessing it needs try/catch or conditional checks

- **Planning advice**:
  - CX polish tasks are highly parallelizable — config, ErrorBoundary, page wiring, component styling, and layout CSS all touch different files
  - Combining "loading" and "toast" into one task per-page avoids merge conflicts (same files)
  - haiku model agents may commit directly to main instead of worktree branches — use sonnet for tasks that need merge control
  - Always test config changes that access browser globals — they break in test environments

- **CX scores**: T6 CX review: 9/10
- **CX gaps found**: Missing aria-hidden on some decorative icons, missing form validation on CircuitBreakers, some hardcoded hex colors
- **CX improvements made**: Form validation with per-field errors, aria-invalid attributes, aria-hidden on icons, role/aria-live on empty state

### Run 20260423-prod-hardening — Production Hardening (2026-04-23)

- **What worked**:
  - 5/5 tasks completed on first attempt, zero rejections
  - Level 0 parallelism (4 agents) with clean merging — only one handoff file conflict (trivially resolved)
  - T2 (tracing) committed directly to main, avoiding worktree merge complexity
  - T4 confirmed health checks and Docker healthchecks already existed across all services — no code changes needed (just stub cleanup)
  - 79 Go packages + 404 admin-ui tests all passing after merge

- **What failed**:
  - T3 worktree had 0 branch commits (changes uncommitted) — had to copy files manually
  - T1 merge had a handoff file conflict (both T1 and T2 committed to main created parallel handoff files)
  - Go binary not in default PATH (/usr/local/go/bin/go) — agents need explicit PATH setup

- **New knowledge**:
  - src/shared/internal/observability/ now has: metrics.go (counters + histograms + MetricsServer), middleware.go (MetricsMiddleware), tracing.go (W3C traceparent), logger.go
  - All 9 Go services already had /healthz and /readyz with atomic readyFlag pattern
  - Docker Compose already had healthcheck directives using shared YAML anchor x-healthcheck-http
  - E2e tests had 3 bug classes: gateway auth ordering, compliance json tags, array vs object expectations
  - W3C traceparent format: 00-{trace_id 32hex}-{span_id 16hex}-{flags 2hex}

- **Planning advice**:
  - Backend observability tasks parallelize well — metrics, tracing, health checks touch different files
  - "Verify and enhance" tasks (T3, T4) are faster than "create from scratch" — existing code was better than expected
  - Always set PATH=$PATH:/usr/local/go/bin in Go task prompts for this environment
  - When T4 found everything already done, it still provided value by confirming the state — verification tasks are worth including

### Run 20260423-securities-specs — Securities Module Specs (2026-04-23)

- **What worked**:
  - 3/3 tasks completed on first attempt, zero rejections
  - Opus model for T1 (architecture spec) produced a ~1,100-line comprehensive spec with exact field names, enum values, SQL DDL, API contracts, and Kafka topics — implementation-ready quality
  - T2 + T3 ran in parallel after T1 dependency — clean serialization
  - All three agents committed directly to main (no worktree branch issues)
  - docs/securities-architecture.md serves as single source of truth — T2 and T3 consumed it directly

- **What failed**:
  - Nothing — clean run

- **New knowledge**:
  - Securities module scope: 18 database tables across 3 migrations (V26-V28)
  - OpenAPI spec: 21 endpoints, 36 named schemas, ~1,973 lines
  - T+2 settlement has 7 states (PENDING/AFFIRMED/NETTED/INSTRUCTED/SETTLING/SETTLED/FAILED)
  - CSD integration needs 8 corporate action types (DIVIDEND through SPIN_OFF)
  - Securities reuses existing gateway, auth, compliance — new: securities-service + CSD adapter

- **Planning advice**:
  - Specs-only runs are fast (3 tasks, ~20min total) and set up clean implementation sprints
  - Use opus for architecture specs that define data models — the precision on field names/types is critical
  - Migrations and OpenAPI can run in parallel since they consume the same spec but produce different files
  - Specs-only approach avoids the Phase 7-9 failure (25-task plan that timed out ACP)

### Run 20260423-securities-service — Securities Service Core (2026-04-23)

- **What worked**:
  - 5/5 tasks, zero rejections, 68 tests (store 95.3%, server 73.5%)
  - Specs-only run (previous sprint) paid off — agents referenced docs directly
  - All agents committed directly to main — no worktree merge issues
- **What failed**:
  - Port 8085 already used by auth-service — needs relocation to 8089
- **New knowledge**:
  - Securities-service at src/securities-service/, zero external deps
  - Order validation: instrument exists → ACTIVE → side → type → qty → lot_size → tick_size → stop_price
  - Handler tests need `package server` (internal) since handlers are unexported
- **Planning advice**:
  - Consider combining instrument + order handlers into one task (same package)
  - Sequential chain (4 levels) is clean but slow — 2 levels would suffice

### Run 20260503-ai-admin-ops — AI Admin & Ops Through Chatbot (2026-05-03)
- **What worked**: 6/6 tasks completed on first attempt, zero rejections. All implemented directly in main context after background agents were blocked by tool permissions.
- **What failed**: Background worktree agents cannot prompt for tool permissions — all 3 Level 0 agents (T1, T2, T3) were blocked. Switched to direct implementation in main context.
- **New knowledge**: Alias ordering matters for NLP command parsing — longer aliases must be checked before shorter ones (e.g., "halt all" before "halt"). Built a global alias index sorted by length.
- **Planning advice**: Do NOT use background agents when tool permissions haven't been pre-approved. Either run agents in foreground or implement directly. For small runs (6 tasks), direct implementation is faster than agent overhead.

<!-- LEARNED PATTERNS END -->
