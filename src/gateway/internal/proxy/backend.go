package proxy

import (
	"encoding/json"
	"io"
	"net/http"
)

// BackendClient defines the interface for forwarding requests to backend services.
// This abstraction allows for mock implementations in tests.
type BackendClient interface {
	// Forward sends a request to the backend and returns the response.
	Forward(req *BackendRequest) (*BackendResponse, error)
}

// BackendRequest represents a translated request to send to a backend service.
type BackendRequest struct {
	Service string          // e.g. "matching-engine"
	Method  string          // e.g. "OrderService/SubmitOrder"
	Body    json.RawMessage // JSON body (for translation to protobuf)
	// Metadata carries gRPC-like metadata forwarded to the backend as request
	// headers (x-user-id, x-request-id, x-roles, ...). For tenant-scoped routes
	// the gateway also injects the resolved tenant under the canonical
	// X-GarudaX-Tenant header (see handler.forward / SubmitOrder), so downstream
	// services receive the same tenant the gateway validated.
	Metadata   map[string]string
	PathParams map[string]string // extracted path parameters
	Query      map[string]string // query parameters
}

// BackendResponse represents a response from a backend service.
type BackendResponse struct {
	StatusCode int
	Body       json.RawMessage
	Headers    map[string]string
}

// StubBackendClient is a stub implementation that returns service-unavailable.
// In production, this would be replaced with a real gRPC client.
type StubBackendClient struct{}

// Forward returns a 503 indicating the backend is not yet connected.
func (s *StubBackendClient) Forward(req *BackendRequest) (*BackendResponse, error) {
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    "SERVICE_UNAVAILABLE",
			"message": "Backend service " + req.Service + " is not available",
		},
	}
	body, _ := json.Marshal(resp)
	return &BackendResponse{
		StatusCode: http.StatusServiceUnavailable,
		Body:       body,
	}, nil
}

// ReadJSONBody reads and parses a JSON request body.
func ReadJSONBody(r *http.Request, maxSize int64) (json.RawMessage, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	// Validate it's valid JSON
	if !json.Valid(body) {
		return nil, &json.SyntaxError{}
	}
	return body, nil
}
