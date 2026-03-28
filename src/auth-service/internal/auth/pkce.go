package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

var (
	ErrInvalidCodeVerifier  = errors.New("invalid code verifier: must be 43-128 characters")
	ErrInvalidChallengeMethod = errors.New("invalid challenge method: only S256 is supported")
	ErrCodeChallengeMismatch = errors.New("code challenge does not match verifier")
)

// GenerateCodeChallenge produces an S256 code challenge from a code verifier.
// verifier must be 43-128 characters per RFC 7636.
func GenerateCodeChallenge(verifier string) (string, error) {
	if len(verifier) < 43 || len(verifier) > 128 {
		return "", ErrInvalidCodeVerifier
	}
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:]), nil
}

// ValidateCodeVerifier checks that the verifier matches the stored challenge (S256 only).
func ValidateCodeVerifier(verifier, storedChallenge, method string) error {
	if method != "S256" {
		return ErrInvalidChallengeMethod
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		return ErrInvalidCodeVerifier
	}
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	if computed != storedChallenge {
		return ErrCodeChallengeMismatch
	}
	return nil
}
