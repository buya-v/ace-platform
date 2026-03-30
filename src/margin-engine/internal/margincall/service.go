package margincall

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// IDGenerator generates unique IDs for margin calls.
type IDGenerator interface {
	NewID() string
}

// CallHandler is invoked when a new margin call is issued.
type CallHandler func(call types.MarginCall)

// Service manages margin call lifecycle: issuance, tracking, and resolution.
type Service struct {
	mu      sync.Mutex
	idGen   IDGenerator
	calls   map[string]*types.MarginCall // callID -> call
	active  map[string]string            // participantID -> active callID
	handler CallHandler
	deadline time.Duration // How long participants have to meet a margin call
}

func NewService(idGen IDGenerator, deadline time.Duration) *Service {
	return &Service{
		idGen:    idGen,
		calls:    make(map[string]*types.MarginCall),
		active:   make(map[string]string),
		deadline: deadline,
	}
}

// SetHandler sets the callback for new margin calls.
func (s *Service) SetHandler(h CallHandler) {
	s.handler = h
}

// Evaluate checks a portfolio margin result and issues/resolves margin calls as needed.
func (s *Service) Evaluate(pm types.PortfolioMargin) (*types.MarginCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pm.ExcessDeficit.IsNeg() {
		// Deficit — need a margin call
		return s.issueOrUpdateCall(pm)
	}

	// No deficit — resolve any active call
	s.resolveCall(pm.ParticipantID)
	return nil, nil
}

func (s *Service) issueOrUpdateCall(pm types.PortfolioMargin) (*types.MarginCall, error) {
	deficit := pm.ExcessDeficit.Negate() // Make positive

	// Check if there's already an active call for this participant
	if activeID, ok := s.active[pm.ParticipantID]; ok {
		existing := s.calls[activeID]
		// Update the deficit amount (margin requirements may have changed)
		existing.Required = pm.TotalRequired
		existing.OnHand = pm.CollateralOnHand
		existing.Deficit = deficit
		return existing, nil
	}

	// Issue new margin call
	now := time.Now()
	call := &types.MarginCall{
		CallID:        s.idGen.NewID(),
		ParticipantID: pm.ParticipantID,
		Required:      pm.TotalRequired,
		OnHand:        pm.CollateralOnHand,
		Deficit:       deficit,
		Deadline:      now.Add(s.deadline),
		Status:        types.MarginCallIssued,
		IssuedAt:      now,
	}

	s.calls[call.CallID] = call
	s.active[pm.ParticipantID] = call.CallID

	if s.handler != nil {
		s.handler(*call)
	}

	return call, nil
}

func (s *Service) resolveCall(participantID string) {
	activeID, ok := s.active[participantID]
	if !ok {
		return
	}

	call := s.calls[activeID]
	call.Status = types.MarginCallSatisfied
	call.ResolvedAt = time.Now()
	delete(s.active, participantID)
}

// CheckDeadlines scans all active calls and marks those past deadline as breached.
func (s *Service) CheckDeadlines(now time.Time) []types.MarginCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	var breached []types.MarginCall
	for participantID, callID := range s.active {
		call := s.calls[callID]
		if now.After(call.Deadline) {
			call.Status = types.MarginCallBreached
			call.ResolvedAt = now
			breached = append(breached, *call)
			delete(s.active, participantID)
		}
	}
	return breached
}

// GetActive returns the active margin call for a participant, if any.
func (s *Service) GetActive(participantID string) (types.MarginCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeID, ok := s.active[participantID]
	if !ok {
		return types.MarginCall{}, false
	}
	return *s.calls[activeID], true
}

// Get returns a margin call by ID.
func (s *Service) Get(callID string) (types.MarginCall, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	call, ok := s.calls[callID]
	if !ok {
		return types.MarginCall{}, false
	}
	return *call, true
}

// GetAllActive returns all currently active margin calls.
func (s *Service) GetAllActive() []types.MarginCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]types.MarginCall, 0, len(s.active))
	for _, callID := range s.active {
		result = append(result, *s.calls[callID])
	}
	return result
}

// Stats returns summary statistics.
func (s *Service) Stats() CallStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := CallStats{
		TotalIssued: len(s.calls),
		Active:      len(s.active),
	}
	for _, call := range s.calls {
		switch call.Status {
		case types.MarginCallSatisfied:
			stats.Satisfied++
		case types.MarginCallBreached:
			stats.Breached++
		}
	}
	return stats
}

// CallStats holds margin call summary statistics.
type CallStats struct {
	TotalIssued int
	Active      int
	Satisfied   int
	Breached    int
}

func (s CallStats) String() string {
	return fmt.Sprintf("total=%d active=%d satisfied=%d breached=%d",
		s.TotalIssued, s.Active, s.Satisfied, s.Breached)
}
