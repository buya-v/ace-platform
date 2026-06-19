package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrEmailExists         = errors.New("email already registered")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrAccountLocked       = errors.New("account locked due to too many failed attempts")
	ErrSessionRevoked      = errors.New("session has been revoked")
	ErrSessionExpired      = errors.New("session has expired")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrPKCECodeExpired     = errors.New("authorization code expired")
	ErrPKCECodeUsed        = errors.New("authorization code already used")
	ErrAPIKeyNotFound      = errors.New("api key not found")
	ErrTenantAccessDenied  = errors.New("user has no role in the requested tenant")
)

// Store defines the repository interface used by Service.
type Store interface {
	CreateUser(user *types.User) error
	GetUserByID(id string) (*types.User, error)
	GetUserByEmail(email string) (*types.User, error)
	UpdateUser(user *types.User) error

	CreateSession(session *types.Session) error
	GetSessionByID(id string) (*types.Session, error)
	RevokeSession(id string) error
	RevokeUserSessions(userID string) error

	CreateAPIKey(key *types.APIKey) error
	GetAPIKeyByHash(keyHash string) (*types.APIKey, error)
	ListAPIKeysByUser(userID string) ([]*types.APIKey, error)
	RevokeAPIKey(id, userID string) error

	StorePKCEChallenge(challenge *types.PKCEChallenge) error
	GetPKCEChallenge(authCode string) (*types.PKCEChallenge, error)
	MarkPKCEUsed(authCode string) error

	ListUsers() []*types.User

	// GetTenantRoles returns the active (non-revoked) per-tenant role assignments
	// for a user from platform.tenant_user_roles. Platform-scoped assignments
	// (TenantID == types.PlatformScope) are included.
	GetTenantRoles(userID string) ([]types.TenantUserRole, error)
	// AssignTenantRole grants a user a role in a tenant (or platform scope). It is
	// idempotent on (tenant_id, user_id, role).
	AssignTenantRole(assignment *types.TenantUserRole) error
}

type Service struct {
	store             Store
	jwt               *JWTService
	bcryptCost        int
	maxFailedAttempts int
	lockoutDuration   time.Duration
}

func NewService(store Store, jwt *JWTService, bcryptCost, maxFailedAttempts int, lockoutDuration time.Duration) *Service {
	return &Service{
		store:             store,
		jwt:               jwt,
		bcryptCost:        bcryptCost,
		maxFailedAttempts: maxFailedAttempts,
		lockoutDuration:   lockoutDuration,
	}
}

func (s *Service) Register(email, password string, role types.Role) (*types.User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &types.User{
		ID:             generateID(),
		Email:          email,
		HashedPassword: string(hashed),
		Role:           role,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.store.CreateUser(user); err != nil {
		return nil, err
	}
	return user, nil
}

// Login authenticates a user and issues a token pair scoped to the user's
// default active tenant (resolved from their tenant role assignments).
func (s *Service) Login(email, password string) (*types.TokenPair, error) {
	return s.LoginWithTenant(email, password, "")
}

// LoginWithTenant authenticates a user and issues a token pair with the given
// tenant pre-selected as the active tenant. If activeTenant is non-empty and the
// user holds no role in it (and is not a platform-admin), ErrTenantAccessDenied
// is returned — enforcing that tenant context is never silently dropped.
func (s *Service) LoginWithTenant(email, password, activeTenant string) (*types.TokenPair, error) {
	user, err := s.store.GetUserByEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.LockedUntil.IsZero() && time.Now().Before(user.LockedUntil) {
		return nil, ErrAccountLocked
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(password)); err != nil {
		user.FailedAttempts++
		if user.FailedAttempts >= s.maxFailedAttempts {
			user.LockedUntil = time.Now().Add(s.lockoutDuration)
		}
		user.UpdatedAt = time.Now()
		s.store.UpdateUser(user)
		return nil, ErrInvalidCredentials
	}

	// Reset failed attempts on successful login
	if user.FailedAttempts > 0 {
		user.FailedAttempts = 0
		user.LockedUntil = time.Time{}
		user.UpdatedAt = time.Now()
		s.store.UpdateUser(user)
	}

	return s.createTokenPair(user, activeTenant)
}

func (s *Service) RefreshSession(sessionID, refreshToken string) (*types.TokenPair, error) {
	sess, err := s.store.GetSessionByID(sessionID)
	if err != nil {
		return nil, ErrSessionRevoked
	}
	if sess.Revoked {
		return nil, ErrSessionRevoked
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Verify refresh token hash
	tokenHash := hashSHA256(refreshToken)
	if tokenHash != sess.RefreshTokenHash {
		// Potential token theft — revoke all user sessions
		s.store.RevokeUserSessions(sess.UserID)
		return nil, ErrInvalidRefreshToken
	}

	// Revoke old session (rotation)
	s.store.RevokeSession(sessionID)

	user, err := s.store.GetUserByID(sess.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return s.createTokenPair(user, "")
}

func (s *Service) RevokeSession(sessionID string) error {
	return s.store.RevokeSession(sessionID)
}

// AuthorizePKCE initiates a PKCE authorization flow.
func (s *Service) AuthorizePKCE(userID, codeChallenge, codeChallengeMethod, redirectURI string) (string, error) {
	if codeChallengeMethod != "S256" {
		return "", ErrInvalidChallengeMethod
	}

	authCode := generateID()
	challenge := &types.PKCEChallenge{
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		AuthCode:            authCode,
		UserID:              userID,
		RedirectURI:         redirectURI,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if err := s.store.StorePKCEChallenge(challenge); err != nil {
		return "", err
	}
	return authCode, nil
}

// ExchangeCode exchanges an authorization code + code verifier for tokens.
func (s *Service) ExchangeCode(authCode, codeVerifier string) (*types.TokenPair, error) {
	challenge, err := s.store.GetPKCEChallenge(authCode)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if challenge.Used {
		return nil, ErrPKCECodeUsed
	}
	if time.Now().After(challenge.ExpiresAt) {
		return nil, ErrPKCECodeExpired
	}

	if err := ValidateCodeVerifier(codeVerifier, challenge.CodeChallenge, challenge.CodeChallengeMethod); err != nil {
		return nil, err
	}

	s.store.MarkPKCEUsed(authCode)

	user, err := s.store.GetUserByID(challenge.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return s.createTokenPair(user, "")
}

// CreateAPIKey generates a new API key for the user.
func (s *Service) CreateAPIKey(userID, name string, expiresAt time.Time) (*types.APIKey, string, error) {
	rawKey := generateAPIKey()
	keyHash := hashSHA256(rawKey)
	prefix := rawKey[:8]

	key := &types.APIKey{
		ID:        generateID(),
		UserID:    userID,
		Name:      name,
		KeyHash:   keyHash,
		Prefix:    prefix,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	if err := s.store.CreateAPIKey(key); err != nil {
		return nil, "", err
	}
	return key, rawKey, nil
}

// ValidateAPIKey checks if a raw API key is valid.
func (s *Service) ValidateAPIKey(rawKey string) (*types.APIKey, error) {
	keyHash := hashSHA256(rawKey)
	key, err := s.store.GetAPIKeyByHash(keyHash)
	if err != nil {
		return nil, ErrAPIKeyNotFound
	}
	return key, nil
}

// RevokeAPIKey revokes an API key owned by the user.
func (s *Service) RevokeAPIKey(keyID, userID string) error {
	return s.store.RevokeAPIKey(keyID, userID)
}

// ListAPIKeys returns all active API keys for a user.
func (s *Service) ListAPIKeys(userID string) ([]*types.APIKey, error) {
	return s.store.ListAPIKeysByUser(userID)
}

// ListUsers returns all users with passwords stripped.
func (s *Service) ListUsers() []map[string]interface{} {
	users := s.store.ListUsers()
	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]interface{}{
			"id":          u.ID,
			"email":       u.Email,
			"role":        u.Role,
			"status":      "APPROVED",
			"entity_name": u.Email,
			"created_at":  u.CreatedAt,
		})
	}
	return result
}

// ValidateToken validates a JWT and returns the claims.
func (s *Service) ValidateToken(token string) (*JWTClaims, error) {
	return s.jwt.ValidateToken(token)
}

// AssignTenantRole grants a user a role within a tenant (or platform scope).
func (s *Service) AssignTenantRole(tenantID, userID, role, grantedBy string) error {
	return s.store.AssignTenantRole(&types.TenantUserRole{
		TenantID:  tenantID,
		UserID:    userID,
		Role:      role,
		GrantedBy: grantedBy,
		GrantedAt: time.Now(),
	})
}

// buildGrants assembles the multi-tenant authorization grants for a user from
// their platform.tenant_user_roles assignments, resolving the active tenant.
func (s *Service) buildGrants(user *types.User, activeTenant string) (TokenGrants, error) {
	assignments, err := s.store.GetTenantRoles(user.ID)
	if err != nil {
		return TokenGrants{}, fmt.Errorf("load tenant roles: %w", err)
	}

	tenantRoles := make(map[string][]types.Role)
	var platformRoles []types.PlatformRole
	for _, a := range assignments {
		if a.Revoked {
			continue
		}
		if a.TenantID == types.PlatformScope {
			platformRoles = append(platformRoles, types.PlatformRole(a.Role))
			continue
		}
		tenantRoles[a.TenantID] = append(tenantRoles[a.TenantID], types.Role(a.Role))
	}

	// Backward-compat bootstrap: a user with no explicit tenant assignments keeps
	// their top-level role, scoped to the default tenant. This preserves
	// single-tenant logins during the multi-tenant retrofit.
	if len(tenantRoles) == 0 && user.Role != "" {
		tenantRoles[types.DefaultTenant] = []types.Role{user.Role}
	}

	isPlatformAdmin := false
	for _, pr := range platformRoles {
		if pr == types.PlatformRoleAdmin {
			isPlatformAdmin = true
			break
		}
	}

	if activeTenant != "" {
		if _, ok := tenantRoles[activeTenant]; !ok && !isPlatformAdmin {
			return TokenGrants{}, ErrTenantAccessDenied
		}
	} else {
		activeTenant = resolveActiveTenant(tenantRoles)
	}

	return TokenGrants{
		TenantRoles:   tenantRoles,
		PlatformRoles: platformRoles,
		ActiveTenant:  activeTenant,
	}, nil
}

// resolveActiveTenant deterministically picks a default active tenant: the
// DefaultTenant if the user has access to it, otherwise the alphabetically-first
// tenant. Returns "" when the user has no tenant roles.
func resolveActiveTenant(tenantRoles map[string][]types.Role) string {
	if len(tenantRoles) == 0 {
		return ""
	}
	if _, ok := tenantRoles[types.DefaultTenant]; ok {
		return types.DefaultTenant
	}
	keys := make([]string, 0, len(tenantRoles))
	for k := range tenantRoles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys[0]
}

func (s *Service) createTokenPair(user *types.User, activeTenant string) (*types.TokenPair, error) {
	grants, err := s.buildGrants(user, activeTenant)
	if err != nil {
		return nil, err
	}

	sessionID := generateID()
	jti := generateID()

	accessToken, err := s.jwt.GenerateAccessTokenWithGrants(user, jti, grants)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(user, sessionID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	// Store refresh token hash on session (addresses T005 review gap)
	refreshTokenHash := hashSHA256(refreshToken)
	session := &types.Session{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: refreshTokenHash,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
	}

	if err := s.store.CreateSession(session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &types.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.jwt.accessTTL.Seconds()),
	}, nil
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashSHA256(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
