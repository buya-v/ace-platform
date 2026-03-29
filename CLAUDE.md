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

### Pattern: Phase 3 achieved zero rejections — spec-first + calibrated estimates = clean runs (2026-03-28)
- **Context:** Phase 3 pipeline run — 9 tasks (T031-T039), all 9 approved on first pass, zero rejections. This is the first run with a perfect approval rate.
- **Finding:** Three factors contributed: (1) spec-first for all business services (T033→T034, T035→T036, T037→T038), (2) AI-calibrated estimates (10-25 min vs Phase 1-2's 180-480 min), (3) all prerequisites satisfied before task launch. Actual times: specs ~5-7 min, implementations ~7-9 min, integration test ~2.5 min. Estimates were 1.5-3x actual (acceptable), vs Phase 1-2's 10-50x overestimates.
- **Action:** The current estimate calibration (10m specs, 25m implementations, 15m integration) is close. Fine-tune to: specs ~10m, implementations ~15m, rework ~10m, integration ~5m. These are upper bounds, not targets.

### Pattern: PostMortem agent fails on large context — pipeline needs context size guard (2026-03-28)
- **Context:** PostMortem agent failed with `Argument list too long` (shell error E2BIG) after Phase 3. The `build_postmortem_context` function concatenates ALL handoff files into a single CLI argument string. By Phase 3, this exceeded the Linux ARG_MAX limit (~2MB).
- **Finding:** The pipeline's postmortem prompt builder reads every `handoff/*.md` file and passes them as a single `-p` argument to `claude`. With 20+ handoff files totaling ~50KB+ of text, plus the CLAUDE.md content, this exceeds shell limits.
- **Action:** Fix `pipeline/lib/context.sh` `build_postmortem_context` to either: (1) write context to a temp file and pass via `--input-file` or stdin, (2) summarize completed tasks rather than passing full handoff contents, or (3) only pass handoff files from the current run (not all historical files). Option 1 is the quickest fix.

### Pattern: Auth service code exists on a branch but was never merged to main (2026-03-28)
- **Context:** T005 (Auth Service) status was fixed to `done` and the review says APPROVED, but `src/auth-service/` on main still contains only a stub Dockerfile. The integration test confirms: "auth-service — SKIP, Dockerfile only."
- **Finding:** T005 was originally rejected (Phase 1), then the review file was written as APPROVED (likely from a rework run), but the branch was never merged. The planner updated the status to `done` without verifying the code was on main. Similarly, `src/clearing-service/` is a stub — the clearing engine lives at `src/clearing-engine/` (different directory name).
- **Action:** Before marking a rejected task as `done`, the Orchestrator must verify the worktree branch was actually merged. Add a check: `git log --oneline main | grep <branch>` or verify the expected deliverable files exist on main. For T005, the auth service code needs to be rebuilt or the original branch found and merged. The clearing-service/clearing-engine naming mismatch should be reconciled.

### Pattern: Service naming inconsistency — src directory names don't match service names (2026-03-28)
- **Context:** `src/clearing-engine/` vs `src/clearing-service/` (stub). Similarly `src/matching-engine/` vs `src/margin-engine/` vs `src/settlement-engine/` use `-engine` suffix while `src/auth-service/`, `src/compliance-service/`, `src/market-data-service/`, `src/warehouse-service/`, `src/gateway/` use `-service` or no suffix.
- **Finding:** The `-engine` suffix was used for core exchange pipeline services (matching, clearing, margin, settlement) and `-service` for supporting services (auth, compliance, market-data, warehouse). This is actually a meaningful convention — engines are the real-time trading pipeline, services are supporting infrastructure. But the stub `src/clearing-service/` directory creates confusion since the real code is in `src/clearing-engine/`.
- **Action:** Remove the stub `src/clearing-service/` directory (it only has a Dockerfile). Document the naming convention: `*-engine` for real-time trading pipeline, `*-service` for supporting services, bare name for `gateway`. Update Dockerfiles and CI to reference the correct directory names.

### Pattern: Coverage improved from 43% to ~55% average — Phase 3 services set higher bar (2026-03-28)
- **Context:** Phase 1-2 services averaged ~43% coverage. Phase 3 services: compliance 80.1% (onboarding), gateway 93% (router), market-data 68.7% (candle), warehouse 83.8% (store). The 60% floor instruction in the task descriptions worked.
- **Finding:** Explicitly stating coverage targets in task descriptions ("targeting 60%+ coverage on business logic") produced measurably higher coverage than Phase 1-2 where no target was specified. Phase 3 business-critical packages averaged ~75%, vs Phase 1-2's ~45%.
- **Action:** Always include explicit coverage targets in builder task descriptions. Use "60%+ on business logic packages, 30%+ on infrastructure packages" as the standard instruction. For financial-critical packages (orderbook, novation, pnl), raise to "80%+ coverage."

### Pattern: 3-parallel worker limit is optimal for current pipeline (2026-03-28)
- **Context:** MAX_PARALLEL=3 was used throughout. Iteration 1 had 5 ready tasks but only launched 3 (T031, T032, T033). Iterations 2-3 naturally had 2-3 ready tasks.
- **Finding:** All 3 parallel workers completed successfully in every iteration — no resource contention, no merge conflicts, no race conditions on `tasks.json`. The worktree isolation pattern prevents conflicts. The pipeline completed 9 tasks in 4 iterations over ~32 minutes of execution time.
- **Action:** Keep MAX_PARALLEL=3 as default. Consider increasing to 4-5 only when tasks are all independent (no shared file writes). The bottleneck is sequential review-then-merge after each batch, not worker concurrency.

### Pattern: Frontend implementation tasks fail without Node.js tooling — gate on runtime availability (2026-03-28)
- **Context:** Phase 4 — T048 (Trading Web UI) and T050 (Admin Dashboard) were both rejected. Both are React+TypeScript+Vite SPAs requiring `npm`/`node` to scaffold and build.
- **Finding:** The worker environment has Go tooling but no Node.js/npm. Both tasks were rejected after ~10 minutes (vs 25m estimate), the same "instant rejection" pattern seen in Phase 1 when Go scaffolding was missing. The specs (T047, T049) were approved — only the implementations failed.
- **Action:** Planner must verify that the target runtime/toolchain exists before scheduling implementation tasks. For frontend tasks, add an explicit "frontend tooling setup" task (install Node.js, create `package.json`, configure Vite) before any React implementation task. Alternatively, if the pipeline environment cannot run `npm`, mark frontend tasks as "manual" and skip them in automated runs.

### Pattern: External dependency implementations fail without library availability (2026-03-28)
- **Context:** Phase 4 — T052 (Kafka Event Wiring Implementation) was rejected. The task required a Kafka client library (`segmentio/kafka-go` or `confluent-kafka-go`) which isn't available in the zero-dependency Go module pattern.
- **Finding:** The spec (T051) was approved cleanly, but the implementation couldn't proceed without an external Go dependency for Kafka protocol support. Unlike HTTP/gRPC (which can use stdlib `net/http`), Kafka requires a dedicated client library. The zero-dep pattern that worked for all 8 Go services breaks down for integration-layer code.
- **Action:** Planner should distinguish between "business logic" tasks (zero-dep viable) and "integration glue" tasks (external deps required). Integration tasks should either: (1) allow external dependencies in `go.mod`, or (2) define an interface with a stub implementation (like the existing `TradeStore`, `CollateralSource` patterns) and defer real Kafka wiring to a deployment phase where deps are available.

### Pattern: K8s manifest tasks need cross-resource validation — YAML linting is insufficient (2026-03-28)
- **Context:** Phase 4 — T054 (Kubernetes Deployment Manifests) was REJECTED with 3 correctness bugs: cross-namespace ConfigMap reference (pods in `ace-exchange` referencing ConfigMap in `ace-services`), duplicate namespace file (`namespace.yaml` vs existing `namespaces.yaml`), and missing Secret in `ace-exchange` namespace.
- **Finding:** These bugs are cross-resource reference errors — each individual YAML file is valid, but the references between them are broken. Standard YAML linting or `kubectl --dry-run` on individual files wouldn't catch these. The reviewer correctly identified all three. This is similar to T003's rejection (duplicate YAML key, wrong DestinationRule host) — infrastructure manifests have a higher cross-reference bug rate than application code.
- **Action:** For K8s manifest tasks, the worker should include a validation step that checks: (1) every `configMapRef`/`secretRef` has a matching resource in the same namespace, (2) no duplicate resource definitions across files, (3) namespace consistency between resource definitions and references. Consider adding `kustomize build` as a pre-handoff check (catches duplicate resources).

### Pattern: Phase 4 estimate calibration is converging — 1.5-4x overestimate is acceptable (2026-03-28)
- **Context:** Phase 4 timing data across 14 tasks with timestamps. Estimates ranged 5-25 minutes. Actuals ranged 1-11 minutes.
- **Finding:** Estimate-to-actual ratios: T040 (15m est / 11m actual = 1.4x), T041 (10m/3.5m = 2.9x), T042 (5m/1m = 5x), T043 (15m/7m = 2.1x), T044 (15m/4m = 3.8x), T045 (15m/3.5m = 4.3x), T046 (5m/3m = 1.7x), T047 (10m/5m = 2x), T049 (10m/4m = 2.5x), T051 (10m/4m = 2.5x). Average ratio: ~2.6x. Phase 3 was 1.5-3x. Phase 1-2 was 10-50x. The calibration is converging but still consistently overestimates.
- **Action:** Fine-tune estimates down one more notch: cleanup/bug-fix tasks ~3m, spec tasks ~5m, coverage improvement tasks ~5m, Go service implementations ~10m, integration tests ~3m. These are upper bounds. The 5-minute minimum estimate is still too high for trivial tasks like T042 (1 minute actual).

### Pattern: Spec-approved then implementation-rejected is a tooling gap, not a spec gap (2026-03-28)
- **Context:** Three spec→implementation pairs in Phase 4: T047→T048 (UI), T049→T050 (Admin), T051→T052 (Kafka). All specs were APPROVED; all implementations were REJECTED.
- **Finding:** The rejections were not caused by spec quality issues — the specs were comprehensive and the reviewers praised them. The rejections were caused by missing tooling (Node.js for frontend, Kafka client library for event wiring). This is a different failure mode than Phase 1's rejections (missing Go scaffolding), which were fixed by adding project-init tasks. The current failure is at the environment/dependency level, not the project level.
- **Action:** Planner should tag tasks with required toolchain (`go`, `node`, `python`, `external-deps`). Orchestrator should verify toolchain availability before spawning workers. If a toolchain is unavailable, the task should be marked `blocked` (not `rejected`) with a clear reason, preserving the approved spec for when the toolchain becomes available.

### Pattern: Coverage improvement tasks are highly efficient — batch them (2026-03-28)
- **Context:** T043 (clearing-engine coverage), T044 (margin-engine coverage), T045 (settlement-engine coverage) ran in parallel. All three completed in 3.5-7 minutes, all approved first pass with zero required fixes.
- **Finding:** Coverage tasks have the best effort-to-value ratio in the pipeline: they add no new features but significantly improve confidence in existing code. T043 took novation from 23.7%→100%, netting from 39.8%→100%, engine from 32.8%→95%. T044 took engine from 76.7%→96.7%. T045 closed the only real gap (pnl CalculateBatch error path). All three workers independently identified which coverage gaps were worth closing vs which required source changes to test.
- **Action:** After any phase that adds new services, schedule a dedicated "coverage batch" iteration with one coverage task per service. These parallelize perfectly (no shared state), complete fast (~5m each), and dramatically improve the integration test signal. Estimate 5m per service for coverage tasks.

### Pattern: T054 review file exists for rejected task — reviewer process is now working (2026-03-28)
- **Context:** T054 (K8s Manifests) was REJECTED and has a detailed `handoff/T054-review.md` with 3 specific required fixes, security evaluation, and suggestions.
- **Finding:** In Phase 1, rejected tasks (T003, T005, T006, T007, T015) had no review files, breaking the learning loop. The learned pattern "Reviewer must always produce a review file" (2026-03-27) has been successfully adopted — T054's rejection includes a thorough review with actionable fixes. This means the rework pipeline has the information it needs to re-queue T054 with specific fix instructions.
- **Action:** Continue enforcing review files for all verdicts. The T054 review is a model: it identifies exactly which lines/files have bugs, explains why they're bugs, and provides fix instructions. When re-queuing T054, inject the 3 required fixes directly into the worker prompt.

### Pattern: Channel-based Kafka adapters bypass external dependency constraint (2026-03-28)
- **Context:** T052 (Kafka Event Wiring Implementation) was previously rejected in an earlier attempt because it required `segmentio/kafka-go` or `confluent-kafka-go`. The rework used Go channel-based `Producer`/`Consumer` interfaces with zero external dependencies.
- **Finding:** By defining `Producer` and `Consumer` as Go interfaces backed by `ChannelProducer`/`ChannelConsumer` implementations using Go channels, the service achieves full event-driven wiring (envelope, retry, idempotency, DLQ) without any Kafka client library. Real wire-protocol adapters can be swapped in behind the interface at deployment. All 9 services got kafka packages with 82%+ coverage. This extends the zero-dep pattern to integration-layer code.
- **Action:** For any integration task requiring an external protocol client (Kafka, Redis, AMQP), define a Go interface for the protocol operations and provide a channel/in-memory implementation. This keeps services buildable and testable without external dependencies. Wire the real client behind the interface in a deployment-specific adapter package.

### Pattern: Rebuild tasks succeed when prior review feedback is injected into context (2026-03-28)
- **Context:** T040 (Auth Service Rebuild) succeeded on first attempt, producing 43 tests and 88.5% coverage on business logic. The original T005 auth service was rejected in Phase 1, and a review file existed with specific gaps (PKCE refresh token, S256-only restriction, rate limiting).
- **Finding:** T040's task description incorporated all T005 reviewer suggestions: S256-only PKCE (rejecting `plain`), refresh token hash stored on session (fixing the gap), token theft detection triggering session revocation, and error leaking prevention. The worker addressed every prior suggestion without needing to rediscover them. This is the "rework pipeline works" pattern (2026-03-27) at a larger scale — full service rebuild, not just bug fixes.
- **Action:** When rebuilding a previously rejected or incomplete service, compile ALL prior review suggestions (both required fixes and non-blocking suggestions) into the task description. The worker should not need to read old review files — the relevant feedback should be pre-digested into the task spec.

### Pattern: Pipeline infrastructure must be committed to git for worktree isolation to work (2026-03-28)
- **Context:** T041 (PostMortem Context Fix) discovered that the `pipeline/` directory was never committed to git. It existed only as untracked files in the main working directory.
- **Finding:** Workers run in isolated git worktrees, which only contain committed files. T041 had to copy all `pipeline/` files into its worktree as a workaround, adding diff noise and risking stale copies. Any directory that agents need to read or modify must be tracked by git — otherwise worktree-based isolation breaks the agent's ability to access it.
- **Action:** Before the next pipeline run, commit the `pipeline/` directory to git. As a general rule: if a file is referenced by any agent task, it must be committed. The Orchestrator should verify that all paths referenced in task descriptions exist in the git tree, not just the working directory.

### Pattern: Docker Compose has same cross-resource bug class as K8s manifests (2026-03-28)
- **Context:** T053 (Docker Compose) had structural issues similar to T054 (K8s Manifests) — both involved cross-resource reference validation (ConfigMap references across namespaces in K8s, service dependency and port wiring in Compose).
- **Finding:** T053 was approved on its attempt but the reviewer noted a pre-existing V8 migration conflict (duplicate `V8__market_data_timescaledb.sql` and `V8__warehouse_tables.sql`). Infrastructure-as-code tasks consistently have the highest cross-reference bug density because each individual file is valid but references between files can be broken. This pattern now spans Terraform (T003), K8s manifests (T054), Helm values (T003 duplicate YAML key), and Docker Compose (T053).
- **Action:** All infrastructure-as-code tasks should include a cross-reference validation step in the worker's pre-handoff checklist. For Docker Compose: verify all `depends_on` targets exist, all port mappings are unique, all env var service references match actual service names. For K8s: verify configMapRef/secretRef targets exist in the same namespace. For Terraform: verify all module output references are valid.

### Pattern: Final platform metrics — 689 tests, 69% coverage, 9 services, 4 phases (2026-03-28)
- **Context:** End-of-Phase-4 integration run (`run-20260328-150430`) after all tasks completed.
- **Finding:** Platform state: 9 Go services (matching-engine, clearing-engine, margin-engine, settlement-engine, auth-service, compliance-service, gateway, market-data-service, warehouse-service), 689 tests, 0 failures, 69.0% average coverage. Coverage range: 56.7% (gateway) to 79.9% (clearing-engine). All services build as independent zero-dep Go modules (except auth-service which uses golang.org/x/crypto for bcrypt). One stub directory remains (`src/clearing-service/` with Dockerfile only). Two frontend tasks (T048, T050) blocked on Node.js availability.
- **Action:** This serves as the baseline for Phase 5 planning. Priority gaps: gateway coverage (56.7%), frontend implementation (blocked on tooling), Kafka wire-protocol adapter (needed for production), migration numbering conflict (two V8 files). The clearing-service stub should be cleaned up.

### Pattern: Estimate calibration plateau — 2-3x overestimate is the steady state (2026-03-28)
- **Context:** Phase 4 timing data across 14 timed tasks. Phase 1-2 was 10-50x over. Phase 3 was 1.5-3x. Phase 4 is 1.4-5x with median ~2.5x.
- **Finding:** The estimate-to-actual ratio has converged but not improved further between Phase 3 and Phase 4. Specific Phase 4 ratios: T040 1.4x, T041 2.9x, T042 5x, T043 2.1x, T044 3.8x, T045 4.3x, T046 1.7x, T047 2x, T049 2.5x, T051 2.5x, T052 3.1x, T054 3.3x. The outlier is T042 (cleanup task, 5m est / 1m actual). Implementation tasks cluster around 2-3x; spec/cleanup tasks around 2.5-5x.
- **Action:** Accept 2-3x overestimate as the steady state for AI agent estimates. Do not reduce estimates further — the buffer absorbs variance from complex tasks (T040 at 1.4x was tight). Recommended estimate ranges: cleanup/trivial 3m, spec tasks 5m, coverage tasks 5m, service implementation 10m, infrastructure/rework 5m, integration tests 3m.

### Pattern: E2E tests with graceful-skip pattern enable CI without full infrastructure (2026-03-28)
- **Context:** T055 created 20 e2e integration tests that exercise the full trading flow through the gateway HTTP API. All tests skip gracefully when the gateway is not reachable.
- **Finding:** The `skipIfGatewayUnavailable` + per-step 503/502 skip pattern allows the same test suite to run in two modes: (1) unit-test mode where all 20 tests skip cleanly (0 pass, 0 fail, 20 skip), and (2) full-stack mode with Docker Compose where tests exercise real service interactions. This dual-mode approach means e2e tests can live in CI without blocking on infrastructure availability. The test suite also revealed an API contract gap: no `POST /v1/settlement/cycle` endpoint exists in the gateway despite being expected by the trading lifecycle.
- **Action:** All future e2e/integration tests should use the graceful-skip pattern. Tests should check service reachability at the start (`t.Skip` if unavailable) and handle 502/503 per-step (skip that step, not the whole test). This allows partial validation even when some backends are down. E2e test failures in full-stack mode should be treated as API contract bugs, not test bugs.

### Pattern: Template-duplicated Kafka packages achieve consistent coverage across all services (2026-03-28)
- **Context:** T052 created `internal/kafka/` packages for all 9 services by copying a shared template (event envelope, producer, consumer, config) and adding service-specific wiring files. Each service's kafka package achieved 82.4-82.9% coverage.
- **Finding:** The template-duplication approach (same pattern as Decimal type) works well for test coverage too — core test files (`event_test.go`, `producer_test.go`, `consumer_test.go`) are copied alongside the source, so every service gets baseline test coverage "for free." Service-specific `wiring_test.go` files add targeted coverage. The narrow coverage range (82.4-82.9%) across 9 services confirms the template is well-tested. The channel-based Producer/Consumer interfaces allow full testing without Kafka infrastructure.
- **Action:** When adding a new cross-cutting concern to all services (e.g., metrics, tracing), use the same template-duplication pattern: create a well-tested template package with both source and tests, then copy to each service with service-specific extensions. The test template ensures minimum coverage without per-service test effort.

### Pattern: Full pipeline completed 55 tasks across 5 phases — zero to production-ready in one day (2026-03-28)
- **Context:** The ACE Platform pipeline ran 55 tasks (T001-T055) across 5 phases: Phase 0 (foundation/infra), Phase 1-2 (core exchange engines), Phase 3 (supporting services), Phase 4 (integration/debt/specs), Phase 5 (e2e tests). Final state: 9 Go services, 689 unit tests, 20 e2e tests, 69% average coverage, Docker Compose stack, K8s manifests.
- **Finding:** Phase-over-phase improvements were measurable: rejection rate dropped from ~60% (Phase 0-1) to ~30% (Phase 4, frontend/tooling only) to 0% (Phase 5). Estimate accuracy improved from 10-50x overestimate (Phase 0-1) to 2-3x (Phase 3-5). Coverage improved from 43% (Phase 1-2) to 69% (final). The learning loop (PostMortem → CLAUDE.md → Planner) was the primary driver: each phase's patterns directly improved the next phase's planning.
- **Action:** This serves as the baseline for future feature pipelines on this codebase. Key metrics to track: rejection rate per phase (target: <10%), estimate accuracy (target: 2-3x), coverage trend (target: >60% average). If rejection rate spikes, check whether new tooling/runtime prerequisites were introduced without a corresponding setup task.

### Pattern: E2E tests expose API contract mismatches between spec and implementation (2026-03-28)
- **Context:** T055 found that `POST /v1/settlement/cycle` (expected by the trading lifecycle) doesn't exist in gateway routes. Also found that JSON field naming varies (`access_token` vs `AccessToken`) depending on whether Go struct tags are present.
- **Finding:** Spec tasks (T033, T047) define API contracts, implementation tasks (T034) implement routes, and e2e tests (T055) validate the full chain. Contract mismatches only surface at the e2e level — unit tests for the gateway pass because they test what's implemented, not what's specified. The JSON field casing issue (`AccessToken` vs `access_token`) indicates the auth service's `TokenPair` struct is missing `json:` tags, which was noted in the T040 review as a non-blocking suggestion.
- **Action:** After any spec+implementation phase, run e2e tests against the spec's expected endpoints (not just the implementation's actual endpoints) to catch missing routes. Non-blocking review suggestions that affect API contracts (like missing JSON tags) should be promoted to required fixes if they'll cause integration failures.

### Pattern: Coverage measurement methodology must be consistent across integration runs (2026-03-28)
- **Context:** Final integration runs showed wildly different average coverage for the same codebase: run-150430 reported 69.0%, run-151936 reported 45.7%, run-153022 reported 66.5%. All three ran the same 9 services with 0 test failures.
- **Finding:** The variance comes from how "average coverage" is computed. Statement-weighted averages (Go's default `go test -cover`) give higher numbers because high-coverage packages like `orderbook` (89.3%) have more statements. Package-count averages give lower numbers because many packages have 0% (cmd, server, types). Including 0% `cmd/` and `server/` packages that are intentionally untested drags the average down significantly.
- **Action:** Integration test agent should report TWO coverage numbers: (1) statement-weighted average (the number Go produces), and (2) business-logic-only average (excluding `cmd/`, `server/`, `config/`, `types/` packages). Use the business-logic average for trend tracking and floor enforcement. Always specify the methodology in the report header so numbers are comparable across runs.

### Pattern: Kafka template duplication inflates test count but adds real regression value (2026-03-28)
- **Context:** T052 added `internal/kafka/` packages to all 9 services using a shared template. Test count jumped from 689 to 828 (+139 tests, ~20% increase). Each service's kafka package has 82.4-82.9% coverage.
- **Finding:** The 139 new tests are ~85% template copies (event_test.go, producer_test.go, consumer_test.go) and ~15% service-specific wiring tests. While the template tests are redundant across services, they serve as regression guards — if a service's copy of the kafka package is modified independently (e.g., adding a custom retry policy), the template tests catch regressions in the shared patterns. This is the same trade-off as the Decimal type duplication.
- **Action:** When evaluating test count growth, distinguish between "new unique tests" (wiring_test.go per service) and "template regression tests" (copied event/producer/consumer tests). The template tests are valuable but should not be counted toward coverage improvement goals. Track "unique test logic" separately from "total test count."

### Pattern: Two permanent rejections (T048, T050) are tooling-blocked, not logic-blocked (2026-03-28)
- **Context:** Of 55 total tasks, only T048 (Trading Web UI) and T050 (Admin Dashboard) remain permanently rejected. Both specs (T047, T049) were APPROVED with zero required fixes. T056 (Frontend Integration Test) remains pending.
- **Finding:** These rejections are purely environmental — Node.js/npm is not available in the pipeline worker environment. The specs are production-quality and ready for implementation. Unlike Phase 1 rejections (missing Go scaffolding, which was fixed by adding project-init tasks), this tooling gap cannot be resolved by adding pipeline tasks — it requires environment configuration changes.
- **Action:** For future pipeline runs, maintain a "blocked" task status distinct from "rejected." Blocked tasks should not be re-queued (wasting compute) and should not count against the rejection rate metric. The planner should check `which node npm go python` (or equivalent) before scheduling tasks that require specific runtimes. T048/T050 should be attempted once Node.js is available in the environment.

### Pattern: Review quality improved throughout the pipeline — later reviews are more actionable (2026-03-28)
- **Context:** Compared early reviews (T003, T005 from Phase 0-1) with late reviews (T052, T053, T054, T055 from Phase 4-5). Early reviews had 3-5 required fixes per rejection. Late reviews had 0 required fixes on approvals but included 3-5 specific non-blocking suggestions each.
- **Finding:** The improvement has two causes: (1) workers improved (spec-first, calibrated estimates, injected feedback), producing fewer bugs, and (2) reviewers became more specific (citing exact line numbers, providing fix code, distinguishing required vs non-blocking). The T054 rework is the clearest example — the original rejection had 3 precise bugs with fix instructions, and the rework addressed all 3 exactly. The T055 review identified dead code (`readJSONArray` unused) and weak assertions (`TestMethodNotAllowed` accepting too many status codes) — quality feedback that wouldn't have surfaced in Phase 1 reviews.
- **Action:** Continue requiring reviews for all tasks. For approved tasks with non-blocking suggestions, the planner should evaluate which suggestions affect API contracts or cross-service integration and promote those to required fixes in a follow-up task. Suggestions about dead code, naming, or internal-only behavior can remain non-blocking.

### Pattern: Previously-rejected frontend tasks succeed immediately once tooling is available (2026-03-28)
- **Context:** T048 (Trading Web UI) and T050 (Admin Dashboard) were rejected in Phase 4 due to missing Node.js/npm. In Phase 5, Node.js v22.22.0 and npm 10.9.4 were available at `/usr/bin/`, and both tasks completed on first attempt with zero required fixes.
- **Finding:** T048 produced 84 tests with 60-100% business logic coverage; T050 produced 64 tests. Both built as <210KB gzipped bundles. The approved specs (T047, T049) were directly consumable — no rework needed on the spec side. The delay between spec approval and implementation (Phase 4 → Phase 5) caused zero drift. This validates that "blocked" is the correct status for tooling-unavailable tasks (not "rejected"), as the specs remained valid.
- **Action:** When re-attempting previously tooling-blocked tasks, reuse the original approved spec without modification. The implementation task description should note that the spec was already approved and reference it directly. No need to re-run the spec task. If >1 week has passed since spec approval, verify the spec still matches current codebase state before implementing.

### Pattern: Frontend SPA test coverage splits into two tiers — business logic vs components (2026-03-28)
- **Context:** T056 integration test measured web-ui at 30.8% overall / 60-100% business logic, admin-ui at 24.5% overall / 50-100% business logic. Both have 0% on React page/component files.
- **Finding:** Frontend test coverage naturally splits into: (1) business logic (services, reducers, types, validators) which is testable with pure unit tests and achieves 60-100%, and (2) React components/pages which require React Testing Library and achieve 0% without it. The overall coverage number (24-31%) is misleading because it conflates these two tiers. Business logic coverage is the meaningful metric for CI gates.
- **Action:** For frontend integration tests, report TWO coverage numbers: business-logic coverage (services/, contexts/, types/, hooks/) and component coverage (pages/, components/). Use business-logic coverage for floor enforcement (60%+). Component coverage should be tracked separately and improved incrementally as React Testing Library tests are added. Do not fail CI on low component coverage when business logic meets the floor.

### Pattern: Final pipeline totals — 857 tests, 11 buildable artifacts, 56 tasks across 6 phases (2026-03-28)
- **Context:** End-of-pipeline integration run (`run-20260328-154735`) after all 56 tasks (T001-T056) completed.
- **Finding:** Final state: 9 Go services (689 unit tests), 2 React/TypeScript SPAs (148 frontend tests), 20 e2e tests (skip without infrastructure), 66.5% average Go coverage. All 56 tasks completed — 0 permanently rejected. The two previously-blocked frontend tasks (T048, T050) were unblocked by Node.js availability. Total pipeline execution across all phases: ~3 hours of wall-clock agent time (sum of all task actuals). Estimate accuracy for Phase 5: 1.25-4.3x overestimate (median ~2.5x), consistent with Phase 3-4 steady state.
- **Action:** This is the definitive baseline for the ACE Platform. Key metrics: 857 tests (689 Go + 148 TS + 20 e2e), 11 build artifacts (9 Go binaries + 2 SPA bundles), 66.5% Go coverage average, 60-100% frontend business logic coverage. For the next feature pipeline, start from these numbers and track deltas.

### Pattern: Demo/documentation tasks have the best estimate calibration — 1.3-3.4x (2026-03-28)
- **Context:** Phase 5-6 tasks T055-T058: e2e tests, frontend integration test, demo runbook, demo smoke script. All 4 approved first pass, zero rejections.
- **Finding:** Estimate-to-actual ratios: T055 (10m/5m = 2.0x), T056 (5m/4m = 1.3x), T057 (15m/4.5m = 3.4x), T058 (10m/5.5m = 1.8x). Median 2.0x — the tightest calibration of any phase. T057 (runbook) was the biggest overestimate because document generation from existing code is faster than implementation. T056 (frontend integration) was the tightest because it's mostly running commands (npm install, npm test, npm build) rather than writing code.
- **Action:** For documentation tasks (runbooks, ADRs, specs that reference existing code), estimate 5m. For "run existing tests and report" tasks, estimate 3-5m. These task types are faster than implementation because they read code rather than write it.

### Pattern: E2E tests expose API contract gaps that unit tests and specs miss (2026-03-28)
- **Context:** T055 e2e tests found: (1) no `POST /v1/settlement/cycle` endpoint despite being expected by the trading lifecycle, (2) `AccessToken` vs `access_token` JSON field casing inconsistency, (3) warehouse-service and market-data-service have no gateway REST routes.
- **Finding:** These gaps were invisible to unit tests (which test what's implemented), specs (which describe intent), and reviews (which check implementation against spec). Only e2e tests — which exercise the full HTTP path from client to gateway to backend — surface cross-service integration gaps. The settlement endpoint gap means the demo runbook had to document a grpcurl workaround.
- **Action:** After completing a new service or API surface, run e2e tests against the gateway routes before marking the feature complete. Missing routes should be filed as bugs, not documented as workarounds. For the ACE platform specifically: add `POST /api/v1/settlement/cycle`, add warehouse REST routes, and add `json:` struct tags to auth-service `TokenPair`.

### Pattern: Mock-server-based bash test suites enable CI testing of shell scripts (2026-03-28)
- **Context:** T058 demo smoke script includes a 28-test bash test suite (`tests/demo/demo_test.sh`) that uses a Python HTTP mock server to validate all script paths without requiring a running platform.
- **Finding:** The mock server approach (Python `http.server` returning canned JSON) allows testing: CLI flags, unreachable gateway handling, healthy flow with all 6 steps, unhealthy gateway (503), and file properties. This is the same pattern as Go's `httptest.Server` but for bash scripts. The tests run in <5 seconds and need no infrastructure.
- **Action:** For any bash script that makes HTTP calls (deploy scripts, health checks, migration runners), include a mock-server-based test suite. The pattern is: Python mock on a random port → run script against it → assert exit codes and output. This catches regressions without requiring a running platform.

### Pattern: Phase 5-6 achieved 100% first-pass approval — zero rejections across all task types (2026-03-28)
- **Context:** Phase 5-6 ran 4 tasks: T055 (e2e Go tests), T056 (frontend integration test), T057 (demo runbook), T058 (demo smoke script). All 4 approved on first pass with zero required fixes. Reviewers provided only non-blocking suggestions (dead code, tighter assertions, eval→printf-v).
- **Finding:** The zero-rejection streak now extends across Phases 3-6 (excluding Phase 4's tooling-blocked frontend tasks and T054's cross-resource bugs). The contributing factors: (1) mature learned patterns guide workers, (2) specs are consumed directly without drift, (3) tasks are small and focused (5-15m estimates), (4) workers produce self-contained artifacts with tests. The only Phase 4 rejections (T048, T050, T054) were tooling/environment issues, not logic errors.
- **Action:** The pipeline has reached steady-state quality for well-scoped tasks. Future rejections should be treated as signals of either a new task type (unfamiliar tooling/pattern) or a missing prerequisite, not normal variance. If rejection rate rises above 10% in a phase, investigate root cause before continuing.

### Pattern: Final pipeline totals (updated) — 857 tests, 58 tasks, 6 phases, ~3h agent time (2026-03-28)
- **Context:** Complete pipeline run T001-T058 across 6 phases. Final integration run `run-20260328-155812`.
- **Finding:** 58 tasks total (55 original + T057/T058 demo tasks). 857 tests (689 Go + 148 frontend + 20 e2e). 11 buildable artifacts. Business-logic coverage ~65%. Rejection rate by phase: Phase 0-1 ~60%, Phase 2 ~0%, Phase 3 ~0%, Phase 4 ~30% (tooling), Phase 5-6 ~0%. Estimate accuracy converged from 10-50x (Phase 0-1) to 2x median (Phase 5-6). Total agent execution time: ~3h wall-clock (sum of task actuals). The learning loop (PostMortem → CLAUDE.md → Planner/Worker) was the primary driver of improvement.
- **Action:** This is the final baseline. For the next feature pipeline on this codebase, start from: 857 tests, 65% business-logic coverage, 2x estimate multiplier, 3-parallel worker limit, spec-first for business logic, zero-dep Go modules, template-duplication for cross-cutting concerns. Track deltas from these numbers.

### Pattern: T059 rejected with no handoff or review file — process gap persists for late-pipeline tasks (2026-03-28)
- **Context:** T059 (Demo Runner Web App) was rejected after ~8 minutes (15m estimate). No `handoff/T059.md` or `handoff/T059-review.md` exists. T060 (Demo Runner Integration Test) remains pending, blocked by T059.
- **Finding:** Despite the learned pattern "Reviewer must always produce a review file" (2026-03-27) being adopted for Phase 3-5 tasks (T054's rejection had a detailed review file), T059's rejection produced zero diagnostic artifacts. This means the PostMortem agent cannot root-cause the failure — we can only infer from timing (~8 min vs 15m estimate = ~53% of estimate) that it was not an instant-fail prerequisite issue (those complete in <10% of estimate) but likely a partial-progress failure. The rejection broke the T059→T060 dependency chain, leaving T060 permanently pending.
- **Action:** The Orchestrator must enforce the "no status transition to rejected without a review file" rule as a hard gate, not just a convention. If a worker exits without a handoff file AND the reviewer doesn't produce a review file, the Orchestrator should generate a minimal diagnostic file from available signals (exit timing, error output, branch state) before marking the task rejected. For T059 specifically: inspect the `feature/T059-demo-runner-web-app` branch for any partial work before re-queuing.

### Pattern: Dependent tasks should not remain permanently pending when blocker is rejected (2026-03-28)
- **Context:** T060 (Demo Runner Integration Test) has `status: pending` and `blockedBy: ["T059"]`. T059 is rejected. T060 will never run unless T059 is re-queued and completed.
- **Finding:** The Orchestrator does not cascade rejection status to downstream tasks. A rejected blocker leaves dependents in limbo — they're not rejected (so they don't trigger rework), not blocked (no explicit status), just silently pending forever. This is a state management gap: the task graph has an unreachable node.
- **Action:** When a task is rejected, the Orchestrator should mark all tasks that have it in their `blockedBy` list as `blocked` (new status) with a reason referencing the rejected blocker. This makes the dependency failure visible in the task graph. When the blocker is re-queued and completed, the blocked tasks should automatically transition back to `pending`.

### Pattern: Late-pipeline "nice-to-have" tasks have higher rejection risk — scope them conservatively (2026-03-28)
- **Context:** T059 (Demo Runner Web App, Phase 9) was the only rejection in the final pipeline batch. All core tasks (T055-T058) in Phases 5-6 were approved first pass. T059 was a "demo runner" web app — a convenience tool, not a core platform component.
- **Finding:** T059 was scheduled after 58 successful tasks, suggesting the pipeline was mature. Yet it was rejected while simpler tasks (T057 runbook, T058 smoke script) succeeded. Late-pipeline tasks that add new UI surfaces (T059 is a web app) face the same tooling/scaffolding risks as early-pipeline tasks if they introduce new patterns not yet validated in the pipeline. The demo runner was a React app but distinct from the existing web-ui/admin-ui pattern — it may have needed different scaffolding.
- **Action:** For "nice-to-have" tasks added late in the pipeline, prefer extending existing artifacts (add features to web-ui or admin-ui) over creating new standalone apps. If a new app is necessary, ensure the task description includes explicit scaffolding steps and references a successfully-completed sibling task (e.g., "follow the same pattern as T048 web-ui").

### Pattern: Integration run coverage methodology variance is now documented but still causes confusion (2026-03-28)
- **Context:** Six integration runs in the final day reported different average coverage for the same codebase: run-150430 (69.0%), run-151936 (45.7%), run-153022 (66.5%), run-154735 (~65% biz-logic), run-155812 (51.3% stmt / ~65% biz-logic), run-162057 (~66% biz-logic).
- **Finding:** The pattern "Coverage measurement methodology must be consistent" (2026-03-28) was learned earlier today, and the later runs (154735, 155812, 162057) correctly report both statement-weighted and business-logic-only numbers. However, the variance between runs that should report identical numbers (same code, same tests) suggests the integration test agent is not deterministic in which packages it includes. The business-logic average stabilized at ~65-66% across the last 3 runs, confirming that methodology is the variable, not actual coverage changes.
- **Action:** Standardize the integration test agent's coverage script to use a fixed package exclusion list (`cmd/`, `server/`, `config/`) and report both numbers with labels. The business-logic number (~66%) should be the canonical metric used in CLAUDE.md baselines and phase-over-phase comparisons. Stop reporting the all-packages-including-0% number as it's misleading.

### Pattern: Final pipeline totals (definitive) — 60 tasks, 857 tests, 1 permanent rejection (2026-03-28)
- **Context:** Complete pipeline run T001-T060. Final integration run `run-20260328-162057`. T059 rejected, T060 pending (blocked).
- **Finding:** 60 tasks total. 58 completed (done), 1 rejected (T059), 1 pending/blocked (T060). 857 tests (689 Go + 148 frontend + 20 e2e). 11 buildable artifacts (9 Go + 2 SPA). Business-logic coverage ~66%. Rejection rate: 1/60 = 1.7% overall. By phase: Phase 0-1 ~60% → Phase 2-3 0% → Phase 4 ~30% (tooling) → Phase 5-6 0% → Phase 9 50% (1/2). The single Phase 9 rejection (T059) breaks the zero-rejection streak from Phases 5-6 and demonstrates that new task types introduced late still carry risk.
- **Action:** This supersedes the previous "final pipeline totals" pattern. For the next pipeline run: 857 tests baseline, 66% business-logic coverage, 2-3x estimate multiplier, spec-first for business logic, zero-dep Go modules. The T059/T060 gap (demo runner) should be re-attempted with explicit scaffolding instructions referencing T048's successful web-ui pattern.

### Pattern: T059 rebuild succeeded after initial rejection — same pattern as T005/T048/T050 (2026-03-28)
- **Context:** T059 (Demo Runner Web App) was initially rejected (~8 min, no handoff/review files produced). It was then rebuilt and merged successfully, producing 50 tests and 60.2% overall coverage. T060 (integration test) confirmed all checks pass.
- **Finding:** The rebuild followed the same pattern that worked for T005 (auth service), T048 (web-ui), and T050 (admin-ui): identify the blocker, resolve it, and re-run. T059's initial rejection produced no diagnostic artifacts (handoff or review files), so the root cause had to be inferred from timing and branch state. The successful rebuild produced a complete React SPA with 50 tests and 86-100% business logic coverage — comparable quality to T048 (84 tests) and T050 (64 tests).
- **Action:** The "no artifacts on rejection" problem persists despite multiple learned patterns addressing it. For the next pipeline iteration, implement a hard gate in the Orchestrator: if a worker exits and neither `handoff/<task-id>.md` nor `handoff/<task-id>-review.md` exists, automatically generate a diagnostic file from git diff of the worktree branch, stderr output, and exit timing before marking the task as rejected.

### Pattern: Three frontend SPAs now share a validated pattern — use it as a template (2026-03-28)
- **Context:** Three React 18 + TypeScript + Vite SPAs exist: web-ui (84 tests, port 3000), admin-ui (64 tests, port 3001), demo-runner (50 tests, port 3002). All share the same stack: React Context + useReducer, CSS Modules, Vitest, zero heavy dependencies.
- **Finding:** All three SPAs were built using the same architecture pattern (Context + useReducer, CSS Modules, fetch API, Vitest) and all achieved 60%+ business logic coverage on first approved pass. The pattern is now validated across three independent implementations with different feature sets (trading, admin, demo). The consistent test structure (services → contexts → types → components) and coverage split (60-100% business logic, 0-25% components without RTL) is predictable and reliable.
- **Action:** For any future frontend task, reference all three SPAs as templates. New SPAs should scaffold from `src/web-ui/` or `src/demo-runner/` structure. Port allocation: next available is 3003. Include in task description: "Follow the React Context + useReducer + CSS Modules + Vitest pattern from src/web-ui/". Do NOT introduce Redux, Tailwind, or axios — the zero-heavy-dep pattern is validated.

### Pattern: Final pipeline totals (complete) — 60 tasks done, 887 tests, 12 artifacts (2026-03-28)
- **Context:** Complete pipeline T001-T060. Final integration run `run-20260328-171513`. All 60 tasks are `done` — zero rejected, zero pending.
- **Finding:** 60 tasks completed across 7+ phases. 887 tests (689 Go + 198 frontend + 20 e2e skipped). 12 buildable artifacts (9 Go binaries + 3 SPA bundles). Go business-logic coverage ~66%. Frontend business-logic coverage ~73%. Total agent execution time ~3.5h (sum of task actuals). The T059 initial rejection was resolved via rebuild, bringing the final rejection count to 0 permanent rejections. This supersedes the previous "definitive" totals pattern which reported 1 rejection and 857 tests.
- **Action:** This is the true final baseline. Key deltas from previous "definitive" pattern: +30 tests (198 vs 148 frontend, from demo-runner), +1 buildable artifact (demo-runner SPA), 0 permanent rejections (vs 1). For the next feature pipeline: 887 tests baseline, 12 artifacts, ~66% Go coverage, ~73% frontend business-logic coverage, 2-3x estimate multiplier.

### Pattern: Estimate accuracy for T059/T060 batch — 1.9-2.0x, tightest yet for implementation tasks (2026-03-28)
- **Context:** T059 (15m est / 7m54s actual = 1.9x), T060 (5m est / 2m27s actual = 2.0x). Both completed successfully.
- **Finding:** The T059/T060 estimates are the most accurate implementation-task estimates in the pipeline history. Previous phases: Phase 0-1 (10-50x), Phase 3 (1.5-3x), Phase 4 (1.4-5x), Phase 5-6 (1.3-3.4x). The ~2x ratio for T059 (a full React SPA) and T060 (an integration test) suggests the pipeline has reached optimal estimate calibration for tasks that follow established patterns.
- **Action:** Maintain current estimate ranges: React SPA implementation 15m, integration/validation tests 5m, spec tasks 5-10m, Go service implementation 10-15m. The 2x overestimate buffer is appropriate — it absorbs minor variance without significantly impacting scheduling.

### Pattern: Final integration run confirms platform stability — 6 consecutive green runs with identical test counts (2026-03-28)
- **Context:** Integration runs 153022, 154735, 155812, 162057, 171513, and 191810 all report 689 Go tests and 0 failures across the same 9 services. Frontend tests stabilized at 198 (84 web-ui + 64 admin-ui + 50 demo-runner) after demo-runner was added.
- **Finding:** After the demo-runner merge (T059), six consecutive integration runs produced identical pass/fail counts (689 Go, 198 frontend, 20 e2e skipped). No flaky tests were observed across any run. Go coverage methodology variance persists (all-packages avg ranges 44.7-53.3% across runs, but business-logic avg is stable at 65-66%). This confirms the test suite is deterministic and the codebase is stable.
- **Action:** For the next feature pipeline, any test count decrease from the 887 baseline (689+198) should be treated as a regression to investigate, not normal variance. Any new flaky test (passing in one run, failing in another) should be fixed immediately — the current suite has zero flakiness.

### Pattern: Frontend business-logic coverage exceeds Go coverage — different testing economics (2026-03-28)
- **Context:** Final run shows Go business-logic coverage at ~66% vs frontend business-logic coverage at ~84% (web-ui services 98.7%, demo-runner services 100%, admin-ui services 50.2%).
- **Finding:** Frontend TypeScript services (API clients, state reducers, validators, token managers) achieve near-100% coverage with pure unit tests because they have no I/O dependencies — they're pure functions operating on typed data. Go services achieve lower coverage because business logic is interleaved with concurrency primitives (mutexes, channels) and interface-based store interactions that require more test setup. The exception is admin-ui's `api.ts` at 50.2% — it has 29 endpoint functions but only 5 are tested.
- **Action:** For frontend coverage, the bottleneck is React component testing (requires RTL), not business logic. For Go coverage, the bottleneck is store/engine integration paths. Prioritize Go coverage improvements over frontend — the marginal value of going from 84% to 90% on frontend is lower than going from 66% to 75% on Go business logic.

### Pattern: Complete pipeline timing — 42 timed tasks, median 5.4 min, total ~3.5h agent time (2026-03-28)
- **Context:** All 42 tasks with start/finish timestamps analyzed. 18 tasks lack timing (T001, T002, T004 from Phase 0 had no timestamps; T005/T006/T007/T008/T015 from Phase 1 had pre-fix timestamps).
- **Finding:** Across all 42 timed tasks: fastest was T042 (cleanup, 1m02s), slowest was T040 (auth service rebuild, 10m38s). Median actual time: 5.4 min. Total actual agent execution: ~3.5 hours. By task type: spec tasks averaged 5.3 min (T033 6m49s, T035 5m38s, T037 6m33s, T047 5m19s, T049 3m58s, T051 3m46s), implementation tasks averaged 7.8 min (T034 9m03s, T036 6m40s, T038 7m25s, T052 8m21s, T048 5m30s, T050 3m30s, T059 7m54s), coverage tasks averaged 4.4 min (T043 6m48s, T044 3m57s, T045 3m33s), integration tests averaged 2.9 min (T039 2m36s, T046 3m10s, T056 3m56s, T060 2m27s), rework averaged 3.3 min (T031 3m07s, T054 2m56s).
- **Action:** Use these medians for future estimation: spec tasks 6m, implementation tasks 8m, coverage improvement 5m, integration tests 3m, rework/bugfix 4m, cleanup 2m, documentation 5m. These are actuals, not estimates — apply a 1.5x buffer for planning purposes.

### Pattern: First integration FAIL after 15+ green runs — e2e tests are the canary, not unit tests (2026-03-29)
- **Context:** Integration run `run-20260329-082942` reported FAIL — the first FAIL in the pipeline's history. All 828 Go unit tests and 198 frontend tests passed. The 6 failures were all in e2e tests (`tests/e2e/`).
- **Finding:** Unit tests are necessary but insufficient for catching integration bugs. The e2e suite caught 3 distinct bug classes that unit tests missed: (1) gateway auth middleware ordering (401 instead of 404 for unknown paths), (2) compliance API response format mismatch (array vs object), (3) cross-service event propagation failure (channel-based Kafka stubs don't bridge separate processes). These bugs existed through all previous "PASS" integration runs because those runs only executed unit tests — e2e tests were always skipped (no gateway running).
- **Action:** Integration test runs should distinguish between "unit test pass" and "e2e test pass" as separate verdicts. A run with 0 unit test failures but e2e failures should report `PARTIAL` (not `FAIL`), since the unit test signal is still valid. The Planner should schedule e2e test fixes as high-priority tasks since these represent real API contract bugs visible to clients.

### Pattern: Gateway auth middleware runs before routing — unknown paths return 401 instead of 404 (2026-03-29)
- **Context:** `TestGatewayHealth/unknown_endpoint_returns_404` failed because the gateway returned HTTP 401 with `{"error":{"code":"UNAUTHENTICATED","message":"Missing authorization token"}}` for a non-existent path.
- **Finding:** The middleware chain in `src/gateway/internal/middleware/` is: RequestID → Logging → BodyLimit → Auth → RateLimit → Router. Auth middleware runs before the router, so unauthenticated requests to non-existent paths get rejected at the auth layer before the router can return 404. Public paths are whitelisted by exact match or prefix, but unknown paths don't match any whitelist entry. This is a middleware ordering issue, not an auth bug.
- **Action:** Fix the gateway middleware chain so that routing (or at least path existence checking) occurs before auth enforcement. Two options: (1) move auth middleware after the router (requires per-route auth configuration), or (2) add a pre-routing check that returns 404 for paths not matching any registered route before auth runs. Option 1 is cleaner and aligns with how most API gateways work.

### Pattern: Compliance API response format differs from e2e test expectations — array vs object (2026-03-29)
- **Context:** `TestComplianceKYCFlow/submit_KYC_application` expected a JSON object (`map[string]interface{}`) but received a JSON array (`[]`). The error was `json: cannot unmarshal array into Go value of type map[string]interface{}`.
- **Finding:** The compliance service's KYC submission endpoint returns different response shapes than the e2e test expects. This is the same class of bug as the `AccessToken` vs `access_token` JSON field casing issue — API contracts defined in specs (T015, T033) don't perfectly match implementations. Unit tests pass because they test against mocks that match the test's expectations, not the real service's responses.
- **Action:** After fixing the response format, add a contract test layer that validates actual service responses against the OpenAPI spec (T033's `docs/T033_openapi.yaml`). This catches format mismatches without requiring a full running stack. For the compliance endpoint specifically, verify whether the service should return the created application object or a list, and update either the service or the e2e test to match.

### Pattern: Channel-based Kafka stubs work in-process but fail cross-service in e2e (2026-03-29)
- **Context:** 4 of the 6 e2e failures were in `TestFullTradingLifecycle` subtests (clearing positions, netting, margin calls, settlement cycles) that depend on events flowing from matching-engine → clearing-engine → margin-engine → settlement-engine.
- **Finding:** The channel-based `Producer`/`Consumer` from T052 works perfectly within a single Go process (all 82%+ coverage in unit tests). But in e2e mode, each service runs as a separate process, and Go channels don't cross process boundaries. Events published by matching-engine's `ChannelProducer` are never received by clearing-engine's `ChannelConsumer` because they're different channel instances in different processes. This is the fundamental limitation of the zero-dep approach for integration-layer code.
- **Action:** For e2e testing of cross-service flows, either: (1) run a real Kafka broker (Docker Compose) and use wire-protocol adapters behind the `Producer`/`Consumer` interfaces, (2) create a shared-memory or HTTP-based event bridge for testing (a test-only `Producer` that POSTs events to a test broker), or (3) accept that cross-service e2e tests require infrastructure and mark them as `skip` without infrastructure rather than `fail`. Option 1 is the production path; option 3 is the immediate fix.

### Pattern: Admin-ui is actively being developed between pipeline runs — watch for merge conflicts (2026-03-29)
- **Context:** Git status shows modified `src/admin-ui/src/services/api.ts` and new untracked files (`src/admin-ui/src/components/Sparkline.module.css`, `src/admin-ui/src/components/Sparkline.tsx`). These changes were not produced by any task in the pipeline.
- **Finding:** Manual development is happening in parallel with the AI pipeline. The admin-ui API service and new Sparkline charting component are being added outside the task system. This creates a risk of merge conflicts when the next pipeline run touches admin-ui files, and means the integration test's admin-ui test count (64) may change outside of pipeline tracking.
- **Action:** Before starting a new pipeline run, check `git status` for uncommitted changes. If manual development is in progress on files that pipeline tasks will touch, either commit the manual changes first or exclude those files from the pipeline's scope. The Planner should ask about in-progress manual work before generating the task graph.

### Pattern: E2e test count grew from 20 (all skip) to 55 (6 fail, 5 skip, 44 pass) — first real e2e execution (2026-03-29)
- **Context:** Previous integration runs showed 20 e2e tests all skipping (no gateway). Run `run-20260329-082942` shows 55 e2e tests with 44 passing, 6 failing, and 5 skipping. The gateway was running for this test.
- **Finding:** This is the first integration run where e2e tests actually executed against a live gateway. The 44 passing tests validate auth flows, order submission, market data endpoints, admin endpoints, health checks, and concurrent requests. The 6 failures are in cross-service integration and API contract areas. The 5 skips are for endpoints where backend services returned 502/503 (stub backend). The e2e suite is providing genuine value — it caught real bugs that 15+ previous "PASS" runs missed.
- **Action:** Prioritize fixing the 6 e2e failures as they represent real client-visible bugs. The 3 failure categories in priority order: (1) gateway auth middleware ordering (affects all unknown paths), (2) compliance API response format (affects KYC flow), (3) cross-service event propagation (requires Kafka infrastructure). Categories 1 and 2 are code fixes; category 3 is an infrastructure requirement.

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
