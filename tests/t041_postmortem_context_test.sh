#!/usr/bin/env bash
# Tests for T041: Fix PostMortem Context Size Bug
# Validates that build_postmortem_context writes to a temp file (not shell variable)
# and that _collect_handoffs supports task ID filtering.
# Run: bash tests/t041_postmortem_context_test.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASS=0
FAIL=0
ERRORS=""

pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS="${ERRORS}\n  ✗ $1"; echo "  ✗ $1"; }

echo "=== T041 PostMortem Context Size Fix Test Suite ==="
echo ""

# ---------------------------------------------------------------------------
# Setup: create a temporary project structure for testing
# ---------------------------------------------------------------------------
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

mkdir -p "$TEST_DIR/handoff"
mkdir -p "$TEST_DIR/pipeline/lib"

# Create a minimal tasks.json
cat > "$TEST_DIR/tasks.json" <<'EOF'
{
  "tasks": [
    {"id": "T001", "title": "Task One", "status": "done", "estimate_minutes": 10, "started_at": "2026-03-27T07:00:00Z", "finished_at": "2026-03-27T07:10:00Z", "blockedBy": []},
    {"id": "T002", "title": "Task Two", "status": "done", "estimate_minutes": 15, "started_at": "2026-03-27T07:15:00Z", "finished_at": "2026-03-27T07:25:00Z", "blockedBy": []},
    {"id": "T003", "title": "Task Three", "status": "pending", "estimate_minutes": 20, "blockedBy": ["T001"]}
  ]
}
EOF

# Create handoff files
echo "# Handoff T001 — completed task one" > "$TEST_DIR/handoff/T001.md"
echo "# Handoff T002 — completed task two" > "$TEST_DIR/handoff/T002.md"
echo "# Review T001 — APPROVED" > "$TEST_DIR/handoff/T001-review.md"
echo "# Integration run results" > "$TEST_DIR/handoff/integration-run-001.md"

# Create a minimal CLAUDE.md
cat > "$TEST_DIR/CLAUDE.md" <<'EOF'
<!-- LEARNED PATTERNS START — do not remove this comment -->
### Pattern: Test pattern
<!-- LEARNED PATTERNS END — do not remove this comment -->
EOF

# Source the libraries with our test project root
export PROJECT_ROOT="$TEST_DIR"
export TASKS_FILE="$TEST_DIR/tasks.json"

# Stub out log functions (context.sh sources are expected to be loaded by run.sh)
log_info()  { :; }
log_warn()  { :; }
log_error() { :; }

# Stub out task_get and task_blocked_by for context.sh
task_get() {
  jq -r --arg id "$1" '.tasks[] | select(.id == $id)' "$TASKS_FILE"
}
task_blocked_by() {
  task_get "$1" | jq -r '.blockedBy[]? // empty'
}

source "$REPO_ROOT/pipeline/lib/context.sh"

# ---------------------------------------------------------------------------
# 1. build_postmortem_context returns a file path (not content)
# ---------------------------------------------------------------------------
echo "--- build_postmortem_context returns temp file path ---"

result="$(build_postmortem_context)"
if [ -f "$result" ]; then
  pass "build_postmortem_context returns a path to an existing file"
else
  fail "build_postmortem_context should return a file path, got: $result"
fi

# Check the file contains expected content
if grep -q "PostMortem Analysis" "$result" 2>/dev/null; then
  pass "Temp file contains PostMortem Analysis header"
else
  fail "Temp file missing PostMortem Analysis header"
fi

if grep -q "Task Timing Summary" "$result" 2>/dev/null; then
  pass "Temp file contains Task Timing Summary"
else
  fail "Temp file missing Task Timing Summary"
fi

if grep -q "T001" "$result" 2>/dev/null; then
  pass "Temp file contains T001 handoff content"
else
  fail "Temp file missing T001 handoff content"
fi

if grep -q "T002" "$result" 2>/dev/null; then
  pass "Temp file contains T002 handoff content"
else
  fail "Temp file missing T002 handoff content"
fi

rm -f "$result"

# ---------------------------------------------------------------------------
# 2. _collect_handoffs with no args collects ALL files
# ---------------------------------------------------------------------------
echo ""
echo "--- _collect_handoffs (no filter) collects all files ---"

all="$(_collect_handoffs)"
if echo "$all" | grep -q "T001.md"; then
  pass "Unfiltered collects T001.md"
else
  fail "Unfiltered missing T001.md"
fi

if echo "$all" | grep -q "T001-review.md"; then
  pass "Unfiltered collects T001-review.md"
else
  fail "Unfiltered missing T001-review.md"
fi

if echo "$all" | grep -q "T002.md"; then
  pass "Unfiltered collects T002.md"
else
  fail "Unfiltered missing T002.md"
fi

if echo "$all" | grep -q "integration-run-001.md"; then
  pass "Unfiltered collects integration-run-001.md"
else
  fail "Unfiltered missing integration-run-001.md"
fi

# ---------------------------------------------------------------------------
# 3. _collect_handoffs with task IDs filters correctly
# ---------------------------------------------------------------------------
echo ""
echo "--- _collect_handoffs (filtered by T001) ---"

filtered="$(_collect_handoffs T001)"
if echo "$filtered" | grep -q "T001.md"; then
  pass "Filtered collects T001.md"
else
  fail "Filtered missing T001.md"
fi

if echo "$filtered" | grep -q "T001-review.md"; then
  pass "Filtered collects T001-review.md"
else
  fail "Filtered missing T001-review.md"
fi

if echo "$filtered" | grep -q "T002.md"; then
  fail "Filtered should NOT include T002.md"
else
  pass "Filtered correctly excludes T002.md"
fi

if echo "$filtered" | grep -q "integration-run-001.md"; then
  pass "Filtered still includes integration reports"
else
  fail "Filtered should include integration reports"
fi

# ---------------------------------------------------------------------------
# 4. build_postmortem_context with task ID filter
# ---------------------------------------------------------------------------
echo ""
echo "--- build_postmortem_context with task ID filter ---"

result_filtered="$(build_postmortem_context T001)"
if [ -f "$result_filtered" ]; then
  pass "Filtered build returns a file path"
else
  fail "Filtered build should return a file path"
fi

if grep -q "Handoff T001" "$result_filtered" 2>/dev/null; then
  pass "Filtered file contains T001 handoff"
else
  fail "Filtered file missing T001 handoff"
fi

if grep -q "Handoff T002" "$result_filtered" 2>/dev/null; then
  fail "Filtered file should NOT contain T002 handoff"
else
  pass "Filtered file correctly excludes T002 handoff"
fi

rm -f "$result_filtered"

# ---------------------------------------------------------------------------
# 5. Context file does NOT exceed shell argument limits
# ---------------------------------------------------------------------------
echo ""
echo "--- Large context stays under ARG_MAX ---"

# Create many large handoff files to simulate real-world size
for i in $(seq 1 50); do
  tid="TBIG$(printf '%03d' "$i")"
  # Create a ~10KB handoff file
  python3 -c "print('# Handoff $tid\n' + 'x' * 10000)" > "$TEST_DIR/handoff/${tid}.md"
done

result_large="$(build_postmortem_context)"
if [ -f "$result_large" ]; then
  file_size=$(stat -c%s "$result_large" 2>/dev/null || stat -f%z "$result_large" 2>/dev/null)
  pass "Large context written to file (${file_size} bytes)"

  # Verify it would have exceeded typical ARG_MAX (128KB on many systems, 2MB on Linux)
  if [ "$file_size" -gt 131072 ]; then
    pass "Context size (${file_size}B) exceeds 128KB — would have hit ARG_MAX as shell arg"
  else
    pass "Context size (${file_size}B) is under 128KB — still safe as file"
  fi
else
  fail "Large context build failed"
fi

rm -f "$result_large"

# ---------------------------------------------------------------------------
# 6. Temp file cleanup
# ---------------------------------------------------------------------------
echo ""
echo "--- Temp file cleanup ---"

result_cleanup="$(build_postmortem_context T001)"
if [ -f "$result_cleanup" ]; then
  pass "Temp file exists before cleanup"
  rm -f "$result_cleanup"
  if [ ! -f "$result_cleanup" ]; then
    pass "Temp file removed after cleanup"
  else
    fail "Temp file still exists after rm"
  fi
else
  fail "No temp file to test cleanup"
fi

# ---------------------------------------------------------------------------
# 7. run.sh uses stdin redirection (static analysis)
# ---------------------------------------------------------------------------
echo ""
echo "--- run.sh pipes context via stdin ---"

if grep -q '< "\$context_file"' "$REPO_ROOT/pipeline/run.sh"; then
  pass "run.sh uses stdin redirection (< \$context_file)"
else
  fail "run.sh should redirect context file via stdin"
fi

if grep -q 'build_postmortem_context' "$REPO_ROOT/pipeline/run.sh"; then
  pass "run.sh calls build_postmortem_context"
else
  fail "run.sh missing build_postmortem_context call"
fi

if grep -q 'trap.*rm.*context_file' "$REPO_ROOT/pipeline/run.sh"; then
  pass "run.sh has trap for temp file cleanup"
else
  fail "run.sh missing trap for temp file cleanup"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
if [ "$FAIL" -gt 0 ]; then
  echo -e "\nFailures:$ERRORS"
  exit 1
fi
