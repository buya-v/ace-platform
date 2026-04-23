package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/garudax-platform/gateway/internal/auth"
)

func newTestValidator() *auth.JWTValidator {
	v := auth.NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")
	v.SetNowFunc(func() time.Time { return time.Unix(1700000000, 0) })
	return v
}

func newTestToken(v *auth.JWTValidator, roles []string) string {
	claims := &auth.Claims{
		Sub:           "user-123",
		ParticipantID: "part-456",
		Roles:         roles,
		Issuer:        "ace-auth-service",
		Audience:      "ace-api-gateway",
		ExpiresAt:     1700003600,
		IssuedAt:      1700000000,
	}
	token, _ := v.CreateToken(claims)
	return token
}

func TestAuthMiddlewarePublicPath(t *testing.T) {
	v := newTestValidator()
	cfg := &AuthConfig{
		PublicPaths: map[string]bool{
			"/healthz":                true,
			"POST /api/v1/auth/login": true,
		},
	}

	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("public path: status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	v := newTestValidator()
	cfg := &AuthConfig{}

	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("missing token: status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	v := newTestValidator()
	cfg := &AuthConfig{}
	token := newTestToken(v, []string{"trader"})

	var gotClaims *auth.Claims
	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClaims = ClaimsFromContext(r.Context())
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("valid token: status = %d, want 200", rec.Code)
	}
	if gotClaims == nil {
		t.Fatal("expected claims in context")
	}
	if gotClaims.Sub != "user-123" {
		t.Errorf("Sub = %q, want %q", gotClaims.Sub, "user-123")
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	v := auth.NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")
	v.SetNowFunc(func() time.Time { return time.Unix(1700010000, 0) })

	cfg := &AuthConfig{}
	claims := &auth.Claims{
		Sub:       "user-123",
		Issuer:    "ace-auth-service",
		Audience:  "ace-api-gateway",
		ExpiresAt: 1700003600,
	}
	creator := auth.NewJWTValidator("test-secret", "ace-auth-service", "ace-api-gateway")
	token, _ := creator.CreateToken(claims)

	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("expired token: status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddlewareInvalidFormat(t *testing.T) {
	v := newTestValidator()
	cfg := &AuthConfig{}

	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/v1/orders", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 401 {
		t.Errorf("invalid format: status = %d, want 401", rec.Code)
	}
}

func TestRequireRolesMiddleware(t *testing.T) {
	v := newTestValidator()

	tests := []struct {
		name     string
		roles    []string
		required []string
		wantCode int
	}{
		{"has role", []string{"trader"}, []string{"trader"}, 200},
		{"has one of", []string{"trader"}, []string{"trader", "exchange_admin"}, 200},
		{"missing role", []string{"trader"}, []string{"exchange_admin"}, 403},
		{"no claims", nil, []string{"trader"}, 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			})
			handler := RequireRoles(tt.required...)(inner)

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.roles != nil {
				token := newTestToken(v, tt.roles)
				claims, _ := v.ValidateToken(token)
				ctx := context.WithValue(req.Context(), ClaimsContextKey, claims)
				req = req.WithContext(ctx)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}

func TestPublicPrefix(t *testing.T) {
	v := newTestValidator()
	cfg := &AuthConfig{
		PublicPrefixes: []string{"/api/v1/instruments/"},
	}

	handler := Auth(v, cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/v1/instruments/WHT/book", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("public prefix: status = %d, want 200", rec.Code)
	}
}
