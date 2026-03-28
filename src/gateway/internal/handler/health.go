package handler

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// readyFlag tracks whether the gateway is ready to serve traffic.
var readyFlag int32

// SetReady marks the gateway as ready.
func SetReady() {
	atomic.StoreInt32(&readyFlag, 1)
}

// IsReady returns whether the gateway is ready.
func IsReady() bool {
	return atomic.LoadInt32(&readyFlag) == 1
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "ace-gateway",
	})
}

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
		})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}
