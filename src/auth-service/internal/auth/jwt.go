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
	ErrInvalidToken   = errors.New("invalid token")
	ErrTokenExpired   = errors.New("token expired")
	ErrUnsupportedAlg = errors.New("unsupported signing algorithm")
)

// Issuer is the canonical `iss` claim for every token GarudaX auth mints
// (platform-architecture.md §5.2).
const Issuer = "garudax-auth"

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

// TokenGrants carries the multi-tenant authorization data embedded into an
// access token. It is assembled by the Service from platform.tenant_user_roles.
type TokenGrants struct {
	// TenantRoles maps a tenant_id to the roles the user holds in that tenant.
	// An empty/absent entry means no access to that tenant.
	TenantRoles map[string][]types.Role
	// PlatformRoles are cross-tenant roles (e.g. platform-admin). Empty for
	// ordinary users.
	PlatformRoles []types.PlatformRole
	// ActiveTenant is the tenant the user selected at login. Informational only —
	// the authoritative tenant for a request is the X-GarudaX-Tenant header.
	ActiveTenant string
}

// JWTClaims is the GarudaX access/refresh token payload. Multi-tenant claims
// follow platform-architecture.md §5.2.
type JWTClaims struct {
	Sub  string `json:"sub"`
	Iss  string `json:"iss,omitempty"`
	Name string `json:"name,omitempty"`

	Email string `json:"email,omitempty"`

	// Multi-tenant authorization claims.
	TenantRoles   map[string][]types.Role `json:"tenant_roles,omitempty"`
	PlatformRoles []types.PlatformRole    `json:"platform_roles,omitempty"`
	ActiveTenant  string                  `json:"active_tenant,omitempty"`

	// Permissions are the resolved permissions for the ActiveTenant's roles —
	// a convenience for downstream services so they need not re-derive them.
	Permissions []types.Permission `json:"permissions,omitempty"`

	// Role is the user's primary role in the active tenant. Retained for
	// backward compatibility with single-tenant consumers.
	Role types.Role `json:"role,omitempty"`

	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
	Jti  string `json:"jti,omitempty"`
	Type string `json:"type"`
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

// GenerateAccessToken mints a single-tenant access token. The user's top-level
// role is scoped to the DefaultTenant. Retained for backward compatibility;
// multi-tenant callers should use GenerateAccessTokenWithGrants.
func (j *JWTService) GenerateAccessToken(user *types.User, jti string) (string, error) {
	grants := TokenGrants{
		TenantRoles:  map[string][]types.Role{types.DefaultTenant: {user.Role}},
		ActiveTenant: types.DefaultTenant,
	}
	return j.GenerateAccessTokenWithGrants(user, jti, grants)
}

// GenerateAccessTokenWithGrants mints an access token carrying the user's full
// per-tenant role map, platform roles, and active tenant (platform-architecture.md §5.2).
func (j *JWTService) GenerateAccessTokenWithGrants(user *types.User, jti string, grants TokenGrants) (string, error) {
	now := time.Now()

	activeRoles := grants.TenantRoles[grants.ActiveTenant]
	claims := JWTClaims{
		Sub:           user.ID,
		Iss:           Issuer,
		Name:          user.Name,
		Email:         user.Email,
		TenantRoles:   grants.TenantRoles,
		PlatformRoles: grants.PlatformRoles,
		ActiveTenant:  grants.ActiveTenant,
		Permissions:   types.PermissionsForRoles(activeRoles),
		Role:          primaryRole(activeRoles),
		Iat:           now.Unix(),
		Exp:           now.Add(j.accessTTL).Unix(),
		Jti:           jti,
		Type:          "access",
	}
	return j.sign(claims)
}

// primaryRole returns the first role in the active-tenant role set, used to
// populate the backward-compatible single `role` claim.
func primaryRole(roles []types.Role) types.Role {
	if len(roles) == 0 {
		return ""
	}
	return roles[0]
}

func (j *JWTService) GenerateRefreshToken(user *types.User, jti string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		Sub:  user.ID,
		Iss:  Issuer,
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
