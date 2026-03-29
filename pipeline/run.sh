#!/usr/bin/env bash
set -euo pipefail

# ── ACE Platform — AI Development Pipeline Orchestrator ──────────────
#
# Usage:
#   ./pipeline/run.sh "Build the auth service"
#   ./pipeline/run.sh --resume                    # Resume from tasks.json state
#   ./pipeline/run.sh --status                    # Print task summary
#
# Environment variables:
#   MAX_PARALLEL      Max concurrent worker agents (default: 3)
#   BUDGET_PER_AGENT  USD budget per agent call (default: 5)
#   DRY_RUN           Set to "true" to print commands without executing (default: false)
# ─────────────────────────────────────────────────────────────────────

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PROJECT_ROOT

source "${PROJECT_ROOT}/pipeline/lib/log.sh"
source "${PROJECT_ROOT}/pipeline/lib/state.sh"
source "${PROJECT_ROOT}/pipeline/lib/context.sh"
source "${PROJECT_ROOT}/pipeline/lib/worktree.sh"

MAX_PARALLEL="${MAX_PARALLEL:-3}"
BUDGET_PER_AGENT="${BUDGET_PER_AGENT:-5}"
DRY_RUN="${DRY_RUN:-false}"
INTEGRATION_RUN_ID="run-$(date -u +%Y%m%d-%H%M%S)"

# ── Helpers ──────────────────────────────────────────────────────────

_claude() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log_info "[DRY RUN] claude $*"
    return 0
  fi

  local attempt max_attempts=3 delay=30
  for attempt in $(seq 1 $max_attempts); do
    if claude "$@" 2>&1; then
      return 0
    fi
    local exit_code=$?
    if [ "$attempt" -lt "$max_attempts" ]; then
      log_warn "Claude call failed (attempt $attempt/$max_attempts), retrying in ${delay}s..."
      sleep "$delay"
      delay=$((delay * 2))
    else
      log_error "Claude call failed after $max_attempts attempts"
      return "$exit_code"
    fi
  done
}

usage() {
  cat <<EOF
Usage: $(basename "$0") [OPTIONS] [REQUIREMENT]

Options:
  --resume    Resume pipeline from current tasks.json state (skip planner)
  --status    Print task summary and exit
  --dry-run   Print agent commands without executing
  -h, --help  Show this help

Arguments:
  REQUIREMENT   Natural language description of the feature to build

Examples:
  $(basename "$0") "Build the auth service with JWT + OAuth2"
  $(basename "$0") --resume
  $(basename "$0") --status
EOF
}

# ── Agent Runners ────────────────────────────────────────────────────

run_planner() {
  local requirement="$1"
  log_section "Phase 1: Planning"
  log_info "Running Planner Agent..."

  # Write context to temp file to avoid MAX_ARG_STRLEN (128KB per-arg limit)
  local context_file
  context_file="$(mktemp "${TMPDIR:-/tmp}/planner-context.XXXXXX")"
  build_planner_context "$requirement" > "$context_file"

  _claude -p \
    --append-system-prompt "$(cat "${PROJECT_ROOT}/pipeline/prompts/planner.md")" \
    --tools "Read,Write,Edit,Glob,Grep" \
    --permission-mode bypassPermissions \
    --max-budget-usd "$BUDGET_PER_AGENT" \
    < "$context_file"

  rm -f "$context_file"
  log_success "Planner complete — tasks.json updated"
  echo ""
  log_info "Task summary after planning:"
  task_summary
}

run_worker() {
  local task_id="$1"
  local task_json branch role prompt_file wt_path context

  task_json="$(task_get "$task_id")"
  branch="$(echo "$task_json" | jq -r '.worktree_branch')"
  role="$(echo "$task_json" | jq -r '.agent_role')"

  task_set_status "$task_id" "in_progress"
  task_set_started "$task_id"
  log_info "Spawning worker for $task_id [$role] on branch $branch"

  wt_path="$(worktree_create "$branch")"
  context="$(build_worker_context "$task_id")"

  # Select prompt based on role
  prompt_file="${PROJECT_ROOT}/pipeline/prompts/coder.md"
  case "$role" in
    test_writer) prompt_file="${PROJECT_ROOT}/pipeline/prompts/test-writer.md" ;;
  esac

  # Write context to temp file to avoid MAX_ARG_STRLEN (128KB per-arg limit)
  local context_file
  context_file="$(mktemp "${TMPDIR:-/tmp}/worker-context.XXXXXX")"
  echo "$context" > "$context_file"

  (cd "$wt_path" && _claude -p \
    --append-system-prompt "$(cat "$prompt_file")" \
    --tools "Read,Write,Edit,Bash,Glob,Grep" \
    --permission-mode bypassPermissions \
    --max-budget-usd "$BUDGET_PER_AGENT" \
    < "$context_file")

  rm -f "$context_file"

  task_set_status "$task_id" "done"
  task_set_finished "$task_id"
  log_success "Worker complete: $task_id"
}

run_reviewer() {
  local task_id="$1"
  log_info "Running Reviewer for $task_id..."

  local context
  # Write context to temp file to avoid MAX_ARG_STRLEN (128KB per-arg limit)
  local context_file
  context_file="$(mktemp "${TMPDIR:-/tmp}/reviewer-context.XXXXXX")"
  build_reviewer_context "$task_id" > "$context_file"

  _claude -p \
    --append-system-prompt "$(cat "${PROJECT_ROOT}/pipeline/prompts/reviewer.md")" \
    --tools "Read,Write,Glob,Grep" \
    --permission-mode bypassPermissions \
    --max-budget-usd "$BUDGET_PER_AGENT" \
    < "$context_file"

  rm -f "$context_file"

  # Check the verdict
  local review_file="${HANDOFF_DIR}/${task_id}-review.md"
  if [ -f "$review_file" ] && head -5 "$review_file" | grep -qi "APPROVED"; then
    local branch
    branch="$(task_branch "$task_id")"
    if worktree_merge "$branch"; then
      worktree_cleanup "$branch"
      log_success "APPROVED & merged: $task_id"
    else
      log_error "Merge failed for $task_id — manual resolution needed"
      task_set_status "$task_id" "rejected"
    fi
  else
    task_set_status "$task_id" "rejected"
    local branch
    branch="$(task_branch "$task_id")"
    worktree_cleanup "$branch"
    log_warn "REJECTED: $task_id — see handoff/${task_id}-review.md"
  fi
}

run_integration_test() {
  log_section "Phase 3: Integration Test"
  log_info "Running integration test (run: $INTEGRATION_RUN_ID)..."

  local context
  context="$(build_integration_context "$INTEGRATION_RUN_ID")"

  _claude -p \
    --append-system-prompt "$(cat "${PROJECT_ROOT}/pipeline/prompts/integration-test.md")" \
    --tools "Read,Write,Bash,Glob,Grep" \
    --permission-mode bypassPermissions \
    --max-budget-usd "$BUDGET_PER_AGENT" \
    "$context"

  log_success "Integration test complete — see handoff/integration-${INTEGRATION_RUN_ID}.md"
}

run_postmortem() {
  log_section "Phase 4: PostMortem"
  log_info "Running PostMortem Agent..."

  # Collect task IDs from this run (all non-pending tasks) for focused context.
  local run_task_ids
  run_task_ids="$(jq -r '.tasks[] | select(.status != "pending") | .id' "$TASKS_FILE")"

  # build_postmortem_context writes to a temp file and returns its path.
  # This avoids passing large context as a shell argument (ARG_MAX / E2BIG).
  local context_file
  # shellcheck disable=SC2086
  context_file="$(build_postmortem_context $run_task_ids)"
  trap 'rm -f "$context_file"' EXIT

  _claude -p \
    --append-system-prompt "$(cat "${PROJECT_ROOT}/pipeline/prompts/postmortem.md")" \
    --tools "Read,Write,Edit,Bash,Glob,Grep" \
    --permission-mode bypassPermissions \
    --max-budget-usd "$BUDGET_PER_AGENT" \
    < "$context_file"

  rm -f "$context_file"
  log_success "PostMortem complete — CLAUDE.md updated with learned patterns"
}

# ── Main Loop ────────────────────────────────────────────────────────

execute_tasks() {
  log_section "Phase 2: Execute Tasks"

  local iteration=0
  local max_iterations=50  # safety valve

  while ! tasks_all_done; do
    iteration=$((iteration + 1))
    if [ "$iteration" -gt "$max_iterations" ]; then
      log_error "Max iterations ($max_iterations) reached — aborting"
      break
    fi

    local ready_tasks
    ready_tasks="$(task_list_ready)"

    if [ -z "$ready_tasks" ]; then
      local pending in_progress
      pending="$(task_count_pending)"
      in_progress="$(task_count_in_progress)"
      if [ "$pending" -gt 0 ] || [ "$in_progress" -gt 0 ]; then
        log_warn "No ready tasks but $pending pending, $in_progress in-progress — possible deadlock"
        log_warn "Check tasks.json for circular dependencies or all-rejected tasks"
      fi
      break
    fi

    log_info "Iteration $iteration — ready tasks: $(echo "$ready_tasks" | tr '\n' ' ')"

    # Collect batch (up to MAX_PARALLEL)
    local batch=()
    while IFS= read -r tid; do
      batch+=("$tid")
      [ "${#batch[@]}" -ge "$MAX_PARALLEL" ] && break
    done <<< "$ready_tasks"

    # Spawn workers in parallel
    local pids=()
    for tid in "${batch[@]}"; do
      run_worker "$tid" &
      pids+=($!)
      log_info "Spawned worker PID ${pids[-1]} for $tid"
    done

    # Wait and review each
    for i in "${!pids[@]}"; do
      local pid="${pids[$i]}"
      local tid="${batch[$i]}"
      if wait "$pid"; then
        run_reviewer "$tid"
      else
        log_error "Worker for $tid exited with error"
        task_set_status "$tid" "rejected"
        task_set_finished "$tid"
      fi
    done

    echo ""
    log_info "Task summary after iteration $iteration:"
    task_summary
    echo ""
  done
}

main() {
  local mode="full"
  local requirement=""

  # Parse arguments
  while [ $# -gt 0 ]; do
    case "$1" in
      --resume)  mode="resume"; shift ;;
      --status)  mode="status"; shift ;;
      --dry-run) DRY_RUN="true"; shift ;;
      -h|--help) usage; exit 0 ;;
      *)         requirement="$1"; shift ;;
    esac
  done

  log_section "ACE Platform — AI Development Pipeline"
  log_info "Project root: $PROJECT_ROOT"
  log_info "Mode: $mode | Max parallel: $MAX_PARALLEL | Budget/agent: \$${BUDGET_PER_AGENT}"

  case "$mode" in
    status)
      task_summary
      exit 0
      ;;
    resume)
      log_info "Resuming from tasks.json state..."
      execute_tasks
      run_integration_test
      run_postmortem
      ;;
    full)
      if [ -z "$requirement" ]; then
        log_error "No requirement provided. Usage: $0 \"Build feature X\""
        exit 1
      fi
      run_planner "$requirement"
      execute_tasks
      run_integration_test
      run_postmortem
      ;;
  esac

  log_section "Pipeline Complete"
  task_summary
}

main "$@"
