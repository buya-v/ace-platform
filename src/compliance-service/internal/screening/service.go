package screening

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ace-platform/compliance-service/internal/onboarding"
	"github.com/ace-platform/compliance-service/internal/types"
)

var idCounter uint64

func newID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// Service handles watchlist screening, risk scoring, and monitoring.
type Service struct {
	store         Store
	provider      Provider
	onboardStore  onboarding.Store
}

func NewService(store Store, provider Provider, onboardStore onboarding.Store) *Service {
	return &Service{
		store:        store,
		provider:     provider,
		onboardStore: onboardStore,
	}
}

// ScreenParticipant runs watchlist screening for a participant.
func (s *Service) ScreenParticipant(applicationID, participantID string, forceRescreen bool) (*types.ScreeningResult, error) {
	if participantID == "" {
		return nil, fmt.Errorf("participant_id is required")
	}

	// Check if we have a recent screening (within 24h) unless force rescreen
	if !forceRescreen {
		existing, err := s.store.GetLatestScreening(participantID)
		if err == nil && time.Since(existing.ScreenedAt) < 24*time.Hour {
			return existing, nil
		}
	}

	// Look up participant info for screening
	var name, nationality string
	if applicationID != "" {
		app, err := s.onboardStore.GetApplication(applicationID)
		if err == nil {
			name = app.LegalName
			nationality = app.Nationality
		}
	}
	if name == "" {
		// Try by participant ID
		app, err := s.onboardStore.GetApplicationByParticipant(participantID)
		if err != nil {
			return nil, fmt.Errorf("cannot find participant info for screening: %w", err)
		}
		name = app.LegalName
		nationality = app.Nationality
		if applicationID == "" {
			applicationID = app.ApplicationID
		}
	}

	// Run screening via provider
	providerMatches, err := s.provider.Screen(name, nationality)
	now := time.Now().UTC()

	result := &types.ScreeningResult{
		ScreeningID:   newID("scr"),
		ApplicationID: applicationID,
		ParticipantID: participantID,
		Provider:      s.provider.Name(),
		ScreenedAt:    now,
	}

	if err != nil {
		result.Outcome = types.ScreeningError
		if saveErr := s.store.SaveScreeningResult(result); saveErr != nil {
			return nil, fmt.Errorf("saving screening error result: %w", saveErr)
		}
		return result, nil
	}

	if len(providerMatches) == 0 {
		result.Outcome = types.ScreeningClear
	} else {
		result.Outcome = types.ScreeningMatchFound
		for _, pm := range providerMatches {
			match := types.ScreeningMatch{
				MatchID:         newID("match"),
				ScreeningID:     result.ScreeningID,
				MatchedName:     pm.MatchedName,
				MatchedEntityID: pm.MatchedEntityID,
				ListSource:      pm.ListSource,
				MatchType:       pm.MatchType,
				MatchScore:      pm.Score,
			}
			result.Matches = append(result.Matches, match)
			if err := s.store.SaveMatch(&match); err != nil {
				return nil, fmt.Errorf("saving match: %w", err)
			}
		}
	}

	if err := s.store.SaveScreeningResult(result); err != nil {
		return nil, fmt.Errorf("saving screening result: %w", err)
	}

	return result, nil
}

// GetScreeningResult retrieves a screening result by ID.
func (s *Service) GetScreeningResult(screeningID string) (*types.ScreeningResult, error) {
	return s.store.GetScreeningResult(screeningID)
}

// BatchScreen runs screening for multiple participants.
func (s *Service) BatchScreen(participantIDs []string, reason string) (*BatchScreenResult, error) {
	result := &BatchScreenResult{
		Total: uint32(len(participantIDs)),
	}

	for _, pid := range participantIDs {
		sr, err := s.ScreenParticipant("", pid, true)
		if err != nil {
			result.Errors++
			continue
		}
		result.ScreeningIDs = append(result.ScreeningIDs, sr.ScreeningID)
		switch sr.Outcome {
		case types.ScreeningClear:
			result.Clear++
		case types.ScreeningMatchFound:
			result.MatchesFound++
		case types.ScreeningError:
			result.Errors++
		}
	}

	return result, nil
}

// BatchScreenResult holds the aggregate results of batch screening.
type BatchScreenResult struct {
	Total        uint32
	Clear        uint32
	MatchesFound uint32
	Errors       uint32
	ScreeningIDs []string
}

// ResolveMatch resolves a screening match with a compliance officer's disposition.
func (s *Service) ResolveMatch(matchID, officerID string, isTrueMatch bool, notes string) (*types.ScreeningMatch, error) {
	match, err := s.store.GetMatch(matchID)
	if err != nil {
		return nil, err
	}

	if match.Resolved {
		return nil, fmt.Errorf("match %s is already resolved", matchID)
	}
	if officerID == "" {
		return nil, fmt.Errorf("officer_id is required")
	}

	match.Resolved = true
	match.IsTrueMatch = isTrueMatch
	match.ResolvedBy = officerID
	match.ResolutionNotes = notes
	match.ResolvedAt = time.Now().UTC()

	if err := s.store.SaveMatch(match); err != nil {
		return nil, err
	}
	return match, nil
}

// CalculateRiskScore computes and stores a risk score for a participant.
func (s *Service) CalculateRiskScore(participantID, reason string) (*types.RiskScore, error) {
	app, err := s.onboardStore.GetApplicationByParticipant(participantID)
	if err != nil {
		return nil, fmt.Errorf("participant not found: %w", err)
	}

	// Build factor breakdown
	factors := types.RiskFactorBreakdown{
		ParticipantTypeScore: ParticipantTypeRisk(app.ParticipantType),
		CountryRiskScore:     CountryRisk(app.Nationality),
	}

	// Screening result factor
	latestScreening, err := s.store.GetLatestScreening(participantID)
	if err == nil {
		factors.ScreeningResultScore = ScreeningResultRisk(latestScreening.Outcome, len(latestScreening.Matches))
	}

	// Source of funds - simple heuristic
	if app.SourceOfFunds == "" {
		factors.SourceOfFundsScore = 50
	} else {
		factors.SourceOfFundsScore = 20
	}

	// Document quality - based on how many docs are verified
	docs, _ := s.onboardStore.ListDocuments(app.ApplicationID)
	required := types.RequiredDocuments(app.ParticipantType)
	if len(required) > 0 {
		verified := 0
		for _, d := range docs {
			if d.Status == types.DocStatusVerified {
				verified++
			}
		}
		factors.DocumentQualityScore = uint32(100 - (verified*100)/len(required))
	}

	// Transaction profile defaults to low (no transaction history yet)
	factors.TransactionProfileScore = 10

	overallScore, tier := ComputeRiskScore(factors)

	now := time.Now().UTC()
	var nextReview time.Time
	switch tier {
	case types.RiskLow:
		nextReview = now.AddDate(1, 0, 0)
	case types.RiskMedium:
		nextReview = now.AddDate(0, 6, 0)
	case types.RiskHigh:
		nextReview = now.AddDate(0, 3, 0)
	case types.RiskProhibited:
		nextReview = now.AddDate(0, 1, 0)
	}

	score := &types.RiskScore{
		ScoreID:       newID("risk"),
		ParticipantID: participantID,
		OverallScore:  overallScore,
		Tier:          tier,
		ModelVersion:  riskModelVersion,
		Factors:       factors,
		ComputedAt:    now,
		NextReviewAt:  nextReview,
	}

	if err := s.store.SaveRiskScore(score); err != nil {
		return nil, fmt.Errorf("saving risk score: %w", err)
	}

	// Update the application's risk tier
	app.RiskTier = tier
	app.UpdatedAt = now
	if err := s.onboardStore.SaveApplication(app); err != nil {
		return nil, fmt.Errorf("updating application risk tier: %w", err)
	}

	return score, nil
}

// GetRiskScore returns the latest risk score for a participant.
func (s *Service) GetRiskScore(participantID string) (*types.RiskScore, error) {
	return s.store.GetLatestRiskScore(participantID)
}

// ProcessApplicationScreening runs screening and risk scoring for an application,
// then transitions it through the workflow (SCREENING_IN_PROGRESS -> RISK_SCORING -> APPROVED/MANUAL_REVIEW).
func (s *Service) ProcessApplicationScreening(applicationID string) (*types.KYCApplication, error) {
	app, err := s.onboardStore.GetApplication(applicationID)
	if err != nil {
		return nil, err
	}

	if app.Status != types.StatusScreeningInProgress {
		return nil, fmt.Errorf("application must be in SCREENING_IN_PROGRESS status, got %s", app.Status)
	}

	// Run screening
	result, err := s.ScreenParticipant(applicationID, app.ParticipantID, true)
	if err != nil {
		return nil, fmt.Errorf("screening failed: %w", err)
	}

	// If matches found, escalate to manual review
	if result.Outcome == types.ScreeningMatchFound {
		app.Status = types.StatusManualReview
		app.UpdatedAt = time.Now().UTC()
		if err := s.onboardStore.SaveApplication(app); err != nil {
			return nil, err
		}
		return app, nil
	}

	// Move to risk scoring
	app.Status = types.StatusRiskScoring
	app.UpdatedAt = time.Now().UTC()
	if err := s.onboardStore.SaveApplication(app); err != nil {
		return nil, err
	}

	// Calculate risk score
	riskScore, err := s.CalculateRiskScore(app.ParticipantID, "onboarding")
	if err != nil {
		return nil, fmt.Errorf("risk scoring failed: %w", err)
	}

	// Auto-approve low/medium risk; escalate high to manual review
	switch riskScore.Tier {
	case types.RiskLow, types.RiskMedium:
		now := time.Now().UTC()
		app.Status = types.StatusApproved
		app.ApprovedAt = now
		app.ExpiresAt = now.AddDate(1, 0, 0)
		app.UpdatedAt = now
	case types.RiskHigh:
		app.Status = types.StatusManualReview
		app.UpdatedAt = time.Now().UTC()
	case types.RiskProhibited:
		app.Status = types.StatusRejected
		app.RejectionReason = "Prohibited risk tier"
		app.UpdatedAt = time.Now().UTC()
	}

	if err := s.onboardStore.SaveApplication(app); err != nil {
		return nil, err
	}
	return app, nil
}

// CreateAlert creates a new monitoring alert.
func (s *Service) CreateAlert(participantID, ruleID, description, details string) (*types.MonitoringAlert, error) {
	alert := &types.MonitoringAlert{
		AlertID:       newID("alert"),
		ParticipantID: participantID,
		RuleID:        ruleID,
		Status:        types.AlertOpen,
		Description:   description,
		Details:       details,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.store.SaveAlert(alert); err != nil {
		return nil, err
	}
	return alert, nil
}

// ResolveAlert resolves a monitoring alert.
func (s *Service) ResolveAlert(alertID, officerID string, resolution types.AlertStatus, notes string) (*types.MonitoringAlert, error) {
	alert, err := s.store.GetAlert(alertID)
	if err != nil {
		return nil, err
	}

	if alert.Status != types.AlertOpen && alert.Status != types.AlertUnderReview {
		return nil, fmt.Errorf("alert %s is already resolved", alertID)
	}

	switch resolution {
	case types.AlertResolvedFalsePositive, types.AlertResolvedConfirmed, types.AlertSARFiled:
		// valid resolution statuses
	default:
		return nil, fmt.Errorf("invalid resolution status: %s", resolution)
	}

	alert.Status = resolution
	alert.ResolvedBy = officerID
	alert.ResolutionNotes = notes
	alert.ResolvedAt = time.Now().UTC()

	if err := s.store.SaveAlert(alert); err != nil {
		return nil, err
	}
	return alert, nil
}

// ListAlerts lists monitoring alerts with optional filters.
func (s *Service) ListAlerts(statusFilter types.AlertStatus, participantID string, limit int) ([]*types.MonitoringAlert, error) {
	return s.store.ListAlerts(statusFilter, participantID, limit)
}

// FileSAR creates a Suspicious Activity Report.
func (s *Service) FileSAR(participantID, alertID, officerID, narrative, supportingEvidence string) (*types.SARFiling, error) {
	if participantID == "" || officerID == "" || narrative == "" {
		return nil, fmt.Errorf("participant_id, officer_id, and narrative are required")
	}

	sar := &types.SARFiling{
		SARID:              newID("sar"),
		ParticipantID:      participantID,
		AlertID:            alertID,
		OfficerID:          officerID,
		Narrative:          narrative,
		SupportingEvidence: supportingEvidence,
		FiledAt:            time.Now().UTC(),
	}

	if err := s.store.SaveSARFiling(sar); err != nil {
		return nil, err
	}
	return sar, nil
}
