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
	Email             string
	Phone             string
	ContactPersonName string
}

// Address holds a physical address.
type Address struct {
	Line1      string
	Line2      string
	City       string
	Province   string
	PostalCode string
	Country    string
}

// KYCApplication represents a KYC onboarding application.
type KYCApplication struct {
	ApplicationID      string
	ParticipantID      string
	ParticipantType    ParticipantType
	Status             KYCStatus
	LegalName          string
	TradingName        string
	Nationality        string
	RegistrationNumber string
	TaxID              string
	Contact            ContactInfo
	RegisteredAddress  Address
	SourceOfFunds      string
	RiskTier           RiskTier
	AssignedOfficerID  string
	RejectionReason    string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ApprovedAt         time.Time
	ExpiresAt          time.Time
}

// Document represents a KYC verification document.
type Document struct {
	DocumentID        string
	ApplicationID     string
	DocumentType      DocumentType
	Status            DocumentStatus
	Filename          string
	ContentType       string
	StorageKey        string
	FileSizeBytes     uint64
	VerificationNotes string
	UploadedAt        time.Time
	VerifiedAt        time.Time
	ExpiresAt         time.Time
}

// ScreeningResult represents the outcome of a watchlist screening.
type ScreeningResult struct {
	ScreeningID   string
	ApplicationID string
	ParticipantID string
	Outcome       ScreeningOutcome
	Matches       []ScreeningMatch
	Provider      string
	ListVersions  string
	ScreenedAt    time.Time
}

// ScreeningMatch represents a match found during screening.
type ScreeningMatch struct {
	MatchID         string
	ScreeningID     string
	MatchedName     string
	MatchedEntityID string
	ListSource      string
	MatchType       MatchType
	MatchScore      float64
	Resolved        bool
	IsTrueMatch     bool
	ResolvedBy      string
	ResolutionNotes string
	ResolvedAt      time.Time
}

// RiskScore represents a computed risk assessment.
type RiskScore struct {
	ScoreID       string
	ParticipantID string
	OverallScore  uint32
	Tier          RiskTier
	ModelVersion  string
	Factors       RiskFactorBreakdown
	ComputedAt    time.Time
	NextReviewAt  time.Time
}

// RiskFactorBreakdown holds the individual factor scores.
type RiskFactorBreakdown struct {
	ParticipantTypeScore    uint32
	CountryRiskScore        uint32
	ScreeningResultScore    uint32
	TransactionProfileScore uint32
	SourceOfFundsScore      uint32
	DocumentQualityScore    uint32
}

// MonitoringAlert represents a transaction monitoring alert.
type MonitoringAlert struct {
	AlertID         string
	ParticipantID   string
	RuleID          string
	Status          AlertStatus
	Description     string
	Details         string
	ResolvedBy      string
	ResolutionNotes string
	CreatedAt       time.Time
	ResolvedAt      time.Time
}

// SARFiling represents a Suspicious Activity Report.
type SARFiling struct {
	SARID              string
	ParticipantID      string
	AlertID            string
	OfficerID          string
	Narrative          string
	SupportingEvidence string
	ReferenceNumber    string
	FiledAt            time.Time
	AcknowledgedAt     time.Time
}

// ParticipantStatus is the response for CheckParticipantStatus.
type ParticipantStatus struct {
	ParticipantID  string
	Result         ComplianceCheckResult
	RiskTier       RiskTier
	LastScreenedAt time.Time
	KYCExpiresAt   time.Time
}
