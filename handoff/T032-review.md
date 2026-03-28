# Review — T032: Compliance Service (KYC/AML)

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The implementation covers the full KYC onboarding workflow (APPLICATION_SUBMITTED → DOCUMENTS_PENDING → DOCUMENTS_UPLOADED → VERIFICATION_IN_PROGRESS → SCREENING_IN_PROGRESS → RISK_SCORING → APPROVED/MANUAL_REVIEW/REJECTED), document management, watchlist screening with pluggable providers, risk scoring with 6 weighted factors and 4 tiers, monitoring alerts, and SAR filing. The state machine transitions are well-defined in `ValidateStatusTransition` and tested for both valid and invalid paths.

Key logic verified:
- Risk score weights sum to 100 (15+20+25+15+15+10=100) — correct.
- `ScoreToTier` boundaries are clean: 0-25=Low, 26-50=Medium, 51-75=High, 76+=Prohibited.
- `ScreeningResultRisk` caps at 100 — correct.
- 24-hour screening cache with force-rescreen bypass works as tested.
- Auto-approve (Low/Medium), escalate (High), auto-reject (Prohibited) logic in `ProcessApplicationScreening` matches the T015 spec's tiered approach.
- `CheckParticipantStatus` handles expiry check correctly.

Minor note: `SubmitApplication` immediately transitions from APPLICATION_SUBMITTED to DOCUMENTS_PENDING without using `ValidateStatusTransition`. This is intentional (submit always moves to pending) but slightly inconsistent with `StartVerification` which does use the validator. Not a bug — the transition is hardcoded and correct.

### Security: PASS

- No SQL — uses in-memory stores with interface abstraction for future PostgreSQL. No injection vectors.
- No user-controlled data flows into shell commands or templates.
- File upload has a 20MB size limit enforced server-side.
- HTTP endpoints use query parameters with proper validation (empty checks, 400 responses).
- No hardcoded secrets or credentials.
- Country risk lists and screening logic are server-side only.
- `atomic.AddUint64` for ID generation is safe for concurrent use.

Note: The HTTP endpoints (`/application`, `/participant-status`) have no authentication. The handoff correctly identifies this as a follow-up for T005 (auth service JWT integration). Acceptable for phase 3 with in-memory stores.

### Code Quality: PASS

- Follows existing project conventions: zero-dependency Go module, `internal/` package layout, `cmd/` entrypoint, same server pattern as matching-engine et al.
- Port allocation (50056/8086) follows the established convention documented in CLAUDE.md.
- Clean separation: `onboarding` package for application lifecycle, `screening` package for watchlist/risk/alerts/SAR, `types` for domain types, `server` for HTTP/gRPC.
- Store interfaces allow swapping implementations — consistent with other services.
- `sync.RWMutex` used appropriately in both in-memory stores.
- Two separate `idCounter` variables exist (one in `onboarding/service.go`, one in `screening/service.go`) with identical `newID` functions. Minor duplication but avoids cross-package coupling — consistent with the project's zero-dep philosophy.

### Test Coverage: PASS

43 tests covering:
- **Onboarding**: submit, validation (4 cases), get, upload, size limit, document submission with required-doc checks, verification pipeline, approve/reject with officer validation, suspend/reinstate, participant status (not found/approved/suspended), list with filters.
- **Screening**: clear path, match path with static provider, 24h cache + force rescreen, batch screen, match resolution (including double-resolve guard), risk score calculation, get risk score, full application screening (clear path → auto-approve, match path → manual review).
- **Alerts**: create, resolve, double-resolve guard, invalid resolution status, list with filters.
- **SAR**: file, validation (3 missing-field cases).
- **Risk scoring**: weighted computation (3 scenarios), tier boundaries (8 boundary values), participant type risk ordering, country risk (5 countries), screening result risk.
- **Types/validation**: participant type validation, required documents per type, status transition matrix (14 valid, 5 invalid).
- **Config**: defaults, env var override.

Edge cases covered: oversized file upload, missing required documents, invalid status transitions, double-resolution of matches/alerts, expired KYC check, unknown participant lookup.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **`onboarding.Service.GetStore()` exposes internals** — The screening service receives `onboarding.Store` directly in its constructor, so `GetStore()` on the service appears unused. Consider removing it to keep the API surface minimal.

2. **Global `idCounter` variables** — The two `idCounter` variables in separate packages will produce overlapping ID sequences (both start at 1). In production with a real database this is irrelevant, but for integration testing with both services running together, IDs like `app-1` and `scr-1` could be confused. Consider using different starting offsets or a shared counter if cross-service testing is planned.

3. **`InMemoryStore.SaveApplication` overwrites `UpdatedAt`** — The store's `SaveApplication` sets `UpdatedAt = time.Now().UTC()` unconditionally, which means the service-level `UpdatedAt` assignments are immediately overwritten. This is harmless (both set to "now") but could mask bugs if the service ever sets a specific timestamp. Consider having the store respect the caller's `UpdatedAt` value.

4. **`DocumentQualityScore` inverted logic** — In `CalculateRiskScore`, the document quality score is `100 - (verified*100)/len(required)`. This means "all docs verified" = score 0 (low risk) and "no docs verified" = score 100 (high risk). The inversion is correct for risk scoring, but the field name `DocumentQualityScore` suggests higher = better quality. Consider renaming to `DocumentQualityRiskScore` or adding a comment.
