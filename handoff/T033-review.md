# Review — T033: API Gateway Architecture Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec comprehensively covers all 6 backend services with 52 REST endpoints mapped to their corresponding gRPC RPCs or Go methods. Key observations:

- Port assignments (8080 HTTP, 8090 health) align with the established convention in CLAUDE.md learned patterns.
- Backend service ports (50051-50056) match the documented port allocation table.
- REST endpoint design follows standard conventions: plural nouns, appropriate HTTP verbs (POST for create, DELETE for cancel, PATCH for modify, GET for queries).
- The OpenAPI 3.0 spec is internally consistent with the architecture doc — all 52 endpoints from Section 10's mapping table have corresponding path definitions in the YAML.
- gRPC error code to HTTP status mapping is correct per the gRPC specification (e.g., UNAUTHENTICATED→401, PERMISSION_DENIED→403, NOT_FOUND→404).
- JWT validation flow correctly delegates token issuance to auth-service and limits the gateway to validation + claim extraction.
- The handoff correctly identifies that clearing/margin/settlement engines lack proto definitions and flags this as a follow-up concern for the implementation task.

### Security: PASS

- Auth endpoints (login, register, password reset, refresh) are correctly marked as `security: []` (no auth required) in the OpenAPI spec.
- Public market data endpoints are correctly unauthenticated — standard exchange practice.
- Owner-scoped access (Section 4.4) enforces participant_id matching from JWT claims, preventing horizontal privilege escalation.
- Rate limiting on auth endpoints (5 req/s) mitigates brute-force attacks.
- WebSocket auth via query parameter token is documented for the executions stream (authenticated), while market data streams are public.
- No secrets or credentials are hardcoded.
- Request body size limit (1MB) is specified to prevent abuse.
- The spec correctly delegates TLS termination to Istio ingress rather than implementing it in the gateway.

### Code Quality: PASS

- The spec is well-structured with 14 clear sections plus appendices.
- Consistent formatting throughout: tables for endpoint mappings, code blocks for examples, ASCII diagrams for architecture.
- OpenAPI YAML follows best practices: reusable components (schemas, parameters, responses), consistent naming, proper use of `$ref`.
- Schema definitions are thorough — all request/response types have explicit field types, formats (uuid, date-time, email), and constraints (minimum, maximum, enum values).
- The handoff file includes actionable follow-up items with specific task IDs, port numbers, and implementation guidance.
- Follows the established spec-first pattern that was validated in earlier phases (T007→T008).

### Test Coverage: PASS (N/A)

This is a specification task — no executable code was produced, so test coverage is not applicable. The OpenAPI spec itself serves as a testable contract for the implementation task (T034). The spec includes concrete request/response examples (Section 11) that can be used as test fixtures.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Market data service routing**: The spec routes market data endpoints to matching-engine:50051, but per the port convention table there's a suggested market-data-service:50057. If a separate market-data-service is planned, the spec should note this as a future routing change.

2. **WebSocket authentication**: The executions WebSocket (`/api/v1/ws/executions?token=<JWT>`) passes the JWT as a query parameter. This is a common pattern but means the token appears in server access logs and potentially browser history. Consider documenting that the token can alternatively be sent as the first WebSocket message after connection upgrade.

3. **CORS**: The spec doesn't mention CORS headers. If browser-based clients will call the API directly, CORS configuration should be documented (allowed origins, methods, headers).

4. **Request ID propagation**: Section 6.1 mentions `X-Request-ID` but doesn't specify whether the gateway generates one if the client doesn't provide it. Recommend the gateway always generates a request ID and returns it in the response for traceability.

5. **OpenAPI `margin/calls/stats` schema**: The response uses `additionalProperties: true` which is a catch-all. Once the margin-engine stats response is finalized, this should be replaced with explicit fields.

6. **Health endpoints**: `/healthz` and `/readyz` are listed in Section 4.3 (public endpoints) but don't have OpenAPI path definitions. Consider adding them for completeness.
