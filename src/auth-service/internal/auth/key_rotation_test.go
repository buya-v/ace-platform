package auth

import (
	"sync"
	"testing"

	"github.com/garudax-platform/auth-service/internal/types"
)

func TestKeySetHS256BasicOperations(t *testing.T) {
	svc := NewJWTService("initial-key-long-enough-for-hmac", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	// Generate and validate with current key
	token, err := ks.GenerateAccessToken(user, "jti-1")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := ks.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Sub != "user-1" {
		t.Errorf("Sub = %q, want user-1", claims.Sub)
	}

	if ks.CurrentAlgorithm() != AlgHS256 {
		t.Errorf("CurrentAlgorithm = %q, want HS256", ks.CurrentAlgorithm())
	}

	if ks.HasPreviousKey() {
		t.Error("should not have a previous key initially")
	}
}

func TestKeySetHS256Rotation(t *testing.T) {
	svc := NewJWTService("key-one-long-enough-for-hmac-256", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	// Generate token with old key
	oldToken, err := ks.GenerateAccessToken(user, "jti-old")
	if err != nil {
		t.Fatalf("GenerateAccessToken (old): %v", err)
	}

	// Rotate to new key
	ks.RotateHS256("key-two-long-enough-for-hmac-256", 900, 86400)

	if !ks.HasPreviousKey() {
		t.Error("should have a previous key after rotation")
	}

	// Old token should still validate (via previous key fallback)
	claims, err := ks.ValidateToken(oldToken)
	if err != nil {
		t.Fatalf("ValidateToken (old token after rotation): %v", err)
	}
	if claims.Sub != "user-1" {
		t.Errorf("Sub = %q, want user-1", claims.Sub)
	}

	// New token should validate
	newToken, err := ks.GenerateAccessToken(user, "jti-new")
	if err != nil {
		t.Fatalf("GenerateAccessToken (new): %v", err)
	}

	claims, err = ks.ValidateToken(newToken)
	if err != nil {
		t.Fatalf("ValidateToken (new token): %v", err)
	}
	if claims.Jti != "jti-new" {
		t.Errorf("Jti = %q, want jti-new", claims.Jti)
	}
}

func TestKeySetHS256DoubleRotation(t *testing.T) {
	svc := NewJWTService("key-alpha-long-enough-hmac-256", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	// Generate token with first key
	token1, _ := ks.GenerateAccessToken(user, "jti-1")

	// First rotation
	ks.RotateHS256("key-bravo-long-enough-hmac-256", 900, 86400)
	token2, _ := ks.GenerateAccessToken(user, "jti-2")

	// Second rotation - token1 should no longer validate
	// because only current and previous are kept
	ks.RotateHS256("key-charlie-long-enough-hmac256", 900, 86400)

	// token2 should still validate (now the "previous" key)
	_, err := ks.ValidateToken(token2)
	if err != nil {
		t.Errorf("token2 should validate after second rotation: %v", err)
	}

	// token1 should NOT validate (two rotations ago)
	_, err = ks.ValidateToken(token1)
	if err == nil {
		t.Error("token1 should NOT validate after two rotations")
	}
}

func TestKeySetRS256Rotation(t *testing.T) {
	priv1, pub1, _ := GenerateRSAKeyPair()
	priv2, pub2, _ := GenerateRSAKeyPair()

	svc := NewJWTServiceRS256(priv1, pub1, "key-1", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	// Generate with first key
	token1, err := ks.GenerateAccessToken(user, "jti-1")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Rotate to second RSA key
	ks.RotateRS256(priv2, pub2, "key-2", 900, 86400)

	// Old token should still validate
	claims, err := ks.ValidateToken(token1)
	if err != nil {
		t.Fatalf("old RS256 token should validate: %v", err)
	}
	if claims.Sub != "user-1" {
		t.Errorf("Sub = %q, want user-1", claims.Sub)
	}

	// New token with new key
	token2, err := ks.GenerateAccessToken(user, "jti-2")
	if err != nil {
		t.Fatalf("GenerateAccessToken (new key): %v", err)
	}

	claims, err = ks.ValidateToken(token2)
	if err != nil {
		t.Fatalf("new RS256 token should validate: %v", err)
	}
	if claims.Jti != "jti-2" {
		t.Errorf("Jti = %q, want jti-2", claims.Jti)
	}
}

func TestKeySetClearPreviousKey(t *testing.T) {
	svc := NewJWTService("key-one-long-enough-for-hmac-256", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}
	oldToken, _ := ks.GenerateAccessToken(user, "jti-old")

	// Rotate
	ks.RotateHS256("key-two-long-enough-for-hmac-256", 900, 86400)

	// Old token validates
	_, err := ks.ValidateToken(oldToken)
	if err != nil {
		t.Fatalf("should validate before clear: %v", err)
	}

	// Clear previous key
	ks.ClearPreviousKey()

	if ks.HasPreviousKey() {
		t.Error("should not have previous key after clear")
	}

	// Old token should no longer validate
	_, err = ks.ValidateToken(oldToken)
	if err == nil {
		t.Error("old token should NOT validate after clearing previous key")
	}
}

func TestKeySetRefreshToken(t *testing.T) {
	svc := NewJWTService("key-for-refresh-test-hmac-256", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}
	token, err := ks.GenerateRefreshToken(user, "session-1")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := ks.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != "refresh" {
		t.Errorf("Type = %q, want refresh", claims.Type)
	}
}

func TestKeySetConcurrentAccess(t *testing.T) {
	// Test that concurrent reads don't panic or corrupt data.
	// Note: tokens generated during rotation may fail validation if
	// the key changes between sign and verify, which is expected behavior.
	svc := NewJWTService("concurrent-test-key-hmac-256", 900, 86400)
	ks := NewKeySet(svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	var wg sync.WaitGroup
	panics := make(chan string, 100)

	// Concurrent token generation (no validation — rotation can make tokens invalid)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- "generate panic"
				}
			}()
			_, _ = ks.GenerateAccessToken(user, "jti")
		}()
	}

	// Concurrent rotation
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- "rotate panic"
				}
			}()
			ks.RotateHS256("rotated-key-concurrent-test-"+string(rune('0'+n)), 900, 86400)
		}(i)
	}

	// Concurrent validation (may return errors, but must not panic)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics <- "validate panic"
				}
			}()
			token, _ := ks.GenerateAccessToken(user, "jti")
			_, _ = ks.ValidateToken(token)
		}()
	}

	wg.Wait()
	close(panics)

	for p := range panics {
		t.Errorf("unexpected panic during concurrent access: %s", p)
	}

	// After rotations settle, generate and validate should work
	token, err := ks.GenerateAccessToken(user, "post-rotation")
	if err != nil {
		t.Fatalf("post-rotation GenerateAccessToken: %v", err)
	}
	claims, err := ks.ValidateToken(token)
	if err != nil {
		t.Fatalf("post-rotation ValidateToken: %v", err)
	}
	if claims.Jti != "post-rotation" {
		t.Errorf("Jti = %q, want post-rotation", claims.Jti)
	}
}

func TestKeySetMixedAlgorithmRotation(t *testing.T) {
	// Start with HS256
	hs256Svc := NewJWTService("hs256-key-long-enough-for-hmac", 900, 86400)
	ks := NewKeySet(hs256Svc)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleTrader}

	hsToken, _ := ks.GenerateAccessToken(user, "hs-jti")

	// Rotate to RS256
	privKey, pubKey, _ := GenerateRSAKeyPair()
	ks.RotateRS256(privKey, pubKey, "rsa-key-1", 900, 86400)

	if ks.CurrentAlgorithm() != AlgRS256 {
		t.Errorf("CurrentAlgorithm after rotation = %q, want RS256", ks.CurrentAlgorithm())
	}

	// HS256 token should still validate via previous key
	claims, err := ks.ValidateToken(hsToken)
	if err != nil {
		t.Fatalf("HS256 token should validate after RS256 rotation: %v", err)
	}
	if claims.Sub != "user-1" {
		t.Errorf("Sub = %q, want user-1", claims.Sub)
	}

	// New RS256 token
	rsaToken, err := ks.GenerateAccessToken(user, "rsa-jti")
	if err != nil {
		t.Fatalf("GenerateAccessToken RS256: %v", err)
	}
	claims, err = ks.ValidateToken(rsaToken)
	if err != nil {
		t.Fatalf("RS256 token should validate: %v", err)
	}
	if claims.Jti != "rsa-jti" {
		t.Errorf("Jti = %q, want rsa-jti", claims.Jti)
	}
}
