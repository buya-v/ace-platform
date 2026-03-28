package store

import (
	"errors"
	"sync"
	"time"

	"github.com/ace-platform/auth-service/internal/types"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// InMemoryStore implements the auth.Store interface for testing and development.
type InMemoryStore struct {
	mu       sync.RWMutex
	users    map[string]*types.User
	emails   map[string]string // email -> userID
	sessions map[string]*types.Session
	apiKeys  map[string]*types.APIKey
	keyHash  map[string]string // hash -> keyID
	pkce     map[string]*types.PKCEChallenge
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		users:    make(map[string]*types.User),
		emails:   make(map[string]string),
		sessions: make(map[string]*types.Session),
		apiKeys:  make(map[string]*types.APIKey),
		keyHash:  make(map[string]string),
		pkce:     make(map[string]*types.PKCEChallenge),
	}
}

func (s *InMemoryStore) CreateUser(user *types.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.emails[user.Email]; exists {
		return ErrAlreadyExists
	}
	s.users[user.ID] = user
	s.emails[user.Email] = user.ID
	return nil
}

func (s *InMemoryStore) GetUserByID(id string) (*types.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *InMemoryStore) GetUserByEmail(email string) (*types.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uid, ok := s.emails[email]
	if !ok {
		return nil, ErrNotFound
	}
	return s.users[uid], nil
}

func (s *InMemoryStore) UpdateUser(user *types.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[user.ID]; !ok {
		return ErrNotFound
	}
	s.users[user.ID] = user
	return nil
}

func (s *InMemoryStore) CreateSession(session *types.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *InMemoryStore) GetSessionByID(id string) (*types.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return sess, nil
}

func (s *InMemoryStore) RevokeSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return ErrNotFound
	}
	sess.Revoked = true
	return nil
}

func (s *InMemoryStore) RevokeUserSessions(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.UserID == userID {
			sess.Revoked = true
		}
	}
	return nil
}

func (s *InMemoryStore) CreateAPIKey(key *types.APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiKeys[key.ID] = key
	s.keyHash[key.KeyHash] = key.ID
	return nil
}

func (s *InMemoryStore) GetAPIKeyByHash(hash string) (*types.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keyID, ok := s.keyHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	key := s.apiKeys[keyID]
	if key.Revoked || (!key.ExpiresAt.IsZero() && time.Now().After(key.ExpiresAt)) {
		return nil, ErrNotFound
	}
	return key, nil
}

func (s *InMemoryStore) ListAPIKeysByUser(userID string) ([]*types.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var keys []*types.APIKey
	for _, k := range s.apiKeys {
		if k.UserID == userID && !k.Revoked {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s *InMemoryStore) RevokeAPIKey(id, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.apiKeys[id]
	if !ok {
		return ErrNotFound
	}
	if key.UserID != userID {
		return ErrNotFound
	}
	key.Revoked = true
	return nil
}

func (s *InMemoryStore) StorePKCEChallenge(challenge *types.PKCEChallenge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pkce[challenge.AuthCode] = challenge
	return nil
}

func (s *InMemoryStore) GetPKCEChallenge(authCode string) (*types.PKCEChallenge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.pkce[authCode]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (s *InMemoryStore) MarkPKCEUsed(authCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.pkce[authCode]
	if !ok {
		return ErrNotFound
	}
	c.Used = true
	return nil
}
