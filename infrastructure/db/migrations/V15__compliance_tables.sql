-- V15: Compliance service PostgreSQL persistence tables (T107)
-- Replaces in-memory stores for KYC applications, screening, alerts, audit, and SAR filings.
-- Uses VARCHAR IDs to match Go service ID generation (e.g. "app-1", "scr-2").
-- The V7 migration created UUID-based tables; these tables use the compliance_v2 prefix
-- to coexist during migration. Services should use these tables going forward.

CREATE SCHEMA IF NOT EXISTS compliance;

-- KYC applications
CREATE TABLE IF NOT EXISTS compliance.kyc_applications_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    participant_type    VARCHAR(50) NOT NULL,
    status              VARCHAR(30) NOT NULL DEFAULT 'APPLICATION_SUBMITTED',
    legal_name          VARCHAR(255) NOT NULL,
    trading_name        VARCHAR(255),
    nationality         CHAR(2) NOT NULL,
    registration_number VARCHAR(64),
    tax_id              VARCHAR(64),
    email               VARCHAR(255),
    phone               VARCHAR(20),
    contact_person_name VARCHAR(255),
    address_line1       VARCHAR(255),
    address_line2       VARCHAR(255),
    city                VARCHAR(100),
    province            VARCHAR(100),
    postal_code         VARCHAR(20),
    country             CHAR(2) NOT NULL DEFAULT 'MN',
    source_of_funds     TEXT,
    risk_tier           VARCHAR(12),
    assigned_officer_id VARCHAR(64),
    rejection_reason    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at         TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kyc_app_v2_participant ON compliance.kyc_applications_v2(participant_id);
CREATE INDEX IF NOT EXISTS idx_kyc_app_v2_status ON compliance.kyc_applications_v2(status);
CREATE INDEX IF NOT EXISTS idx_kyc_app_v2_created ON compliance.kyc_applications_v2(created_at);

-- KYC documents
CREATE TABLE IF NOT EXISTS compliance.documents_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    application_id      VARCHAR(64) NOT NULL REFERENCES compliance.kyc_applications_v2(id),
    doc_type            VARCHAR(50) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'UPLOADED',
    filename            VARCHAR(255),
    content_type        VARCHAR(100),
    storage_key         VARCHAR(512),
    file_size_bytes     BIGINT NOT NULL DEFAULT 0,
    verification_notes  TEXT,
    uploaded_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at         TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_doc_v2_application ON compliance.documents_v2(application_id);

-- Screening results
CREATE TABLE IF NOT EXISTS compliance.screening_results_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    application_id      VARCHAR(64),
    participant_id      VARCHAR(64) NOT NULL,
    outcome             VARCHAR(20) NOT NULL,
    provider            VARCHAR(100),
    list_versions       TEXT,
    screened_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_screening_v2_participant ON compliance.screening_results_v2(participant_id);
CREATE INDEX IF NOT EXISTS idx_screening_v2_screened ON compliance.screening_results_v2(screened_at);

-- Screening matches
CREATE TABLE IF NOT EXISTS compliance.screening_matches_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    screening_id        VARCHAR(64) NOT NULL REFERENCES compliance.screening_results_v2(id),
    matched_name        VARCHAR(255) NOT NULL,
    matched_entity_id   VARCHAR(128),
    list_source         VARCHAR(50) NOT NULL,
    match_type          VARCHAR(20) NOT NULL,
    match_score         DOUBLE PRECISION NOT NULL DEFAULT 0,
    resolved            BOOLEAN NOT NULL DEFAULT FALSE,
    is_true_match       BOOLEAN NOT NULL DEFAULT FALSE,
    resolved_by         VARCHAR(64),
    resolution_notes    TEXT,
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_match_v2_screening ON compliance.screening_matches_v2(screening_id);

-- Risk scores
CREATE TABLE IF NOT EXISTS compliance.risk_scores_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    overall_score       INT NOT NULL DEFAULT 0,
    risk_tier           VARCHAR(12) NOT NULL,
    model_version       VARCHAR(20) NOT NULL,
    participant_type_score    INT NOT NULL DEFAULT 0,
    country_risk_score        INT NOT NULL DEFAULT 0,
    screening_result_score    INT NOT NULL DEFAULT 0,
    transaction_profile_score INT NOT NULL DEFAULT 0,
    source_of_funds_score     INT NOT NULL DEFAULT 0,
    document_quality_score    INT NOT NULL DEFAULT 0,
    computed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_review_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_risk_v2_participant ON compliance.risk_scores_v2(participant_id);

-- Monitoring alerts
CREATE TABLE IF NOT EXISTS compliance.alerts_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64),
    rule_id             VARCHAR(50),
    alert_type          VARCHAR(50) NOT NULL DEFAULT '',
    severity            VARCHAR(20) NOT NULL DEFAULT 'MEDIUM',
    status              VARCHAR(30) NOT NULL DEFAULT 'OPEN',
    description         TEXT,
    details             TEXT,
    resolved_by         VARCHAR(64),
    resolution_notes    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alert_v2_participant ON compliance.alerts_v2(participant_id);
CREATE INDEX IF NOT EXISTS idx_alert_v2_status ON compliance.alerts_v2(status);
CREATE INDEX IF NOT EXISTS idx_alert_v2_created ON compliance.alerts_v2(created_at);

-- Audit log
CREATE TABLE IF NOT EXISTS compliance.audit_log (
    id                  BIGSERIAL PRIMARY KEY,
    actor_id            VARCHAR(64),
    action              VARCHAR(100) NOT NULL,
    target_id           VARCHAR(64),
    details_json        JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_actor ON compliance.audit_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_created ON compliance.audit_log(created_at);

-- SAR filings
CREATE TABLE IF NOT EXISTS compliance.sar_filings_v2 (
    id                  VARCHAR(64) PRIMARY KEY,
    participant_id      VARCHAR(64) NOT NULL,
    alert_id            VARCHAR(64),
    officer_id          VARCHAR(64),
    narrative           TEXT NOT NULL,
    supporting_evidence TEXT,
    reference_number    VARCHAR(100),
    filed_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sar_v2_participant ON compliance.sar_filings_v2(participant_id);
CREATE INDEX IF NOT EXISTS idx_sar_v2_filed ON compliance.sar_filings_v2(filed_at);
