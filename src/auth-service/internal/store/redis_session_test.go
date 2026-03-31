package store

import (
	"testing"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
	"github.com/redis/go-redis/v9"
)

// testRedisStore creates a RedisSessionStore backed by an in-memory
// InMemoryStore and a real redis.Client.  Tests that call this will be
// skipped if REDIS_TEST_URL is not set.
func testRedisStore(t *testing.T) *RedisSessionStore {
	t.Helper()
	// Use a real Redis for integration tests.  In CI or without Redis,
	// these tests are skipped — the unit-level InMemoryStore tests still
	// cover all session logic.
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // use DB 15 for tests to avoid collisions
	})
	// Attempt a ping; skip if Redis is not available.
	if err := rdb.Ping(t.Context()).Err(); err != nil {
		rdb.Close()
		t.Skip("Redis not available, skipping Redis session store tests")
	}
	// Flush test DB before each test.
	rdb.FlushDB(t.Context())

	inner := NewInMemoryStore()
	store := NewRedisSessionStoreFromClient(inner, rdb, 10*time.Minute)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestRedisCreateAndGetSession(t *testing.T) {
	rs := testRedisStore(t)

	session := &types.Session{
		ID:               "sess-001",
		UserID:           "user-001",
		RefreshTokenHash: "hash-abc",
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
	}

	if err := rs.CreateSession(session); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := rs.GetSessionByID("sess-001")
	if err != nil {
		t.Fatalf("GetSessionByID: %v", err)
	}
	if got.UserID != "user-001" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user-001")
	}
	if got.RefreshTokenHash != "hash-abc" {
		t.Errorf("RefreshTokenHash = %q, want %q", got.RefreshTokenHash, "hash-abc")
	}
}

func TestRedisGetSessionNotFound(t *testing.T) {
	rs := testRedisStore(t)

	_, err := rs.GetSessionByID("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRedisRevokeSession(t *testing.T) {
	rs := testRedisStore(t)

	session := &types.Session{
		ID:        "sess-002",
		UserID:    "user-002",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	rs.CreateSession(session)

	if err := rs.RevokeSession("sess-002"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	_, err := rs.GetSessionByID("sess-002")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after revoke, got %v", err)
	}
}

func TestRedisRevokeSessionNotFound(t *testing.T) {
	rs := testRedisStore(t)

	err := rs.RevokeSession("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRedisRevokeUserSessions(t *testing.T) {
	rs := testRedisStore(t)

	for i, id := range []string{"sess-a", "sess-b", "sess-c"} {
		_ = i
		rs.CreateSession(&types.Session{
			ID:        id,
			UserID:    "user-multi",
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		})
	}

	if err := rs.RevokeUserSessions("user-multi"); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}

	for _, id := range []string{"sess-a", "sess-b", "sess-c"} {
		_, err := rs.GetSessionByID(id)
		if err != ErrNotFound {
			t.Errorf("session %s: expected ErrNotFound after RevokeUserSessions, got %v", id, err)
		}
	}
}

func TestRedisRevokeUserSessionsEmpty(t *testing.T) {
	rs := testRedisStore(t)

	// Should not error when no sessions exist for user.
	if err := rs.RevokeUserSessions("no-sessions-user"); err != nil {
		t.Fatalf("RevokeUserSessions (empty): %v", err)
	}
}

func TestRedisDelegatesUserMethods(t *testing.T) {
	rs := testRedisStore(t)

	user := &types.User{
		ID:             "u1",
		Email:          "test@example.com",
		HashedPassword: "hashed",
		Role:           types.RoleTrader,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := rs.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := rs.GetUserByID("u1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "test@example.com")
	}

	got2, err := rs.GetUserByEmail("test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got2.ID != "u1" {
		t.Errorf("ID = %q, want %q", got2.ID, "u1")
	}

	user.Role = types.RoleAdmin
	if err := rs.UpdateUser(user); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	users := rs.ListUsers()
	if len(users) != 1 {
		t.Errorf("ListUsers returned %d users, want 1", len(users))
	}
}

func TestRedisDelegatesPKCE(t *testing.T) {
	rs := testRedisStore(t)

	challenge := &types.PKCEChallenge{
		AuthCode:            "code-1",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		UserID:              "u1",
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if err := rs.StorePKCEChallenge(challenge); err != nil {
		t.Fatalf("StorePKCEChallenge: %v", err)
	}

	got, err := rs.GetPKCEChallenge("code-1")
	if err != nil {
		t.Fatalf("GetPKCEChallenge: %v", err)
	}
	if got.CodeChallenge != "challenge" {
		t.Errorf("CodeChallenge = %q, want %q", got.CodeChallenge, "challenge")
	}

	if err := rs.MarkPKCEUsed("code-1"); err != nil {
		t.Fatalf("MarkPKCEUsed: %v", err)
	}
}

func TestRedisDelegatesAPIKey(t *testing.T) {
	rs := testRedisStore(t)

	key := &types.APIKey{
		ID:        "k1",
		UserID:    "u1",
		Name:      "test-key",
		KeyHash:   "hash123",
		Prefix:    "gxk_",
		CreatedAt: time.Now(),
	}

	if err := rs.CreateAPIKey(key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := rs.GetAPIKeyByHash("hash123")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if got.Name != "test-key" {
		t.Errorf("Name = %q, want %q", got.Name, "test-key")
	}

	keys, err := rs.ListAPIKeysByUser("u1")
	if err != nil {
		t.Fatalf("ListAPIKeysByUser: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("ListAPIKeysByUser returned %d keys, want 1", len(keys))
	}

	if err := rs.RevokeAPIKey("k1", "u1"); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
}

func TestParseRedisURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"redis://localhost:6379/0", "localhost:6379"},
		{"redis://myhost:6380/2", "myhost:6380"},
		{"rediss://secure:6379/1", "secure:6379"},
		{"redis://host", "host"}, // no path separator, prefix stripped
	}

	for _, tt := range tests {
		got := ParseRedisURL(tt.url)
		if got != tt.want {
			t.Errorf("ParseRedisURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
