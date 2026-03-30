package auth

import (
	"testing"
	"time"

	"github.com/garudax-platform/auth-service/internal/store"
	"github.com/garudax-platform/auth-service/internal/types"
)

func newTestService() *Service {
	repo := store.NewInMemoryStore()
	jwt := NewJWTService("test-signing-key-that-is-long-enough", 900, 86400)
	return NewService(repo, jwt, 4, 3, 30*time.Minute) // cost 4 for fast tests, 3 attempts
}

func TestRegister(t *testing.T) {
	svc := newTestService()

	user, err := svc.Register("alice@example.com", "password123", types.RoleTrader)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", user.Email, "alice@example.com")
	}
	if user.Role != types.RoleTrader {
		t.Errorf("Role = %q, want %q", user.Role, types.RoleTrader)
	}
	if user.HashedPassword == "password123" {
		t.Error("password should be hashed, not stored in plain text")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc := newTestService()

	_, err := svc.Register("dup@example.com", "password123", types.RoleTrader)
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err = svc.Register("dup@example.com", "password456", types.RoleViewer)
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

func TestLoginSuccess(t *testing.T) {
	svc := newTestService()

	_, err := svc.Register("bob@example.com", "correctpassword", types.RoleTrader)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	tokens, err := svc.Login("bob@example.com", "correctpassword")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if tokens.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}
	if tokens.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900", tokens.ExpiresIn)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc := newTestService()

	svc.Register("charlie@example.com", "correctpassword", types.RoleTrader)

	_, err := svc.Login("charlie@example.com", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	svc := newTestService()

	_, err := svc.Login("nobody@example.com", "password")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAccountLockout(t *testing.T) {
	svc := newTestService() // 3 max attempts

	svc.Register("lockme@example.com", "correct", types.RoleTrader)

	for i := 0; i < 3; i++ {
		svc.Login("lockme@example.com", "wrong")
	}

	_, err := svc.Login("lockme@example.com", "correct")
	if err != ErrAccountLocked {
		t.Errorf("expected ErrAccountLocked after 3 failed attempts, got %v", err)
	}
}

func TestLoginResetsFailedAttempts(t *testing.T) {
	svc := newTestService() // 3 max attempts

	svc.Register("reset@example.com", "correct", types.RoleTrader)

	svc.Login("reset@example.com", "wrong")
	svc.Login("reset@example.com", "wrong")

	_, err := svc.Login("reset@example.com", "correct")
	if err != nil {
		t.Fatalf("Login should succeed: %v", err)
	}

	svc.Login("reset@example.com", "wrong")
	svc.Login("reset@example.com", "wrong")

	_, err = svc.Login("reset@example.com", "correct")
	if err != nil {
		t.Errorf("Login should still succeed after reset: %v", err)
	}
}

func TestRefreshSession(t *testing.T) {
	svc := newTestService()

	svc.Register("refresh@example.com", "password", types.RoleTrader)
	tokens, _ := svc.Login("refresh@example.com", "password")

	claims, err := svc.ValidateToken(tokens.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateToken on refresh: %v", err)
	}

	newTokens, err := svc.RefreshSession(claims.Jti, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshSession: %v", err)
	}

	if newTokens.AccessToken == "" {
		t.Error("new AccessToken should not be empty")
	}
	if newTokens.AccessToken == tokens.AccessToken {
		t.Error("new AccessToken should differ from old")
	}
}

func TestRefreshSessionInvalidToken(t *testing.T) {
	svc := newTestService()

	svc.Register("r2@example.com", "password", types.RoleTrader)
	tokens, _ := svc.Login("r2@example.com", "password")

	claims, _ := svc.ValidateToken(tokens.RefreshToken)

	_, err := svc.RefreshSession(claims.Jti, "wrong-token")
	if err != ErrInvalidRefreshToken {
		t.Errorf("expected ErrInvalidRefreshToken, got %v", err)
	}
}

func TestRevokeSession(t *testing.T) {
	svc := newTestService()

	svc.Register("revoke@example.com", "password", types.RoleTrader)
	tokens, _ := svc.Login("revoke@example.com", "password")

	claims, _ := svc.ValidateToken(tokens.RefreshToken)

	err := svc.RevokeSession(claims.Jti)
	if err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	_, err = svc.RefreshSession(claims.Jti, tokens.RefreshToken)
	if err != ErrSessionRevoked {
		t.Errorf("expected ErrSessionRevoked, got %v", err)
	}
}

func TestPKCEFlow(t *testing.T) {
	svc := newTestService()

	user, _ := svc.Register("pkce@example.com", "password", types.RoleTrader)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, _ := GenerateCodeChallenge(verifier)

	authCode, err := svc.AuthorizePKCE(user.ID, challenge, "S256", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("AuthorizePKCE: %v", err)
	}

	tokens, err := svc.ExchangeCode(authCode, verifier)
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}

	if tokens.AccessToken == "" {
		t.Error("expected access token from PKCE flow")
	}
}

func TestPKCERejectsPlain(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("pkce2@example.com", "password", types.RoleTrader)

	_, err := svc.AuthorizePKCE(user.ID, "challenge", "plain", "https://example.com")
	if err != ErrInvalidChallengeMethod {
		t.Errorf("expected ErrInvalidChallengeMethod, got %v", err)
	}
}

func TestPKCECodeReuse(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("pkce3@example.com", "password", types.RoleTrader)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, _ := GenerateCodeChallenge(verifier)

	authCode, _ := svc.AuthorizePKCE(user.ID, challenge, "S256", "https://example.com")

	_, err := svc.ExchangeCode(authCode, verifier)
	if err != nil {
		t.Fatalf("first ExchangeCode: %v", err)
	}

	_, err = svc.ExchangeCode(authCode, verifier)
	if err != ErrPKCECodeUsed {
		t.Errorf("expected ErrPKCECodeUsed, got %v", err)
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	svc := newTestService()
	user, _ := svc.Register("apikey@example.com", "password", types.RoleTrader)

	key, rawKey, err := svc.CreateAPIKey(user.ID, "test-key", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key.Name != "test-key" {
		t.Errorf("Name = %q, want %q", key.Name, "test-key")
	}
	if rawKey == "" {
		t.Error("rawKey should not be empty")
	}
	if key.Prefix != rawKey[:8] {
		t.Errorf("Prefix = %q, want %q", key.Prefix, rawKey[:8])
	}

	validated, err := svc.ValidateAPIKey(rawKey)
	if err != nil {
		t.Fatalf("ValidateAPIKey: %v", err)
	}
	if validated.ID != key.ID {
		t.Errorf("validated key ID = %q, want %q", validated.ID, key.ID)
	}

	keys, err := svc.ListAPIKeys(user.ID)
	if err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("ListAPIKeys count = %d, want 1", len(keys))
	}

	err = svc.RevokeAPIKey(key.ID, user.ID)
	if err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}

	_, err = svc.ValidateAPIKey(rawKey)
	if err != ErrAPIKeyNotFound {
		t.Errorf("expected ErrAPIKeyNotFound after revoke, got %v", err)
	}
}

func TestAPIKeyWrongUser(t *testing.T) {
	svc := newTestService()
	user1, _ := svc.Register("user1@example.com", "password", types.RoleTrader)
	user2, _ := svc.Register("user2@example.com", "password", types.RoleTrader)

	key, _, _ := svc.CreateAPIKey(user1.ID, "key1", time.Time{})

	err := svc.RevokeAPIKey(key.ID, user2.ID)
	if err == nil {
		t.Error("expected error revoking another user's key")
	}
}
