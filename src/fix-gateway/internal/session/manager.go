package session

import (
	"fmt"
	"sync"
	"time"
)

// SessionState represents the state of a FIX session.
type SessionState int

const (
	Disconnected SessionState = iota
	LogonSent
	Active
	LogoutSent
)

// String returns a human-readable session state.
func (s SessionState) String() string {
	switch s {
	case Disconnected:
		return "DISCONNECTED"
	case LogonSent:
		return "LOGON_SENT"
	case Active:
		return "ACTIVE"
	case LogoutSent:
		return "LOGOUT_SENT"
	default:
		return "UNKNOWN"
	}
}

// Session represents a FIX protocol session between the gateway and a broker.
type Session struct {
	SenderCompID      string       `json:"sender_comp_id"`
	TargetCompID      string       `json:"target_comp_id"`
	TenantID          string       `json:"tenant_id"`
	State             SessionState `json:"state"`
	InSeqNum          uint64       `json:"in_seq_num"`
	OutSeqNum         uint64       `json:"out_seq_num"`
	HeartbeatInterval int          `json:"heartbeat_interval"`
	LastRecvTime      time.Time    `json:"last_recv_time"`
	CreatedAt         time.Time    `json:"created_at"`
}

// SessionKey returns the composite session key: SenderCompID:TargetCompID:TenantID.
func (s *Session) SessionKey() string {
	return fmt.Sprintf("%s:%s:%s", s.SenderCompID, s.TargetCompID, s.TenantID)
}

// MarshalState returns a JSON-friendly state string.
func (s *Session) MarshalState() string {
	return s.State.String()
}

// SessionManager manages FIX sessions.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // keyed by session key
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session and stores it.
func (m *SessionManager) CreateSession(senderCompID, targetCompID, tenantID string, heartbeatInterval int) *Session {
	s := &Session{
		SenderCompID:      senderCompID,
		TargetCompID:      targetCompID,
		TenantID:          tenantID,
		State:             Disconnected,
		InSeqNum:          1,
		OutSeqNum:         1,
		HeartbeatInterval: heartbeatInterval,
		LastRecvTime:      time.Now(),
		CreatedAt:         time.Now(),
	}

	m.mu.Lock()
	m.sessions[s.SessionKey()] = s
	m.mu.Unlock()

	return s
}

// GetSession returns a session by its composite key, or nil if not found.
func (m *SessionManager) GetSession(senderCompID, targetCompID, tenantID string) *Session {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[key]
}

// ProcessLogon transitions a session to Active state.
func (m *SessionManager) ProcessLogon(senderCompID, targetCompID, tenantID string) error {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	s.State = Active
	s.LastRecvTime = time.Now()
	return nil
}

// ProcessLogout transitions a session to Disconnected state.
func (m *SessionManager) ProcessLogout(senderCompID, targetCompID, tenantID string) error {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	s.State = Disconnected
	return nil
}

// IncrementOutSeq increments and returns the outbound sequence number for a session.
func (m *SessionManager) IncrementOutSeq(senderCompID, targetCompID, tenantID string) (uint64, error) {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		return 0, fmt.Errorf("session not found: %s", key)
	}

	seq := s.OutSeqNum
	s.OutSeqNum++
	return seq, nil
}

// ValidateInSeq validates and increments the inbound sequence number.
// Returns nil if the sequence number matches, error otherwise.
func (m *SessionManager) ValidateInSeq(senderCompID, targetCompID, tenantID string, seqNum uint64) error {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[key]
	if !ok {
		return fmt.Errorf("session not found: %s", key)
	}

	if seqNum < s.InSeqNum {
		return fmt.Errorf("sequence number too low: got %d, expected %d", seqNum, s.InSeqNum)
	}
	if seqNum > s.InSeqNum {
		return fmt.Errorf("sequence gap: got %d, expected %d", seqNum, s.InSeqNum)
	}

	s.InSeqNum++
	s.LastRecvTime = time.Now()
	return nil
}

// ListSessions returns all sessions.
func (m *SessionManager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// UpdateLastRecv updates the LastRecvTime for a session to now.
func (m *SessionManager) UpdateLastRecv(senderCompID, targetCompID, tenantID string) {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[key]; ok {
		s.LastRecvTime = time.Now()
	}
}

// RemoveSession removes a session by its composite key.
func (m *SessionManager) RemoveSession(senderCompID, targetCompID, tenantID string) {
	key := fmt.Sprintf("%s:%s:%s", senderCompID, targetCompID, tenantID)
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, key)
}

// SessionCount returns the number of tracked sessions.
func (m *SessionManager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
