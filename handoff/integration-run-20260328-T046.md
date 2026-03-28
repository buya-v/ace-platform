# Integration Test Report — Phase Debt (T046)

**Run ID:** run-20260328-T046
**Date:** 2026-03-28
**Trigger:** Phase Debt completion (T040-T045)
**Result:** **PASS** — 513 tests, 0 failures, 9 services

---

## Service Build + Test Summary

| Service | Build | Tests | Failures | Overall Coverage | Status |
|---------|-------|-------|----------|-----------------|--------|
| auth-service | OK | 45 | 0 | 56.3% | PASS |
| clearing-engine | OK | 116 | 0 | 78.4% | PASS |
| margin-engine | OK | 71 | 0 | 55.7% | PASS |
| settlement-engine | OK | 68 | 0 | 54.5% | PASS |
| matching-engine | OK | 53 | 0 | 67.3% | PASS |
| compliance-service | OK | 39 | 0 | 70.7% | PASS |
| gateway | OK | 30 | 0 | 49.1% | PASS |
| market-data-service | OK | 36 | 0 | 66.9% | PASS |
| warehouse-service | OK | 55 | 0 | 75.0% | PASS |
| **Total** | **9/9** | **513** | **0** | **63.8% avg** | **PASS** |

---

## Phase Debt Task Verification

### T040 — Auth Service Rebuild: PASS
- Compiles cleanly, 45 tests pass, 0 failures
- Business logic (`internal/auth`): **88.5%** coverage — exceeds 60% floor
- Handler (`internal/handler`): 34.1% — infrastructure package, below 60% but acceptable per convention
- Overall: 56.3%

### T041 — PostMortem Pipeline Fix: PASS
- `pipeline/lib/context.sh` exists with `build_postmortem_context` using temp file approach
- Function writes to `mktemp` file and returns path (line 241-263)
- No shell variable accumulation — ARG_MAX error is resolved
- Includes `_collect_handoffs` with task ID filtering

### T042 — Naming Cleanup: PASS
- `src/clearing-service/` directory does not exist — successfully removed
- `src/clearing-engine/` contains full Go module with 116 passing tests
- No broken references found (`go vet` clean on all services)

### T043 — Clearing Engine Coverage: PASS
Coverage targets met (isolated per-package):

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `internal/novation/` | 100.0% | 60%+ | PASS |
| `internal/netting/` | 100.0% | 60%+ | PASS |
| `internal/engine/` | 95.0% | 60%+ | PASS |
| `internal/position/` | 98.5% | — | PASS |
| `internal/store/` | 100.0% | — | PASS |
| `internal/types/` | 100.0% | — | PASS |

### T044 — Margin Engine Coverage: PASS
Coverage targets met (isolated per-package):

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `internal/scanner/` | 95.2% | 60%+ | PASS |
| `internal/engine/` | 96.7% | 60%+ | PASS |

Note: Cross-package (`./...`) coverage shows lower numbers (32-38%) due to Go's coverage counting including all transitively imported code. Isolated per-package coverage is the accurate measure.

### T045 — Settlement Engine Coverage: PASS
Coverage targets met (isolated per-package):

| Package | Coverage | Target | Status |
|---------|----------|--------|--------|
| `internal/payment/` | 100.0% | 60%+ | PASS |
| `internal/pnl/` | 100.0% | 60%+ | PASS |
| `internal/valuation/` | 100.0% | 60%+ | PASS |

---

## Regression Check

All Phase 1-3 services continue to compile and pass tests:

| Service | Phase | Tests | Status |
|---------|-------|-------|--------|
| matching-engine | 1-2 | 53 | PASS |
| clearing-engine | 1-2 | 116 | PASS |
| margin-engine | 1-2 | 71 | PASS |
| settlement-engine | 1-2 | 68 | PASS |
| compliance-service | 3 | 39 | PASS |
| gateway | 3 | 30 | PASS |
| market-data-service | 3 | 36 | PASS |
| warehouse-service | 3 | 55 | PASS |

## Code Quality Checks

- **go vet:** Clean on all 9 services — no import cycles, no dependency conflicts
- **Import cycles:** None detected
- **Dependency conflicts:** None — each service is an independent Go module

---

## Per-Package Coverage Detail (all services)

### auth-service (Phase Debt — T040)
| Package | Coverage |
|---------|----------|
| internal/auth | 88.5% |
| internal/handler | 34.1% |
| internal/config | 0.0% (no tests) |
| internal/store | 0.0% (no tests) |
| internal/types | 0.0% (no tests) |
| internal/server | 0.0% (no tests) |

### clearing-engine (Phase 1-2 + T043)
| Package | Coverage |
|---------|----------|
| internal/novation | 100.0% |
| internal/netting | 100.0% |
| internal/engine | 95.0% |
| internal/position | 98.5% |
| internal/store | 100.0% |
| internal/types | 100.0% |
| internal/server | 0.0% (no tests) |

### margin-engine (Phase 1-2 + T044)
| Package | Coverage |
|---------|----------|
| internal/scanner | 95.2% |
| internal/engine | 96.7% |
| internal/margincall | 44.1% |
| internal/margin | 41.4% |
| internal/params | 40.5% |
| internal/types | 0.0% (no tests) |
| internal/server | 0.0% (no tests) |

### settlement-engine (Phase 1-2 + T045)
| Package | Coverage |
|---------|----------|
| internal/payment | 100.0% |
| internal/pnl | 100.0% |
| internal/valuation | 100.0% |
| internal/engine | 41.6% |
| internal/settlement | 29.9% |
| internal/types | 0.0% (no tests) |
| internal/server | 0.0% (no tests) |

### matching-engine (Phase 1-2)
| Package | Coverage |
|---------|----------|
| internal/orderbook | 89.3% |
| internal/store | 88.9% |
| internal/types | 58.0% |
| internal/server | 53.5% |
| internal/engine | 47.1% |

### compliance-service (Phase 3)
| Package | Coverage |
|---------|----------|
| internal/types | 100.0% |
| internal/onboarding | 80.1% |
| internal/screening | 75.4% |
| internal/server | 22.4% |

### gateway (Phase 3)
| Package | Coverage |
|---------|----------|
| internal/router | 93.0% |
| internal/auth | 84.6% |
| internal/middleware | 62.7% |
| internal/handler | 54.0% |
| internal/config | 0.0% (no tests) |
| internal/proxy | 0.0% (no tests) |
| internal/types | 0.0% (no tests) |
| internal/websocket | 0.0% (no tests) |

### market-data-service (Phase 3)
| Package | Coverage |
|---------|----------|
| internal/candle | 68.7% |
| internal/ticker | 55.7% |
| internal/store | 48.5% |
| internal/server | 40.8% |
| internal/streaming | 39.0% |
| internal/ddl | 0.0% (no tests) |
| internal/retention | 0.0% (no tests) |
| internal/types | 0.0% (no tests) |

### warehouse-service (Phase 3)
| Package | Coverage |
|---------|----------|
| internal/types | 86.3% |
| internal/store | 83.8% |
| internal/service | 70.7% |
| internal/server | 47.1% |
