#!/usr/bin/env bash
# Git worktree management for isolated agent workspaces.

WORKTREE_BASE="${WORKTREE_BASE:-${PROJECT_ROOT}/.worktrees}"

# Create a worktree for a branch. Prints the worktree path.
worktree_create() {
  local branch="$1"
  local safe_name
  safe_name="$(echo "$branch" | tr '/' '-')"
  local wt_path="${WORKTREE_BASE}/${safe_name}"

  mkdir -p "$WORKTREE_BASE"

  if git -C "$PROJECT_ROOT" worktree list --porcelain | grep -q "$wt_path"; then
    log_warn "Worktree already exists at $wt_path — reusing"
  else
    git -C "$PROJECT_ROOT" worktree add "$wt_path" -b "$branch" >/dev/null 2>&1 \
      || git -C "$PROJECT_ROOT" worktree add "$wt_path" "$branch" >/dev/null 2>&1
    log_info "Created worktree: $wt_path (branch: $branch)"
  fi

  printf '%s' "$wt_path"
}

# Merge a worktree branch into main (no-ff).
worktree_merge() {
  local branch="$1"

  log_info "Merging $branch into main..."
  git -C "$PROJECT_ROOT" checkout main
  if git -C "$PROJECT_ROOT" merge --no-ff "$branch" -m "merge: $branch into main"; then
    log_success "Merged $branch"
    return 0
  else
    log_error "Merge conflict on $branch — needs manual resolution"
    git -C "$PROJECT_ROOT" merge --abort
    return 1
  fi
}

# Get the diff between main and a branch (for reviewer context).
worktree_diff() {
  local branch="$1"
  git -C "$PROJECT_ROOT" diff "main...$branch" 2>/dev/null
}

# Get a stat summary of changes on a branch.
worktree_stat() {
  local branch="$1"
  git -C "$PROJECT_ROOT" diff --stat "main...$branch" 2>/dev/null
}

# Clean up a worktree and optionally delete the branch.
worktree_cleanup() {
  local branch="$1"
  local safe_name
  safe_name="$(echo "$branch" | tr '/' '-')"
  local wt_path="${WORKTREE_BASE}/${safe_name}"

  # Make sure we're on main before cleanup
  git -C "$PROJECT_ROOT" checkout main 2>/dev/null || true

  if [ -d "$wt_path" ]; then
    git -C "$PROJECT_ROOT" worktree remove "$wt_path" --force 2>/dev/null || true
    log_info "Removed worktree: $wt_path"
  fi

  # Delete branch only if already merged
  git -C "$PROJECT_ROOT" branch -d "$branch" 2>/dev/null && \
    log_info "Deleted merged branch: $branch" || true
}

# List all active worktrees.
worktree_list() {
  git -C "$PROJECT_ROOT" worktree list
}
