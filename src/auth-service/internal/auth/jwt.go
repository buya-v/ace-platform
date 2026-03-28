package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ace-platform/auth-service/internal/types"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
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

type JWTService struct {
	signingKey []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewJWTService(signingKey string, accessTTLSecs, refreshTTLSecs int) *JWTService {
	return &JWTService{
		signingKey: []byte(signingKey),
		accessTTL:  time.Duration(accessTTLSecs) * time.Second,
		refreshTTL: time.Duration(refreshTTLSecs) * time.Second,
	}
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

func (j *JWTService) ValidateToken(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := j.computeHMAC([]byte(signingInput))

	gotSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, ErrInvalidToken
	}

	if !hmac.Equal(expectedSig, gotSig) {
		return nil, ErrInvalidToken
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
	header := JWTHeader{Alg: "HS256", Typ: "JWT"}

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
	signature := j.computeHMAC([]byte(signingInput))
	signatureB64 := base64URLEncode(signature)

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
