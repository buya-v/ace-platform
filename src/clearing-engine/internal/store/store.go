package store

import (
	"sync"

	"github.com/garudax-platform/clearing-engine/internal/types"
)

// ObligationStore provides append-only storage for clearing obligations.
// In production, this would be backed by PostgreSQL.
type ObligationStore interface {
	Append(obl types.ClearingObligation) error
	ByTrade(tradeID string) []types.ClearingObligation
	ByParticipant(participantID string) []types.ClearingObligation
	ByInstrument(instrumentID string) []types.ClearingObligation
	ByStatus(status types.ClearingStatus) []types.ClearingObligation
	All() []types.ClearingObligation
}

// InMemoryObligationStore is an in-memory implementation for development/testing.
type InMemoryObligationStore struct {
	mu          sync.RWMutex
	obligations []types.ClearingObligation
	byTrade     map[string][]int // tradeID -> indices
	byParticipant map[string][]int
	byInstrument  map[string][]int
}

func NewInMemoryObligationStore() *InMemoryObligationStore {
	return &InMemoryObligationStore{
		byTrade:       make(map[string][]int),
		byParticipant: make(map[string][]int),
		byInstrument:  make(map[string][]int),
	}
}

func (s *InMemoryObligationStore) Append(obl types.ClearingObligation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := len(s.obligations)
	s.obligations = append(s.obligations, obl)
	s.byTrade[obl.TradeID] = append(s.byTrade[obl.TradeID], idx)
	s.byParticipant[obl.ParticipantID] = append(s.byParticipant[obl.ParticipantID], idx)
	s.byInstrument[obl.InstrumentID] = append(s.byInstrument[obl.InstrumentID], idx)
	return nil
}

func (s *InMemoryObligationStore) ByTrade(tradeID string) []types.ClearingObligation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gather(s.byTrade[tradeID])
}

func (s *InMemoryObligationStore) ByParticipant(participantID string) []types.ClearingObligation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gather(s.byParticipant[participantID])
}

func (s *InMemoryObligationStore) ByInstrument(instrumentID string) []types.ClearingObligation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gather(s.byInstrument[instrumentID])
}

func (s *InMemoryObligationStore) ByStatus(status types.ClearingStatus) []types.ClearingObligation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []types.ClearingObligation
	for _, obl := range s.obligations {
		if obl.Status == status {
			result = append(result, obl)
		}
	}
	return result
}

func (s *InMemoryObligationStore) All() []types.ClearingObligation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]types.ClearingObligation, len(s.obligations))
	copy(out, s.obligations)
	return out
}

func (s *InMemoryObligationStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.obligations)
}

func (s *InMemoryObligationStore) gather(indices []int) []types.ClearingObligation {
	result := make([]types.ClearingObligation, 0, len(indices))
	for _, idx := range indices {
		result = append(result, s.obligations[idx])
	}
	return result
}
