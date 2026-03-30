package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/garudax-platform/gateway/internal/auth"
	"github.com/garudax-platform/gateway/internal/types"
)

type contextKey string

const (
	ClaimsContextKey contextKey = "claims"
	RequestIDKey     contextKey = "request_id"
)

// ClaimsFromContext extracts JWT claims from the request context.
func ClaimsFromContext(ctx context.Context) *auth.Claims {
	if c, ok := ctx.Value(ClaimsContextKey).(*auth.Claims); ok {
		return c
	}
	return nil
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// AuthConfig defines which paths are public (no auth needed).
type AuthConfig struct {
	PublicPaths map[string]bool
	PublicPrefixes []string
}

// Auth creates JWT authentication middleware.
func Auth(validator *auth.JWTValidator, cfg *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := RequestIDFromContext(r.Context())

			// CORS preflight requests are always allowed
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Check if path is public
			if isPublicPath(r.URL.Path, r.Method, cfg) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract Bearer token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				types.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED",
					"Missing authorization token", reqID)
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				types.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED",
					"Invalid authorization header format", reqID)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := validator.ValidateToken(tokenStr)
			if err != nil {
				code := "UNAUTHENTICATED"
				msg := "Invalid token"
				switch err {
				case auth.ErrExpiredToken:
					code = "TOKEN_EXPIRED"
					msg = "Token has expired"
				case auth.ErrInvalidToken:
					msg = "Invalid token signature"
				case auth.ErrMalformedToken:
					msg = "Malformed token"
				}
				types.WriteError(w, http.StatusUnauthorized, code, msg, reqID)
				return
			}

			// Attach claims to context
			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isPublicPath(path, method string, cfg *AuthConfig) bool {
	if cfg == nil {
		return false
	}

	// Check exact path match (with method)
	key := method + " " + path
	if cfg.PublicPaths[key] {
		return true
	}

	// Check path-only match (any method)
	if cfg.PublicPaths[path] {
		return true
	}

	// Check prefix match
	for _, prefix := range cfg.PublicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// RequireRoles creates middleware that checks if the user has any of the required roles.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				reqID := RequestIDFromContext(r.Context())
				types.WriteError(w, http.StatusUnauthorized, "UNAUTHENTICATED",
					"Authentication required", reqID)
				return
			}

			if !claims.HasAnyRole(roles...) {
				reqID := RequestIDFromContext(r.Context())
				types.WriteError(w, http.StatusForbidden, "PERMISSION_DENIED",
					"Insufficient permissions", reqID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
