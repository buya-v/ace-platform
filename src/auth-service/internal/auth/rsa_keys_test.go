package auth

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/garudax-platform/auth-service/internal/types"
)

func TestGenerateRSAKeyPair(t *testing.T) {
	privKey, pubKey, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}
	if privKey == nil {
		t.Fatal("private key is nil")
	}
	if pubKey == nil {
		t.Fatal("public key is nil")
	}
	if privKey.N.BitLen() < 2048 {
		t.Errorf("expected key size >= 2048 bits, got %d", privKey.N.BitLen())
	}
}

func TestWriteAndLoadRSAKeyPair(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	privKey, _, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}

	if err := WriteRSAKeyPairPEM(privKey, privPath, pubPath); err != nil {
		t.Fatalf("WriteRSAKeyPairPEM: %v", err)
	}

	// Verify file permissions on private key
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("private key permissions = %o, want 0600", perm)
	}

	// Load and verify
	loadedPriv, loadedPub, err := LoadRSAKeyPair(privPath, pubPath)
	if err != nil {
		t.Fatalf("LoadRSAKeyPair: %v", err)
	}

	if loadedPriv.D.Cmp(privKey.D) != 0 {
		t.Error("loaded private key does not match original")
	}
	if loadedPub.N.Cmp(privKey.PublicKey.N) != 0 {
		t.Error("loaded public key does not match original")
	}
}

func TestLoadRSAPrivateKeyOnly(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	privKey, _, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}
	if err := WriteRSAKeyPairPEM(privKey, privPath, pubPath); err != nil {
		t.Fatalf("WriteRSAKeyPairPEM: %v", err)
	}

	loadedPriv, loadedPub, err := LoadRSAPrivateKeyOnly(privPath)
	if err != nil {
		t.Fatalf("LoadRSAPrivateKeyOnly: %v", err)
	}
	if loadedPriv == nil || loadedPub == nil {
		t.Fatal("loaded keys should not be nil")
	}
	if loadedPub.N.Cmp(loadedPriv.PublicKey.N) != 0 {
		t.Error("derived public key should match private key's public component")
	}
}

func TestLoadRSAKeyPair_PKCS8(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "private_pkcs8.pem")
	pubPath := filepath.Join(dir, "public.pem")

	privKey, _, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}

	// Write private key in PKCS#8 format
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		t.Fatalf("write PKCS8 key: %v", err)
	}

	// Write public key normally
	if err := WriteRSAKeyPairPEM(privKey, filepath.Join(dir, "unused.pem"), pubPath); err != nil {
		t.Fatalf("WriteRSAKeyPairPEM: %v", err)
	}

	loaded, pub, err := LoadRSAKeyPair(privPath, pubPath)
	if err != nil {
		t.Fatalf("LoadRSAKeyPair with PKCS8: %v", err)
	}
	if loaded.D.Cmp(privKey.D) != 0 {
		t.Error("PKCS8 loaded key does not match")
	}
	if pub == nil {
		t.Error("public key should not be nil")
	}
}

func TestLoadRSAKeyPair_Errors(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing private key file", func(t *testing.T) {
		_, _, err := LoadRSAKeyPair("/nonexistent/path", filepath.Join(dir, "pub.pem"))
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("missing public key file", func(t *testing.T) {
		privPath := filepath.Join(dir, "priv_for_err.pem")
		privKey, _, _ := GenerateRSAKeyPair()
		_ = WriteRSAKeyPairPEM(privKey, privPath, filepath.Join(dir, "pub_for_err.pem"))

		_, _, err := LoadRSAKeyPair(privPath, "/nonexistent/pub.pem")
		if err == nil {
			t.Error("expected error for missing public key file")
		}
	})

	t.Run("invalid PEM content", func(t *testing.T) {
		badFile := filepath.Join(dir, "bad.pem")
		os.WriteFile(badFile, []byte("not a PEM file"), 0600)

		_, _, err := LoadRSAKeyPair(badFile, badFile)
		if err == nil {
			t.Error("expected error for invalid PEM")
		}
	})

	t.Run("wrong PEM block type", func(t *testing.T) {
		badFile := filepath.Join(dir, "wrong_type.pem")
		block := &pem.Block{Type: "CERTIFICATE", Bytes: []byte("fake")}
		os.WriteFile(badFile, pem.EncodeToMemory(block), 0600)

		_, _, err := LoadRSAKeyPair(badFile, badFile)
		if err == nil {
			t.Error("expected error for wrong PEM block type")
		}
	})
}

func TestRS256SignAndValidate(t *testing.T) {
	privKey, pubKey, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}

	svc := NewJWTServiceRS256(privKey, pubKey, "test-key-1", 900, 86400)

	if svc.Algorithm() != AlgRS256 {
		t.Errorf("Algorithm = %q, want RS256", svc.Algorithm())
	}

	user := &types.User{
		ID:    "user-rs256",
		Email: "rs256@example.com",
		Role:  types.RoleTrader,
	}

	token, err := svc.GenerateAccessToken(user, "jti-rs256")
	if err != nil {
		t.Fatalf("GenerateAccessToken RS256: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken RS256: %v", err)
	}

	if claims.Sub != "user-rs256" {
		t.Errorf("Sub = %q, want %q", claims.Sub, "user-rs256")
	}
	if claims.Email != "rs256@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "rs256@example.com")
	}
	if len(claims.Permissions) == 0 {
		t.Error("expected permissions in access token")
	}
}

func TestRS256RefreshToken(t *testing.T) {
	privKey, pubKey, _ := GenerateRSAKeyPair()
	svc := NewJWTServiceRS256(privKey, pubKey, "key-1", 900, 86400)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}
	token, err := svc.GenerateRefreshToken(user, "session-1")
	if err != nil {
		t.Fatalf("GenerateRefreshToken RS256: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Type != "refresh" {
		t.Errorf("Type = %q, want refresh", claims.Type)
	}
}

func TestRS256WrongKey(t *testing.T) {
	priv1, _, _ := GenerateRSAKeyPair()
	_, pub2, _ := GenerateRSAKeyPair()

	signer := NewJWTServiceRS256(priv1, &priv1.PublicKey, "key-1", 900, 86400)
	validator := NewJWTServiceRS256(priv1, pub2, "key-2", 900, 86400)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}
	token, _ := signer.GenerateAccessToken(user, "jti")

	_, err := validator.ValidateToken(token)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestHS256CannotValidateRS256(t *testing.T) {
	privKey, _, _ := GenerateRSAKeyPair()
	rs256Svc := NewJWTServiceRS256(privKey, &privKey.PublicKey, "key-1", 900, 86400)

	hs256Svc := NewJWTService("test-secret-key-256bit-long-enough", 900, 86400)

	user := &types.User{ID: "user-1", Email: "a@b.com", Role: types.RoleViewer}
	token, _ := rs256Svc.GenerateAccessToken(user, "jti")

	_, err := hs256Svc.ValidateToken(token)
	if err != ErrUnsupportedAlg {
		t.Errorf("expected ErrUnsupportedAlg, got %v", err)
	}
}

func TestRS256WithLoadedKeys(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "priv.pem")
	pubPath := filepath.Join(dir, "pub.pem")

	origPriv, _, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}
	if err := WriteRSAKeyPairPEM(origPriv, privPath, pubPath); err != nil {
		t.Fatalf("WriteRSAKeyPairPEM: %v", err)
	}

	loadedPriv, loadedPub, err := LoadRSAKeyPair(privPath, pubPath)
	if err != nil {
		t.Fatalf("LoadRSAKeyPair: %v", err)
	}

	svc := NewJWTServiceRS256(loadedPriv, loadedPub, "loaded-key", 900, 86400)

	user := &types.User{ID: "loaded-user", Email: "loaded@test.com", Role: types.RoleAdmin}
	token, err := svc.GenerateAccessToken(user, "jti-loaded")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.Sub != "loaded-user" {
		t.Errorf("Sub = %q, want loaded-user", claims.Sub)
	}
}
