package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrMissingToken   = errors.New("missing authorization token")
	ErrMalformedToken = errors.New("malformed token")
	ErrInvalidToken   = errors.New("invalid token signature")
	ErrExpiredToken   = errors.New("token has expired")
	ErrInvalidIssuer  = errors.New("invalid token issuer")
	ErrInvalidAudience = errors.New("invalid token audience")
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

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	secret   []byte
	issuer   string
	audience string
	nowFunc  func() time.Time
}

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(secret, issuer, audience string) *JWTValidator {
	return &JWTValidator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
		nowFunc:  time.Now,
	}
}

// SetNowFunc sets a custom time function (for testing).
func (v *JWTValidator) SetNowFunc(fn func() time.Time) {
	v.nowFunc = fn
}

// ValidateToken validates a JWT token string and returns the claims.
func (v *JWTValidator) ValidateToken(tokenStr string) (*Claims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	// Verify signature (HMAC-SHA256)
	signingInput := parts[0] + "." + parts[1]
	expectedSig, err := v.computeSignature(signingInput)
	if err != nil {
		return nil, ErrMalformedToken
	}

	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrMalformedToken
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidToken
	}

	// Decode header to verify algorithm
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
	if header.Alg != "HS256" {
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
