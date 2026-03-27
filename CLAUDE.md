# Self-Learning Softhouse — Architecture Reference

This file is the persistent memory for an AI-driven software development pipeline.
Read this at the start of every session before planning or spawning agents.

---

## Architecture Overview

```
User requirement
  → Planner Agent        (task graph + dependency tree + estimates)
  → Orchestrator         (TaskCreate · spawn workers · monitor · merge branches)
      → Coder            (isolated worktree per task)
      → Test Writer      (isolated worktree per task)
      → Docs             (isolated worktree per task)
      → Security         (isolated worktree per task)
      → Arch Review      (isolated worktree per task)
  → Reviewer Agent       (code quality · security · approve/reject)
  → Integration Test     (full build · CI pass · coverage gate)
  → PostMortem Agent     (writes outcomes back to this file)
```

---

## Agent Roles

### Planner Agent
- Input: raw user requirement (natural language)
- Output: structured `tasks.json` with fields: `id`, `title`, `description`, `dependencies: []`, `estimate_minutes`, `worktree_branch`
- Always reads the **Learned Patterns** section below before generating the plan
- Uses `subagent_type: Plan`

### Orchestrator
- Reads `tasks.json` and creates tasks via `TaskCreate`
- Spawns one worker agent per task using `isolation: "worktree"`
- Runs independent tasks in parallel (background: true)
- Monitors completion via task notifications
- Merges approved branches only
- Checkpoints state into git after every agent completes — never hold state only in memory

### Worker Agents (Coder, Test Writer, Docs, Security, Arch Review)
- Each runs in its own isolated git worktree branch
- Writes a `handoff/<task-id>.md` on completion with: summary, decisions made, blockers found, suggested follow-ups
- Reads `handoff/` files from upstream tasks before starting

### Reviewer Agent
- Reads worker output + `handoff/<task-id>.md`
- Evaluates: correctness, security, style, test coverage
- Writes verdict to `handoff/<task-id>-review.md`: APPROVED / REJECTED + reason
- Rejected tasks are re-queued by the Orchestrator with reviewer notes injected into context

### Integration Test Agent
- Triggers after all branches merged
- Runs full build + test suite
- Writes pass/fail + coverage to `handoff/integration-<run-id>.md`
- Failure triggers PostMortem Agent

### PostMortem Agent
- Runs after every completed feature (success or failure)
- Extracts: what worked, what failed, what estimate was wrong, any new project-specific patterns
- **Appends findings to the Learned Patterns section of this file**
- Commits the updated `CLAUDE.md` to git

---

## State Management (Persistence Strategy)

Sessions do not survive terminal close. Compensate by externalizing all state:

| State | Storage location |
|---|---|
| Task graph | `tasks.json` (committed to git) |
| Agent handoffs | `handoff/<task-id>.md` (committed) |
| Review verdicts | `handoff/<task-id>-review.md` (committed) |
| Learned patterns | This file, **Learned Patterns** section |
| Session recovery | Read `tasks.json` + `handoff/` on startup to reconstruct context |

**Session startup checklist:**
1. Read this file (`CLAUDE.md`)
2. Read `tasks.json` — identify incomplete tasks
3. Read all `handoff/` files for completed tasks
4. Resume from last incomplete task

---

## Agent-to-Agent Communication

Agents cannot message each other directly. Use the shared `handoff/` directory:

```
handoff/
  task-3.md              # Coder output summary
  task-3-review.md       # Reviewer verdict
  task-5.md
  integration-run-1.md
```

Convention:
- Worker writes `handoff/<task-id>.md` on completion
- Reviewer writes `handoff/<task-id>-review.md`
- Downstream agents read upstream `handoff/` files before starting
- Orchestrator reads all `handoff/` files to track global state

---

## The Learning Loop

This is what makes it a softhouse rather than a pipeline.

After each feature:
1. PostMortem Agent reads: reviewer notes, integration results, test failures, timing deltas
2. Extracts reusable patterns specific to this codebase and domain
3. Appends to **Learned Patterns** below
4. Planner reads Learned Patterns on every new task → plans improve over time

The learning is not gradient descent — it is structured knowledge accumulation.
It compounds: week-1 plans are generic, week-8 plans are codebase-aware.

---

## Known Limitations

| Limitation | Current workaround |
|---|---|
| Sessions don't persist | Externalize all state to git (see above) |
| No agent-to-agent messaging | `handoff/` directory convention |
| No mid-flight plan restructuring | Orchestrator re-plans from scratch on critical failure; reads `handoff/` for context |
| Finite orchestrator context | Summarize completed task outputs; don't pass full file contents up |
| No external triggers (CI, Slack) | Manual session restart with `tasks.json` resume |

---

## Task JSON Schema

```json
{
  "tasks": [
    {
      "id": "task-1",
      "title": "Short title",
      "description": "Detailed description for the worker agent",
      "agent_role": "coder | test_writer | docs | security | arch_review",
      "dependencies": [],
      "blockedBy": ["task-0"],
      "estimate_minutes": 20,
      "status": "pending | in_progress | done | rejected",
      "worktree_branch": "feature/task-1-short-title"
    }
  ],
  "feature": "Human-readable feature name",
  "created_at": "ISO timestamp",
  "last_updated": "ISO timestamp"
}
```

---

## Learned Patterns

> PostMortem Agent appends new entries here after each completed feature.
> Planner Agent reads this section before every planning pass.

<!-- LEARNED PATTERNS START — do not remove this comment -->

### Pattern: Design-first tasks succeed; implementation tasks need code scaffolding (2026-03-27)
- **Context:** Phase 0 foundation run — 12 tasks planned, 3 completed (T001, T002, T004), 5 rejected (T003, T005, T006, T007, T015), 4 still pending.
- **Finding:** All 3 completed tasks produced documents or declarative artifacts (ADR, Terraform modules, SQL migrations). All 5 rejected tasks required either a running environment (T003 EKS/Istio, T006 CI/CD) or application code implementation (T005 Auth, T007 Exchange spec, T015 KYC spec). Rejected tasks had no review files (`*-review.md`), so rejection reasons are undocumented — a process gap.
- **Action:** Planner should ensure that implementation tasks (agent_role: builder) are not scheduled until the language-level scaffolding exists (go.mod, main.go, basic project structure). Add an explicit "project init" task per service before any builder task. Architect spec tasks (T007, T015) should depend on project init too, so specs can reference real package paths.

### Pattern: Pre-provision downstream dependencies in infrastructure tasks (2026-03-27)
- **Context:** T002 (Terraform) pre-created the IRSA OIDC provider and Karpenter role for T003 to consume.
- **Finding:** This forward-provisioning pattern meant T003 had clear inputs. T002's handoff explicitly listed what T003 should use. This is the right model even though T003 was ultimately rejected for other reasons.
- **Action:** Infrastructure tasks should always pre-provision IAM roles, secrets, and config that downstream tasks need, and list them explicitly in the handoff file under "Suggested Follow-ups."

### Pattern: Handoff cross-references are high-value — enforce them (2026-03-27)
- **Context:** T001, T002, and T004 handoffs all included specific "Suggested Follow-ups" naming downstream task IDs and which sections/outputs to reference.
- **Finding:** These cross-references (e.g., "T005 should use the `auth` schema", "T003 needs OIDC provider ARN from eks module output") create a dependency contract that downstream agents can act on without re-reading all upstream code.
- **Action:** Worker agent prompt should require a "Suggested Follow-ups" section in every handoff file, with at minimum: downstream task IDs, specific artifacts/outputs they should consume, and any constraints they must respect.

### Pattern: Reviewer must always produce a review file — even for rejections (2026-03-27)
- **Context:** 5 tasks were rejected (T003, T005, T006, T007, T015) but zero `handoff/*-review.md` files exist.
- **Finding:** Without review files, the PostMortem agent cannot root-cause rejections, and the Orchestrator cannot inject reviewer notes when re-queuing. The learning loop is broken for rejected tasks.
- **Action:** Orchestrator must enforce that every status transition to "rejected" is accompanied by a `handoff/<task-id>-review.md` file. If a task is rejected without a review file, flag it as a process violation before re-queuing.

### Pattern: Integration tests are vacuous without application code — gate on code existence (2026-03-27)
- **Context:** Integration test run `run-20260327-074658` passed with 0 tests, 0 failures, N/A coverage. All 7 services had only stub Dockerfiles.
- **Finding:** A green integration run with no tests gives false confidence. The integration agent correctly noted this but the result was still "PASS."
- **Action:** Integration test agent should report `SKIP` (not `PASS`) when no buildable source code or test files are found. Planner should not schedule integration runs until at least one service has compilable code and tests.

### Pattern: Estimate data is unusable without start/finish timestamps on completed tasks (2026-03-27)
- **Context:** T001, T002, T004 are marked "done" but have no `started_at` or `finished_at` fields. Only rejected tasks have timestamps, and those are all identical (instant rejection).
- **Finding:** Without timing data on successful tasks, the PostMortem agent cannot evaluate estimate accuracy, which means the Planner cannot calibrate future estimates.
- **Action:** Orchestrator must record `started_at` when a worker agent begins and `finished_at` when it completes (for both done and rejected). These fields should be mandatory in `tasks.json` for any non-pending task.

### Pattern: Learning loop is functional — second integration run self-corrected (2026-03-27)
- **Context:** Phase 0 second integration run (`run-20260327-075008`) followed the first run (`run-20260327-074658`) which incorrectly reported PASS with zero tests.
- **Finding:** The second run correctly reported `SKIP` instead of `PASS`, citing the learned pattern from the first postmortem. This confirms the PostMortem → CLAUDE.md → Agent read loop is working as designed.
- **Action:** Continue the pattern of having agents read Learned Patterns before execution. When a pattern corrects agent behavior successfully, note it so we can distinguish validated patterns from untested ones.

### Pattern: Rejected tasks produce no handoff files — workers need a fail-safe output (2026-03-27)
- **Context:** T003, T005, T006, T007, T015 were all rejected but none produced a `handoff/<task-id>.md` file. Only the completed tasks (T001, T002, T004) have handoff files.
- **Finding:** Workers that fail early (3-7 min vs 180-300 min estimate) exit without writing any handoff. This means neither the Reviewer, Orchestrator, nor PostMortem agent has any diagnostic data. Combined with the missing review files (Pattern #4 above), rejected tasks are completely opaque.
- **Action:** Worker agent prompt should mandate writing a handoff file as the FIRST action (with status "in-progress") and updating it on exit regardless of success/failure. The handoff should include: what was attempted, what blocked progress, and what prerequisites were missing. This is separate from the Reviewer's review file.

### Pattern: Parallel scheduling of tasks with shared missing prerequisites wastes all slots (2026-03-27)
- **Context:** T003 (EKS+Istio), T005 (Auth Service), and T006 (CI/CD) were all started in parallel at `2026-03-27T07:50:08Z`. All three failed within 7 minutes for the same root cause: no application code scaffolding exists.
- **Finding:** Running 3 agents in parallel when they share the same unmet prerequisite (no `go.mod`, no source files) means 3x the compute cost for zero output. A single "canary" task could have detected the blocker before committing the other two slots.
- **Action:** When the Orchestrator launches a batch of parallel tasks that share a common dependency (e.g., "compilable source code exists"), run ONE task first as a canary. If it fails within 20% of its estimate, hold the remaining tasks and surface the blocker to the Planner for re-planning.

### Pattern: Rejected task timing reveals instant-fail vs partial-progress — use this signal (2026-03-27)
- **Context:** All 5 rejected tasks completed in 3-7 minutes against estimates of 180-300 minutes. T003/T005/T006 took ~7 min; T007/T015 took ~3 min.
- **Finding:** A task finishing in <5% of its estimate is a strong signal that prerequisites were missing entirely, not that the task was difficult. This is qualitatively different from a task that runs for 80% of its estimate and then fails (which would indicate a real implementation problem).
- **Action:** Orchestrator should detect "instant rejection" (completion in <10% of estimate) and treat it as a prerequisite failure, not a task failure. Instead of re-queuing the same task, it should flag the missing prerequisite and ask the Planner to insert a new dependency task.

### Pattern: AI agent estimates need 10-20x reduction from human estimates (2026-03-27)
- **Context:** Phase 1-2 pipeline run — 10 tasks with timing data. Estimates ranged 180-480 minutes. Actual completion times ranged 5-21 minutes.
- **Finding:** Every completed task finished in <5% of its estimate. T008 (Matching Engine, est 480m) took 21 min. T027 (Clearing Engine, est 480m) took 8.5 min. T006 (CI/CD, est 180m) took 5 min. Spec tasks (T007, T015) took 8-10 min against 180-240m estimates. The estimates appear calibrated for human developers, not AI agents working in isolated worktrees.
- **Action:** Planner should use estimates of 10-30 minutes for AI agent tasks, not human-scale estimates. Reserve 180+ minute estimates only for tasks requiring external system interaction (cloud provisioning, CI pipeline runs). Use actual timing from this run as calibration: spec tasks ~10 min, service implementation ~15-20 min, infrastructure/CI ~5-10 min.

### Pattern: Zero-dependency Go module per service is the winning architecture pattern (2026-03-27)
- **Context:** Four exchange-core services (matching-engine, clearing-engine, margin-engine, settlement-engine) were each built as independent Go modules with zero external dependencies.
- **Finding:** All four built cleanly, passed all 165 tests, and achieved ~43% average coverage. The zero-dep approach eliminated version conflicts, simplified builds, and made each service independently testable. The identical `Decimal(18,4)` type was copied into each module — duplication but zero coupling.
- **Action:** Continue the zero-dep Go module pattern for new services. Accept Decimal type duplication across services rather than introducing a shared library (the coupling cost outweighs the DRY benefit at this stage). When a shared types library becomes warranted (5+ services duplicating), extract `pkg/types/decimal.go` as an internal module.

### Pattern: Port allocation convention — document to prevent collisions (2026-03-27)
- **Context:** Four services independently chose non-colliding port pairs: matching-engine (50051/8081), clearing-engine (50052/8082), margin-engine (50053/8083), settlement-engine (50054/8084).
- **Finding:** The pattern emerged naturally from handoff cross-references (each service noted the previous service's ports). Format is `gRPC: 5005x / health HTTP: 808x` where x increments per service.
- **Action:** Planner should assign port numbers in task descriptions for new services. Next available: 50055/8085. Maintain this table: matching=50051, clearing=50052, margin=50053, settlement=50054, auth=50055(suggested), compliance=50056(suggested), gateway=8080(HTTP), market-data=50057(suggested), warehouse=50058(suggested).

### Pattern: Spec-first then implement produces clean code with fewer review issues (2026-03-27)
- **Context:** T007 (Exchange Spec) → T008 (Matching Engine) pipeline. T015 (KYC/AML Spec) produced clean artifacts. Both spec tasks were APPROVED with zero required fixes.
- **Finding:** T008 implementation referenced T007's spec directly (protobuf contracts, SQL migration, matching algorithm pseudocode) and was APPROVED with zero required fixes and only non-blocking suggestions. By contrast, T003 (no spec, direct implementation of EKS+Istio) was REJECTED with 3 correctness bugs. Specs create a verifiable contract that downstream workers can implement against.
- **Action:** For any service with business logic (not pure infrastructure), Planner should create a spec task before the implementation task. Spec task produces: architecture doc, protobuf/API contracts, SQL migration. Implementation task consumes these as inputs.

### Pattern: Code review catches real bugs in infrastructure code — keep reviews mandatory (2026-03-27)
- **Context:** T003 (EKS+Istio) was REJECTED by the reviewer. T008 (Matching Engine) was APPROVED but with 4 non-blocking suggestions including an overflow risk.
- **Finding:** T003 review caught: (1) duplicate YAML key silently dropping `holdApplicationUntilProxyStarts: true` — a startup reliability bug, (2) wrong DestinationRule host pattern (`*.local` vs `*.svc.cluster.local`) — an mTLS routing bug, (3) variable type mismatch for node groups. These are subtle bugs that YAML/Terraform linting alone wouldn't catch. The reviewer agent is providing genuine value.
- **Action:** Never skip reviews for cost/speed reasons. For infrastructure code specifically, add a suggestion to include YAML lint and Terraform validate in the worker's pre-handoff checks so the reviewer can focus on semantic correctness rather than syntactic issues.

### Pattern: Rework pipeline works — T005 rejected then approved on second pass (2026-03-27)
- **Context:** T005 (Auth Service) was rejected in the first pass (~8.5 min, instant-fail pattern) then re-run and approved with a thorough review (JWT, PKCE, RBAC, bcrypt, parameterized SQL — all passing).
- **Finding:** The second run produced a complete, production-quality auth service with zero required fixes. The reviewer noted only non-blocking suggestions (PKCE refresh token gap, S256-only restriction, rate limiting). This confirms the rework loop works when the prerequisite issue (missing scaffolding) is resolved.
- **Action:** When re-queuing a previously rejected task, inject both the original rejection reason AND any new context (e.g., scaffolding now exists, upstream handoffs available) into the worker's prompt. The worker should not need to rediscover what went wrong.

### Pattern: Integration test run correctly distinguished SKIP vs PASS vs real PASS (2026-03-27)
- **Context:** Three integration runs: run-074658 (PASS with 0 tests — incorrect), run-075008 (SKIP — correct per learned pattern), run-080509 (PASS with 165 tests — genuinely correct).
- **Finding:** The third integration run validated 4 services (matching, clearing, margin, settlement) with 165 passing tests and ~43% average coverage. Coverage ranged from 23.7% (novation) to 89.3% (orderbook). The core matching logic (orderbook) has the highest coverage, which is appropriate for a financial system.
- **Action:** Set a coverage floor of 60% for business-critical packages (orderbook, position, novation, pnl) and 30% for infrastructure packages (server, config, store). The current ~43% average is acceptable for phase 1-2 but should increase as services mature.

<!-- LEARNED PATTERNS END — do not remove this comment -->

---

## File Layout Convention

```
project-root/
  CLAUDE.md              ← this file (session memory + learned patterns)
  tasks.json             ← current task graph
  handoff/               ← agent-to-agent communication
    task-<id>.md
    task-<id>-review.md
    integration-<run>.md
  pipeline/              ← AI pipeline orchestrator
    run.sh               ← main entry point (./pipeline/run.sh "requirement")
    lib/                 ← shell libraries (state, context, worktree, log)
    prompts/             ← agent role prompt templates
  src/                   ← application code
  tests/                 ← test suite
```

---

*This file is both documentation and executable memory.
Keep it committed. Keep it accurate. The pipeline is only as good as what is written here.*
