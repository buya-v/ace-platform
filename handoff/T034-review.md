APPROVED

# Review — T034: API Gateway Service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation covers all 51 REST endpoints from the T033 spec plus 3 WebSocket endpoints and 2 health endpoints. Route registration in `routes.go` maps correctly to the handler methods, and each handler forwards to the appropriate backend service and gRPC method.

Key details verified:
- JWT validation correctly checks signature (HMAC-SHA256), expiration, issuer, and audience — and importantly checks the `alg` header to prevent algorithm confusion attacks.
- Token bucket rate limiter correctly implements per-user per-group limiting with burst support.
- Path parameter extraction via `matchPath` handles `{param}` placeholders correctly and edge cases (different segment counts, no params, multiple params).
- Middleware chain order (RequestID → Logging → BodyLimit → Auth → RateLimit → Router) is correct — rate limiting runs after auth so it can use `claims.Sub` as the key.
- Graceful shutdown properly shuts down both HTTP and health servers.
- `readyFlag` uses `atomic` correctly for concurrent access.

Minor note: The `readyz` endpoint is registered on both the main router (via `handler.RegisterRoutes`) and the health server mux in `main.go`. The main router version uses `handler.readyz` (lowercase, on the Handler struct), while the health server uses an inline function calling `handler.IsReady()`. Both work, but the duplication means the main-port `/readyz` and health-port `/readyz` could theoretically diverge. Not a bug — just worth noting.

### Security: PASS

- **JWT validation** is solid: HMAC signature verified with `hmac.Equal` (constant-time comparison), algorithm header checked (`HS256` only), expiration/issuer/audience validated.
- **No hardcoded secrets in code**: The default JWT secret `"ace-dev-secret-change-in-production"` is in the config fallback, which is standard for dev environments. Production would override via env var.
- **Body size limiting**: Both `Content-Length` check and `http.MaxBytesReader` are applied — defense in depth.
- **Rate limiting** prevents abuse with per-user and per-IP fallback for unauthenticated requests.
- **Bearer token extraction** properly validates the `Bearer ` prefix before stripping.
- **No SQL injection surface** — the gateway is a pure proxy with no database interaction.
- **Request ID**: Accepts client-provided `X-Request-ID` or generates a cryptographically random one. Accepting client IDs is standard for distributed tracing.

One area to watch (non-blocking): The rate limiter's `buckets` map grows unboundedly — every unique IP/user gets an entry that's never evicted. In production under sustained traffic from many IPs, this would be a slow memory leak. A TTL-based eviction or periodic cleanup would be needed before production deployment.

### Code Quality: PASS

- Follows the established zero-dependency Go module pattern used by other ACE services.
- Clean package structure: `auth`, `config`, `handler`, `middleware`, `proxy`, `router`, `types`, `websocket` — each with a clear responsibility.
- Interface-based `BackendClient` allows clean testing with mocks.
- Middleware uses standard `func(http.Handler) http.Handler` pattern.
- Config uses environment variables with typed defaults — clean and standard.
- Error responses use a consistent `ErrorResponse` envelope with code, message, and request ID.
- No dead code or unnecessary complexity. The `Chain` helper in `chain.go` is defined but not used in `main.go` (the chain is built manually) — very minor, not blocking.
- Router is simple and fit-for-purpose. Linear route scan is fine for ~50 routes.

### Test Coverage: PASS

31 tests across 4 test files:
- **jwt_test.go (7 tests)**: Valid token, expired, invalid signature, invalid issuer, invalid audience, malformed tokens (4 sub-cases), role checking.
- **auth_test.go (7 tests)**: Public path bypass, missing token, valid token with claims propagation, expired token, invalid format, role-based authorization (4 sub-cases), public prefix.
- **ratelimit_test.go (3 tests)**: Token bucket burst/exhaustion, middleware integration (200 then 429), group configuration defaults.
- **handler_test.go (11 tests)**: Route registration count, order forwarding with correct service/method, order book public access, service routing for all 7 backends, health endpoint, readyz not-ready state, claims forwarding to metadata, 404 handling, backend error → 502, API version header.
- **router_test.go (3 tests)**: Path matching (8 sub-cases), basic routing with method dispatch (6 sub-cases), path parameter extraction with complex instrument IDs.

Critical paths are well covered. The mock client pattern in handler tests properly verifies that requests are routed to the correct backend service and gRPC method.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **Rate limiter memory growth**: The `buckets` map in `RateLimiter` has no eviction. Add a periodic cleanup goroutine or use a time-bounded cache before production deployment.

2. **WebSocket handler has no read loop**: The `streamHeartbeats` function only writes heartbeats but never reads from the connection. Client disconnect detection relies on write errors, which may be delayed. A concurrent read goroutine that detects close frames would improve responsiveness.

3. **`Chain` function unused**: `middleware/chain.go` defines a `Chain` helper that isn't used in `main.go`. Either use it or remove it — currently it's dead code.

4. **Path params overwrite query params**: In `router.go:78`, path parameters are merged into `r.URL.Query()` via `q.Set(k, v)`. If a client sends a query parameter with the same name as a path parameter (e.g., `?order_id=evil`), the path param wins (correct behavior), but this implicit overwrite is worth a comment.

5. **WebSocket `streamHeartbeats` blocks forever on `select`**: The `for { select { case <-ticker.C: ... } }` loop has no exit condition other than write failure. Adding a `context.Done()` case from the request context or a shutdown channel would allow cleaner termination during graceful shutdown.
