package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ace-platform/auth-service/internal/auth"
	"github.com/ace-platform/auth-service/internal/store"
)

func newTestHandler() *AuthHandler {
	repo := store.NewInMemoryStore()
	jwt := auth.NewJWTService("test-signing-key-that-is-long-enough", 900, 86400)
	svc := auth.NewService(repo, jwt, 4, 5, 30*time.Minute)
	return NewAuthHandler(svc)
}

func doPost(h http.HandlerFunc, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestRegisterHandler(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "handler@example.com",
		"password": "password123",
		"role":     "trader",
	})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["email"] != "handler@example.com" {
		t.Errorf("email = %v, want handler@example.com", resp["email"])
	}
}

func TestRegisterHandlerMissingFields(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email": "a@b.com",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRegisterHandlerInvalidEmail(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "not-an-email",
		"password": "password123",
		"role":     "trader",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRegisterHandlerShortPassword(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "short@example.com",
		"password": "1234567",
		"role":     "trader",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRegisterHandlerInvalidRole(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "role@example.com",
		"password": "password123",
		"role":     "hacker",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestRegisterDuplicateHandler(t *testing.T) {
	h := newTestHandler()

	doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "dup@example.com",
		"password": "password123",
		"role":     "trader",
	})

	rr := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "dup@example.com",
		"password": "password456",
		"role":     "viewer",
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestLoginHandler(t *testing.T) {
	h := newTestHandler()

	doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "login@example.com",
		"password": "password123",
		"role":     "trader",
	})

	rr := doPost(h.Login, "/api/v1/login", map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["AccessToken"] == nil && resp["access_token"] == nil {
		t.Error("expected access token in response")
	}
}

func TestLoginHandlerWrongPassword(t *testing.T) {
	h := newTestHandler()

	doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "wrong@example.com",
		"password": "password123",
		"role":     "trader",
	})

	rr := doPost(h.Login, "/api/v1/login", map[string]string{
		"email":    "wrong@example.com",
		"password": "wrongpassword",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestLoginHandlerMissingFields(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.Login, "/api/v1/login", map[string]string{
		"email": "a@b.com",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/register", nil)
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestValidateTokenHandler(t *testing.T) {
	h := newTestHandler()

	doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "validate@example.com",
		"password": "password123",
		"role":     "trader",
	})

	loginRR := doPost(h.Login, "/api/v1/login", map[string]string{
		"email":    "validate@example.com",
		"password": "password123",
	})

	var loginResp map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResp)

	token, _ := loginResp["AccessToken"].(string)
	if token == "" {
		t.Fatal("no access token in login response")
	}

	rr := doPost(h.ValidateToken, "/api/v1/token/validate", map[string]string{
		"token": token,
	})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestValidateTokenHandlerInvalid(t *testing.T) {
	h := newTestHandler()

	rr := doPost(h.ValidateToken, "/api/v1/token/validate", map[string]string{
		"token": "invalid-token",
	})

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestCreateAPIKeyHandler(t *testing.T) {
	h := newTestHandler()

	regRR := doPost(h.Register, "/api/v1/register", map[string]string{
		"email":    "apikey@example.com",
		"password": "password123",
		"role":     "trader",
	})

	var regResp map[string]interface{}
	json.Unmarshal(regRR.Body.Bytes(), &regResp)
	userID, _ := regResp["id"].(string)

	rr := doPost(h.CreateAPIKey, "/api/v1/apikey/create", map[string]interface{}{
		"user_id":          userID,
		"name":             "my-key",
		"expires_in_hours": 24,
	})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d, body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}
}
