APPROVED

# Review — T058: Demo Smoke Test Script

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The script correctly implements a full demo smoke test flow covering registration, authentication, KYC, trading, and post-trade verification. All endpoints used (`/api/v1/auth/register`, `/api/v1/auth/login`, `/api/v1/participants`, `/api/v1/orders`, `/api/v1/clearing/positions`, `/api/v1/margin`, `/api/v1/settlement/cycles`, `/api/v1/instruments/*/book`, etc.) match the actual gateway routes in `src/gateway/internal/handler/routes.go`. The graceful-skip pattern for 502/503 and unreachable services is correctly implemented. The script correctly handles both `access_token` and `AccessToken` JSON field naming, matching the known casing inconsistency. Exit codes are correct: 0 on all-pass, 1 on any failure.

### Security: PASS

The script is a local smoke test tool, not production code exposed to untrusted input. Two `eval` calls exist (`login_user` line ~253, `submit_kyc` line ~303) that pass server-returned values. In the context of a demo script running against a known gateway, this is acceptable — the script operator controls the target server. No secrets are hardcoded; credentials are generated per-run with `DemoPass123!` which is appropriate for a demo script. `set -euo pipefail` is properly set.

**Note:** The `eval` usage would be a concern if this script were used against untrusted servers, but for its stated purpose (demo/smoke test against your own gateway) it's fine.

### Code Quality: PASS

Well-structured with clear section separators, consistent helper functions (`do_curl`, `record`, `json_field`, `get_check`), and appropriate use of color output (disabled when not a TTY). The `jq`-optional pattern with grep/sed fallback is a good choice for zero-dependency operation. Port allocation for service health checks (8081-8088) matches the documented convention. Code is readable and follows bash best practices (`set -euo pipefail`, quoted variables, local declarations).

### Test Coverage: PASS

The test suite (`tests/demo/demo_test.sh`) covers 28 tests across 5 test groups:
1. CLI flags (`--help`, `-h`)
2. Gateway unreachable (exit code, output messages)
3. Full healthy flow (all 6 steps, registration, login, trading output)
4. Unhealthy gateway (503 handling)
5. File properties (executable bit, shebang)

The Python mock server approach is well-designed — it simulates realistic responses for all endpoints. Tests verify both exit codes and output content. The mock server covers the happy path thoroughly.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Replace `eval` with `declare -g`** — `eval "${token_var}='${tok}'"` could be replaced with `printf -v "$token_var" '%s' "$tok"` (bash 3.1+) to eliminate eval entirely. Same for `result_var` in `submit_kyc`. Not a security risk in current usage, but a cleaner pattern.

2. **`json_field_num` is defined but never used** — the numeric JSON extractor (lines ~91-98) has no callers. Could be removed or used for verifying trade quantities/prices in future steps.

3. **Test for `--no-color` or piped output** — the color-disable logic (`[ -t 1 ]`) works correctly but isn't tested. A test like `output=$("$DEMO_SCRIPT" --help | cat)` piped through cat would verify no ANSI codes appear.

4. **Mock server port for service health checks** — the healthy mock only listens on one port. The service health checks (8081-8088) will always SKIP in tests. Consider binding the mock to those ports too, or testing that SKIPs are counted correctly.
