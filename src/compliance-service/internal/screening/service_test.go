package screening

import (
	"testing"

	"github.com/garudax-platform/compliance-service/internal/onboarding"
	"github.com/garudax-platform/compliance-service/internal/types"
)

func setupTestServices() (*Service, *onboarding.Service, *InMemoryStore) {
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := NewInMemoryStore()
	provider := NewDefaultProvider()
	svc := NewService(screenStore, provider, onboardStore)
	return svc, onboardSvc, screenStore
}

func createTestApplication(t *testing.T, onboardSvc *onboarding.Service) *types.KYCApplication {
	t.Helper()
	app, err := onboardSvc.SubmitApplication(
		"part-1",
		types.ParticipantIndividual,
		"Test User",
		"Test Trading",
		"MN",
		types.ContactInfo{Email: "test@example.com"},
		types.Address{City: "Ulaanbaatar", Country: "MN"},
		"business income",
	)
	if err != nil {
		t.Fatalf("SubmitApplication failed: %v", err)
	}
	return app
}

func TestScreenParticipant_Clear(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	app := createTestApplication(t, onboardSvc)

	result, err := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)
	if err != nil {
		t.Fatalf("ScreenParticipant failed: %v", err)
	}

	if result.Outcome != types.ScreeningClear {
		t.Errorf("expected CLEAR outcome, got %s", result.Outcome)
	}
	if result.Provider != "default-dev" {
		t.Errorf("expected default-dev provider, got %s", result.Provider)
	}
	if result.ScreeningID == "" {
		t.Error("expected non-empty screening ID")
	}
}

func TestScreenParticipant_WithMatches(t *testing.T) {
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := NewInMemoryStore()

	matches := []ProviderMatch{
		{
			MatchedName:     "Test User",
			MatchedEntityID: "OFAC-12345",
			ListSource:      "OFAC_SDN",
			MatchType:       types.MatchExactName,
			Score:           0.95,
		},
	}
	provider := NewStaticProvider(matches, nil)
	svc := NewService(screenStore, provider, onboardStore)

	app := createTestApplication(t, onboardSvc)

	result, err := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)
	if err != nil {
		t.Fatalf("ScreenParticipant failed: %v", err)
	}

	if result.Outcome != types.ScreeningMatchFound {
		t.Errorf("expected MATCH_FOUND outcome, got %s", result.Outcome)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	if result.Matches[0].MatchedName != "Test User" {
		t.Errorf("wrong matched name: %s", result.Matches[0].MatchedName)
	}
	if result.Matches[0].MatchScore != 0.95 {
		t.Errorf("wrong match score: %f", result.Matches[0].MatchScore)
	}
}

func TestScreenParticipant_CachesRecent(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	app := createTestApplication(t, onboardSvc)

	// First screening
	result1, _ := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)

	// Second screening (should return cached result)
	result2, _ := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)

	if result1.ScreeningID != result2.ScreeningID {
		t.Error("expected cached screening result within 24h")
	}

	// Force rescreen should create new result
	result3, _ := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, true)
	if result3.ScreeningID == result1.ScreeningID {
		t.Error("force rescreen should create a new screening")
	}
}

func TestGetScreeningResult(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	app := createTestApplication(t, onboardSvc)

	result, _ := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)

	retrieved, err := svc.GetScreeningResult(result.ScreeningID)
	if err != nil {
		t.Fatalf("GetScreeningResult failed: %v", err)
	}
	if retrieved.ScreeningID != result.ScreeningID {
		t.Error("screening ID mismatch")
	}

	_, err = svc.GetScreeningResult("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent screening")
	}
}

func TestBatchScreen(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	createTestApplication(t, onboardSvc)

	result, err := svc.BatchScreen([]string{"part-1"}, "daily_delta")
	if err != nil {
		t.Fatalf("BatchScreen failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
	if result.Clear != 1 {
		t.Errorf("expected 1 clear, got %d", result.Clear)
	}
	if len(result.ScreeningIDs) != 1 {
		t.Errorf("expected 1 screening ID, got %d", len(result.ScreeningIDs))
	}
}

func TestResolveMatch(t *testing.T) {
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := NewInMemoryStore()
	matches := []ProviderMatch{
		{MatchedName: "Test User", ListSource: "OFAC_SDN", MatchType: types.MatchFuzzyName, Score: 0.87},
	}
	provider := NewStaticProvider(matches, nil)
	svc := NewService(screenStore, provider, onboardStore)

	app := createTestApplication(t, onboardSvc)
	result, _ := svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, false)

	matchID := result.Matches[0].MatchID
	resolved, err := svc.ResolveMatch(matchID, "officer-1", false, "false positive - different person")
	if err != nil {
		t.Fatalf("ResolveMatch failed: %v", err)
	}
	if !resolved.Resolved {
		t.Error("expected match to be resolved")
	}
	if resolved.IsTrueMatch {
		t.Error("expected false match")
	}
	if resolved.ResolvedBy != "officer-1" {
		t.Errorf("wrong resolved_by: %s", resolved.ResolvedBy)
	}

	// Resolving again should fail
	_, err = svc.ResolveMatch(matchID, "officer-2", true, "re-resolve")
	if err == nil {
		t.Error("expected error resolving already-resolved match")
	}
}

func TestCalculateRiskScore(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	app := createTestApplication(t, onboardSvc)

	// Upload and verify docs first
	onboardSvc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("id"))
	onboardSvc.UploadDocument(app.ApplicationID, types.DocProofOfAddress, "addr.pdf", "application/pdf", []byte("addr"))

	// Run screening first
	svc.ScreenParticipant(app.ApplicationID, app.ParticipantID, true)

	// Calculate risk score
	score, err := svc.CalculateRiskScore("part-1", "onboarding")
	if err != nil {
		t.Fatalf("CalculateRiskScore failed: %v", err)
	}

	if score.OverallScore > 100 {
		t.Errorf("overall score %d exceeds 100", score.OverallScore)
	}
	if score.Tier == "" {
		t.Error("expected non-empty risk tier")
	}
	if score.ModelVersion != riskModelVersion {
		t.Errorf("expected model version %s, got %s", riskModelVersion, score.ModelVersion)
	}
	if score.NextReviewAt.IsZero() {
		t.Error("expected non-zero next_review_at")
	}
}

func TestGetRiskScore(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	createTestApplication(t, onboardSvc)
	svc.ScreenParticipant("", "part-1", true)
	svc.CalculateRiskScore("part-1", "onboarding")

	score, err := svc.GetRiskScore("part-1")
	if err != nil {
		t.Fatalf("GetRiskScore failed: %v", err)
	}
	if score.ParticipantID != "part-1" {
		t.Error("participant ID mismatch")
	}

	_, err = svc.GetRiskScore("unknown")
	if err == nil {
		t.Error("expected error for unknown participant")
	}
}

func TestProcessApplicationScreening_ClearPath(t *testing.T) {
	svc, onboardSvc, _ := setupTestServices()
	app := createTestApplication(t, onboardSvc)

	// Upload docs, submit, verify
	onboardSvc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("id"))
	onboardSvc.UploadDocument(app.ApplicationID, types.DocProofOfAddress, "addr.pdf", "application/pdf", []byte("addr"))
	onboardSvc.SubmitDocuments(app.ApplicationID)
	onboardSvc.StartVerification(app.ApplicationID)

	// Process screening (default provider returns clear)
	result, err := svc.ProcessApplicationScreening(app.ApplicationID)
	if err != nil {
		t.Fatalf("ProcessApplicationScreening failed: %v", err)
	}

	// Low risk individual in MN with clear screening should be auto-approved
	if result.Status != types.StatusApproved {
		t.Errorf("expected APPROVED, got %s", result.Status)
	}
}

func TestProcessApplicationScreening_MatchPath(t *testing.T) {
	onboardStore := onboarding.NewInMemoryStore()
	onboardSvc := onboarding.NewService(onboardStore)
	screenStore := NewInMemoryStore()
	matches := []ProviderMatch{
		{MatchedName: "Test User", ListSource: "UN_SANCTIONS", MatchType: types.MatchExactName, Score: 0.95},
	}
	provider := NewStaticProvider(matches, nil)
	svc := NewService(screenStore, provider, onboardStore)

	app := createTestApplication(t, onboardSvc)
	onboardSvc.UploadDocument(app.ApplicationID, types.DocNationalID, "id.pdf", "application/pdf", []byte("id"))
	onboardSvc.UploadDocument(app.ApplicationID, types.DocProofOfAddress, "addr.pdf", "application/pdf", []byte("addr"))
	onboardSvc.SubmitDocuments(app.ApplicationID)
	onboardSvc.StartVerification(app.ApplicationID)

	result, err := svc.ProcessApplicationScreening(app.ApplicationID)
	if err != nil {
		t.Fatalf("ProcessApplicationScreening failed: %v", err)
	}

	// Match found should escalate to manual review
	if result.Status != types.StatusManualReview {
		t.Errorf("expected MANUAL_REVIEW, got %s", result.Status)
	}
}

func TestCreateAndResolveAlert(t *testing.T) {
	svc, _, _ := setupTestServices()

	alert, err := svc.CreateAlert("part-1", "TXN-001", "Large trade detected", `{"amount":"50000"}`)
	if err != nil {
		t.Fatalf("CreateAlert failed: %v", err)
	}
	if alert.Status != types.AlertOpen {
		t.Errorf("expected OPEN status, got %s", alert.Status)
	}

	// Resolve alert
	resolved, err := svc.ResolveAlert(alert.AlertID, "officer-1", types.AlertResolvedFalsePositive, "normal business")
	if err != nil {
		t.Fatalf("ResolveAlert failed: %v", err)
	}
	if resolved.Status != types.AlertResolvedFalsePositive {
		t.Errorf("expected RESOLVED_FALSE_POSITIVE, got %s", resolved.Status)
	}

	// Resolving again should fail
	_, err = svc.ResolveAlert(alert.AlertID, "officer-2", types.AlertResolvedConfirmed, "")
	if err == nil {
		t.Error("expected error resolving already-resolved alert")
	}
}

func TestResolveAlertInvalidStatus(t *testing.T) {
	svc, _, _ := setupTestServices()

	alert, _ := svc.CreateAlert("part-1", "TXN-001", "test", "")

	_, err := svc.ResolveAlert(alert.AlertID, "officer-1", types.AlertOpen, "")
	if err == nil {
		t.Error("expected error for invalid resolution status")
	}
}

func TestListAlerts(t *testing.T) {
	svc, _, _ := setupTestServices()

	svc.CreateAlert("part-1", "TXN-001", "alert 1", "")
	svc.CreateAlert("part-1", "TXN-002", "alert 2", "")
	svc.CreateAlert("part-2", "TXN-001", "alert 3", "")

	// All alerts
	alerts, _ := svc.ListAlerts("", "", 50)
	if len(alerts) != 3 {
		t.Errorf("expected 3 alerts, got %d", len(alerts))
	}

	// Filter by participant
	alerts, _ = svc.ListAlerts("", "part-1", 50)
	if len(alerts) != 2 {
		t.Errorf("expected 2 alerts for part-1, got %d", len(alerts))
	}

	// Filter by status
	alerts, _ = svc.ListAlerts(types.AlertOpen, "", 50)
	if len(alerts) != 3 {
		t.Errorf("expected 3 open alerts, got %d", len(alerts))
	}
}

func TestFileSAR(t *testing.T) {
	svc, _, _ := setupTestServices()

	sar, err := svc.FileSAR("part-1", "alert-1", "officer-1", "Suspicious trading pattern", `{"txn_ids":["t1","t2"]}`)
	if err != nil {
		t.Fatalf("FileSAR failed: %v", err)
	}

	if sar.SARID == "" {
		t.Error("expected non-empty SAR ID")
	}
	if sar.ParticipantID != "part-1" {
		t.Error("participant ID mismatch")
	}
	if sar.Narrative != "Suspicious trading pattern" {
		t.Error("narrative mismatch")
	}
}

func TestFileSARValidation(t *testing.T) {
	svc, _, _ := setupTestServices()

	_, err := svc.FileSAR("", "alert-1", "officer-1", "narrative", "")
	if err == nil {
		t.Error("expected error for missing participant_id")
	}

	_, err = svc.FileSAR("part-1", "alert-1", "", "narrative", "")
	if err == nil {
		t.Error("expected error for missing officer_id")
	}

	_, err = svc.FileSAR("part-1", "alert-1", "officer-1", "", "")
	if err == nil {
		t.Error("expected error for missing narrative")
	}
}
