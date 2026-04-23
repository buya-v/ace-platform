package screening

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/compliance-service/internal/types"
)

// Store defines the persistence interface for screening data.
type Store interface {
	SaveScreeningResult(result *types.ScreeningResult) error
	GetScreeningResult(screeningID string) (*types.ScreeningResult, error)
	GetLatestScreening(participantID string) (*types.ScreeningResult, error)

	SaveMatch(match *types.ScreeningMatch) error
	GetMatch(matchID string) (*types.ScreeningMatch, error)

	SaveRiskScore(score *types.RiskScore) error
	GetLatestRiskScore(participantID string) (*types.RiskScore, error)

	SaveAlert(alert *types.MonitoringAlert) error
	GetAlert(alertID string) (*types.MonitoringAlert, error)
	ListAlerts(statusFilter types.AlertStatus, participantID string, limit int) ([]*types.MonitoringAlert, error)

	SaveSARFiling(sar *types.SARFiling) error
}

// InMemoryStore is an in-memory store for development and testing.
type InMemoryStore struct {
	mu         sync.RWMutex
	screenings map[string]*types.ScreeningResult
	matches    map[string]*types.ScreeningMatch
	riskScores map[string]*types.RiskScore  // participantID -> latest
	allScores  []*types.RiskScore
	alerts     map[string]*types.MonitoringAlert
	sarFilings map[string]*types.SARFiling
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		screenings: make(map[string]*types.ScreeningResult),
		matches:    make(map[string]*types.ScreeningMatch),
		riskScores: make(map[string]*types.RiskScore),
		alerts:     make(map[string]*types.MonitoringAlert),
		sarFilings: make(map[string]*types.SARFiling),
	}
}

func (s *InMemoryStore) SaveScreeningResult(result *types.ScreeningResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.screenings[result.ScreeningID] = result
	return nil
}

func (s *InMemoryStore) GetScreeningResult(screeningID string) (*types.ScreeningResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.screenings[screeningID]
	if !ok {
		return nil, fmt.Errorf("screening result %s not found", screeningID)
	}
	return r, nil
}

func (s *InMemoryStore) GetLatestScreening(participantID string) (*types.ScreeningResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *types.ScreeningResult
	for _, r := range s.screenings {
		if r.ParticipantID == participantID {
			if latest == nil || r.ScreenedAt.After(latest.ScreenedAt) {
				latest = r
			}
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no screening found for participant %s", participantID)
	}
	return latest, nil
}

func (s *InMemoryStore) SaveMatch(match *types.ScreeningMatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.matches[match.MatchID] = match
	return nil
}

func (s *InMemoryStore) GetMatch(matchID string) (*types.ScreeningMatch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.matches[matchID]
	if !ok {
		return nil, fmt.Errorf("match %s not found", matchID)
	}
	return m, nil
}

func (s *InMemoryStore) SaveRiskScore(score *types.RiskScore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.riskScores[score.ParticipantID] = score
	s.allScores = append(s.allScores, score)
	return nil
}

func (s *InMemoryStore) GetLatestRiskScore(participantID string) (*types.RiskScore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	score, ok := s.riskScores[participantID]
	if !ok {
		return nil, fmt.Errorf("no risk score found for participant %s", participantID)
	}
	return score, nil
}

func (s *InMemoryStore) SaveAlert(alert *types.MonitoringAlert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts[alert.AlertID] = alert
	return nil
}

func (s *InMemoryStore) GetAlert(alertID string) (*types.MonitoringAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.alerts[alertID]
	if !ok {
		return nil, fmt.Errorf("alert %s not found", alertID)
	}
	return a, nil
}

func (s *InMemoryStore) ListAlerts(statusFilter types.AlertStatus, participantID string, limit int) ([]*types.MonitoringAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var result []*types.MonitoringAlert
	for _, a := range s.alerts {
		if statusFilter != "" && a.Status != statusFilter {
			continue
		}
		if participantID != "" && a.ParticipantID != participantID {
			continue
		}
		result = append(result, a)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *InMemoryStore) SaveSARFiling(sar *types.SARFiling) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sarFilings[sar.SARID] = sar
	return nil
}

// GetSARFiling retrieves a SAR filing by ID (used for testing).
func (s *InMemoryStore) GetSARFiling(sarID string) (*types.SARFiling, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.sarFilings[sarID]
	if !ok {
		return nil, fmt.Errorf("SAR filing %s not found", sarID)
	}
	return f, nil
}

// GetLatestAlert returns the most recent alert (used for testing).
func (s *InMemoryStore) GetLatestAlert() *types.MonitoringAlert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *types.MonitoringAlert
	for _, a := range s.alerts {
		if latest == nil || a.CreatedAt.After(latest.CreatedAt) {
			latest = a
		}
	}
	return latest
}

// GetLatestSAR returns the most recent SAR filing (used for testing).
func (s *InMemoryStore) GetLatestSAR() *types.SARFiling {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *types.SARFiling
	for _, f := range s.sarFilings {
		if latest == nil || f.FiledAt.After(latest.FiledAt) {
			latest = f
		}
	}
	return latest
}

// GetScreeningsByParticipant returns all screenings for a participant (for testing/last screened).
func (s *InMemoryStore) GetScreeningsByParticipant(participantID string) []*types.ScreeningResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*types.ScreeningResult
	for _, r := range s.screenings {
		if r.ParticipantID == participantID {
			result = append(result, r)
		}
	}
	return result
}

// TimeSince is a helper to check staleness (exported for testing).
func TimeSince(t time.Time) time.Duration {
	return time.Since(t)
}
