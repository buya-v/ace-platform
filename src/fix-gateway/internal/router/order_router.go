package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OrderRouter routes FIX order operations to the securities service REST API.
type OrderRouter struct {
	securitiesServiceURL string
	httpClient           *http.Client
}

// NewOrderRouter creates an OrderRouter targeting the given securities service base URL.
func NewOrderRouter(url string) *OrderRouter {
	return &OrderRouter{
		securitiesServiceURL: url,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SubmitOrder sends a new order to the securities service.
// It POSTs to /api/v1/securities/orders and returns the response body as a map.
func (r *OrderRouter) SubmitOrder(order map[string]interface{}, tenantID string) (map[string]interface{}, error) {
	body, err := json.Marshal(order)
	if err != nil {
		return nil, fmt.Errorf("marshal order: %w", err)
	}

	url := r.securitiesServiceURL + "/api/v1/securities/orders"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GarudaX-Tenant", tenantID)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("securities service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return result, nil
}

// CancelOrder sends a cancel request to the securities service.
// It DELETEs /api/v1/securities/orders/{orderID}.
func (r *OrderRouter) CancelOrder(orderID, tenantID string) error {
	url := r.securitiesServiceURL + "/api/v1/securities/orders/" + orderID
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create cancel request: %w", err)
	}
	req.Header.Set("X-GarudaX-Tenant", tenantID)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cancel order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("securities service returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
