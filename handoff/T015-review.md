APPROVED

# Review — T015: KYC/AML Architecture Spec

**Verdict:** APPROVED
**Reviewer:** AI Reviewer Agent

---

## Evaluation

### Correctness: PASS

The spec comprehensively covers the KYC/AML domain for an agriculture commodity exchange: 5 participant types, 11-state onboarding lifecycle, document verification pipeline, watchlist screening with fuzzy matching, quantitative risk scoring (6 weighted factors summing to 1.0), 6 transaction monitoring rules, and SAR filing workflow.

The protobuf contract defines 3 well-scoped services (OnboardingService, ScreeningService, ComplianceAdminService) with 20 RPCs. All enums use UNSPECIFIED=0 per proto3 convention. Message types align with the SQL schema and spec document.

The V7 SQL migration creates 7 tables in the `compliance` schema with proper foreign keys, CHECK constraints on status fields, and append-only rules (via PostgreSQL RULEs) on `screening_results`, `risk_scores`, and `sar_filings` — consistent with T004's immutability pattern. The `screening_matches` table correctly allows UPDATE for match resolution workflow while keeping its parent `screening_results` immutable.

Risk score is constrained to 0-100, match score to 0-1, and all status enums are CHECK-constrained in SQL and mirrored in the proto enums.

### Security: PASS

- **Append-only audit trail**: screening_results, risk_scores, and sar_filings use PostgreSQL RULEs to block UPDATE/DELETE — enforcing immutability at the DB level.
- **Least-privilege grants**: `ace_compliance_svc` gets INSERT+SELECT (no UPDATE) on immutable tables, full CRUD only where needed (kyc_applications, kyc_documents, monitoring_alerts). `ace_exchange_svc` gets read-only on kyc_applications and risk_scores only.
- **PII handling**: Spec calls for column-level AES-256 encryption for restricted PII (national ID, passport scans) and S3 SSE for document storage.
- **No hardcoded secrets** in any artifact.
- **Input validation**: CHECK constraints at the DB boundary for all enum-like fields, score ranges, and status values.
- **Access control matrix** (Section 12.2) defines 5 roles with appropriate permission boundaries.

### Code Quality: PASS

- Proto file follows proto3 best practices: versioned package (`ace.compliance.v1`), proper go_package option, timestamp imports, string representation for decimal match_score (avoiding float precision issues).
- SQL migration uses consistent naming conventions (`idx_` prefix for indexes, `no_update_`/`no_delete_` for rules), partial indexes where appropriate (e.g., `idx_kyc_app_officer WHERE assigned_officer_id IS NOT NULL`).
- Spec document is well-structured with 13 numbered sections, ASCII diagrams, and clear tables. References upstream tasks (T004, T005, T007/T008) correctly.
- Handoff file includes actionable follow-ups with specific artifact paths and integration points for downstream tasks.

### Test Coverage: PASS

30 tests across 3 classes validate:
- **Spec document**: existence, 13 required sections, participant types, onboarding states, risk tiers, monitoring rules, Kafka topics, REST endpoints.
- **Proto schema**: package/go_package, 3 services with all RPCs, 9 enums, 10+ key messages, timestamp import, string decimal for match_score.
- **SQL migration**: 7 tables in compliance schema, append-only rules on 3 tables, role grants, UUID primary keys, index coverage, CHECK constraints for status and score ranges.

Tests are structural validation which is appropriate for a spec/schema task — they verify artifact completeness and internal consistency rather than runtime behavior.

## Required Fixes

None.

## Suggestions (non-blocking)

1. **gRPC streaming for document upload**: `UploadDocumentRequest.content` accepts up to 20MB as `bytes` in a unary RPC. For production, consider a client-streaming RPC to handle large files without buffering the entire payload in memory. Fine for the spec phase.

2. **`screening_matches` UPDATE grant vs append-only pattern**: The match resolution workflow correctly requires UPDATE on `screening_matches`, but this breaks the pure append-only pattern used by its parent table. Consider whether resolution should instead insert a new "resolution" record linked to the original match, keeping full history. Current approach is pragmatic and acceptable.

3. **Migration version gap**: This is V7 but the diff doesn't show V5 or V6. Ensure the migration chain is contiguous when applied. The handoff correctly notes V7 must follow V6.

4. **Proto field `list_versions` as string**: `ScreeningResult.list_versions` is typed as `string` with a comment "JSON of list version metadata". Consider using `google.protobuf.Struct` or a dedicated `ListVersion` message for stronger typing. Acceptable for initial spec.
