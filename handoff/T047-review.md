APPROVED

# Review — T047: Trading Web UI Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec thoroughly covers all expected areas for a trading web UI: authentication flow, real-time data via WebSocket, order entry for all four order types (limit, market, IOC, FOK), candlestick charting, position/P&L/margin display, and instrument selection for the 14 commodities from T004 seed data.

Key correctness observations:
- WebSocket message formats align with the gateway's `websocket.Message` envelope pattern.
- Decimal string convention for prices/quantities is consistent with the protobuf convention established in T007.
- Sequence gap detection with REST snapshot fallback is the correct approach for maintaining order book consistency.
- The 14 commodities listed match the seed data from T004.
- REST endpoint paths align with the gateway routing conventions from T033.
- Order types (limit, market, IOC, FOK) match the matching engine's supported types from T008.

Minor note: The `SubmitOrderRequest` has both `order_type` (with values `ioc`/`fok`) and `time_in_force` (also with `ioc`/`fok`). This overlap could cause confusion — the implementation task should clarify whether `order_type: 'ioc'` implies `time_in_force: 'ioc'` or if both must be set. This is non-blocking for a spec.

### Security: PASS

The spec makes sound security decisions:
- JWT access tokens stored in a closure variable (never localStorage/sessionStorage) — mitigates XSS token theft.
- Refresh tokens in httpOnly, Secure, SameSite=Strict cookies — correct browser security posture.
- Silent refresh at 80% token lifetime prevents expiry during active sessions.
- WebSocket auth via query parameter is acknowledged as a browser limitation; the gateway validates during upgrade handshake.
- PKCE mentioned in design principles for login flow.
- `credentials: 'include'` on fetch for cookie transmission.

One consideration for the implementation: JWT tokens in WebSocket query parameters will appear in server access logs. The spec should note that gateway access logs should redact the `token` query parameter. Non-blocking for a spec task.

### Code Quality: PASS

This is a spec/documentation task — evaluated on spec quality rather than code:
- Well-structured with 20 clearly delineated sections.
- Component tree is detailed and maps cleanly to the project structure.
- TypeScript interfaces are precise and complete for all state domains.
- ASCII diagrams for layouts and data flow are clear and useful.
- REST endpoint table with poll intervals provides actionable implementation guidance.
- Responsive breakpoints with layout diagrams for all three tiers (desktop/tablet/mobile).
- Performance targets are specific and measurable.
- Accessibility section covers ARIA labels, keyboard navigation, and color contrast — often omitted in specs.
- Handoff file is thorough with 10 documented decisions and 6 specific follow-up items referencing downstream tasks.

### Test Coverage: PASS (N/A)

This is a specification document — no code to test. The spec does specify the testing stack (Vitest + React Testing Library) and performance measurement approaches for the implementation task.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Clarify order_type vs time_in_force overlap**: The `SubmitOrderRequest` has `order_type: 'ioc' | 'fok'` and separately `time_in_force: 'ioc' | 'fok'`. Consider documenting whether these are redundant or serve different purposes, so T048 implementers don't have to guess.

2. **WebSocket token in logs**: Note that the `?token=` query parameter on WebSocket URLs will appear in server/proxy access logs. Recommend the gateway redact this parameter in logging.

3. **Offline/degraded mode**: The spec covers WebSocket reconnection well but doesn't address what happens if REST polling also fails (e.g., positions/margin showing stale data). A simple "last updated N seconds ago" indicator would help traders know when data is stale.

4. **Order confirmation dialog**: For a financial trading UI, consider specifying whether large orders or market orders should show a confirmation dialog before submission. This is a common safeguard against fat-finger errors.

5. **Dark mode**: Trading UIs are conventionally dark-themed. The CSS custom properties approach supports theming, but the spec doesn't mention a default theme. Worth noting for T048.
