-- V7: KYC/AML compliance tables (T015)
-- Adds: kyc_applications, kyc_documents, screening_results, screening_matches,
--        risk_scores, monitoring_alerts, sar_filings
-- These tables support the KYC/AML compliance service spec.

-- KYC onboarding applications
CREATE TABLE compliance.kyc_applications (
    application_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_id      UUID NOT NULL,
    participant_type    VARCHAR(20) NOT NULL
                        CHECK (participant_type IN (
                            'INDIVIDUAL','CORPORATE','COOPERATIVE','BROKER','INSTITUTIONAL'
                        )),
    status              VARCHAR(30) NOT NULL DEFAULT 'APPLICATION_SUBMITTED'
                        CHECK (status IN (
                            'APPLICATION_SUBMITTED','DOCUMENTS_PENDING','DOCUMENTS_UPLOADED',
                            'VERIFICATION_IN_PROGRESS','SCREENING_IN_PROGRESS','RISK_SCORING',
                            'MANUAL_REVIEW','APPROVED','REJECTED','SUSPENDED','EXPIRED'
                        )),
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
    risk_tier           VARCHAR(12)
                        CHECK (risk_tier IN ('LOW','MEDIUM','HIGH','PROHIBITED')),
    assigned_officer_id UUID,
    rejection_reason    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at         TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ
);

CREATE INDEX idx_kyc_app_participant ON compliance.kyc_applications(participant_id);
CREATE INDEX idx_kyc_app_status ON compliance.kyc_applications(status);
CREATE INDEX idx_kyc_app_officer ON compliance.kyc_applications(assigned_officer_id)
    WHERE assigned_officer_id IS NOT NULL;
CREATE INDEX idx_kyc_app_expires ON compliance.kyc_applications(expires_at)
    WHERE expires_at IS NOT NULL AND status = 'APPROVED';

-- KYC document metadata (actual files stored in S3)
CREATE TABLE compliance.kyc_documents (
    document_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id      UUID NOT NULL REFERENCES compliance.kyc_applications(application_id),
    document_type       VARCHAR(30) NOT NULL
                        CHECK (document_type IN (
                            'NATIONAL_ID','PASSPORT','PROOF_OF_ADDRESS',
                            'COMPANY_REGISTRATION','BENEFICIAL_OWNERSHIP',
                            'FINANCIAL_STATEMENTS','BROKER_LICENSE',
                            'BOARD_RESOLUTION','TAX_REGISTRATION',
                            'COOPERATIVE_MEMBERSHIP'
                        )),
    status              VARCHAR(15) NOT NULL DEFAULT 'UPLOADED'
                        CHECK (status IN (
                            'UPLOADED','VALIDATING','VERIFIED','REJECTED','EXPIRED','NEEDS_REVIEW'
                        )),
    filename            VARCHAR(255) NOT NULL,
    content_type        VARCHAR(100) NOT NULL,
    storage_key         VARCHAR(512) NOT NULL,
    file_size_bytes     BIGINT NOT NULL,
    verification_notes  TEXT,
    uploaded_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at         TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ
);

CREATE INDEX idx_kyc_doc_application ON compliance.kyc_documents(application_id);
CREATE INDEX idx_kyc_doc_status ON compliance.kyc_documents(status);

-- Watchlist screening results
CREATE TABLE compliance.screening_results (
    screening_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id      UUID REFERENCES compliance.kyc_applications(application_id),
    participant_id      UUID NOT NULL,
    outcome             VARCHAR(15) NOT NULL
                        CHECK (outcome IN ('CLEAR','MATCH_FOUND','ERROR')),
    provider            VARCHAR(50) NOT NULL,
    list_versions       JSONB,
    screened_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Block DELETE/UPDATE on screening_results (append-only audit requirement)
CREATE RULE no_update_screening_results AS ON UPDATE TO compliance.screening_results
    DO INSTEAD NOTHING;
CREATE RULE no_delete_screening_results AS ON DELETE TO compliance.screening_results
    DO INSTEAD NOTHING;

CREATE INDEX idx_screening_participant ON compliance.screening_results(participant_id);
CREATE INDEX idx_screening_application ON compliance.screening_results(application_id)
    WHERE application_id IS NOT NULL;
CREATE INDEX idx_screening_time ON compliance.screening_results(screened_at);

-- Individual matches within a screening result
CREATE TABLE compliance.screening_matches (
    match_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    screening_id        UUID NOT NULL REFERENCES compliance.screening_results(screening_id),
    matched_name        VARCHAR(255) NOT NULL,
    matched_entity_id   VARCHAR(128),
    list_source         VARCHAR(50) NOT NULL,
    match_type          VARCHAR(15) NOT NULL
                        CHECK (match_type IN ('EXACT_ID','EXACT_NAME','FUZZY_NAME','PHONETIC')),
    match_score         NUMERIC(4,3) NOT NULL CHECK (match_score BETWEEN 0 AND 1),
    resolved            BOOLEAN NOT NULL DEFAULT FALSE,
    is_true_match       BOOLEAN,
    resolved_by         UUID,
    resolution_notes    TEXT,
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX idx_screening_match_screening ON compliance.screening_matches(screening_id);
CREATE INDEX idx_screening_match_unresolved ON compliance.screening_matches(resolved)
    WHERE resolved = FALSE;

-- Risk scores (append-only — new scores are added, never overwritten)
CREATE TABLE compliance.risk_scores (
    score_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_id      UUID NOT NULL,
    overall_score       SMALLINT NOT NULL CHECK (overall_score BETWEEN 0 AND 100),
    risk_tier           VARCHAR(12) NOT NULL
                        CHECK (risk_tier IN ('LOW','MEDIUM','HIGH','PROHIBITED')),
    model_version       VARCHAR(20) NOT NULL,
    -- Factor breakdown (each 0–100)
    participant_type_score   SMALLINT NOT NULL,
    country_risk_score       SMALLINT NOT NULL,
    screening_result_score   SMALLINT NOT NULL,
    transaction_profile_score SMALLINT NOT NULL,
    source_of_funds_score    SMALLINT NOT NULL,
    document_quality_score   SMALLINT NOT NULL,
    computed_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_review_at      TIMESTAMPTZ NOT NULL
);

-- Block DELETE/UPDATE on risk_scores (append-only)
CREATE RULE no_update_risk_scores AS ON UPDATE TO compliance.risk_scores
    DO INSTEAD NOTHING;
CREATE RULE no_delete_risk_scores AS ON DELETE TO compliance.risk_scores
    DO INSTEAD NOTHING;

CREATE INDEX idx_risk_score_participant ON compliance.risk_scores(participant_id);
CREATE INDEX idx_risk_score_time ON compliance.risk_scores(computed_at);
CREATE INDEX idx_risk_score_review ON compliance.risk_scores(next_review_at);

-- Transaction monitoring alerts
CREATE TABLE compliance.monitoring_alerts (
    alert_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_id      UUID NOT NULL,
    rule_id             VARCHAR(20) NOT NULL,
    status              VARCHAR(30) NOT NULL DEFAULT 'OPEN'
                        CHECK (status IN (
                            'OPEN','UNDER_REVIEW','RESOLVED_FALSE_POSITIVE',
                            'RESOLVED_CONFIRMED','SAR_FILED'
                        )),
    description         TEXT NOT NULL,
    details             JSONB,
    resolved_by         UUID,
    resolution_notes    TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX idx_alert_participant ON compliance.monitoring_alerts(participant_id);
CREATE INDEX idx_alert_status ON compliance.monitoring_alerts(status);
CREATE INDEX idx_alert_rule ON compliance.monitoring_alerts(rule_id);
CREATE INDEX idx_alert_created ON compliance.monitoring_alerts(created_at);

-- Suspicious Activity Report filings
CREATE TABLE compliance.sar_filings (
    sar_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    participant_id      UUID NOT NULL,
    alert_id            UUID REFERENCES compliance.monitoring_alerts(alert_id),
    officer_id          UUID NOT NULL,
    narrative           TEXT NOT NULL,
    supporting_evidence JSONB,
    reference_number    VARCHAR(64),
    filed_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at     TIMESTAMPTZ
);

-- Block DELETE/UPDATE on sar_filings (regulatory immutability requirement)
CREATE RULE no_update_sar_filings AS ON UPDATE TO compliance.sar_filings
    DO INSTEAD NOTHING;
CREATE RULE no_delete_sar_filings AS ON DELETE TO compliance.sar_filings
    DO INSTEAD NOTHING;

CREATE INDEX idx_sar_participant ON compliance.sar_filings(participant_id);
CREATE INDEX idx_sar_alert ON compliance.sar_filings(alert_id) WHERE alert_id IS NOT NULL;
CREATE INDEX idx_sar_filed ON compliance.sar_filings(filed_at);

-- Grant access to compliance service role
GRANT SELECT, INSERT, UPDATE ON compliance.kyc_applications TO garudax_compliance_svc;
GRANT SELECT, INSERT, UPDATE ON compliance.kyc_documents TO garudax_compliance_svc;
GRANT SELECT, INSERT ON compliance.screening_results TO garudax_compliance_svc;
GRANT SELECT, INSERT, UPDATE ON compliance.screening_matches TO garudax_compliance_svc;
GRANT SELECT, INSERT ON compliance.risk_scores TO garudax_compliance_svc;
GRANT SELECT, INSERT, UPDATE ON compliance.monitoring_alerts TO garudax_compliance_svc;
GRANT SELECT, INSERT ON compliance.sar_filings TO garudax_compliance_svc;

-- Exchange service gets read-only access to participant compliance status
GRANT SELECT ON compliance.kyc_applications TO garudax_exchange_svc;
GRANT SELECT ON compliance.risk_scores TO garudax_exchange_svc;
