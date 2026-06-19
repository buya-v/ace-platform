package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
)

// pathPrefix is the root of the platform-admin API. These routes are platform-level
// and intentionally carry NO tenant middleware — this service IS the platform.
const pathPrefix = "/platform/v1/tenants"

// API is the HTTP surface of the platform control plane. It is an http.Handler so it
// can be mounted directly or wrapped by the gateway.
type API struct {
	reg   *TenantRegistry
	ready atomic.Int32
}

// NewAPI returns an API backed by the given registry.
func NewAPI(reg *TenantRegistry) *API {
	return &API{reg: reg}
}

// SetReady marks the service ready to serve traffic (drives /readyz).
func (a *API) SetReady() { a.ready.Store(1) }

func (a *API) isReady() bool { return a.ready.Load() == 1 }

// Handler returns an http.Handler with all routes registered.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/readyz", a.handleReadyz)
	// Collection: GET (list), POST (create).
	mux.HandleFunc(pathPrefix, a.handleCollection)
	// Item and sub-resources: /platform/v1/tenants/{id}[/suspend|/activate|/decommission|/status|/audit].
	mux.HandleFunc(pathPrefix+"/", a.handleItem)
	return mux
}

// --- collection ---

func (a *API) handleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
		if status != "" && !validStatuses[status] {
			a.writeError(w, http.StatusBadRequest, "INVALID_STATUS",
				"status filter must be one of ONBOARDING, ACTIVE, SUSPENDED, DECOMMISSIONED", nil)
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]interface{}{"tenants": a.reg.List(status)})
	case http.MethodPost:
		a.handleCreate(w, r)
	default:
		a.methodNotAllowed(w)
	}
}

func (a *API) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateTenantRequest
	if err := decodeJSON(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	t, err := a.reg.Create(req)
	if err != nil {
		a.writeRegistryError(w, err)
		return
	}
	a.writeJSON(w, http.StatusCreated, t)
}

// --- item & sub-resources ---

// handleItem parses /platform/v1/tenants/{id} and its sub-resources.
func (a *API) handleItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, pathPrefix+"/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant id required", nil)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch sub {
	case "":
		a.handleSingleTenant(w, r, id)
	case "suspend":
		a.handleAction(w, r, id, StatusSuspended)
	case "activate":
		a.handleAction(w, r, id, StatusActive)
	case "decommission":
		a.handleAction(w, r, id, StatusDecommissioned)
	case "status":
		a.handleStatus(w, r, id)
	case "audit":
		a.handleAudit(w, r, id)
	default:
		a.writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown sub-resource: "+sub, nil)
	}
}

func (a *API) handleSingleTenant(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		t, err := a.reg.Get(id)
		if err != nil {
			a.writeRegistryError(w, err)
			return
		}
		a.writeJSON(w, http.StatusOK, t)
	case http.MethodPatch:
		var req UpdateTenantRequest
		if err := decodeJSON(r, &req); err != nil {
			a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
			return
		}
		t, err := a.reg.Update(id, req)
		if err != nil {
			a.writeRegistryError(w, err)
			return
		}
		a.writeJSON(w, http.StatusOK, t)
	default:
		a.methodNotAllowed(w)
	}
}

// handleAction handles the convenience lifecycle endpoints (POST .../suspend etc.).
// Body is optional: {"actor": "...", "reason": "..."}.
func (a *API) handleAction(w http.ResponseWriter, r *http.Request, id, target string) {
	if r.Method != http.MethodPost {
		a.methodNotAllowed(w)
		return
	}
	var req StatusChangeRequest
	// Body is optional for action endpoints; ignore decode errors on empty bodies.
	_ = decodeJSON(r, &req)
	t, err := a.reg.TransitionStatus(id, target, req.Actor, req.Reason)
	if err != nil {
		a.writeRegistryError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, t)
}

// handleStatus handles PUT /platform/v1/tenants/{id}/status with an explicit target.
func (a *API) handleStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPut {
		a.methodNotAllowed(w)
		return
	}
	var req StatusChangeRequest
	if err := decodeJSON(r, &req); err != nil {
		a.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	t, err := a.reg.TransitionStatus(id, strings.ToUpper(strings.TrimSpace(req.Status)), req.Actor, req.Reason)
	if err != nil {
		a.writeRegistryError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, t)
}

func (a *API) handleAudit(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		a.methodNotAllowed(w)
		return
	}
	entries, err := a.reg.Audit(id)
	if err != nil {
		a.writeRegistryError(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]interface{}{"audit": entries})
}

// --- health ---

func (a *API) handleHealthz(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "platform-control"})
}

func (a *API) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if !a.isReady() {
		a.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// --- error mapping & helpers ---

// writeRegistryError maps registry sentinel errors to HTTP responses.
func (a *API) writeRegistryError(w http.ResponseWriter, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		a.writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "request validation failed", verr.Fields)
	case errors.Is(err, ErrNotFound):
		a.writeError(w, http.StatusNotFound, "TENANT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, ErrAlreadyExists):
		a.writeError(w, http.StatusConflict, "TENANT_ALREADY_EXISTS", err.Error(), nil)
	case errors.Is(err, ErrFlagshipConflict):
		a.writeError(w, http.StatusConflict, "FLAGSHIP_CONFLICT", err.Error(), nil)
	case errors.Is(err, ErrInvalidTransition):
		a.writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error(), nil)
	case errors.Is(err, ErrTerminal):
		a.writeError(w, http.StatusConflict, "TENANT_DECOMMISSIONED", err.Error(), nil)
	default:
		a.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
	}
}

func (a *API) methodNotAllowed(w http.ResponseWriter) {
	a.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
}

func (a *API) writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (a *API) writeError(w http.ResponseWriter, status int, code, message string, details []string) {
	a.writeJSON(w, status, ErrorResponse{Error: ErrorDetail{Code: code, Message: message, Details: details}})
}

// decodeJSON decodes the request body into v. An empty body decodes to a zero value
// without error so optional-body endpoints work.
func decodeJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		// io.EOF means an empty body — treat as a no-op for optional-body endpoints.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}
