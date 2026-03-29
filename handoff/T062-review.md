APPROVED

# Review — T062: Admin UI — WebSocket Real-Time Hook

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

All four deliverables from the task description are implemented correctly:

1. **useWebSocket hook** — Implements the specified API (`path`, `options.enabled`, returns `data/status/reconnectCount`). Exponential backoff is correct: `1000 * 2^attempt` capped at 30s. Backoff resets on successful open (`attemptRef.current = 0` in `onopen`). Cleanup on unmount and path/enabled changes is handled via the `cleanup` callback and `unmountedRef` guard.

2. **OrderBook integration** — WS data takes priority when connected; polling is disabled via `!wsConnected` passed to `usePolling`'s `enabled` parameter (which already supports this). The `WsStatusDot` component shows green/yellow/red as specified.

3. **TopBar badge** — Standalone WS health check connection (`/health` path) with styled badge showing "WS: Connected" / "WS: Disconnected". Uses existing `statusPill` via CSS `composes`, consistent with existing TopBar patterns.

4. **URL construction** — Correctly strips leading slashes from path and trailing slashes from base URL to avoid double-slash issues.

One minor note: the `reconnectCount` increments on every close (including the first), so it counts disconnections rather than reconnection attempts. This is consistent with the test expectations and reasonable behavior.

### Security: PASS

- No hardcoded secrets or credentials.
- WebSocket URL is built from `getConfig()` which reads from a runtime config object — no user-controlled input flows unsanitized into the URL (the `path` parameter is developer-supplied, not user input).
- JSON parsing is wrapped in try/catch — malformed messages are silently ignored, preventing parse errors from crashing the UI.
- The `unmountedRef` guard prevents state updates after unmount, avoiding React warnings and potential memory leaks.

### Code Quality: PASS

- Follows existing project patterns: hooks in `src/hooks/`, tests in `src/__tests__/`, CSS Modules for styling.
- The `usePolling` integration is clean — leverages the existing `enabled` parameter rather than modifying the polling hook.
- CSS uses `composes: statusPill` to reuse existing styles rather than duplicating.
- The `WsStatusDot` inline component in OrderBook.tsx is appropriately scoped — it's only used in that file and doesn't warrant its own module.
- Good use of refs (`wsRef`, `reconnectTimerRef`, `attemptRef`, `unmountedRef`) to avoid stale closure issues in WebSocket callbacks.

### Test Coverage: PASS

17 tests covering all critical paths:
- Connection lifecycle: connect, open, message, close, error
- Reconnection: exponential backoff timing (1s, 2s, 4s), backoff cap at 30s, backoff reset on successful connection
- Cleanup: WebSocket close on unmount, timer cancellation on unmount
- Options: `enabled: false` prevents connection, `enabled` toggle triggers connection
- Path changes: old WS closed, new WS opened with correct URL
- Edge cases: non-JSON messages ignored, multiple messages update data, leading slash normalization

The mock WebSocket approach is well-structured and tests real timing behavior via `vi.useFakeTimers()`.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **TopBar WS badge shows "WS: Disconnected" during `connecting` state** — The ternary at TopBar.tsx:67 only checks `connected`, so `connecting` shows "WS: Disconnected" text despite having the yellow `wsConnecting` class. Consider showing "WS: Connecting" as a third label for better UX.

2. **OrderBook WsStatusDot shows red for `disconnected`** — The dot shows red (`--accent-red`) for both `error` and `disconnected` states. Since `disconnected` is the normal state before first connection and during reconnect, yellow or gray might be more appropriate for `disconnected` to distinguish it from `error`.

3. **No test for the `error` → reconnect path** — The tests verify that `onerror` sets status to `error`, but don't verify whether reconnection is attempted after an error. Currently `onerror` sets status but doesn't call `scheduleReconnect()` — only `onclose` does. If the WebSocket fires `onerror` without a subsequent `onclose`, no reconnection occurs. This is technically correct per the WebSocket spec (browsers always fire `onclose` after `onerror`), but worth noting.
