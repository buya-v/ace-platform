package membership

import (
	"fmt"
	"time"
)

// IDGenerator is a function that produces unique IDs.
type IDGenerator func() string

// Service provides membership lifecycle operations.
type Service struct {
	store Store
	genID IDGenerator
	now   func() time.Time
}

// NewService creates a MembershipService with the given store and ID generator.
func NewService(store Store, genID IDGenerator) *Service {
	return &Service{
		store: store,
		genID: genID,
		now:   time.Now,
	}
}

// withNow overrides the clock for testing.
func (s *Service) withNow(fn func() time.Time) {
	s.now = fn
}

// CreateMember registers a new participant member in PENDING status.
func (s *Service) CreateMember(userID, legalName string, entityType EntityType, tier Tier) (*Member, error) {
	if userID == "" {
		return nil, ErrMissingUserID
	}
	if legalName == "" {
		return nil, ErrMissingLegalName
	}
	if !IsValidTier(tier) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTier, tier)
	}

	now := s.now()
	m := &Member{
		ID:         s.genID(),
		UserID:     userID,
		LegalName:  legalName,
		EntityType: entityType,
		Tier:       tier,
		Status:     StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionCreated,
		NewValue:  string(tier),
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// Activate transitions a PENDING member to ACTIVE status (on KYC approval).
func (s *Service) Activate(memberID, actorID string) (*Member, error) {
	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}

	if m.Status != StatusPending {
		return nil, ErrNotPending
	}

	now := s.now()
	oldStatus := m.Status
	m.Status = StatusActive
	m.OnboardedAt = &now
	m.UpdatedAt = now

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionActivated,
		OldValue:  string(oldStatus),
		NewValue:  string(StatusActive),
		ActorID:   actorID,
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// ChangeTier updates a member's tier and records the change in history.
// Only ACTIVE members can have their tier changed.
func (s *Service) ChangeTier(memberID string, newTier Tier, reason, actorID string) (*Member, error) {
	if !IsValidTier(newTier) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTier, newTier)
	}

	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}

	if m.Status == StatusTerminated {
		return nil, ErrAlreadyTerminated
	}
	if m.Status != StatusActive {
		return nil, ErrNotActive
	}
	if m.Tier == newTier {
		return nil, ErrSameTier
	}

	now := s.now()
	oldTier := m.Tier
	m.Tier = newTier
	m.UpdatedAt = now

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionTierChanged,
		OldValue:  string(oldTier),
		NewValue:  string(newTier),
		Reason:    reason,
		ActorID:   actorID,
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// Suspend transitions an ACTIVE member to SUSPENDED status.
func (s *Service) Suspend(memberID, reason, actorID string) (*Member, error) {
	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}

	if m.Status == StatusTerminated {
		return nil, ErrAlreadyTerminated
	}
	if m.Status != StatusActive {
		return nil, ErrNotActive
	}

	now := s.now()
	m.Status = StatusSuspended
	m.UpdatedAt = now

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionSuspended,
		OldValue:  string(StatusActive),
		NewValue:  string(StatusSuspended),
		Reason:    reason,
		ActorID:   actorID,
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// Reinstate transitions a SUSPENDED member back to ACTIVE status.
func (s *Service) Reinstate(memberID, actorID string) (*Member, error) {
	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}

	if m.Status != StatusSuspended {
		return nil, ErrNotSuspended
	}

	now := s.now()
	m.Status = StatusActive
	m.UpdatedAt = now

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionReinstated,
		OldValue:  string(StatusSuspended),
		NewValue:  string(StatusActive),
		ActorID:   actorID,
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// Terminate transitions a member to TERMINATED status.
// Can be called from ACTIVE or PENDING status.
func (s *Service) Terminate(memberID, reason, actorID string) (*Member, error) {
	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}

	if m.Status == StatusTerminated {
		return nil, ErrAlreadyTerminated
	}
	if m.Status == StatusSuspended {
		return nil, fmt.Errorf("%w: must reinstate before terminating", ErrInvalidTransition)
	}

	now := s.now()
	oldStatus := m.Status
	m.Status = StatusTerminated
	m.UpdatedAt = now

	if err := s.store.SaveMember(m); err != nil {
		return nil, fmt.Errorf("saving member: %w", err)
	}

	entry := &HistoryEntry{
		ID:        s.genID(),
		MemberID:  m.ID,
		Action:    ActionTerminated,
		OldValue:  string(oldStatus),
		NewValue:  string(StatusTerminated),
		Reason:    reason,
		ActorID:   actorID,
		CreatedAt: now,
	}
	if err := s.store.SaveHistory(entry); err != nil {
		return nil, fmt.Errorf("saving history: %w", err)
	}

	return m, nil
}

// GetMember retrieves a member by ID.
func (s *Service) GetMember(memberID string) (*Member, error) {
	return s.store.GetMember(memberID)
}

// ListMembers returns members matching the given filter.
func (s *Service) ListMembers(filter ListFilter) ([]*Member, error) {
	return s.store.ListMembers(filter)
}

// GetMemberHistory returns the history entries for a given member.
func (s *Service) GetMemberHistory(memberID string) ([]*HistoryEntry, error) {
	// Verify member exists
	if _, err := s.store.GetMember(memberID); err != nil {
		return nil, err
	}
	return s.store.GetHistory(memberID)
}

// GetTierConfig returns the operational configuration for a member's tier.
func (s *Service) GetTierConfig(memberID string) (*TierConfig, error) {
	m, err := s.store.GetMember(memberID)
	if err != nil {
		return nil, err
	}
	configs := DefaultTierConfigs()
	cfg, ok := configs[m.Tier]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidTier, m.Tier)
	}
	return &cfg, nil
}
