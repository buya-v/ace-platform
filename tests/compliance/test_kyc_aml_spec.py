"""
Tests for KYC/AML architecture spec artifacts (T015).

Validates:
- Protobuf schema structure and completeness
- SQL migration syntax and table coverage
- Architecture spec document completeness
"""

import os
import re

import pytest

REPO_ROOT = os.path.join(os.path.dirname(__file__), "..", "..")
SPEC_PATH = os.path.join(REPO_ROOT, "docs", "specs", "T015_KYC_AML_Architecture.md")
PROTO_PATH = os.path.join(
    REPO_ROOT, "src", "compliance-service", "proto", "compliance.proto"
)
MIGRATION_PATH = os.path.join(
    REPO_ROOT, "infrastructure", "db", "migrations", "V007__kyc_aml_tables.sql"
)


@pytest.fixture
def spec_content():
    with open(SPEC_PATH) as f:
        return f.read()


@pytest.fixture
def proto_content():
    with open(PROTO_PATH) as f:
        return f.read()


@pytest.fixture
def migration_content():
    with open(MIGRATION_PATH) as f:
        return f.read()


# ---------------------------------------------------------------------------
# Architecture Spec Tests
# ---------------------------------------------------------------------------


class TestArchitectureSpec:
    """Validate the KYC/AML architecture spec document."""

    def test_spec_exists(self):
        assert os.path.isfile(SPEC_PATH), f"Spec not found at {SPEC_PATH}"

    def test_spec_has_required_sections(self, spec_content):
        required_sections = [
            "Overview",
            "System Context",
            "Participant Onboarding Workflow",
            "Document Verification",
            "Watchlist Screening",
            "Risk Scoring",
            "Ongoing Monitoring",
            "API Contracts",
            "Data Model",
            "Integration Points",
            "Regulatory Requirements",
            "Security & Privacy",
            "Failure Modes & Recovery",
        ]
        for section in required_sections:
            assert section.lower() in spec_content.lower(), (
                f"Missing required section: {section}"
            )

    def test_spec_defines_participant_types(self, spec_content):
        participant_types = [
            "INDIVIDUAL",
            "CORPORATE",
            "COOPERATIVE",
            "BROKER",
            "INSTITUTIONAL",
        ]
        for pt in participant_types:
            assert pt in spec_content, f"Missing participant type: {pt}"

    def test_spec_defines_onboarding_states(self, spec_content):
        states = [
            "APPLICATION_SUBMITTED",
            "DOCUMENTS_PENDING",
            "DOCUMENTS_UPLOADED",
            "VERIFICATION_IN_PROGRESS",
            "SCREENING_IN_PROGRESS",
            "RISK_SCORING",
            "MANUAL_REVIEW",
            "APPROVED",
            "REJECTED",
            "SUSPENDED",
            "EXPIRED",
        ]
        for state in states:
            assert state in spec_content, f"Missing onboarding state: {state}"

    def test_spec_defines_risk_tiers(self, spec_content):
        tiers = ["Low", "Medium", "High", "Prohibited"]
        for tier in tiers:
            assert tier in spec_content, f"Missing risk tier: {tier}"

    def test_spec_defines_monitoring_rules(self, spec_content):
        rules = ["TXN-001", "TXN-002", "TXN-003", "TXN-004", "TXN-005", "TXN-006"]
        for rule in rules:
            assert rule in spec_content, f"Missing monitoring rule: {rule}"

    def test_spec_references_kafka_topics(self, spec_content):
        topics = [
            "compliance.participant.status",
            "compliance.screening.completed",
            "compliance.alert.created",
        ]
        for topic in topics:
            assert topic in spec_content, f"Missing Kafka topic: {topic}"

    def test_spec_defines_rest_endpoints(self, spec_content):
        endpoints = [
            "/api/v1/participants",
            "/api/v1/screening",
            "/api/v1/risk-scores",
            "/api/v1/compliance/alerts",
            "/api/v1/compliance/sar",
        ]
        for endpoint in endpoints:
            assert endpoint in spec_content, f"Missing REST endpoint: {endpoint}"


# ---------------------------------------------------------------------------
# Protobuf Schema Tests
# ---------------------------------------------------------------------------


class TestProtobufSchema:
    """Validate the compliance protobuf contract."""

    def test_proto_exists(self):
        assert os.path.isfile(PROTO_PATH), f"Proto not found at {PROTO_PATH}"

    def test_proto_package(self, proto_content):
        assert "package ace.compliance.v1;" in proto_content

    def test_proto_go_package(self, proto_content):
        assert "compliance-service/proto/compliance/v1" in proto_content

    def test_proto_has_required_services(self, proto_content):
        services = ["OnboardingService", "ScreeningService", "ComplianceAdminService"]
        for svc in services:
            assert f"service {svc}" in proto_content, f"Missing gRPC service: {svc}"

    def test_onboarding_service_rpcs(self, proto_content):
        rpcs = [
            "SubmitApplication",
            "GetApplication",
            "ListApplications",
            "UploadDocument",
            "ListDocuments",
            "ApproveApplication",
            "RejectApplication",
        ]
        for rpc in rpcs:
            assert f"rpc {rpc}" in proto_content, (
                f"Missing OnboardingService RPC: {rpc}"
            )

    def test_screening_service_rpcs(self, proto_content):
        rpcs = [
            "ScreenParticipant",
            "GetScreeningResult",
            "BatchScreen",
            "ResolveMatch",
            "GetRiskScore",
            "RecalculateRiskScore",
        ]
        for rpc in rpcs:
            assert f"rpc {rpc}" in proto_content, (
                f"Missing ScreeningService RPC: {rpc}"
            )

    def test_admin_service_rpcs(self, proto_content):
        rpcs = [
            "CheckParticipantStatus",
            "ListAlerts",
            "ResolveAlert",
            "GetAuditTrail",
            "FileSAR",
            "SuspendParticipant",
            "ReinstateParticipant",
        ]
        for rpc in rpcs:
            assert f"rpc {rpc}" in proto_content, (
                f"Missing ComplianceAdminService RPC: {rpc}"
            )

    def test_proto_has_required_enums(self, proto_content):
        enums = [
            "ParticipantType",
            "KYCStatus",
            "DocumentType",
            "DocumentStatus",
            "MatchType",
            "ScreeningOutcome",
            "RiskTier",
            "AlertStatus",
            "ComplianceCheckResult",
        ]
        for enum in enums:
            assert f"enum {enum}" in proto_content, f"Missing enum: {enum}"

    def test_proto_has_required_messages(self, proto_content):
        messages = [
            "KYCApplication",
            "Document",
            "ScreeningResult",
            "ScreeningMatch",
            "RiskScore",
            "RiskFactorBreakdown",
            "MonitoringAlert",
            "AuditEvent",
            "SARFiling",
            "CheckParticipantStatusResponse",
        ]
        for msg in messages:
            assert f"message {msg}" in proto_content, f"Missing message: {msg}"

    def test_proto_uses_timestamp_import(self, proto_content):
        assert 'import "google/protobuf/timestamp.proto"' in proto_content

    def test_proto_uses_string_decimals(self, proto_content):
        """Match scores should use string representation to avoid float precision."""
        assert 'string match_score' in proto_content


# ---------------------------------------------------------------------------
# SQL Migration Tests
# ---------------------------------------------------------------------------


class TestMigration:
    """Validate the V7 KYC/AML migration."""

    def test_migration_exists(self):
        assert os.path.isfile(MIGRATION_PATH), (
            f"Migration not found at {MIGRATION_PATH}"
        )

    def test_migration_creates_required_tables(self, migration_content):
        tables = [
            "compliance.kyc_applications",
            "compliance.kyc_documents",
            "compliance.screening_results",
            "compliance.screening_matches",
            "compliance.risk_scores",
            "compliance.monitoring_alerts",
            "compliance.sar_filings",
        ]
        for table in tables:
            assert f"CREATE TABLE {table}" in migration_content, (
                f"Missing table: {table}"
            )

    def test_migration_uses_compliance_schema(self, migration_content):
        create_statements = re.findall(
            r"CREATE TABLE (\w+)\.", migration_content
        )
        for schema in create_statements:
            assert schema == "compliance", (
                f"Table in wrong schema: {schema} (expected compliance)"
            )

    def test_migration_has_append_only_rules(self, migration_content):
        """Screening results, risk scores, and SAR filings must be append-only."""
        append_only_tables = [
            "screening_results",
            "risk_scores",
            "sar_filings",
        ]
        for table in append_only_tables:
            assert f"no_update_{table}" in migration_content, (
                f"Missing no-update rule for {table}"
            )
            assert f"no_delete_{table}" in migration_content, (
                f"Missing no-delete rule for {table}"
            )

    def test_migration_grants_compliance_service_role(self, migration_content):
        assert "ace_compliance_svc" in migration_content

    def test_migration_grants_exchange_read_access(self, migration_content):
        assert "ace_exchange_svc" in migration_content

    def test_migration_has_uuid_primary_keys(self, migration_content):
        """All tables should use UUID primary keys."""
        tables = re.findall(
            r"CREATE TABLE compliance\.(\w+)", migration_content
        )
        for table in tables:
            pattern = f"UUID PRIMARY KEY"
            table_block_start = migration_content.index(f"compliance.{table}")
            next_create = migration_content.find("CREATE TABLE", table_block_start + 1)
            if next_create == -1:
                next_create = len(migration_content)
            table_block = migration_content[table_block_start:next_create]
            assert pattern in table_block, (
                f"Table {table} missing UUID PRIMARY KEY"
            )

    def test_migration_has_indexes(self, migration_content):
        """Each table should have at least one index."""
        tables = [
            "kyc_app",
            "kyc_doc",
            "screening_participant",
            "risk_score",
            "alert_participant",
            "sar_participant",
        ]
        for idx_prefix in tables:
            assert f"idx_{idx_prefix}" in migration_content, (
                f"Missing index with prefix idx_{idx_prefix}"
            )

    def test_migration_kyc_applications_has_status_check(self, migration_content):
        """The kyc_applications table must constrain status values."""
        assert "APPLICATION_SUBMITTED" in migration_content
        assert "MANUAL_REVIEW" in migration_content

    def test_migration_risk_scores_constrained(self, migration_content):
        """Risk scores must be 0-100."""
        assert "BETWEEN 0 AND 100" in migration_content

    def test_migration_match_score_constrained(self, migration_content):
        """Match scores must be 0-1."""
        assert "BETWEEN 0 AND 1" in migration_content
