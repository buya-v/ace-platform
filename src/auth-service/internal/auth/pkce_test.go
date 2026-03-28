package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateCodeChallenge(t *testing.T) {
	// RFC 7636 Appendix B test vector (43+ chars)
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge, err := GenerateCodeChallenge(verifier)
	if err != nil {
		t.Fatalf("GenerateCodeChallenge: %v", err)
	}

	// Compute expected
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != expected {
		t.Errorf("challenge = %q, want %q", challenge, expected)
	}
}

func TestGenerateCodeChallengeTooShort(t *testing.T) {
	_, err := GenerateCodeChallenge("short")
	if err != ErrInvalidCodeVerifier {
		t.Errorf("expected ErrInvalidCodeVerifier, got %v", err)
	}
}

func TestGenerateCodeChallengeTooLong(t *testing.T) {
	verifier := strings.Repeat("a", 129)
	_, err := GenerateCodeChallenge(verifier)
	if err != ErrInvalidCodeVerifier {
		t.Errorf("expected ErrInvalidCodeVerifier, got %v", err)
	}
}

func TestValidateCodeVerifierSuccess(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, _ := GenerateCodeChallenge(verifier)

	err := ValidateCodeVerifier(verifier, challenge, "S256")
	if err != nil {
		t.Errorf("ValidateCodeVerifier: %v", err)
	}
}

func TestValidateCodeVerifierWrongVerifier(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge, _ := GenerateCodeChallenge(verifier)

	wrong := "wrong-verifier-that-is-43-characters-long!!"
	err := ValidateCodeVerifier(wrong, challenge, "S256")
	if err != ErrCodeChallengeMismatch {
		t.Errorf("expected ErrCodeChallengeMismatch, got %v", err)
	}
}

func TestValidateCodeVerifierPlainRejected(t *testing.T) {
	err := ValidateCodeVerifier("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", "challenge", "plain")
	if err != ErrInvalidChallengeMethod {
		t.Errorf("expected ErrInvalidChallengeMethod, got %v", err)
	}
}

func TestValidateCodeVerifierBoundaryLengths(t *testing.T) {
	// Exactly 43 chars
	v43 := strings.Repeat("a", 43)
	challenge, err := GenerateCodeChallenge(v43)
	if err != nil {
		t.Fatalf("43-char verifier should be valid: %v", err)
	}
	if err := ValidateCodeVerifier(v43, challenge, "S256"); err != nil {
		t.Errorf("43-char validation failed: %v", err)
	}

	// Exactly 128 chars
	v128 := strings.Repeat("b", 128)
	challenge, err = GenerateCodeChallenge(v128)
	if err != nil {
		t.Fatalf("128-char verifier should be valid: %v", err)
	}
	if err := ValidateCodeVerifier(v128, challenge, "S256"); err != nil {
		t.Errorf("128-char validation failed: %v", err)
	}
}
