#!/usr/bin/env bash
# Context builders — assemble prompts for each agent role by reading
# tasks.json, handoff/ files, and CLAUDE.md learned patterns.

HANDOFF_DIR="${PROJECT_ROOT}/handoff"

# Extract the Learned Patterns section from CLAUDE.md.
_learned_patterns() {
  sed -n '/<!-- LEARNED PATTERNS START/,/<!-- LEARNED PATTERNS END/p' \
    "${PROJECT_ROOT}/CLAUDE.md" 2>/dev/null || echo "(none)"
}

# Read a handoff file for a task, or return empty string.
_handoff_content() {
  local task_id="$1"
  local f="${HANDOFF_DIR}/${task_id}.md"
  if [ -f "$f" ]; then
    cat "$f"
  fi
}

# Read a review verdict file for a task, or return empty string.
_review_content() {
  local task_id="$1"
  local f="${HANDOFF_DIR}/${task_id}-review.md"
  if [ -f "$f" ]; then
    cat "$f"
  fi
}

# Collect upstream handoff content for a task (from its blockedBy deps).
_upstream_handoffs() {
  local task_id="$1"
  local deps
  deps="$(task_blocked_by "$task_id")"
  local result=""
  for dep_id in $deps; do
    local content
    content="$(_handoff_content "$dep_id")"
    if [ -n "$content" ]; then
      result="${result}
--- Handoff from ${dep_id} ---
${content}
"
    fi
  done
  echo "$result"
}

# ── Planner ──────────────────────────────────────────────────────────

build_planner_context() {
  local requirement="$1"
  local patterns
  patterns="$(_learned_patterns)"
  local current_tasks
  current_tasks="$(cat "$TASKS_FILE")"

  cat <<PROMPT
## User Requirement

${requirement}

## Learned Patterns (from previous features)

${patterns}

## Current tasks.json

\`\`\`json
${current_tasks}
\`\`\`

## Instructions

Read the requirement above. Update tasks.json with new or modified tasks.
Follow the schema from CLAUDE.md. Assign IDs that don't conflict with existing tasks.
Set status to "pending" for all new tasks. Wire blockedBy dependencies correctly.
Write the updated tasks.json to disk, then summarize your plan.
PROMPT
}

# ── Worker (Coder / Test Writer) ─────────────────────────────────────

build_worker_context() {
  local task_id="$1"
  local task_json
  task_json="$(task_get "$task_id")"
  local title description branch
  title="$(echo "$task_json" | jq -r '.title')"
  description="$(echo "$task_json" | jq -r '.description')"
  branch="$(echo "$task_json" | jq -r '.worktree_branch')"
  local upstream
  upstream="$(_upstream_handoffs "$task_id")"
  local patterns
  patterns="$(_learned_patterns)"

  cat <<PROMPT
## Your Task

**ID:** ${task_id}
**Title:** ${title}
**Branch:** ${branch}

**Description:**
${description}

## Upstream Context (completed dependencies)

${upstream:-No upstream handoffs — this task has no dependencies.}

## Learned Patterns

${patterns}

## Instructions

Implement the task described above. When done:
1. Write a handoff file to handoff/${task_id}.md
2. Commit all your changes to this branch.
PROMPT
}

# ── Reviewer ─────────────────────────────────────────────────────────

build_reviewer_context() {
  local task_id="$1"
  local task_json
  task_json="$(task_get "$task_id")"
  local title branch
  title="$(echo "$task_json" | jq -r '.title')"
  branch="$(echo "$task_json" | jq -r '.worktree_branch')"
  local handoff
  handoff="$(_handoff_content "$task_id")"
  local diff
  diff="$(worktree_diff "$branch")"

  cat <<PROMPT
## Review: ${task_id} — ${title}

### Worker Handoff

${handoff:-No handoff file found.}

### Code Changes (diff main...${branch})

\`\`\`diff
${diff:-No diff available.}
\`\`\`

## Instructions

Evaluate correctness, security, style, and test coverage.
Write your verdict to handoff/${task_id}-review.md.
Start the file with either "APPROVED" or "REJECTED" on the first line.
PROMPT
}

# ── Integration Test ─────────────────────────────────────────────────

build_integration_context() {
  local run_id="$1"

  cat <<PROMPT
## Integration Test Run: ${run_id}

All approved branches have been merged to main.

## Instructions

1. Inspect the project for build/test tooling (Makefile, go.mod, build.gradle, package.json, etc.)
2. Run the full build.
3. Run the full test suite.
4. Report: pass/fail, coverage percentage, and any failures.
5. Write results to handoff/integration-${run_id}.md.
PROMPT
}

# ── PostMortem ───────────────────────────────────────────────────────

# Collect handoff file contents, optionally filtered by task IDs.
# Usage: _collect_handoffs [task_id ...]
#   With no args: collects ALL handoff files (legacy behavior).
#   With task IDs: collects only handoff and review files for those tasks,
#                  plus any integration-*.md files.
_collect_handoffs() {
  local result=""
  if [ $# -eq 0 ]; then
    # No filter — collect all handoff files
    for f in "${HANDOFF_DIR}"/*.md; do
      [ -f "$f" ] || continue
      result="${result}
--- $(basename "$f") ---
$(cat "$f")
"
    done
  else
    # Filtered — collect only files for specified task IDs + integration reports
    local tid
    for tid in "$@"; do
      local hf="${HANDOFF_DIR}/${tid}.md"
      if [ -f "$hf" ]; then
        result="${result}
--- $(basename "$hf") ---
$(cat "$hf")
"
      fi
      local rf="${HANDOFF_DIR}/${tid}-review.md"
      if [ -f "$rf" ]; then
        result="${result}
--- $(basename "$rf") ---
$(cat "$rf")
"
      fi
    done
    # Also include integration reports from this run
    for f in "${HANDOFF_DIR}"/integration-*.md; do
      [ -f "$f" ] || continue
      result="${result}
--- $(basename "$f") ---
$(cat "$f")
"
    done
  fi
  printf '%s' "$result"
}

# Build postmortem context and write it to a temp file.
# Usage: build_postmortem_context [task_id ...]
#   With no args: includes ALL handoff files.
#   With task IDs: includes only handoff files for those tasks.
# Outputs the path to the temp file (caller must clean up).
build_postmortem_context() {
  local all_handoffs
  all_handoffs="$(_collect_handoffs "$@")"

  local tasks_summary
  tasks_summary="$(jq -r '.tasks[] | "- \(.id) [\(.status)] \(.title) (est: \(.estimate_minutes)m, started: \(.started_at // "n/a"), finished: \(.finished_at // "n/a"))"' "$TASKS_FILE")"

  local tmpfile
  tmpfile="$(mktemp "${TMPDIR:-/tmp}/postmortem-context.XXXXXX")"

  cat > "$tmpfile" <<PROMPT
## PostMortem Analysis

### All Handoff Files

${all_handoffs}

### Task Timing Summary

${tasks_summary}

## Instructions

Analyze what worked, what failed, and estimate accuracy.
Extract reusable patterns specific to THIS codebase.
Append findings to the "Learned Patterns" section of CLAUDE.md
(between the LEARNED PATTERNS START and END comment markers).
Then commit the updated CLAUDE.md.
PROMPT

  printf '%s' "$tmpfile"
}
