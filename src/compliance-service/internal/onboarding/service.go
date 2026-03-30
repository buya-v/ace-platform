package onboarding

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/garudax-platform/compliance-service/internal/types"
)

var idCounter uint64

func newID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// Service handles KYC onboarding workflows.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// SubmitApplication creates a new KYC application.
func (s *Service) SubmitApplication(participantID string, participantType types.ParticipantType, legalName, tradingName, nationality string, contact types.ContactInfo, address types.Address, sourceOfFunds string) (*types.KYCApplication, error) {
	if participantID == "" {
		return nil, fmt.Errorf("participant_id is required")
	}
	if legalName == "" {
		return nil, fmt.Errorf("legal_name is required")
	}
	if !types.ValidParticipantType(participantType) {
		return nil, fmt.Errorf("invalid participant_type: %s", participantType)
	}
	if len(nationality) != 2 {
		return nil, fmt.Errorf("nationality must be ISO 3166-1 alpha-2 (2 chars)")
	}

	now := time.Now().UTC()
	app := &types.KYCApplication{
		ApplicationID:   newID("app"),
		ParticipantID:   participantID,
		ParticipantType: participantType,
		Status:          types.StatusApplicationSubmitted,
		LegalName:       legalName,
		TradingName:     tradingName,
		Nationality:     nationality,
		Contact:         contact,
		RegisteredAddress: address,
		SourceOfFunds:   sourceOfFunds,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.SaveApplication(app); err != nil {
		return nil, fmt.Errorf("saving application: %w", err)
	}

	// Transition to DOCUMENTS_PENDING
	app.Status = types.StatusDocumentsPending
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveApplication(app); err != nil {
		return nil, fmt.Errorf("updating application status: %w", err)
	}

	return app, nil
}

// GetApplication retrieves an application by ID.
func (s *Service) GetApplication(applicationID string) (*types.KYCApplication, error) {
	return s.store.GetApplication(applicationID)
}

// ListApplications lists applications with optional filters.
func (s *Service) ListApplications(statusFilter types.KYCStatus, typeFilter types.ParticipantType, limit int) ([]*types.KYCApplication, error) {
	return s.store.ListApplications(statusFilter, typeFilter, limit)
}

// UploadDocument adds a document to an application.
func (s *Service) UploadDocument(applicationID string, docType types.DocumentType, filename, contentType string, content []byte) (*types.Document, error) {
	app, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, fmt.Errorf("application not found: %w", err)
	}

	if app.Status != types.StatusDocumentsPending && app.Status != types.StatusDocumentsUploaded {
		return nil, fmt.Errorf("cannot upload documents in status %s", app.Status)
	}

	if len(content) > 20*1024*1024 {
		return nil, fmt.Errorf("file size exceeds 20MB limit")
	}

	now := time.Now().UTC()
	doc := &types.Document{
		DocumentID:    newID("doc"),
		ApplicationID: applicationID,
		DocumentType:  docType,
		Status:        types.DocStatusUploaded,
		Filename:      filename,
		ContentType:   contentType,
		StorageKey:    fmt.Sprintf("compliance/%s/%s/%s", app.ParticipantID, applicationID, filename),
		FileSizeBytes: uint64(len(content)),
		UploadedAt:    now,
	}

	if err := s.store.SaveDocument(doc); err != nil {
		return nil, fmt.Errorf("saving document: %w", err)
	}

	return doc, nil
}

// ListDocuments lists all documents for an application.
func (s *Service) ListDocuments(applicationID string) ([]*types.Document, error) {
	return s.store.ListDocuments(applicationID)
}

// SubmitDocuments transitions an application to DOCUMENTS_UPLOADED after checking required docs are present.
func (s *Service) SubmitDocuments(applicationID string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusDocumentsPending {
		return nil, fmt.Errorf("cannot submit documents in status %s", app.Status)
	}

	docs, err := s.store.ListDocuments(applicationID)
	if err != nil {
		return nil, err
	}

	required := types.RequiredDocuments(app.ParticipantType)
	uploaded := make(map[types.DocumentType]bool)
	for _, d := range docs {
		uploaded[d.DocumentType] = true
	}

	for _, req := range required {
		if !uploaded[req] {
			return nil, fmt.Errorf("missing required document: %s", req)
		}
	}

	app.Status = types.StatusDocumentsUploaded
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// StartVerification transitions the application to VERIFICATION_IN_PROGRESS and
// verifies all uploaded documents. This simulates the automated verification pipeline.
func (s *Service) StartVerification(applicationID string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, err
	}
	if err := types.ValidateStatusTransition(app.Status, types.StatusVerificationInProgress); err != nil {
		return nil, err
	}

	app.Status = types.StatusVerificationInProgress
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}

	// Simulate document verification - mark all docs as verified
	docs, err := s.store.ListDocuments(applicationID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, doc := range docs {
		doc.Status = types.DocStatusVerified
		doc.VerifiedAt = now
		if err := s.store.SaveDocument(doc); err != nil {
			return nil, err
		}
	}

	// Transition to SCREENING_IN_PROGRESS
	app.Status = types.StatusScreeningInProgress
	app.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}

	return app, nil
}

// ApproveApplication allows a compliance officer to approve an application in MANUAL_REVIEW.
func (s *Service) ApproveApplication(applicationID, officerID, notes string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusManualReview && app.Status != types.StatusRiskScoring {
		return nil, fmt.Errorf("cannot approve application in status %s", app.Status)
	}
	if officerID == "" {
		return nil, fmt.Errorf("officer_id is required for approval")
	}

	now := time.Now().UTC()
	app.Status = types.StatusApproved
	app.AssignedOfficerID = officerID
	app.ApprovedAt = now
	app.ExpiresAt = now.AddDate(1, 0, 0) // KYC valid for 1 year
	app.UpdatedAt = now

	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// RejectApplication allows a compliance officer to reject an application.
func (s *Service) RejectApplication(applicationID, officerID, reason string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusManualReview {
		return nil, fmt.Errorf("cannot reject application in status %s", app.Status)
	}
	if officerID == "" {
		return nil, fmt.Errorf("officer_id is required for rejection")
	}
	if reason == "" {
		return nil, fmt.Errorf("rejection reason is required")
	}

	app.Status = types.StatusRejected
	app.AssignedOfficerID = officerID
	app.RejectionReason = reason
	app.UpdatedAt = time.Now().UTC()

	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// SuspendParticipant suspends a previously approved participant.
func (s *Service) SuspendParticipant(participantID, officerID, reason string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplicationByParticipant(participantID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusApproved {
		return nil, fmt.Errorf("can only suspend approved participants, current status: %s", app.Status)
	}

	app.Status = types.StatusSuspended
	app.AssignedOfficerID = officerID
	app.RejectionReason = reason
	app.UpdatedAt = time.Now().UTC()

	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// ReinstateParticipant reinstates a suspended participant.
func (s *Service) ReinstateParticipant(participantID, officerID, notes string) (*types.KYCApplication, error) {
	app, err := s.store.GetApplicationByParticipant(participantID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusSuspended {
		return nil, fmt.Errorf("can only reinstate suspended participants, current status: %s", app.Status)
	}

	now := time.Now().UTC()
	app.Status = types.StatusApproved
	app.AssignedOfficerID = officerID
	app.RejectionReason = ""
	app.UpdatedAt = now
	app.ExpiresAt = now.AddDate(1, 0, 0)

	if err := s.store.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// CheckParticipantStatus checks if a participant is cleared to trade.
func (s *Service) CheckParticipantStatus(participantID string) (*types.ParticipantStatus, error) {
	app, err := s.store.GetApplicationByParticipant(participantID)
	if err != nil {
		return &types.ParticipantStatus{
			ParticipantID: participantID,
			Result:        types.CheckNotFound,
		}, nil
	}

	status := &types.ParticipantStatus{
		ParticipantID: participantID,
		RiskTier:      app.RiskTier,
		KYCExpiresAt:  app.ExpiresAt,
	}

	switch app.Status {
	case types.StatusApproved:
		if !app.ExpiresAt.IsZero() && time.Now().UTC().After(app.ExpiresAt) {
			status.Result = types.CheckExpired
		} else {
			status.Result = types.CheckApproved
		}
	case types.StatusSuspended:
		status.Result = types.CheckSuspended
	default:
		status.Result = types.CheckNotFound
	}

	return status, nil
}

// GetStore returns the underlying store (for use by screening service).
func (s *Service) GetStore() Store {
	return s.store
}
