package types

import (
	"encoding/json"
	"net/http"
)

// ErrorDetail represents a field-level validation error.
type ErrorDetail struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// APIError is the standard error response envelope.
type APIError struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []ErrorDetail `json:"details,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
}

// ErrorResponse wraps APIError in the response envelope.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, status int, code, message, requestID string) {
	resp := ErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

// WriteErrorWithDetails writes a JSON error response with field-level details.
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message, requestID string, details []ErrorDetail) {
	resp := ErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
