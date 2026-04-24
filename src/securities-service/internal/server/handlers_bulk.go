// Package server — bulk operation HTTP handlers (P3b).
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// bulkInstrumentItem is a single entry in the bulk instrument upload array.
type bulkInstrumentItem struct {
	Ticker            string           `json:"ticker"`
	Name              string           `json:"name"`
	AssetClass        types.AssetClass `json:"asset_class"`
	ExchangeCode      string           `json:"exchange_code"`
	LotSize           int              `json:"lot_size"`
	TickSize          float64          `json:"tick_size"`
	Currency          string           `json:"currency"`
	ISIN              string           `json:"isin,omitempty"`
	OutstandingShares int64            `json:"outstanding_shares,omitempty"`
}

// handleBulkInstruments handles POST /api/v1/securities/bulk/instruments.
//
// Accepts a JSON array of instrument definitions. Each entry is validated
// independently; valid entries are created and invalid entries are collected
// into the errors list. The endpoint always returns 200 with a BulkUploadResult —
// partial success is not treated as an HTTP error.
func (s *Server) handleBulkInstruments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	var items []bulkInstrumentItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a JSON array of instrument objects", nil)
		return
	}

	result := types.BulkUploadResult{
		Total:  len(items),
		Errors: []types.BulkError{},
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for i, item := range items {
		// Validate required fields.
		if err := validateBulkItem(i, item); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, *err)
			continue
		}

		id, genErr := newUUID()
		if genErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, types.BulkError{
				Index:  i,
				Ticker: item.Ticker,
				Error:  "failed to generate id: " + genErr.Error(),
			})
			continue
		}

		currency := item.Currency
		if currency == "" {
			currency = "MNT"
		}

		inst := &types.Instrument{
			ID:                id,
			Ticker:            item.Ticker,
			Name:              item.Name,
			AssetClass:        item.AssetClass,
			ExchangeCode:      item.ExchangeCode,
			LotSize:           item.LotSize,
			TickSize:          item.TickSize,
			Currency:          currency,
			ISIN:              item.ISIN,
			OutstandingShares: item.OutstandingShares,
			TradingStatus:     types.TradingStatusActive,
			ListingDate:       now[:10],
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if storeErr := s.instrumentStore.Create(inst); storeErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, types.BulkError{
				Index:  i,
				Ticker: item.Ticker,
				Error:  storeErr.Error(),
			})
			continue
		}
		result.Created++
	}

	s.writeJSON(w, http.StatusOK, result)
}

// validateBulkItem checks mandatory fields and numeric constraints for a single
// bulk item. Returns a BulkError on the first violation, or nil if valid.
func validateBulkItem(index int, item bulkInstrumentItem) *types.BulkError {
	if item.Ticker == "" {
		return &types.BulkError{Index: index, Ticker: item.Ticker, Error: "ticker is required"}
	}
	if item.Name == "" {
		return &types.BulkError{Index: index, Ticker: item.Ticker, Error: "name is required"}
	}
	if item.AssetClass == "" {
		return &types.BulkError{Index: index, Ticker: item.Ticker, Error: "asset_class is required"}
	}
	if item.LotSize <= 0 {
		return &types.BulkError{Index: index, Ticker: item.Ticker, Error: fmt.Sprintf("lot_size must be > 0, got %d", item.LotSize)}
	}
	if item.TickSize <= 0 {
		return &types.BulkError{Index: index, Ticker: item.Ticker, Error: fmt.Sprintf("tick_size must be > 0, got %g", item.TickSize)}
	}
	return nil
}
