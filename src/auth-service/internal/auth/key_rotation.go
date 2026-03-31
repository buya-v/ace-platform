package auth

import (
	"crypto/rsa"
	"sync"

	"github.com/garudax-platform/auth-service/internal/types"
)

// KeySet holds the current and previous signing keys to support
// zero-downtime key rotation. Tokens signed with the previous key
// remain valid until they expire naturally.
type KeySet struct {
	mu       sync.RWMutex
	current  *JWTService
	previous *JWTService
}

// NewKeySet creates a KeySet with the given JWTService as the current key.
func NewKeySet(current *JWTService) *KeySet {
	return &KeySet{
		current: current,
	}
}

// RotateHS256 rotates to a new HS256 signing key.
// The current key becomes the previous key (used for validation only).
// New tokens are signed with the new key.
func (ks *KeySet) RotateHS256(newSigningKey string, accessTTLSecs, refreshTTLSecs int) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.previous = ks.current
	ks.current = NewJWTService(newSigningKey, accessTTLSecs, refreshTTLSecs)
}

// RotateRS256 rotates to a new RS256 key pair.
// The current key becomes the previous key (used for validation only).
// New tokens are signed with the new private key.
func (ks *KeySet) RotateRS256(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, keyID string, accessTTLSecs, refreshTTLSecs int) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.previous = ks.current
	ks.current = NewJWTServiceRS256(privKey, pubKey, keyID, accessTTLSecs, refreshTTLSecs)
}

// GenerateAccessToken generates an access token using the current key.
func (ks *KeySet) GenerateAccessToken(user *types.User, jti string) (string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.current.GenerateAccessToken(user, jti)
}

// GenerateRefreshToken generates a refresh token using the current key.
func (ks *KeySet) GenerateRefreshToken(user *types.User, jti string) (string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.current.GenerateRefreshToken(user, jti)
}

// ValidateToken tries validation with the current key first,
// then falls back to the previous key. This allows tokens signed
// with the old key to remain valid during the rotation window.
func (ks *KeySet) ValidateToken(tokenStr string) (*JWTClaims, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	// Try current key first
	claims, err := ks.current.ValidateToken(tokenStr)
	if err == nil {
		return claims, nil
	}

	// Fall back to previous key if available
	if ks.previous != nil {
		prevClaims, prevErr := ks.previous.ValidateToken(tokenStr)
		if prevErr == nil {
			return prevClaims, nil
		}
	}

	// Return the original error from the current key
	return nil, err
}

// CurrentAlgorithm returns the algorithm of the current signing key.
func (ks *KeySet) CurrentAlgorithm() SigningAlgorithm {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.current.Algorithm()
}

// HasPreviousKey returns true if a previous key is still held for validation.
func (ks *KeySet) HasPreviousKey() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.previous != nil
}

// ClearPreviousKey removes the previous key, meaning tokens signed
// with it will no longer validate. Call this after the rotation
// window has passed (i.e., all old tokens have expired).
func (ks *KeySet) ClearPreviousKey() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.previous = nil
}
