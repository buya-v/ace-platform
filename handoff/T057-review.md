APPROVED

# Review — T057: Demo Runbook Document

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The runbook is well-structured and covers all 9 services plus both frontend SPAs. Key correctness observations:

- API paths use `/api/v1/` prefix consistently, matching the gateway router registration.
- The `AccessToken` vs `access_token` casing issue is correctly documented with a jq workaround (`jq -r '.AccessToken // .access_token'`).
- Known gaps are honestly flagged: missing `POST /settlement/cycle` gateway route, no gateway routes for warehouse-service and market-data-service, V8 migration conflict.
- Integer enum values for order fields (side: 1=BUY, order_type: 1=LIMIT, time_in_force: 2=GTC) match Go type definitions.
- Port allocation table matches the established convention (50051-50058 for gRPC, 8081-8088 for health).
- The production readiness checklist (Section 9) accurately reflects platform state: 857 tests, 66.5% coverage, known gaps in monitoring/benchmarks/DR.

One minor note: Some admin endpoints (halt, resume, bust, circuit-breaker, mass-cancel, disable) may not all be implemented in the gateway — the runbook presents them as available. This is acceptable for a demo runbook that also serves as a target API spec, and the troubleshooting notes cover 502 responses.

### Security: PASS

- No secrets or credentials hardcoded — example passwords are clearly demo values.
- All authenticated endpoints include `Authorization: Bearer` headers.
- The security checklist (Section 9.3) correctly flags HMAC JWT as needing upgrade to RSA/ECDSA for production.
- PKCE S256-only enforcement is documented.
- No command injection risk — all curl commands use proper quoting and variable expansion.

### Code Quality: PASS

- Clear table of contents with anchor links.
- Each section follows a consistent pattern: command → expected response → "what this proves" → troubleshooting.
- Troubleshooting tables are practical and cover common failure modes (502, 401, 409, port conflicts).
- The API route reference table at the end is comprehensive (50+ endpoints organized by domain).
- Known issues and gaps are documented inline rather than hidden — good practice for a runbook.
- Markdown formatting is clean and consistent.

### Test Coverage: PASS (N/A)

This is a documentation task — no code or tests to evaluate. The handoff file correctly lists deliverables.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Shell script version**: Consider adding a `scripts/demo.sh` that automates Sections 2-4 (register, login, place orders, check positions) as a single executable smoke test. This would complement the manual runbook.
2. **Response validation**: Some "expected response" blocks show fields that may vary by implementation (e.g., `exec_type: "FILL"` format — could be string or enum int). Adding a note about checking the actual response format on first run would help demo operators.
3. **Token expiry note**: Section 2.4 mentions `ExpiresIn: 3600` but doesn't suggest re-login timing for long demos. A note like "re-login every ~50 minutes to avoid mid-demo 401s" would be practical.
