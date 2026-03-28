#!/usr/bin/env bash
# Task state management — reads/writes tasks.json using jq.
# All updates are atomic: write to tmp file, then mv.

TASKS_FILE="${TASKS_FILE:-${PROJECT_ROOT}/tasks.json}"

_TASKS_LOCK="${TASKS_FILE}.lock"

_tasks_update() {
  local filter="$1"
  local tmp
  # File-based lock to prevent parallel workers from corrupting tasks.json
  (
    flock -w 10 200 || { log_error "Failed to acquire tasks.json lock"; return 1; }
    tmp="$(mktemp "${TASKS_FILE}.tmp.XXXXXX")"
    if jq "$filter" "$TASKS_FILE" > "$tmp"; then
      mv "$tmp" "$TASKS_FILE"
    else
      rm -f "$tmp"
      return 1
    fi
  ) 200>"$_TASKS_LOCK"
}

# Get a single task object by ID (returns JSON).
task_get() {
  local task_id="$1"
  jq -r --arg id "$task_id" '.tasks[] | select(.id == $id)' "$TASKS_FILE"
}

# Set status of a task. Also updates last_updated timestamp.
task_set_status() {
  local task_id="$1" new_status="$2"
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  _tasks_update "
    (.tasks[] | select(.id == \"${task_id}\")).status = \"${new_status}\"
    | .last_updated = \"${now}\"
  "
  log_info "Task $task_id → $new_status"
}

# Record the actual start time for a task.
task_set_started() {
  local task_id="$1"
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  _tasks_update "
    (.tasks[] | select(.id == \"${task_id}\")).started_at = \"${now}\"
  "
}

# Record the actual end time for a task.
task_set_finished() {
  local task_id="$1"
  local now
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  _tasks_update "
    (.tasks[] | select(.id == \"${task_id}\")).finished_at = \"${now}\"
  "
}

# List IDs of tasks that are ready to run:
#   status == "pending" AND every blockedBy task has status == "done".
task_list_ready() {
  jq -r '
    [.tasks[] | select(.status == "done") | .id] as $done |
    .tasks[]
    | select(.status == "pending")
    | . as $task
    | if (($task.blockedBy // []) | length) == 0 then $task.id
      elif (($task.blockedBy // []) | map(. as $dep | $done | index($dep) != null) | all) then $task.id
      else empty
      end
  ' "$TASKS_FILE"
}

# List IDs of tasks with a given status.
task_get_by_status() {
  local status="$1"
  jq -r --arg s "$status" '.tasks[] | select(.status == $s) | .id' "$TASKS_FILE"
}

# Count pending tasks.
task_count_pending() {
  jq '[.tasks[] | select(.status == "pending")] | length' "$TASKS_FILE"
}

# Count in-progress tasks.
task_count_in_progress() {
  jq '[.tasks[] | select(.status == "in_progress")] | length' "$TASKS_FILE"
}

# True (return 0) if no pending or in_progress tasks remain.
tasks_all_done() {
  local remaining
  remaining=$(jq '[.tasks[] | select(.status == "pending" or .status == "in_progress")] | length' "$TASKS_FILE")
  [[ "$remaining" -eq 0 ]]
}

# Get the worktree branch for a task.
task_branch() {
  local task_id="$1"
  task_get "$task_id" | jq -r '.worktree_branch'
}

# Get the agent role for a task.
task_role() {
  local task_id="$1"
  task_get "$task_id" | jq -r '.agent_role'
}

# Get blockedBy list for a task (newline-separated IDs).
task_blocked_by() {
  local task_id="$1"
  task_get "$task_id" | jq -r '.blockedBy[]? // empty'
}

# Print a summary table of all tasks.
task_summary() {
  jq -r '.tasks[] | [.id, .status, .agent_role, .title] | @tsv' "$TASKS_FILE" \
    | column -t -s $'\t'
}
