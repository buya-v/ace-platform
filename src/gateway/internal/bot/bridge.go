package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OrchestratorRequest is the payload sent to the orchestrator /chat endpoint.
type OrchestratorRequest struct {
	Message string            `json:"message"`
	Context map[string]string `json:"context,omitempty"`
}

// OrchestratorResponse is the response from the orchestrator /chat endpoint.
type OrchestratorResponse struct {
	Reply       string   `json:"reply"`
	Actions     []Action `json:"actions"`
	Suggestions []string `json:"suggestions"`
}

// Action represents an actionable item returned by the orchestrator.
type Action struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

// Bridge handles communication with the bot orchestrator service.
type Bridge struct {
	orchestratorURL string
	client          *http.Client
}

// NewBridge creates a new orchestrator bridge.
// If orchestratorURL is empty, ProxyToOrchestrator will always return an error,
// causing the handler to fall back to built-in responses.
func NewBridge(orchestratorURL string) *Bridge {
	return &Bridge{
		orchestratorURL: orchestratorURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsAvailable returns true if an orchestrator URL is configured.
func (b *Bridge) IsAvailable() bool {
	return b.orchestratorURL != ""
}

// ProxyToOrchestrator sends a chat message to the orchestrator and returns the response.
// Returns an error if the orchestrator is unavailable or returns a non-200 status.
func (b *Bridge) ProxyToOrchestrator(message string, ctx map[string]string) (*OrchestratorResponse, error) {
	if b.orchestratorURL == "" {
		return nil, fmt.Errorf("orchestrator not configured")
	}

	reqBody := OrchestratorRequest{
		Message: message,
		Context: ctx,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := b.orchestratorURL + "/chat"
	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("orchestrator request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(body))
	}

	var orchResp OrchestratorResponse
	if err := json.NewDecoder(resp.Body).Decode(&orchResp); err != nil {
		return nil, fmt.Errorf("decode orchestrator response: %w", err)
	}

	return &orchResp, nil
}
