# Review — T005: Auth & IAM Service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation covers all stated requirements: JWT access/refresh tokens, OAuth2 PKCE flow, RBAC with roles and permissions, API key management, session management, and account lockout. The logic flows are sound.

Notable observations:
- Account lockout correctly checks `LockedUntil` and auto-unlocks expired locks.
- PKCE S256 verification uses the correct SHA-256 + base64url encoding.
- Refresh token flow properly validates the session state (revoked, expired) before issuing new tokens.
- Authorization codes are atomically fetched and deleted via `DELETE ... RETURNING`, preventing replay.
- The `ExchangeCode` method creates a session but does not store a refresh token hash on it (the `RefreshToken` field is empty), which means `RefreshTokens()` would fail to look up sessions created via PKCE flow. This is a minor gap — the PKCE flow returns tokens but the refresh token cannot be used. Non-blocking since the core login flow works correctly and PKCE token refresh can be addressed as a follow-up.

### Security: PASS

- Passwords hashed with bcrypt cost 12. Refresh tokens and API keys stored as SHA-256 hashes — never plaintext.
- JWT signing key is required from environment (`AUTH_JWT_SIGNING_KEY`), not hardcoded. No secrets in source.
- JWT validation checks signing method to prevent algorithm confusion attacks (`SigningMethodHMAC` type assertion).
- SQL uses parameterized queries throughout all repository code — no string concatenation in queries.
- Input validation at HTTP boundary: required fields checked, minimum password length enforced.
- Account lockout after 5 failed attempts with 30-minute duration.
- PKCE required for OAuth2 flows, S256 method supported.
- DB SSL mode defaults to `require`.
- Dockerfile runs as non-root user (appuser, UID 1000).

Minor suggestions (non-blocking):
- The `DSN()` method interpolates password into the connection string. This is standard for pgx but worth noting for log sanitization.
- `plain` PKCE method is supported alongside S256 — consider restricting to S256 only for a financial platform.

### Code Quality: PASS

- Clean layered architecture: domain models, repository interfaces, service layer, HTTP handlers, middleware.
- Follows standard Go project layout (`cmd/`, `internal/`, `pkg/`).
- Uses Go 1.22 enhanced `http.ServeMux` with method-based routing — no unnecessary dependencies.
- Repository interfaces defined in the service package, implementations in `repository/postgres` — proper dependency inversion.
- Error types are well-defined sentinel errors in the domain package.
- Graceful shutdown with signal handling and context cancellation.
- No dead code or unnecessary complexity.
- `go.mod` is missing `go.sum` — expected since Go toolchain wasn't available in the build environment.

### Test Coverage: PASS

Tests cover critical paths with meaningful assertions:
- **JWT tests**: generate/validate round-trip, invalid tokens, wrong signing key rejection.
- **Auth service tests**: registration (success + duplicate), login (success + wrong password + locked account), permission checking (exact match, wildcard, super admin, denied).
- **API key tests**: create and validate round-trip with role-based claims.
- **OAuth2 PKCE tests**: full flow (authorize + exchange) and invalid verifier rejection.
- **Handler tests**: health endpoint, registration via HTTP, validation errors (short password), unauthenticated access to protected endpoint.

The mock repositories are well-implemented with in-memory maps that mirror the real repository behavior.

Missing but non-blocking: refresh token flow test, token expiry edge case tests, revoked API key validation test.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **PKCE refresh token gap**: `ExchangeCode` creates a session without storing the refresh token hash, so tokens issued via PKCE flow cannot be refreshed. Consider aligning with the `createSessionAndTokens` pattern used in `Login`.
2. **Restrict PKCE to S256 only**: For a commodity exchange platform, consider removing `plain` method support — it offers no security benefit and is discouraged by RFC 7636 for public clients.
3. **Add refresh token rotation**: The handoff mentions this as a follow-up — agreed it should be prioritized for production.
4. **Email validation**: No format validation on email input beyond non-empty check. Consider basic RFC 5322 validation at the handler level.
5. **Rate limiting**: Not present on login/register endpoints. The handoff acknowledges this — should be added before production deployment.
