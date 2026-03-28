APPROVED

# Review — T049: Admin Dashboard Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec is comprehensive and well-aligned with the existing ACE platform architecture:

- All 9 services are correctly referenced with their established port conventions (matching-engine:8081 through warehouse-service:8088, gateway:8080).
- The two-role model (admin, compliance_officer) maps correctly to the auth-service JWT claims documented in prior specs.
- API endpoint paths follow the gateway's `/api/v1/` prefix convention established in T033.
- Component hierarchy is logically structured with clear parent-child relationships.
- Polling intervals are sensibly tiered by urgency (10s for margin/circuit breakers, 60s for warehouse).
- The handoff file correctly identifies the role naming mismatch (`exchange_admin` vs `admin`) as a follow-up item rather than ignoring it.

Minor note: The spec references `GET /api/v1/instruments` routed to matching-engine, but the matching-engine's current API surface may not expose this. This is acceptable for a spec — the implementation task will reconcile.

### Security: PASS

- Access tokens stored in-memory only (not localStorage) — correct practice for SPAs.
- Refresh tokens in httpOnly cookies — prevents XSS token theft.
- Route guards enforce role checks at both the route level (RoleGuard component) and the API level (gateway RBAC).
- No hidden DOM elements for unauthorized features — role-scoped rendering prevents client-side authorization bypasses.
- Destructive actions (halt trading, reject KYC) require confirmation dialogs; halt requires typed confirmation of the instrument ticker.
- No hardcoded secrets or credentials in the spec.
- The rejection reason field is mandatory for KYC rejections — good audit trail practice.

### Code Quality: PASS

- Document is well-organized with a clear table of contents and consistent section structure.
- TypeScript type definitions in the appendix use appropriate types (decimal strings for financial values, ISO 8601 strings for timestamps, union types for status enums).
- The data-fetching hook pattern (useReducer + useEffect + AbortController) is idiomatic React 18.
- Polling pause conditions (tab hidden, modal open, offline) show operational awareness.
- Runtime config injection pattern is a proven approach for containerized SPAs.
- Follows the project's spec-first convention established in earlier phases.

### Test Coverage: PASS (N/A)

This is a specification document — no implementation code or tests are expected. The spec does specify Vitest + React Testing Library as the testing stack for the implementation phase, which is appropriate.

---

## Required Fixes

None.

## Suggestions (non-blocking)

1. **CSRF protection**: The spec describes httpOnly cookie-based refresh tokens but doesn't mention CSRF mitigation. The implementation task should add a CSRF token or use the `SameSite=Strict` cookie attribute.

2. **Rate limiting on admin actions**: Destructive endpoints (halt, reject KYC, trigger margin calc) should have client-side debouncing and the spec could note expected server-side rate limits. Not critical for the spec but worth flagging for implementation.

3. **Accessibility specifics**: The spec mentions WCAG 2.1 AA but doesn't detail keyboard navigation for the data tables or ARIA roles for the status badges/charts. The implementation task should address this.

4. **Error retry backoff**: Section 11 mentions exponential backoff for 429 responses but doesn't specify the backoff parameters (initial delay, max delay, jitter). Consider adding defaults (e.g., 1s initial, 30s max, ±20% jitter).
