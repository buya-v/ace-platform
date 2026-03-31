package membership

// Store defines the persistence interface for membership data.
// Implementations can use PostgreSQL, in-memory maps, etc.
type Store interface {
	// SaveMember persists a new or updated member.
	SaveMember(m *Member) error
	// GetMember retrieves a member by ID.
	GetMember(id string) (*Member, error)
	// ListMembers returns members matching the given filter.
	ListMembers(filter ListFilter) ([]*Member, error)
	// SaveHistory persists a membership history entry.
	SaveHistory(h *HistoryEntry) error
	// GetHistory returns history entries for a given member.
	GetHistory(memberID string) ([]*HistoryEntry, error)
}

// MemoryStore is an in-memory implementation of Store for testing.
type MemoryStore struct {
	members map[string]*Member
	history []*HistoryEntry
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		members: make(map[string]*Member),
		history: make([]*HistoryEntry, 0),
	}
}

func (s *MemoryStore) SaveMember(m *Member) error {
	// Deep copy to avoid mutation
	cp := *m
	s.members[m.ID] = &cp
	return nil
}

func (s *MemoryStore) GetMember(id string) (*Member, error) {
	m, ok := s.members[id]
	if !ok {
		return nil, ErrMemberNotFound
	}
	cp := *m
	return &cp, nil
}

func (s *MemoryStore) ListMembers(filter ListFilter) ([]*Member, error) {
	var result []*Member
	for _, m := range s.members {
		if filter.Status != nil && m.Status != *filter.Status {
			continue
		}
		if filter.Tier != nil && m.Tier != *filter.Tier {
			continue
		}
		if filter.EntityType != nil && m.EntityType != *filter.EntityType {
			continue
		}
		cp := *m
		result = append(result, &cp)
	}
	return result, nil
}

func (s *MemoryStore) SaveHistory(h *HistoryEntry) error {
	cp := *h
	s.history = append(s.history, &cp)
	return nil
}

func (s *MemoryStore) GetHistory(memberID string) ([]*HistoryEntry, error) {
	var result []*HistoryEntry
	for _, h := range s.history {
		if h.MemberID == memberID {
			cp := *h
			result = append(result, &cp)
		}
	}
	return result, nil
}
