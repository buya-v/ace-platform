package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
	"github.com/redis/go-redis/v9"
)

const (
	sessionKeyPrefix   = "session:"
	userSessionsPrefix = "user_sessions:"
	defaultSessionTTL  = 24 * time.Hour
)

// sessionStore is the subset of InMemoryStore / PostgresStore methods that
// RedisSessionStore delegates to for non-session operations.  This avoids
// importing the auth package (which would cause a cycle).
type sessionStore interface {
	CreateUser(user *types.User) error
	GetUserByID(id string) (*types.User, error)
	GetUserByEmail(email string) (*types.User, error)
	UpdateUser(user *types.User) error

	CreateSession(session *types.Session) error
	GetSessionByID(id string) (*types.Session, error)
	RevokeSession(id string) error
	RevokeUserSessions(userID string) error

	CreateAPIKey(key *types.APIKey) error
	GetAPIKeyByHash(keyHash string) (*types.APIKey, error)
	ListAPIKeysByUser(userID string) ([]*types.APIKey, error)
	RevokeAPIKey(id, userID string) error

	StorePKCEChallenge(challenge *types.PKCEChallenge) error
	GetPKCEChallenge(authCode string) (*types.PKCEChallenge, error)
	MarkPKCEUsed(authCode string) error

	ListUsers() []*types.User

	GetTenantRoles(userID string) ([]types.TenantUserRole, error)
	AssignTenantRole(assignment *types.TenantUserRole) error
}

// RedisSessionStore wraps an underlying store and layers Redis-backed
// session storage on top.  All non-session methods delegate to the inner
// store.  Session operations use Redis SETEX with TTL matching the
// refresh-token expiry.
type RedisSessionStore struct {
	inner      sessionStore
	rdb        *redis.Client
	sessionTTL time.Duration
}

// NewRedisSessionStore creates a Redis-backed session store that delegates
// non-session operations to inner.  redisURL should be "redis://host:port/db".
func NewRedisSessionStore(inner sessionStore, redisURL string, sessionTTL time.Duration) (*RedisSessionStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}

	return &RedisSessionStore{
		inner:      inner,
		rdb:        rdb,
		sessionTTL: sessionTTL,
	}, nil
}

// NewRedisSessionStoreFromClient creates a RedisSessionStore from an
// existing redis.Client (useful for testing).
func NewRedisSessionStoreFromClient(inner sessionStore, rdb *redis.Client, sessionTTL time.Duration) *RedisSessionStore {
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}
	return &RedisSessionStore{
		inner:      inner,
		rdb:        rdb,
		sessionTTL: sessionTTL,
	}
}

// --- Session methods (Redis-backed) ---

func (s *RedisSessionStore) CreateSession(session *types.Session) error {
	ctx := context.Background()

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	key := sessionKeyPrefix + session.ID

	// Compute TTL from session expiry, fall back to configured default.
	ttl := s.sessionTTL
	if !session.ExpiresAt.IsZero() {
		remaining := time.Until(session.ExpiresAt)
		if remaining > 0 {
			ttl = remaining
		}
	}

	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, key, data, ttl)
	// Maintain a per-user set of session IDs for RevokeUserSessions.
	pipe.SAdd(ctx, userSessionsPrefix+session.UserID, session.ID)
	pipe.Expire(ctx, userSessionsPrefix+session.UserID, ttl)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis create session: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) GetSessionByID(id string) (*types.Session, error) {
	ctx := context.Background()
	data, err := s.rdb.Get(ctx, sessionKeyPrefix+id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("redis get session: %w", err)
	}

	var sess types.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &sess, nil
}

func (s *RedisSessionStore) RevokeSession(id string) error {
	ctx := context.Background()

	// Read session first to get user ID for set cleanup.
	data, err := s.rdb.Get(ctx, sessionKeyPrefix+id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return ErrNotFound
		}
		return fmt.Errorf("redis get session for revoke: %w", err)
	}

	var sess types.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return fmt.Errorf("unmarshal session for revoke: %w", err)
	}

	pipe := s.rdb.Pipeline()
	pipe.Del(ctx, sessionKeyPrefix+id)
	pipe.SRem(ctx, userSessionsPrefix+sess.UserID, id)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis revoke session: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) RevokeUserSessions(userID string) error {
	ctx := context.Background()

	sessionIDs, err := s.rdb.SMembers(ctx, userSessionsPrefix+userID).Result()
	if err != nil {
		return fmt.Errorf("redis get user sessions: %w", err)
	}

	if len(sessionIDs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(sessionIDs)+1)
	for _, sid := range sessionIDs {
		keys = append(keys, sessionKeyPrefix+sid)
	}
	keys = append(keys, userSessionsPrefix+userID)

	if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis revoke user sessions: %w", err)
	}

	return nil
}

// --- Delegated methods (pass through to inner store) ---

func (s *RedisSessionStore) CreateUser(user *types.User) error {
	return s.inner.CreateUser(user)
}

func (s *RedisSessionStore) GetUserByID(id string) (*types.User, error) {
	return s.inner.GetUserByID(id)
}

func (s *RedisSessionStore) GetUserByEmail(email string) (*types.User, error) {
	return s.inner.GetUserByEmail(email)
}

func (s *RedisSessionStore) UpdateUser(user *types.User) error {
	return s.inner.UpdateUser(user)
}

func (s *RedisSessionStore) CreateAPIKey(key *types.APIKey) error {
	return s.inner.CreateAPIKey(key)
}

func (s *RedisSessionStore) GetAPIKeyByHash(keyHash string) (*types.APIKey, error) {
	return s.inner.GetAPIKeyByHash(keyHash)
}

func (s *RedisSessionStore) ListAPIKeysByUser(userID string) ([]*types.APIKey, error) {
	return s.inner.ListAPIKeysByUser(userID)
}

func (s *RedisSessionStore) RevokeAPIKey(id, userID string) error {
	return s.inner.RevokeAPIKey(id, userID)
}

func (s *RedisSessionStore) StorePKCEChallenge(challenge *types.PKCEChallenge) error {
	return s.inner.StorePKCEChallenge(challenge)
}

func (s *RedisSessionStore) GetPKCEChallenge(authCode string) (*types.PKCEChallenge, error) {
	return s.inner.GetPKCEChallenge(authCode)
}

func (s *RedisSessionStore) MarkPKCEUsed(authCode string) error {
	return s.inner.MarkPKCEUsed(authCode)
}

func (s *RedisSessionStore) ListUsers() []*types.User {
	return s.inner.ListUsers()
}

func (s *RedisSessionStore) GetTenantRoles(userID string) ([]types.TenantUserRole, error) {
	return s.inner.GetTenantRoles(userID)
}

func (s *RedisSessionStore) AssignTenantRole(assignment *types.TenantUserRole) error {
	return s.inner.AssignTenantRole(assignment)
}

// Close closes the Redis connection.
func (s *RedisSessionStore) Close() error {
	return s.rdb.Close()
}

// ConnectRedis creates a redis.Client from a URL string and verifies
// connectivity.
func ConnectRedis(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
}

// ParseRedisURL extracts host info from a Redis URL for logging.
func ParseRedisURL(url string) string {
	s := strings.TrimPrefix(url, "redis://")
	s = strings.TrimPrefix(s, "rediss://")
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	return s
}
