package onboarding

import (
	"testing"

	"github.com/ace-platform/compliance-service/internal/types"
)

func newTestService() *Service {
	store := NewInMemoryStore()
	return NewService(store)
}

func submitTestApplication(t *testing.T, svc *Service, participantType types.ParticipantType) *types.KYCApplication {
	t.Helper()
	app, err := svc.SubmitApplication(
		"part-1",
		participantType,
		"Test Participant",
		"Test Trading",
		"MN",
		types.ContactInfo{Email: "test@example.com", Phone: "+97612345678"},
		types.Address{Line1: "123 Main St", City: "Ulaanbaatar", Country: "MN"},
		"business income",
	)
	if err != nil {
		t.Fatalf("SubmitApplication failed: %v", err)
	}
	return app
}

func TestSubmitApplication(t *testing.T) {
	svc := newTestService()

	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	if app.ApplicationID == "" {
		t.Error("expected non-empty application ID")
	}
	if app.Status != types.StatusDocumentsPending {
		t.Errorf("expected status DOCUMENTS_PENDING, got %s", app.Status)
	}
	if app.ParticipantType != types.ParticipantIndividual {
		t.Errorf("expected INDIVIDUAL, got %s", app.ParticipantType)
	}
	if app.LegalName != "Test Participant" {
		t.Errorf("expected legal name 'Test Participant', got %s", app.LegalName)
	}
}

func TestSubmitApplicationValidation(t *testing.T) {
	svc := newTestService()

	// Missing participant ID
	_, err := svc.SubmitApplication("", types.ParticipantIndividual, "Name", "", "MN",
		types.ContactInfo{}, types.Address{}, "")
	if err == nil {
		t.Error("expected error for empty participant_id")
	}

	// Missing legal name
	_, err = svc.SubmitApplication("p1", types.ParticipantIndividual, "", "", "MN",
		types.ContactInfo{}, types.Address{}, "")
	if err == nil {
		t.Error("expected error for empty legal_name")
	}

	// Invalid participant type
	_, err = svc.SubmitApplication("p1", types.ParticipantType("INVALID"), "Name", "", "MN",
		types.ContactInfo{}, types.Address{}, "")
	if err == nil {
		t.Error("expected error for invalid participant_type")
	}

	// Invalid nationality
	_, err = svc.SubmitApplication("p1", types.ParticipantIndividual, "Name", "", "MONGOLIA",
		types.ContactInfo{}, types.Address{}, "")
	if err == nil {
		t.Error("expected error for invalid nationality")
	}
}

func TestGetApplication(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	retrieved, err := svc.GetApplication(app.ApplicationID)
	if err != nil {
		t.Fatalf("GetApplication failed: %v", err)
	}
	if retrieved.ApplicationID != app.ApplicationID {
		t.Error("retrieved application ID mismatch")
	}

	_, err = svc.GetApplication("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent application")
	}
}

func TestUploadDocument(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	doc, err := svc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("test content"))
	if err != nil {
		t.Fatalf("UploadDocument failed: %v", err)
	}

	if doc.DocumentID == "" {
		t.Error("expected non-empty document ID")
	}
	if doc.Status != types.DocStatusUploaded {
		t.Errorf("expected status UPLOADED, got %s", doc.Status)
	}
	if doc.FileSizeBytes != 12 {
		t.Errorf("expected file size 12, got %d", doc.FileSizeBytes)
	}
}

func TestUploadDocumentSizeLimit(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	bigContent := make([]byte, 21*1024*1024)
	_, err := svc.UploadDocument(app.ApplicationID, types.DocNationalID, "big.pdf", "application/pdf", bigContent)
	if err == nil {
		t.Error("expected error for oversized file")
	}
}

func TestSubmitDocuments(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	// Try submitting without all required docs
	_, err := svc.SubmitDocuments(app.ApplicationID)
	if err == nil {
		t.Error("expected error for missing documents")
	}

	// Upload required docs
	_, err = svc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("id"))
	if err != nil {
		t.Fatalf("upload doc failed: %v", err)
	}
	_, err = svc.UploadDocument(app.ApplicationID, types.DocProofOfAddress, "addr.pdf", "application/pdf", []byte("addr"))
	if err != nil {
		t.Fatalf("upload doc failed: %v", err)
	}

	// Now submit should work
	updated, err := svc.SubmitDocuments(app.ApplicationID)
	if err != nil {
		t.Fatalf("SubmitDocuments failed: %v", err)
	}
	if updated.Status != types.StatusDocumentsUploaded {
		t.Errorf("expected status DOCUMENTS_UPLOADED, got %s", updated.Status)
	}
}

func TestStartVerification(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	// Upload required docs
	svc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("id"))
	svc.UploadDocument(app.ApplicationID, types.DocProofOfAddress, "addr.pdf", "application/pdf", []byte("addr"))
	svc.SubmitDocuments(app.ApplicationID)

	// Start verification
	updated, err := svc.StartVerification(app.ApplicationID)
	if err != nil {
		t.Fatalf("StartVerification failed: %v", err)
	}
	if updated.Status != types.StatusScreeningInProgress {
		t.Errorf("expected status SCREENING_IN_PROGRESS, got %s", updated.Status)
	}

	// Verify docs are marked as verified
	docs, _ := svc.ListDocuments(app.ApplicationID)
	for _, doc := range docs {
		if doc.Status != types.DocStatusVerified {
			t.Errorf("expected doc status VERIFIED, got %s", doc.Status)
		}
	}
}

func TestApproveApplication(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	// Force to MANUAL_REVIEW for testing
	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusManualReview
	svc.store.SaveApplication(a)

	approved, err := svc.ApproveApplication(app.ApplicationID, "officer-1", "all checks passed")
	if err != nil {
		t.Fatalf("ApproveApplication failed: %v", err)
	}
	if approved.Status != types.StatusApproved {
		t.Errorf("expected APPROVED, got %s", approved.Status)
	}
	if approved.AssignedOfficerID != "officer-1" {
		t.Errorf("expected officer-1, got %s", approved.AssignedOfficerID)
	}
	if approved.ApprovedAt.IsZero() {
		t.Error("expected non-zero approved_at")
	}
	if approved.ExpiresAt.IsZero() {
		t.Error("expected non-zero expires_at")
	}
}

func TestApproveApplicationValidation(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	// Can't approve in DOCUMENTS_PENDING status
	_, err := svc.ApproveApplication(app.ApplicationID, "officer-1", "")
	if err == nil {
		t.Error("expected error approving in wrong status")
	}

	// Force to MANUAL_REVIEW
	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusManualReview
	svc.store.SaveApplication(a)

	// Missing officer ID
	_, err = svc.ApproveApplication(app.ApplicationID, "", "")
	if err == nil {
		t.Error("expected error for missing officer_id")
	}
}

func TestRejectApplication(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusManualReview
	svc.store.SaveApplication(a)

	rejected, err := svc.RejectApplication(app.ApplicationID, "officer-1", "failed screening")
	if err != nil {
		t.Fatalf("RejectApplication failed: %v", err)
	}
	if rejected.Status != types.StatusRejected {
		t.Errorf("expected REJECTED, got %s", rejected.Status)
	}
	if rejected.RejectionReason != "failed screening" {
		t.Errorf("wrong rejection reason: %s", rejected.RejectionReason)
	}
}

func TestRejectApplicationValidation(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusManualReview
	svc.store.SaveApplication(a)

	// Missing reason
	_, err := svc.RejectApplication(app.ApplicationID, "officer-1", "")
	if err == nil {
		t.Error("expected error for missing reason")
	}
}

func TestSuspendAndReinstate(t *testing.T) {
	svc := newTestService()
	app := submitTestApplication(t, svc, types.ParticipantIndividual)

	// Force to APPROVED
	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusApproved
	svc.store.SaveApplication(a)

	// Suspend
	suspended, err := svc.SuspendParticipant("part-1", "officer-1", "suspicious activity")
	if err != nil {
		t.Fatalf("SuspendParticipant failed: %v", err)
	}
	if suspended.Status != types.StatusSuspended {
		t.Errorf("expected SUSPENDED, got %s", suspended.Status)
	}

	// Reinstate
	reinstated, err := svc.ReinstateParticipant("part-1", "officer-2", "cleared")
	if err != nil {
		t.Fatalf("ReinstateParticipant failed: %v", err)
	}
	if reinstated.Status != types.StatusApproved {
		t.Errorf("expected APPROVED, got %s", reinstated.Status)
	}
}

func TestCheckParticipantStatus(t *testing.T) {
	svc := newTestService()

	// Not found
	status, err := svc.CheckParticipantStatus("unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Result != types.CheckNotFound {
		t.Errorf("expected NOT_FOUND, got %s", status.Result)
	}

	// Approved
	app := submitTestApplication(t, svc, types.ParticipantIndividual)
	a, _ := svc.GetApplication(app.ApplicationID)
	a.Status = types.StatusApproved
	a.ExpiresAt = a.CreatedAt.AddDate(1, 0, 0)
	a.RiskTier = types.RiskLow
	svc.store.SaveApplication(a)

	status, err = svc.CheckParticipantStatus("part-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Result != types.CheckApproved {
		t.Errorf("expected APPROVED, got %s", status.Result)
	}
	if status.RiskTier != types.RiskLow {
		t.Errorf("expected LOW risk tier, got %s", status.RiskTier)
	}

	// Suspended
	a.Status = types.StatusSuspended
	svc.store.SaveApplication(a)
	status, _ = svc.CheckParticipantStatus("part-1")
	if status.Result != types.CheckSuspended {
		t.Errorf("expected SUSPENDED, got %s", status.Result)
	}
}

func TestListApplications(t *testing.T) {
	svc := newTestService()
	submitTestApplication(t, svc, types.ParticipantIndividual)

	apps, err := svc.ListApplications("", "", 50)
	if err != nil {
		t.Fatalf("ListApplications failed: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("expected 1 application, got %d", len(apps))
	}

	// Filter by status
	apps, _ = svc.ListApplications(types.StatusDocumentsPending, "", 50)
	if len(apps) != 1 {
		t.Errorf("expected 1 app with DOCUMENTS_PENDING, got %d", len(apps))
	}

	apps, _ = svc.ListApplications(types.StatusApproved, "", 50)
	if len(apps) != 0 {
		t.Errorf("expected 0 apps with APPROVED, got %d", len(apps))
	}
}
