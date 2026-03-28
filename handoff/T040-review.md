APPROVED

# Review — T040: Rebuild Auth Service

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS
The implementation covers all required features: JWT access/refresh tokens with HMAC-SHA256, OAuth2 PKCE (S256 only), RBAC with 5 roles and 12 permissions, user registration with bcrypt, login with account lockout (configurable attempts/duration), API key management with SHA-256 hashed storage, session management with refresh token rotation and theft detection. The refresh token hash is stored on the session record, addressing the prior T005 review gap. PKCE code reuse is correctly prevented. Account lockout resets on successful login. All business logic paths are correctly implemented.

### Security: PASS
- Passwords hashed with bcrypt (cost 12 production, configurable).
- JWT uses HMAC-SHA256 with constant-time comparison (`hmac.Equal`).
- Refresh tokens stored as SHA-256 hashes, not plaintext.
- API keys stored as SHA-256 hashes with only prefix exposed.
- PKCE enforces S256 only, rejects `plain` at both service and handler layers.
- Token theft detection triggers revocation of all user sessions.
- Input validation at handler boundary (email regex, password length, role whitelist).
- No hardcoded secrets — signing key required via env var, service refuses to start without it.
- Dockerfile uses non-root user.
- DB SSL mode defaults to `require`.

### Code Quality: PASS
Follows the established zero-dependency Go module pattern (plus `golang.org/x/crypto` for bcrypt). Clean package structure: `types`, `auth`, `store`, `handler`, `server`, `config`. The `Store` interface is consumer-defined in the `auth` package, enabling clean dependency inversion. Consistent error handling patterns. Configuration via environment variables with sensible defaults.

### Test Coverage: PASS
43 tests across 4 test files covering: JWT generation/validation/expiry/tampering (6 tests), PKCE challenge generation/validation/boundary lengths (6 tests), RBAC permissions/roles (4 tests), service-level register/login/lockout/refresh/revoke/PKCE-flow/API-key lifecycle (14 tests), handler-level HTTP request/response validation (13 tests). Critical security paths (lockout, token theft, PKCE reuse, wrong-user API key revoke) are all tested.

## Required Fixes
None.

## Suggestions (non-blocking)

1. **Error leaking in handlers** — `handler.go:261` (`Exchange`) returns `"code exchange failed: "+err.Error()` and `handler.go:341` (`ValidateToken`) returns `"invalid token: "+err.Error()`. These expose internal error messages (e.g., `ErrCodeChallengeMismatch`, `ErrTokenExpired`) to API callers. Consider returning generic error messages and logging the details server-side.

2. **TokenPair missing JSON tags** — `types.TokenPair` fields (`AccessToken`, `RefreshToken`, `ExpiresIn`) have no `json:` struct tags, so they serialize as PascalCase. The rest of the API uses snake_case (e.g., `"access_token"`, `"refresh_token"`, `"expires_in"`). Add JSON tags for consistency with API conventions.

3. **Fragile error matching in handler** — `handler.go:120` uses `strings.Contains(err.Error(), "already exists")` to detect duplicate email errors. Use `errors.Is(err, store.ErrAlreadyExists)` for type-safe error matching.

4. **Unauthenticated user_id in request bodies** — The `/authorize`, `/apikey/create`, and `/apikey/revoke` endpoints accept `user_id` from the request body without verifying the caller's identity. When the gateway integration is wired up, these should extract the user ID from the authenticated JWT context rather than trusting the request body.

5. **bcrypt 72-byte password limit** — bcrypt silently truncates passwords beyond 72 bytes. Consider validating a max password length (e.g., 128 chars) at the handler layer and documenting the effective limit, or pre-hashing with SHA-256 before bcrypt.

6. **Handler coverage** — At 31.9%, the handler layer has the lowest coverage. The refresh, PKCE authorize/exchange, session revoke, and API key validate/revoke handlers lack direct HTTP-level tests. Consider adding tests for these endpoints in a follow-up.
