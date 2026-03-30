package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
)

func TestJWTGenerateAndValidateAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-256bit-long-enough", 900, 86400)

	user := &types.User{
		ID:    "user-123",
		Email: "test@example.com",
		Role:  types.RoleTrader,
	}

	token, err := svc.GenerateAccessToken(user, "jti-abc")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.Sub != "user-123" {
		t.Errorf("Sub = %q, want %q", claims.Sub, "user-123")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if claims.Role != types.RoleTrader {
		t.Errorf("Role = %q, want %q", claims.Role, types.RoleTrader)
	}
	if claims.Type != "access" {
		t.Errorf("Type = %q, want %q", claims.Type, "access")
	}
	if claims.Jti != "jti-abc" {
		t.Errorf("Jti = %q, want %q", claims.Jti, "jti-abc")
	}
	if len(claims.Permissions) == 0 {
		t.Error("expected permissions in access token")
	}
}

func TestJWTGenerateAndValidateRefreshToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-256bit-long-enough", 900, 86400)

	user := &types.User{
		ID:    "user-456",
		Email: "refresh@example.com",
		Role:  types.RoleAdmin,
	}

	token, err := svc.GenerateRefreshToken(user, "session-xyz")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.Sub != "user-456" {
		t.Errorf("Sub = %q, want %q", claims.Sub, "user-456")
	}
	if claims.Type != "refresh" {
		t.Errorf("Type = %q, want %q", claims.Type, "refresh")
	}
}

func TestJWTInvalidSignature(t *testing.T) {
	svc1 := NewJWTService("key-one-is-long-enough-for-hmac", 900, 86400)
	svc2 := NewJWTService("key-two-is-long-enough-for-hmac", 900, 86400)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}
	token, _ := svc1.GenerateAccessToken(user, "jti")

	_, err := svc2.ValidateToken(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTExpiredToken(t *testing.T) {
	svc := NewJWTService("test-key-long-enough-for-hmac256", -1, -1)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}
	token, _ := svc.GenerateAccessToken(user, "jti")

	_, err := svc.ValidateToken(token)
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWTMalformedToken(t *testing.T) {
	svc := NewJWTService("test-key-long-enough-for-hmac256", 900, 86400)

	tests := []string{
		"",
		"a.b",
		"a.b.c.d",
		"not-a-jwt",
		"a.!!!invalid-base64.c",
	}

	for _, tok := range tests {
		_, err := svc.ValidateToken(tok)
		if err == nil {
			t.Errorf("expected error for token %q, got nil", tok)
		}
	}
}

func TestJWTTokenExpiry(t *testing.T) {
	svc := NewJWTService("test-key-long-enough-for-hmac256", 300, 86400)
	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}

	token, _ := svc.GenerateAccessToken(user, "jti")
	claims, _ := svc.ValidateToken(token)

	expectedExp := time.Now().Add(300 * time.Second).Unix()
	if abs(claims.Exp-expectedExp) > 2 {
		t.Errorf("Exp = %d, expected ~%d", claims.Exp, expectedExp)
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
