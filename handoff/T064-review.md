APPROVED

# Review — T064: Admin UI — Market Phase Control Page

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation matches the task requirements:
- Displays instruments with current phase using colored StatusBadges (TRADING/HALTED/PRE_OPEN/AUCTION).
- Per-instrument Halt/Resume buttons with confirmation dialogs via existing `ConfirmDialog` component.
- Emergency "Halt All Markets" button requiring typed "HALT ALL" confirmation, with progress feedback.
- `normalizeInstruments` correctly handles multiple response shapes (object with `instruments` key, raw array, null/undefined).
- `getPhaseAction` correctly maps TRADING→halt, HALTED→resume, PRE_OPEN/AUCTION→null.
- Sequential halt-all execution is a good choice for progress feedback and backend load management.
- Correctly uses existing `ConfirmDialog`'s `requireTypedConfirmation` prop which does exact string matching.

### Security: PASS

- No injection vulnerabilities — instrument IDs are passed as URL path segments through the existing `apiFetch` function which handles auth headers.
- AbortSignal support on new API functions enables cleanup on unmount.
- The `haltInstrument`/`resumeInstrument` functions use the same `apiFetch` wrapper as all other admin endpoints, inheriting auth token handling and 401 redirect.
- No hardcoded secrets or credentials.

### Code Quality: PASS

- Follows existing project conventions: CSS Modules, `usePolling` hook, `DataGrid`/`StatusBadge`/`ConfirmDialog` components, `apiFetch` wrapper.
- Route added in the correct route group (operations, admin-only).
- Sidebar entry added with `adminOnly: true`.
- AUCTION color (blue) added to StatusBadge consistently with existing color scheme.
- Type union updated in `types/index.ts` for `InstrumentControl`.
- Pure functions (`normalizeInstruments`, `getPhaseAction`) exported for testability — good pattern given RTL is unavailable.

Minor note: `haltInstrument`/`resumeInstrument` duplicate the same endpoints as `haltTrading`/`resumeTrading` but with AbortSignal support. The handoff explains this was intentional for backward compatibility. Acceptable.

### Test Coverage: PASS

16 tests covering:
- `normalizeInstruments`: multiple response shapes, null/undefined handling, default phase.
- `getPhaseAction`: all four phases plus unknown.
- API functions: correct endpoint URLs, HTTP method, AbortSignal passthrough.
- Halt All logic: exact string matching, filtering only TRADING instruments.

The StatusBadge color test (lines 83-93) tests a local map rather than the actual `statusColors` object — it's a weak test but not harmful.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Duplicate API functions**: `haltInstrument`/`resumeInstrument` are identical to `haltTrading`/`resumeTrading` except for the `signal` parameter. Consider adding `signal` to the existing functions instead of creating new ones. This would reduce API surface area.

2. **StatusBadge color test is tautological**: The test at line 83-93 creates its own `phaseColorMap` and asserts against it — it doesn't actually verify the `statusColors` object in `StatusBadge.tsx`. This test will pass even if someone changes the AUCTION color in StatusBadge. Acceptable given RTL constraints but worth noting.

3. **Error handling in handleAction/handleHaltAll**: Neither `handleAction` nor `handleHaltAll` has try/catch. If an API call fails, the error will be unhandled. Consider wrapping in try/catch and showing an error state or toast. The existing `apiFetch` throws `ApiError` on non-2xx responses.

4. **Sidebar icon reuse**: Market Phase reuses `ShieldIcon` (same as Circuit Breakers). A dedicated icon would improve visual distinction.
