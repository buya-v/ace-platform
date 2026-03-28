# Review — T041: Fix PostMortem Context Size Bug

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The core fix is correct: `build_postmortem_context` now writes assembled context to a temp file via `mktemp` and returns the file path. The caller in `run_postmortem` pipes this via stdin (`< "$context_file"`) instead of passing it as a `-p` argument, which directly resolves the E2BIG / ARG_MAX bug documented in the learned pattern.

- `_collect_handoffs` filtering logic correctly handles both filtered (task IDs) and unfiltered (all files) modes.
- Integration reports are always included even in filtered mode — correct behavior.
- The `# shellcheck disable=SC2086` on the unquoted `$run_task_ids` is intentional word-splitting for multiple IDs — acceptable.
- The EXIT trap in `run_postmortem` overwrites any prior EXIT trap, but since it's a safety net (explicit `rm -f` runs first) and this is the last phase before script exit, this is benign.

### Security: PASS

- Temp file created via `mktemp` with restricted default permissions — safe.
- No user-controlled input flows into shell evaluation unsafely.
- `jq` calls in `state.sh` use string interpolation in filters (`\"${task_id}\"`) rather than `--arg` — minor hygiene issue but task IDs are system-controlled (T### format), so no practical injection risk.
- No hardcoded secrets or credentials.

### Code Quality: PASS

- The fix is minimal and targeted — `build_postmortem_context` returns a file path, caller reads via stdin. Clean separation.
- `_collect_handoffs` is a well-structured helper with clear filtered/unfiltered behavior.
- The worker had to copy all `pipeline/` files into the worktree because the directory was never committed to git. This adds diff noise (log.sh, state.sh, worktree.sh, all prompts are unchanged copies) but is a legitimate workaround for the blocker described in the handoff. The follow-up to commit `pipeline/` to git is the right fix.
- Naming and structure follow existing project conventions.

### Test Coverage: PASS

23 tests in `tests/t041_postmortem_context_test.sh` covering:
- Temp file output (returns path, file exists, contains expected headers and content)
- Unfiltered collection (all handoff files included)
- Filtered collection (only specified task IDs, excludes others, includes integration reports)
- Filtered `build_postmortem_context` (end-to-end with filter args)
- Large context scenario (50 x 10KB files — validates the fix would have exceeded ARG_MAX)
- Temp file cleanup
- Static analysis of `run.sh` (stdin redirection, trap, function call present)

Tests assert meaningful behavior, not just "runs without error."

## Required Fixes

None.

## Suggestions (non-blocking)

1. **`_tasks_update` jq injection hardening:** In `state.sh:18`, consider using `jq --arg` instead of shell string interpolation inside the jq filter. Not exploitable today (task IDs are system-controlled) but better hygiene for a function that writes to `tasks.json`.
2. **Commit `pipeline/` to git on main.** The worker had to copy all pipeline files as a workaround. A follow-up task should commit the canonical `pipeline/` directory so future worktrees inherit it.
3. **EXIT trap stacking:** `run_postmortem` sets `trap 'rm -f "$context_file"' EXIT` which overwrites any prior EXIT trap. Consider appending to an existing trap or using a cleanup function that handles all temp files.
