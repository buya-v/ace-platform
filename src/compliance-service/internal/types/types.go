package types

import "time"

// ParticipantType represents the type of exchange participant.
type ParticipantType string

const (
	ParticipantIndividual    ParticipantType = "INDIVIDUAL"
	ParticipantCorporate     ParticipantType = "CORPORATE"
	ParticipantCooperative   ParticipantType = "COOPERATIVE"
	ParticipantBroker        ParticipantType = "BROKER"
	ParticipantInstitutional ParticipantType = "INSTITUTIONAL"
)

func ValidParticipantType(pt ParticipantType) bool {
	switch pt {
	case ParticipantIndividual, ParticipantCorporate, ParticipantCooperative,
		ParticipantBroker, ParticipantInstitutional:
		return true
	}
	return false
}

// KYCStatus represents the state of a KYC application.
type KYCStatus string

const (
	StatusApplicationSubmitted   KYCStatus = "APPLICATION_SUBMITTED"
	StatusDocumentsPending       KYCStatus = "DOCUMENTS_PENDING"
	StatusDocumentsUploaded      KYCStatus = "DOCUMENTS_UPLOADED"
	StatusVerificationInProgress KYCStatus = "VERIFICATION_IN_PROGRESS"
	StatusScreeningInProgress    KYCStatus = "SCREENING_IN_PROGRESS"
	StatusRiskScoring            KYCStatus = "RISK_SCORING"
	StatusManualReview           KYCStatus = "MANUAL_REVIEW"
	StatusApproved               KYCStatus = "APPROVED"
	StatusRejected               KYCStatus = "REJECTED"
	StatusSuspended              KYCStatus = "SUSPENDED"
	StatusExpired                KYCStatus = "EXPIRED"
)

// DocumentType represents the type of KYC document.
type DocumentType string

const (
	DocNationalID           DocumentType = "NATIONAL_ID"
	DocPassport             DocumentType = "PASSPORT"
	DocProofOfAddress       DocumentType = "PROOF_OF_ADDRESS"
	DocCompanyRegistration  DocumentType = "COMPANY_REGISTRATION"
	DocBeneficialOwnership  DocumentType = "BENEFICIAL_OWNERSHIP"
	DocFinancialStatements  DocumentType = "FINANCIAL_STATEMENTS"
	DocBrokerLicense        DocumentType = "BROKER_LICENSE"
	DocBoardResolution      DocumentType = "BOARD_RESOLUTION"
	DocTaxRegistration      DocumentType = "TAX_REGISTRATION"
	DocCooperativeMembership DocumentType = "COOPERATIVE_MEMBERSHIP"
)

// DocumentStatus represents the verification status of a document.
type DocumentStatus string

const (
	DocStatusUploaded    DocumentStatus = "UPLOADED"
	DocStatusValidating  DocumentStatus = "VALIDATING"
	DocStatusVerified    DocumentStatus = "VERIFIED"
	DocStatusRejected    DocumentStatus = "REJECTED"
	DocStatusExpired     DocumentStatus = "EXPIRED"
	DocStatusNeedsReview DocumentStatus = "NEEDS_REVIEW"
)

// RiskTier represents the risk classification of a participant.
type RiskTier string

const (
	RiskLow        RiskTier = "LOW"
	RiskMedium     RiskTier = "MEDIUM"
	RiskHigh       RiskTier = "HIGH"
	RiskProhibited RiskTier = "PROHIBITED"
)

// ScreeningOutcome represents the result of a watchlist screening.
type ScreeningOutcome string

const (
	ScreeningClear      ScreeningOutcome = "CLEAR"
	ScreeningMatchFound ScreeningOutcome = "MATCH_FOUND"
	ScreeningError      ScreeningOutcome = "ERROR"
)

// MatchType represents how a screening match was found.
type MatchType string

const (
	MatchExactID   MatchType = "EXACT_ID"
	MatchExactName MatchType = "EXACT_NAME"
	MatchFuzzyName MatchType = "FUZZY_NAME"
	MatchPhonetic  MatchType = "PHONETIC"
)

// AlertStatus represents the status of a monitoring alert.
type AlertStatus string

const (
	AlertOpen                  AlertStatus = "OPEN"
	AlertUnderReview           AlertStatus = "UNDER_REVIEW"
	AlertResolvedFalsePositive AlertStatus = "RESOLVED_FALSE_POSITIVE"
	AlertResolvedConfirmed     AlertStatus = "RESOLVED_CONFIRMED"
	AlertSARFiled              AlertStatus = "SAR_FILED"
)

// ComplianceCheckResult for CheckParticipantStatus RPC.
type ComplianceCheckResult string

const (
	CheckApproved  ComplianceCheckResult = "APPROVED"
	CheckSuspended ComplianceCheckResult = "SUSPENDED"
	CheckExpired   ComplianceCheckResult = "EXPIRED"
	CheckNotFound  ComplianceCheckResult = "NOT_FOUND"
)

// ContactInfo holds contact details for a participant.
type ContactInfo struct {
	Email             string `json:"email,omitempty"`
	Phone             string `json:"phone,omitempty"`
	ContactPersonName string `json:"contact_person_name,omitempty"`
}

// Address holds a physical address.
type Address struct {
	Line1      string `json:"line1,omitempty"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city,omitempty"`
	Province   string `json:"province,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country,omitempty"`
}

// KYCApplication represents a KYC onboarding application.
type KYCApplication struct {
	ApplicationID      string          `json:"application_id"`
	ParticipantID      string          `json:"participant_id"`
	ParticipantType    ParticipantType `json:"participant_type"`
	Status             KYCStatus       `json:"status"`
	LegalName          string          `json:"legal_name"`
	TradingName        string          `json:"trading_name,omitempty"`
	Nationality        string          `json:"nationality"`
	RegistrationNumber string          `json:"registration_number,omitempty"`
	TaxID              string          `json:"tax_id,omitempty"`
	Contact            ContactInfo     `json:"contact"`
	RegisteredAddress  Address         `json:"registered_address"`
	SourceOfFunds      string          `json:"source_of_funds,omitempty"`
	RiskTier           RiskTier        `json:"risk_tier,omitempty"`
	AssignedOfficerID  string          `json:"assigned_officer_id,omitempty"`
	RejectionReason    string          `json:"rejection_reason,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ApprovedAt         time.Time       `json:"approved_at,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at,omitempty"`
}

// Document represents a KYC verification document.
type Document struct {
	DocumentID        string         `json:"document_id"`
	ApplicationID     string         `json:"application_id"`
	DocumentType      DocumentType   `json:"document_type"`
	Status            DocumentStatus `json:"status"`
	Filename          string         `json:"filename,omitempty"`
	ContentType       string         `json:"content_type,omitempty"`
	StorageKey        string         `json:"storage_key,omitempty"`
	FileSizeBytes     uint64         `json:"file_size_bytes,omitempty"`
	VerificationNotes string         `json:"verification_notes,omitempty"`
	UploadedAt        time.Time      `json:"uploaded_at"`
	VerifiedAt        time.Time      `json:"verified_at,omitempty"`
	ExpiresAt         time.Time      `json:"expires_at,omitempty"`
}

// ScreeningResult represents the outcome of a watchlist screening.
type ScreeningResult struct {
	ScreeningID   string           `json:"screening_id"`
	ApplicationID string           `json:"application_id,omitempty"`
	ParticipantID string           `json:"participant_id"`
	Outcome       ScreeningOutcome `json:"outcome"`
	Matches       []ScreeningMatch `json:"matches"`
	Provider      string           `json:"provider,omitempty"`
	ListVersions  string           `json:"list_versions,omitempty"`
	ScreenedAt    time.Time        `json:"screened_at"`
}

// ScreeningMatch represents a match found during screening.
type ScreeningMatch struct {
	MatchID         string    `json:"match_id"`
	ScreeningID     string    `json:"screening_id"`
	MatchedName     string    `json:"matched_name"`
	MatchedEntityID string    `json:"matched_entity_id,omitempty"`
	ListSource      string    `json:"list_source,omitempty"`
	MatchType       MatchType `json:"match_type"`
	MatchScore      float64   `json:"match_score"`
	Resolved        bool      `json:"resolved"`
	IsTrueMatch     bool      `json:"is_true_match"`
	ResolvedBy      string    `json:"resolved_by,omitempty"`
	ResolutionNotes string    `json:"resolution_notes,omitempty"`
	ResolvedAt      time.Time `json:"resolved_at,omitempty"`
}

// RiskScore represents a computed risk assessment.
type RiskScore struct {
	ScoreID       string              `json:"score_id"`
	ParticipantID string              `json:"participant_id"`
	OverallScore  uint32              `json:"overall_score"`
	Tier          RiskTier            `json:"tier"`
	ModelVersion  string              `json:"model_version,omitempty"`
	Factors       RiskFactorBreakdown `json:"factors"`
	ComputedAt    time.Time           `json:"computed_at"`
	NextReviewAt  time.Time           `json:"next_review_at,omitempty"`
}

// RiskFactorBreakdown holds the individual factor scores.
type RiskFactorBreakdown struct {
	ParticipantTypeScore    uint32 `json:"participant_type_score"`
	CountryRiskScore        uint32 `json:"country_risk_score"`
	ScreeningResultScore    uint32 `json:"screening_result_score"`
	TransactionProfileScore uint32 `json:"transaction_profile_score"`
	SourceOfFundsScore      uint32 `json:"source_of_funds_score"`
	DocumentQualityScore    uint32 `json:"document_quality_score"`
}

// MonitoringAlert represents a transaction monitoring alert.
type MonitoringAlert struct {
	AlertID         string      `json:"alert_id"`
	ParticipantID   string      `json:"participant_id"`
	RuleID          string      `json:"rule_id"`
	Status          AlertStatus `json:"status"`
	Description     string      `json:"description"`
	Details         string      `json:"details,omitempty"`
	ResolvedBy      string      `json:"resolved_by,omitempty"`
	ResolutionNotes string      `json:"resolution_notes,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	ResolvedAt      time.Time   `json:"resolved_at,omitempty"`
}

// SARFiling represents a Suspicious Activity Report.
type SARFiling struct {
	SARID              string    `json:"sar_id"`
	ParticipantID      string    `json:"participant_id"`
	AlertID            string    `json:"alert_id,omitempty"`
	OfficerID          string    `json:"officer_id"`
	Narrative          string    `json:"narrative"`
	SupportingEvidence string    `json:"supporting_evidence,omitempty"`
	ReferenceNumber    string    `json:"reference_number,omitempty"`
	FiledAt            time.Time `json:"filed_at"`
	AcknowledgedAt     time.Time `json:"acknowledged_at,omitempty"`
}

// ParticipantStatus is the response for CheckParticipantStatus.
type ParticipantStatus struct {
	ParticipantID  string                `json:"participant_id"`
	Result         ComplianceCheckResult `json:"result"`
	RiskTier       RiskTier              `json:"risk_tier,omitempty"`
	LastScreenedAt time.Time             `json:"last_screened_at,omitempty"`
	KYCExpiresAt   time.Time             `json:"kyc_expires_at,omitempty"`
}
