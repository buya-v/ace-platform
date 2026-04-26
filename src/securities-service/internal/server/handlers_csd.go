// Package server — HTTP handlers for CSD (custody account, balance, and transfer) endpoints.
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleCustodyAccounts dispatches GET /api/v1/securities/csd/accounts (list by firm)
// and POST /api/v1/securities/csd/accounts (create).
func (s *Server) handleCustodyAccounts(w http.ResponseWriter, r *http.Request) {
	if s.custodyAccountStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "custody account store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListCustodyAccounts(w, r)
	case http.MethodPost:
		s.handleCreateCustodyAccount(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleCustodyAccount dispatches GET/sub-resource for /api/v1/securities/csd/accounts/{id}
// and GET /api/v1/securities/csd/accounts/{id}/balances.
func (s *Server) handleCustodyAccount(w http.ResponseWriter, r *http.Request) {
	if s.custodyAccountStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "custody account store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/balances") {
		if r.Method == http.MethodGet {
			s.handleListCustodyBalances(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetCustodyAccount(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleCSDTransfers dispatches GET /api/v1/securities/csd/transfers
// and POST /api/v1/securities/csd/transfers (create transfer).
func (s *Server) handleCSDTransfers(w http.ResponseWriter, r *http.Request) {
	if s.csdTransferStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "CSD transfer store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleCreateCSDTransfer(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleCSDTransfer dispatches actions on /api/v1/securities/csd/transfers/{id}
// including /complete and /fail sub-resources.
func (s *Server) handleCSDTransfer(w http.ResponseWriter, r *http.Request) {
	if s.csdTransferStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "CSD transfer store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/complete") {
		if r.Method == http.MethodPost {
			s.handleCompleteCSDTransfer(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	if strings.HasSuffix(path, "/fail") {
		if r.Method == http.MethodPost {
			s.handleFailCSDTransfer(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetCSDTransfer(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// ── Custody Account handlers ──────────────────────────────────────────────────

func (s *Server) handleListCustodyAccounts(w http.ResponseWriter, r *http.Request) {
	firmID := r.URL.Query().Get("firm_id")
	accounts, err := s.custodyAccountStore.ListByFirm(firmID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  accounts,
		"total": len(accounts),
	})
}

func (s *Server) handleCreateCustodyAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		FirmID   string `json:"firm_id"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.FirmID == "" || req.Name == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "firm_id and name are required", nil)
		return
	}
	id := req.ID
	if id == "" {
		id = "acct-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	account := &types.CustodyAccount{
		ID:        id,
		FirmID:    req.FirmID,
		Name:      req.Name,
		Currency:  req.Currency,
		TenantID:  req.TenantID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.custodyAccountStore.Create(account); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, account)
}

func (s *Server) handleGetCustodyAccount(w http.ResponseWriter, r *http.Request) {
	id := custodyAccountIDFromPath(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "account id is required", nil)
		return
	}
	account, err := s.custodyAccountStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "custody account not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, account)
}

// handleListCustodyBalances handles GET /api/v1/securities/csd/accounts/{id}/balances.
func (s *Server) handleListCustodyBalances(w http.ResponseWriter, r *http.Request) {
	if s.custodyBalanceStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "custody balance store not configured", nil)
		return
	}
	// Extract account ID from path: /api/v1/securities/csd/accounts/{id}/balances
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/balances")
	accountID := strings.TrimPrefix(path, "/api/v1/securities/csd/accounts/")
	if accountID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "account id is required", nil)
		return
	}
	balances, err := s.custodyBalanceStore.ListByAccount(accountID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  balances,
		"total": len(balances),
	})
}

// ── CSD Transfer handlers ─────────────────────────────────────────────────────

func (s *Server) handleCreateCSDTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID               string                  `json:"id"`
		FromAccountID    string                  `json:"from_account_id"`
		ToAccountID      string                  `json:"to_account_id"`
		InstrumentID     string                  `json:"instrument_id"`
		Quantity         int                     `json:"quantity"`
		TransferType     types.CSDTransferType   `json:"transfer_type"`
		SettlementAmount float64                 `json:"settlement_amount"`
		TenantID         string                  `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.FromAccountID == "" || req.ToAccountID == "" || req.InstrumentID == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "from_account_id, to_account_id, and instrument_id are required", nil)
		return
	}
	if req.Quantity <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_QUANTITY", "quantity must be positive", nil)
		return
	}
	if req.TransferType == "" {
		req.TransferType = types.CSDTransferFOP
	}
	if req.TransferType == types.CSDTransferDVP && req.SettlementAmount <= 0 {
		s.writeError(w, http.StatusBadRequest, "INVALID_DVP", "DVP transfers require a positive settlement_amount", nil)
		return
	}

	// Balance validation: if a balance store is configured and the from-account
	// has at least one balance record for any instrument, verify it holds
	// sufficient quantity of the requested instrument.
	// Accounts with no balance records at all (e.g. new accounts) skip this check.
	if s.custodyBalanceStore != nil {
		allBalances, balErr := s.custodyBalanceStore.ListByAccount(req.FromAccountID)
		if balErr == nil && len(allBalances) > 0 {
			held := 0
			for _, b := range allBalances {
				if b.InstrumentID == req.InstrumentID {
					held = b.Quantity
					break
				}
			}
			if held < req.Quantity {
				s.writeError(w, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE",
					"from account has insufficient balance for this transfer", nil)
				return
			}
		}
	}

	id := req.ID
	if id == "" {
		id = "csd-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	transfer := &types.CSDTransfer{
		ID:               id,
		FromAccountID:    req.FromAccountID,
		ToAccountID:      req.ToAccountID,
		InstrumentID:     req.InstrumentID,
		Quantity:         req.Quantity,
		TransferType:     req.TransferType,
		SettlementAmount: req.SettlementAmount,
		Status:           types.CSDTransferPending,
		TenantID:         req.TenantID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.csdTransferStore.Create(transfer); err != nil {
		s.writeError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, transfer)
}

func (s *Server) handleGetCSDTransfer(w http.ResponseWriter, r *http.Request) {
	id := csdTransferIDFromPath(r.URL.Path)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "transfer id is required", nil)
		return
	}
	transfer, err := s.csdTransferStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "CSD transfer not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, transfer)
}

func (s *Server) handleCompleteCSDTransfer(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/securities/csd/transfers/{id}/complete
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/complete")
	id := strings.TrimPrefix(path, "/api/v1/securities/csd/transfers/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "transfer id is required", nil)
		return
	}
	if err := s.csdTransferStore.Complete(id); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "CSD transfer not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}
	transfer, _ := s.csdTransferStore.Get(id)

	// Apply balance movements if a balance store is wired.
	if s.custodyBalanceStore != nil && transfer != nil {
		// Debit from-account.
		s.custodyBalanceStore.GetOrUpdate(transfer.FromAccountID, transfer.InstrumentID, -transfer.Quantity, 0) //nolint:errcheck
		// Credit to-account.
		s.custodyBalanceStore.GetOrUpdate(transfer.ToAccountID, transfer.InstrumentID, transfer.Quantity, 0) //nolint:errcheck
	}

	s.writeJSON(w, http.StatusOK, transfer)
}

func (s *Server) handleFailCSDTransfer(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/securities/csd/transfers/{id}/fail
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/fail")
	id := strings.TrimPrefix(path, "/api/v1/securities/csd/transfers/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_ID", "transfer id is required", nil)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — reason is optional

	if err := s.csdTransferStore.Fail(id, req.Reason); err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "CSD transfer not found", nil)
		return
	} else if err != nil {
		s.writeError(w, http.StatusConflict, "INVALID_STATE", err.Error(), nil)
		return
	}
	transfer, _ := s.csdTransferStore.Get(id)
	s.writeJSON(w, http.StatusOK, transfer)
}

// ── path extraction helpers ───────────────────────────────────────────────────

// custodyAccountIDFromPath extracts the account ID from
// /api/v1/securities/csd/accounts/{id}[/...].
func custodyAccountIDFromPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, "/api/v1/securities/csd/accounts/")
	// Strip any trailing sub-resource (e.g. /balances).
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	return path
}

// csdTransferIDFromPath extracts the transfer ID from
// /api/v1/securities/csd/transfers/{id}[/...].
func csdTransferIDFromPath(path string) string {
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, "/api/v1/securities/csd/transfers/")
	if idx := strings.Index(path, "/"); idx >= 0 {
		path = path[:idx]
	}
	return path
}
