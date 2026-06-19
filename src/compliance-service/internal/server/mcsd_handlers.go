// Package server — HTTP handlers for the MCSD custody/settlement integration.
package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/garudax-platform/compliance-service/integration"
)

// csdStatus maps an MCSD adapter error to an HTTP status code.
func csdStatus(err error) int {
	switch {
	case errors.Is(err, integration.ErrMissingTenant),
		errors.Is(err, integration.ErrMissingFields),
		errors.Is(err, integration.ErrInvalidQuantity),
		errors.Is(err, integration.ErrInvalidAmount):
		return http.StatusBadRequest
	case errors.Is(err, integration.ErrTenantMismatch):
		return http.StatusForbidden
	case errors.Is(err, integration.ErrAccountNotFound),
		errors.Is(err, integration.ErrTransferNotFound):
		return http.StatusNotFound
	case errors.Is(err, integration.ErrInsufficientHoldings):
		return http.StatusUnprocessableEntity
	case errors.Is(err, integration.ErrInvalidState):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// handleCSDAccounts handles POST /csd/accounts (create custody account).
func (s *Server) handleCSDAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req integration.CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	acct, err := s.csd.CreateCustodyAccount(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), csdStatus(err))
		return
	}
	addAuditEvent("CSD_ACCOUNT_CREATED", req.OwnerID, "MCSD custody account created: "+acct.AccountID, "")
	writeJSON(w, http.StatusCreated, acct)
}

// handleCSDBalance handles GET /csd/accounts/balance?account_id=&instrument_id=.
func (s *Server) handleCSDBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	accountID := r.URL.Query().Get("account_id")
	instrumentID := r.URL.Query().Get("instrument_id")
	if accountID == "" || instrumentID == "" {
		http.Error(w, "account_id and instrument_id are required", http.StatusBadRequest)
		return
	}
	bal, err := s.csd.GetBalance(r.Context(), accountID, instrumentID)
	if err != nil {
		http.Error(w, err.Error(), csdStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, bal)
}

// handleCSDTransfers handles GET /csd/transfers?id=<transfer_id> (status query).
func (s *Server) handleCSDTransfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	st, err := s.csd.GetTransferStatus(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), csdStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleCSDInstructDvP handles POST /csd/transfers/dvp.
func (s *Server) handleCSDInstructDvP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req integration.DvPInstruction
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	resp, err := s.csd.InstructDvP(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), csdStatus(err))
		return
	}
	addAuditEvent("CSD_DVP_INSTRUCTED", "", "MCSD DvP transfer "+resp.TransferID+" ("+string(resp.State)+")", "")
	writeJSON(w, http.StatusCreated, resp)
}

// handleCSDInstructFoP handles POST /csd/transfers/fop.
func (s *Server) handleCSDInstructFoP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req integration.FoPInstruction
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	resp, err := s.csd.InstructFoP(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), csdStatus(err))
		return
	}
	addAuditEvent("CSD_FOP_INSTRUCTED", "", "MCSD FoP transfer "+resp.TransferID+" ("+string(resp.State)+")", "")
	writeJSON(w, http.StatusCreated, resp)
}

// handleCSDCorporateActions handles POST /csd/corporate-actions (notify) and
// GET /csd/corporate-actions?action_id=<id> (entitlements).
func (s *Server) handleCSDCorporateActions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var action integration.CorporateAction
		if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := s.csd.NotifyCorporateAction(r.Context(), action); err != nil {
			http.Error(w, err.Error(), csdStatus(err))
			return
		}
		ents, _ := s.csd.GetEntitlements(r.Context(), action.ActionID)
		if ents == nil {
			ents = []integration.Entitlement{}
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"action_id":    action.ActionID,
			"entitlements": ents,
			"total":        len(ents),
		})
	case http.MethodGet:
		actionID := r.URL.Query().Get("action_id")
		if actionID == "" {
			http.Error(w, "action_id is required", http.StatusBadRequest)
			return
		}
		ents, err := s.csd.GetEntitlements(r.Context(), actionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if ents == nil {
			ents = []integration.Entitlement{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"action_id":    actionID,
			"entitlements": ents,
			"total":        len(ents),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
