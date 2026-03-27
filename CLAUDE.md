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

*(No patterns yet — this section grows automatically as features complete.)*

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
  src/                   ← application code
  tests/                 ← test suite
```

---

*This file is both documentation and executable memory.
Keep it committed. Keep it accurate. The pipeline is only as good as what is written here.*
