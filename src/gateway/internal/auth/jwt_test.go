package auth

import (
	"testing"
	"time"
)

func TestJWTValidation(t *testing.T) {
	v := NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")
	v.SetNowFunc(func() time.Time {
		return time.Unix(1700000000, 0)
	})

	claims := &Claims{
		Sub:           "user-123",
		ParticipantID: "part-456",
		Roles:         []string{"trader"},
		Issuer:        "ace-auth-service",
		Audience:      "ace-api-gateway",
		ExpiresAt:     1700003600,
		IssuedAt:      1700000000,
		JTI:           "jti-789",
	}

	token, err := v.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := v.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if got.Sub != "user-123" {
		t.Errorf("Sub = %q, want %q", got.Sub, "user-123")
	}
	if got.ParticipantID != "part-456" {
		t.Errorf("ParticipantID = %q, want %q", got.ParticipantID, "part-456")
	}
	if !got.HasRole("trader") {
		t.Error("expected HasRole(trader) = true")
	}
	if got.HasRole("admin") {
		t.Error("expected HasRole(admin) = false")
	}
}

func TestJWTExpired(t *testing.T) {
	v := NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")
	v.SetNowFunc(func() time.Time {
		return time.Unix(1700010000, 0) // After expiration
	})

	claims := &Claims{
		Sub:       "user-123",
		Issuer:    "ace-auth-service",
		Audience:  "ace-api-gateway",
		ExpiresAt: 1700003600,
		IssuedAt:  1700000000,
	}

	token, err := v.CreateToken(claims)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	_, err = v.ValidateToken(token)
	if err != ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got %v", err)
	}
}

func TestJWTInvalidSignature(t *testing.T) {
	v1 := NewJWTValidator("secret-1", "ace-auth-service", "ace-api-gateway")
	v2 := NewJWTValidator("secret-2", "ace-auth-service", "ace-api-gateway")

	claims := &Claims{
		Sub:      "user-123",
		Issuer:   "ace-auth-service",
		Audience: "ace-api-gateway",
	}

	token, _ := v1.CreateToken(claims)
	_, err := v2.ValidateToken(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTInvalidIssuer(t *testing.T) {
	v := NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")

	claims := &Claims{
		Sub:      "user-123",
		Issuer:   "wrong-issuer",
		Audience: "ace-api-gateway",
	}

	token, _ := v.CreateToken(claims)
	_, err := v.ValidateToken(token)
	if err != ErrInvalidIssuer {
		t.Errorf("expected ErrInvalidIssuer, got %v", err)
	}
}

func TestJWTInvalidAudience(t *testing.T) {
	v := NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")

	claims := &Claims{
		Sub:      "user-123",
		Issuer:   "ace-auth-service",
		Audience: "wrong-audience",
	}

	token, _ := v.CreateToken(claims)
	_, err := v.ValidateToken(token)
	if err != ErrInvalidAudience {
		t.Errorf("expected ErrInvalidAudience, got %v", err)
	}
}

func TestJWTMalformedToken(t *testing.T) {
	v := NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one part", "abc"},
		{"two parts", "abc.def"},
		{"four parts", "a.b.c.d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateToken(tt.token)
			if err != ErrMalformedToken {
				t.Errorf("expected ErrMalformedToken, got %v", err)
			}
		})
	}
}

func TestClaimsHasAnyRole(t *testing.T) {
	c := &Claims{Roles: []string{"trader", "compliance_viewer"}}

	if !c.HasAnyRole("trader", "exchange_admin") {
		t.Error("expected HasAnyRole to return true for trader")
	}
	if c.HasAnyRole("exchange_admin", "clearing_admin") {
		t.Error("expected HasAnyRole to return false")
	}
}
