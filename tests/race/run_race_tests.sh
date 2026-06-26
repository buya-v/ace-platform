#!/usr/bin/env bash
#
# run_race_tests.sh — R009 concurrency race-gate for the GarudaX trading engines.
#
# Drives `go test -race` against the engine packages whose concurrency bugs were
# fixed in R008 (handler-invoked-in-lock, unguarded concurrent map read, and
# unsynchronized handler-field writes). The race detector + the deadlock-regression
# tests prove those fixes hold and guard against regressions.
#
# The engine logic lives in `internal/` packages, so the tests that exercise it
# must live inside each engine module (R008 added them there). This script is the
# tests/-side race-gate that runs them as one suite — suitable for CI (see R014).
#
# Usage:
#   ./tests/race/run_race_tests.sh            # focused: the R008/R009 concurrency tests
#   ./tests/race/run_race_tests.sh --full     # whole affected packages under -race
#   ./tests/race/run_race_tests.sh --verbose  # stream full `go test -v` output
#
# Exit status: 0 if every package is race-clean and all tests pass, 1 otherwise.
#
set -uo pipefail

# --- locate the repo root (this script lives at <root>/tests/race/) -----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# The engine modules declare `go 1.25.x`; allow the toolchain to be fetched if the
# host go is older. This keeps the zero-go.sum/relative-replace build working.
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

# --- flags --------------------------------------------------------------------
FULL=0
VERBOSE=0
for arg in "$@"; do
  case "$arg" in
    --full)    FULL=1 ;;
    --verbose) VERBOSE=1 ;;
    -h|--help)
      grep '^#' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown flag: $arg (try --help)" >&2
      exit 2
      ;;
  esac
done

# --- the race suite -----------------------------------------------------------
# Each entry: "<module-dir>|<package>|<-run regexp>|<human label>"
# The -run regexp targets R008's focused concurrency tests. In --full mode the
# regexp is dropped and the whole package runs under -race.
SUITE=(
  "src/clearing-engine|./internal/engine/|TestClearTradeHandlerRunsOutsideLock|TestClearTradeConcurrentRace|clearing: concurrent ClearTrade + callback"
  "src/settlement-engine|./internal/engine/|TestGetInstrumentConfigConcurrentWithRegister|TestRunSettlementCycleHandlerOutsideLock|TestRunSettlementCycleConcurrentRace|settlement: concurrent RegisterInstrument during a cycle"
  "src/margin-engine|./internal/margincall/|TestEvaluateHandlerRunsOutsideLock|TestEvaluateConcurrentRace|margin: concurrent handler invocation (margincall)"
  "src/margin-engine|./internal/engine/|TestCalculateMargin.*|margin: CalculateMargin + handler (engine pkg)"
)

# Packages run under -race in --full mode (whole-package regression gate).
FULL_TARGETS=(
  "src/clearing-engine|./internal/engine/"
  "src/settlement-engine|./internal/engine/"
  "src/margin-engine|./internal/engine/|./internal/margincall/"
)

pass=0
fail=0
declare -a RESULTS=()

run_one() {
  local label="$1"; shift
  local dir="$1"; shift
  # remaining args: go test args
  local out
  if [[ $VERBOSE -eq 1 ]]; then
    ( cd "${ROOT_DIR}/${dir}" && go test -race -v "$@" )
    local rc=$?
  else
    out="$( cd "${ROOT_DIR}/${dir}" && go test -race "$@" 2>&1 )"
    local rc=$?
    [[ $rc -ne 0 ]] && printf '%s\n' "$out"
  fi
  if [[ $rc -eq 0 ]]; then
    pass=$((pass+1)); RESULTS+=("PASS  ${label}")
    echo "  ✓ PASS  ${label}"
  else
    fail=$((fail+1)); RESULTS+=("FAIL  ${label}")
    echo "  ✗ FAIL  ${label}"
  fi
}

echo "==> GarudaX concurrency race-gate (R009)  [GOTOOLCHAIN=${GOTOOLCHAIN}]"
if [[ $FULL -eq 1 ]]; then
  echo "    mode: --full (whole affected packages under -race)"
  for entry in "${FULL_TARGETS[@]}"; do
    IFS='|' read -r -a parts <<< "$entry"
    dir="${parts[0]}"
    pkgs=("${parts[@]:1}")
    run_one "${dir} ${pkgs[*]}" "$dir" "${pkgs[@]}"
  done
else
  echo "    mode: focused (R008/R009 concurrency tests)"
  for entry in "${SUITE[@]}"; do
    IFS='|' read -r -a parts <<< "$entry"
    dir="${parts[0]}"
    pkg="${parts[1]}"
    # everything except the last field is a test name; last field is the label
    local_n=${#parts[@]}
    label="${parts[$((local_n-1))]}"
    tests=("${parts[@]:2:$((local_n-3))}")
    run_regexp="$(IFS='|'; echo "${tests[*]}")"
    run_one "$label" "$dir" -run "^(${run_regexp})$" "$pkg"
  done
fi

echo ""
echo "==> Summary: ${pass} passed, ${fail} failed"
for r in "${RESULTS[@]}"; do echo "    ${r}"; done

[[ $fail -eq 0 ]] || exit 1
echo "==> All engine packages are race-clean. R008's concurrency fixes hold."
