package auth

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrMissingToken    = errors.New("missing authorization token")
	ErrMalformedToken  = errors.New("malformed token")
	ErrInvalidToken    = errors.New("invalid token signature")
	ErrExpiredToken    = errors.New("token has expired")
	ErrInvalidIssuer   = errors.New("invalid token issuer")
	ErrInvalidAudience = errors.New("invalid token audience")
	ErrUnsupportedAlg  = errors.New("unsupported signing algorithm")
)

// Claims represents JWT claims extracted from the token.
type Claims struct {
	Sub           string   `json:"sub"`
	ParticipantID string   `json:"participant_id"`
	Roles         []string `json:"roles"`
	Issuer        string   `json:"iss"`
	Audience      string   `json:"aud"`
	ExpiresAt     int64    `json:"exp"`
	IssuedAt      int64    `json:"iat"`
	JTI           string   `json:"jti"`
}

// HasRole checks if the claims include a specific role.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the claims include any of the given roles.
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if c.HasRole(role) {
			return true
		}
	}
	return false
}

// JWTValidator validates JWT tokens. Supports both HS256 and RS256.
type JWTValidator struct {
	secret       []byte
	rsaPublicKey *rsa.PublicKey
	issuer       string
	audience     string
	nowFunc      func() time.Time
}

// NewJWTValidator creates a new JWT validator using HS256.
func NewJWTValidator(secret, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
		nowFunc:  time.Now,
	}
}

// NewJWTValidatorRS256 creates a new JWT validator using RS256 with the given public key.
func NewJWTValidatorRS256(pubKey *rsa.PublicKey, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		rsaPublicKey: pubKey,
		issuer:       issuer,
		audience:     audience,
		nowFunc:      time.Now,
	}
}

// NewJWTValidatorDual creates a validator that accepts both HS256 and RS256 tokens.
func NewJWTValidatorDual(secret string, pubKey *rsa.PublicKey, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		secret:       []byte(secret),
		rsaPublicKey: pubKey,
		issuer:       issuer,
		audience:     audience,
		nowFunc:      time.Now,
	}
}

// LoadRSAPublicKeyFromFile loads an RSA public key from a PEM file.
func LoadRSAPublicKeyFromFile(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}

	return rsaPub, nil
}

// SetNowFunc sets a custom time function (for testing).
func (v *JWTValidator) SetNowFunc(fn func() time.Time) {
	v.nowFunc = fn
}

// SetRSAPublicKey sets or updates the RSA public key for RS256 validation.
func (v *JWTValidator) SetRSAPublicKey(pubKey *rsa.PublicKey) {
	v.rsaPublicKey = pubKey
}

// ValidateToken validates a JWT token string and returns the claims.
// Supports both HS256 and RS256 based on the token header.
func (v *JWTValidator) ValidateToken(tokenStr string) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	// Decode header to determine algorithm
	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrMalformedToken
	}

	signingInput := parts[0] + "." + parts[1]
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrMalformedToken
	}

	// Verify signature based on algorithm declared in header
	switch header.Alg {
	case "HS256":
		if len(v.secret) == 0 {
			return nil, ErrUnsupportedAlg
		}
		expectedSig, _ := v.computeSignature(signingInput)
		if !hmac.Equal(expectedSig, actualSig) {
			return nil, ErrInvalidToken
		}
	case "RS256":
		if v.rsaPublicKey == nil {
			return nil, ErrUnsupportedAlg
		}
		hashed := sha256.Sum256([]byte(signingInput))
		if err := rsa.VerifyPKCS1v15(v.rsaPublicKey, crypto.SHA256, hashed[:], actualSig); err != nil {
			return nil, ErrInvalidToken
		}
	default:
		return nil, ErrMalformedToken
	}

	// Decode payload
	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, ErrMalformedToken
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrMalformedToken
	}

	// Validate expiration
	if claims.ExpiresAt > 0 && v.nowFunc().Unix() > claims.ExpiresAt {
		return nil, ErrExpiredToken
	}

	// Validate issuer (skip if not configured or set to "none")
	if v.issuer != "" && v.issuer != "none" && claims.Issuer != v.issuer {
		return nil, ErrInvalidIssuer
	}

	// Validate audience (skip if not configured or set to "none")
	if v.audience != "" && v.audience != "none" && claims.Audience != v.audience {
		return nil, ErrInvalidAudience
	}

	return &claims, nil
}

// CreateToken creates a signed JWT token from claims (for testing).
func (v *JWTValidator) CreateToken(claims *Claims) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
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
	sig, err := v.computeSignature(signingInput)
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64URLEncode(sig), nil
}

func (v *JWTValidator) computeSignature(input string) ([]byte, error) {
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(input))
	return mac.Sum(nil), nil
}

func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
