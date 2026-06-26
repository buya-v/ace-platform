#!/usr/bin/env bash
#
# verify_prefix_failure.sh — proves the R008/R009 concurrency tests have teeth.
#
# A passing race-gate is only meaningful if the tests would FAIL on the buggy
# (pre-R008) code. This script reconstructs the two pre-R008 bug shapes in a
# THROWAWAY copy of the clearing engine (committed src/ is never touched) and
# asserts that the corresponding tests fail:
#
#   bug #1 (handler-invoked-in-lock)  => TestClearTradeHandlerRunsOutsideLock
#                                        deadlocks and FAILS (2s timeout).
#   bug #3 (unsynchronized handler write) => TestClearTradeConcurrentRace
#                                        FAILS with `WARNING: DATA RACE` under -race.
#
# Exit status: 0 if BOTH tests fail on the pre-fix code as expected (i.e. the
# tests are sound), 1 if either unexpectedly passes (tests are toothless).
#
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

SRC="${ROOT_DIR}/src/clearing-engine"
SHARED="${ROOT_DIR}/src/shared/pkg/types/decimal"
TMP="$(mktemp -d /tmp/clearing-prefix.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

cp -r "$SRC"/. "$TMP"/
# The committed module uses a relative `replace` for the shared decimal module;
# rewrite it to an absolute path so the throwaway copy still builds.
( cd "$TMP" && go mod edit -replace "github.com/garudax-platform/decimal=${SHARED}" )

# Reconstruct the pre-R008 engine.go: invoke the handler INSIDE e.mu, and write
# the handler field with no lock.
python3 - "$TMP/internal/engine/engine.go" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()

# bug #1: handler invoked while holding e.mu (re-entrant => deadlock).
s = s.replace(
'''	result, novResult, handler, err := e.clearTradeLocked(trade)
	if err != nil {
		return nil, err
	}

	// Invoke the trade handler OUTSIDE the critical section. Running an
	// arbitrary callback while holding e.mu risks deadlock if it re-enters
	// the engine (e.g. NetObligations) and serializes unrelated clearing.
	if handler != nil {
		handler(trade, novResult)
	}

	return result, nil''',
'''	// PRE-R008 SIMULATION: handler invoked while holding e.mu (bug).
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processedTrades[trade.TradeID] {
		return nil, fmt.Errorf("clearing: trade %s already processed", trade.TradeID)
	}
	novResult, err := e.novationSvc.Novate(trade)
	if err != nil {
		return nil, err
	}
	_ = e.oblStore.Append(novResult.BuyerObligation)
	_ = e.oblStore.Append(novResult.SellerObligation)
	e.processedTrades[trade.TradeID] = true
	if e.tradeHandler != nil {
		e.tradeHandler(trade, novResult) // in-lock => re-entrant NetObligations deadlocks
	}
	return &ClearingResult{}, nil''', 1)

# bug #3: unsynchronized write to the handler field.
s = s.replace(
'''func (e *Engine) SetTradeHandler(h TradeHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tradeHandler = h
}''',
'''func (e *Engine) SetTradeHandler(h TradeHandler) {
	e.tradeHandler = h // PRE-R008: unsynchronized write races with locked reads
}''', 1)

open(p, 'w').write(s)
ok = "PRE-R008 SIMULATION" in s and "PRE-R008: unsynchronized" in s
sys.exit(0 if ok else 3)
PY
if [[ $? -ne 0 ]]; then
  echo "✗ could not apply pre-fix patch (engine.go shape changed?)" >&2
  exit 1
fi

rc=0

echo "==> [pre-fix] bug #1: handler-in-lock should DEADLOCK the test"
out1="$( cd "$TMP" && go test -race -run '^TestClearTradeHandlerRunsOutsideLock$' -timeout 30s ./internal/engine/ 2>&1 )"
if echo "$out1" | grep -q "deadlocked"; then
  echo "  ✓ expected FAIL observed (deadlock detected by the test)"
else
  echo "  ✗ UNEXPECTED: test did not fail on pre-fix code"; echo "$out1" | tail -5; rc=1
fi

echo "==> [pre-fix] bug #3: unsynchronized handler write should trip -race"
out2="$( cd "$TMP" && go test -race -run '^TestClearTradeConcurrentRace$' -timeout 60s ./internal/engine/ 2>&1 )"
if echo "$out2" | grep -q "DATA RACE"; then
  echo "  ✓ expected FAIL observed (WARNING: DATA RACE)"
else
  echo "  ✗ UNEXPECTED: -race did not fire on pre-fix code"; echo "$out2" | tail -5; rc=1
fi

echo ""
if [[ $rc -eq 0 ]]; then
  echo "==> Both tests FAIL on pre-fix code and PASS on the committed (R008-fixed) code."
  echo "    The race-gate is sound."
else
  echo "==> One or more tests did not fail on pre-fix code — they may be toothless."
fi
exit $rc
