// Package middleware — permission-check middleware for RBAC enforcement.
package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/garudax-platform/securities-service/internal/engine"
)

// permissionErrorBody is the JSON error shape for permission errors.
type permissionErrorBody struct {
	Error permissionErrorDetail `json:"error"`
}

type permissionErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writePermissionError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(permissionErrorBody{
		Error: permissionErrorDetail{Code: code, Message: message},
	})
}

// PermissionCheck returns an HTTP middleware that validates whether the caller
// holds requiredPerm before forwarding the request to next.
//
// Participant identity is resolved from the X-Participant-ID request header:
//   - If the header is absent the request is passed through (public / system
//     endpoints that predate RBAC; the handler is responsible for its own auth).
//   - If the header is present but the participant lacks requiredPerm the
//     middleware responds 403 PERMISSION_DENIED immediately.
func PermissionCheck(privilegeEngine *engine.PrivilegeEngine, requiredPerm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			participantID := r.Header.Get("X-Participant-ID")
			if participantID == "" {
				// No participant header — pass through for backwards compatibility.
				next.ServeHTTP(w, r)
				return
			}

			if err := privilegeEngine.HasPermission(participantID, requiredPerm); err != nil {
				writePermissionError(w, http.StatusForbidden,
					"PERMISSION_DENIED",
					err.Error(),
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
