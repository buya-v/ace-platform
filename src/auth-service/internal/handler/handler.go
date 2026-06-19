package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/garudax-platform/auth-service/internal/auth"
	"github.com/garudax-platform/auth-service/internal/types"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

type AuthHandler struct {
	service *auth.Service
}

func NewAuthHandler(service *auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Tenant optionally selects the active tenant for this session. When set, the
	// user must hold a role in it (or be a platform-admin), otherwise login is
	// rejected. It may also be supplied via the X-GarudaX-Tenant header.
	Tenant string `json:"active_tenant"`
}

type refreshRequest struct {
	SessionID    string `json:"session_id"`
	RefreshToken string `json:"refresh_token"`
}

type authorizeRequest struct {
	UserID              string `json:"user_id"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	RedirectURI         string `json:"redirect_uri"`
}

type exchangeRequest struct {
	AuthCode     string `json:"auth_code"`
	CodeVerifier string `json:"code_verifier"`
}

type createAPIKeyRequest struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	ExpiresIn int    `json:"expires_in_hours"`
}

type validateAPIKeyRequest struct {
	APIKey string `json:"api_key"`
}

type revokeAPIKeyRequest struct {
	KeyID  string `json:"key_id"`
	UserID string `json:"user_id"`
}

type validateTokenRequest struct {
	Token string `json:"token"`
}

type revokeSessionRequest struct {
	SessionID string `json:"session_id"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.Role == "" {
		writeError(w, "email, password, and role are required", http.StatusBadRequest)
		return
	}
	if !emailRegex.MatchString(req.Email) {
		writeError(w, "invalid email format", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		writeError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if !types.ValidRole(req.Role) {
		writeError(w, "invalid role", http.StatusBadRequest)
		return
	}

	user, err := h.service.Register(req.Email, req.Password, types.Role(req.Role))
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, "email already registered", http.StatusConflict)
			return
		}
		writeError(w, "registration failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	// The X-GarudaX-Tenant header takes precedence over the body field, matching
	// how the gateway scopes every request to a tenant.
	activeTenant := req.Tenant
	if hdr := r.Header.Get("X-GarudaX-Tenant"); hdr != "" {
		activeTenant = hdr
	}

	tokens, err := h.service.LoginWithTenant(req.Email, req.Password, activeTenant)
	if err != nil {
		switch err {
		case auth.ErrAccountLocked:
			writeError(w, "account locked", http.StatusForbidden)
		case auth.ErrTenantAccessDenied:
			writeError(w, "no access to the requested tenant", http.StatusForbidden)
		default:
			writeError(w, "invalid credentials", http.StatusUnauthorized)
		}
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.RefreshToken == "" {
		writeError(w, "session_id and refresh_token are required", http.StatusBadRequest)
		return
	}

	tokens, err := h.service.RefreshSession(req.SessionID, req.RefreshToken)
	if err != nil {
		writeError(w, "invalid or expired session", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *AuthHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req authorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.CodeChallenge == "" || req.CodeChallengeMethod == "" {
		writeError(w, "user_id, code_challenge, and code_challenge_method are required", http.StatusBadRequest)
		return
	}
	if req.CodeChallengeMethod != "S256" {
		writeError(w, "only S256 challenge method is supported", http.StatusBadRequest)
		return
	}

	authCode, err := h.service.AuthorizePKCE(req.UserID, req.CodeChallenge, req.CodeChallengeMethod, req.RedirectURI)
	if err != nil {
		writeError(w, "authorization failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"auth_code":    authCode,
		"redirect_uri": req.RedirectURI,
	})
}

func (h *AuthHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req exchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.AuthCode == "" || req.CodeVerifier == "" {
		writeError(w, "auth_code and code_verifier are required", http.StatusBadRequest)
		return
	}

	tokens, err := h.service.ExchangeCode(req.AuthCode, req.CodeVerifier)
	if err != nil {
		writeError(w, "code exchange failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, tokens)
}

func (h *AuthHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req createAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.Name == "" {
		writeError(w, "user_id and name are required", http.StatusBadRequest)
		return
	}

	var expiresAt time.Time
	if req.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
	}

	key, rawKey, err := h.service.CreateAPIKey(req.UserID, req.Name, expiresAt)
	if err != nil {
		writeError(w, "failed to create api key", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         key.ID,
		"name":       key.Name,
		"api_key":    rawKey,
		"prefix":     key.Prefix,
		"expires_at": key.ExpiresAt,
	})
}

func (h *AuthHandler) ValidateAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req validateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		writeError(w, "api_key is required", http.StatusBadRequest)
		return
	}

	key, err := h.service.ValidateAPIKey(req.APIKey)
	if err != nil {
		writeError(w, "invalid api key", http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":      key.ID,
		"user_id": key.UserID,
		"name":    key.Name,
		"prefix":  key.Prefix,
	})
}

func (h *AuthHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req revokeAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.KeyID == "" || req.UserID == "" {
		writeError(w, "key_id and user_id are required", http.StatusBadRequest)
		return
	}

	if err := h.service.RevokeAPIKey(req.KeyID, req.UserID); err != nil {
		writeError(w, "failed to revoke api key", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *AuthHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req validateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		writeError(w, "token is required", http.StatusBadRequest)
		return
	}

	claims, err := h.service.ValidateToken(req.Token)
	if err != nil {
		writeError(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, claims)
}

func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req revokeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" {
		writeError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	if err := h.service.RevokeSession(req.SessionID); err != nil {
		writeError(w, "failed to revoke session", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *AuthHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users := h.service.ListUsers()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": users,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, status, errorResponse{Error: msg})
}
