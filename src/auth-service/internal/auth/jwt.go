package auth

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrUnsupportedAlg   = errors.New("unsupported signing algorithm")
)

// SigningAlgorithm represents the JWT signing algorithm.
type SigningAlgorithm string

const (
	AlgHS256 SigningAlgorithm = "HS256"
	AlgRS256 SigningAlgorithm = "RS256"
)

type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

type JWTClaims struct {
	Sub         string             `json:"sub"`
	Email       string             `json:"email"`
	Role        types.Role         `json:"role"`
	Permissions []types.Permission `json:"permissions,omitempty"`
	Iat         int64              `json:"iat"`
	Exp         int64              `json:"exp"`
	Jti         string             `json:"jti,omitempty"`
	Type        string             `json:"type"`
}

// JWTService handles JWT token generation and validation.
// Supports both HS256 (symmetric) and RS256 (asymmetric) signing.
type JWTService struct {
	// HS256 key (used when algorithm is HS256)
	signingKey []byte

	// RS256 keys (used when algorithm is RS256)
	rsaPrivateKey *rsa.PrivateKey
	rsaPublicKey  *rsa.PublicKey

	algorithm  SigningAlgorithm
	accessTTL  time.Duration
	refreshTTL time.Duration
	keyID      string // optional key ID for RS256 rotation
}

// NewJWTService creates a JWTService using HS256 with the given symmetric key.
func NewJWTService(signingKey string, accessTTLSecs, refreshTTLSecs int) *JWTService {
	return &JWTService{
		signingKey: []byte(signingKey),
		algorithm:  AlgHS256,
		accessTTL:  time.Duration(accessTTLSecs) * time.Second,
		refreshTTL: time.Duration(refreshTTLSecs) * time.Second,
	}
}

// NewJWTServiceRS256 creates a JWTService using RS256 with the given RSA key pair.
func NewJWTServiceRS256(privKey *rsa.PrivateKey, pubKey *rsa.PublicKey, keyID string, accessTTLSecs, refreshTTLSecs int) *JWTService {
	return &JWTService{
		rsaPrivateKey: privKey,
		rsaPublicKey:  pubKey,
		algorithm:     AlgRS256,
		keyID:         keyID,
		accessTTL:     time.Duration(accessTTLSecs) * time.Second,
		refreshTTL:    time.Duration(refreshTTLSecs) * time.Second,
	}
}

// Algorithm returns the current signing algorithm.
func (j *JWTService) Algorithm() SigningAlgorithm {
	return j.algorithm
}

func (j *JWTService) GenerateAccessToken(user *types.User, jti string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Sub:         user.ID,
		Email:       user.Email,
		Role:        user.Role,
		Permissions: types.RolePermissions[user.Role],
		Iat:         now.Unix(),
		Exp:         now.Add(j.accessTTL).Unix(),
		Jti:         jti,
		Type:        "access",
	}
	return j.sign(claims)
}

func (j *JWTService) GenerateRefreshToken(user *types.User, jti string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Sub:  user.ID,
		Iat:  now.Unix(),
		Exp:  now.Add(j.refreshTTL).Unix(),
		Jti:  jti,
		Type: "refresh",
	}
	return j.sign(claims)
}

// ValidateToken validates a JWT and returns claims.
// Supports both HS256 and RS256 tokens.
func (j *JWTService) ValidateToken(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Decode header to determine algorithm
	headerJSON, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	gotSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Validate signature based on algorithm in the header
	switch SigningAlgorithm(header.Alg) {
	case AlgHS256:
		if len(j.signingKey) == 0 {
			return nil, ErrUnsupportedAlg
		}
		expectedSig := j.computeHMAC([]byte(signingInput))
		if !hmac.Equal(expectedSig, gotSig) {
			return nil, ErrInvalidToken
		}
	case AlgRS256:
		pubKey := j.rsaPublicKey
		if pubKey == nil {
			return nil, ErrUnsupportedAlg
		}
		hashed := sha256.Sum256([]byte(signingInput))
		if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], gotSig); err != nil {
			return nil, ErrInvalidToken
		}
	default:
		return nil, ErrUnsupportedAlg
	}

	claimsJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

func (j *JWTService) sign(claims JWTClaims) (string, error) {
	header := JWTHeader{Alg: string(j.algorithm), Typ: "JWT"}
	if j.keyID != "" {
		header.Kid = j.keyID
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerB64 := base64URLEncode(headerJSON)
	claimsB64 := base64URLEncode(claimsJSON)

	signingInput := headerB64 + "." + claimsB64

	var signatureBytes []byte
	switch j.algorithm {
	case AlgHS256:
		signatureBytes = j.computeHMAC([]byte(signingInput))
	case AlgRS256:
		hashed := sha256.Sum256([]byte(signingInput))
		sig, err := rsa.SignPKCS1v15(rand.Reader, j.rsaPrivateKey, crypto.SHA256, hashed[:])
		if err != nil {
			return "", err
		}
		signatureBytes = sig
	default:
		return "", ErrUnsupportedAlg
	}

	signatureB64 := base64URLEncode(signatureBytes)
	return signingInput + "." + signatureB64, nil
}

func (j *JWTService) computeHMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, j.signingKey)
	mac.Write(data)
	return mac.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
