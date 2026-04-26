// Package server — bulk operation HTTP handlers (P3b).
package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/store"
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

// handleBulkInstrumentsCSV handles POST /api/v1/securities/bulk/instruments/csv.
//
// Accepts a CSV body with a header row. Recognised column names (case-insensitive):
// ticker, name, asset_class, exchange_code, lot_size, tick_size, currency, isin, outstanding_shares.
// Each data row is validated independently; valid rows create instruments.
// The endpoint always returns 200 with a BulkUploadResult.
func (s *Server) handleBulkInstrumentsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	reader := csv.NewReader(r.Body)
	reader.TrimLeadingSpace = true

	// Read header row.
	headers, err := reader.Read()
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_CSV", "failed to read CSV header row: "+err.Error(), nil)
		return
	}

	// Normalise header names to lower-case for column mapping.
	colIndex := make(map[string]int, len(headers))
	for i, h := range headers {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	result := types.BulkUploadResult{
		Errors: []types.BulkError{},
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rowNum := 0

	for {
		record, readErr := reader.Read()
		if readErr != nil {
			break // EOF or unrecoverable parse error — treat remaining rows as done
		}
		rowNum++

		col := func(name string) string {
			idx, ok := colIndex[name]
			if !ok || idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}

		ticker := col("ticker")
		name := col("name")
		assetClass := types.AssetClass(col("asset_class"))
		exchangeCode := col("exchange_code")
		currency := col("currency")
		if currency == "" {
			currency = "MNT"
		}
		isin := col("isin")

		lotSizeStr := col("lot_size")
		tickSizeStr := col("tick_size")
		outSharesStr := col("outstanding_shares")

		item := bulkInstrumentItem{
			Ticker:       ticker,
			Name:         name,
			AssetClass:   assetClass,
			ExchangeCode: exchangeCode,
			Currency:     currency,
			ISIN:         isin,
		}

		// Parse numeric columns; validation errors are appended as BulkErrors.
		parseErr := false
		if lotSizeStr != "" {
			lotSize, e := strconv.Atoi(lotSizeStr)
			if e != nil {
				result.Failed++
				result.Errors = append(result.Errors, types.BulkError{
					Index:  rowNum - 1,
					Ticker: ticker,
					Error:  fmt.Sprintf("invalid lot_size %q: %s", lotSizeStr, e.Error()),
				})
				parseErr = true
			} else {
				item.LotSize = lotSize
			}
		}
		if tickSizeStr != "" && !parseErr {
			tickSize, e := strconv.ParseFloat(tickSizeStr, 64)
			if e != nil {
				result.Failed++
				result.Errors = append(result.Errors, types.BulkError{
					Index:  rowNum - 1,
					Ticker: ticker,
					Error:  fmt.Sprintf("invalid tick_size %q: %s", tickSizeStr, e.Error()),
				})
				parseErr = true
			} else {
				item.TickSize = tickSize
			}
		}
		if outSharesStr != "" && !parseErr {
			outShares, e := strconv.ParseInt(outSharesStr, 10, 64)
			if e != nil {
				result.Failed++
				result.Errors = append(result.Errors, types.BulkError{
					Index:  rowNum - 1,
					Ticker: ticker,
					Error:  fmt.Sprintf("invalid outstanding_shares %q: %s", outSharesStr, e.Error()),
				})
				parseErr = true
			} else {
				item.OutstandingShares = outShares
			}
		}

		if parseErr {
			result.Total++
			continue
		}

		result.Total++

		if bulkErr := validateBulkItem(rowNum-1, item); bulkErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, *bulkErr)
			continue
		}

		id, genErr := newUUID()
		if genErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, types.BulkError{
				Index:  rowNum - 1,
				Ticker: ticker,
				Error:  "failed to generate id: " + genErr.Error(),
			})
			continue
		}

		inst := &types.Instrument{
			ID:                id,
			Ticker:            item.Ticker,
			Name:              item.Name,
			AssetClass:        item.AssetClass,
			ExchangeCode:      item.ExchangeCode,
			LotSize:           item.LotSize,
			TickSize:          item.TickSize,
			Currency:          item.Currency,
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
				Index:  rowNum - 1,
				Ticker: ticker,
				Error:  storeErr.Error(),
			})
			continue
		}
		result.Created++
	}

	s.writeJSON(w, http.StatusOK, result)
}

// amendInstrumentItem is a single entry in the mass-amend request.
// Only non-zero/non-empty fields are applied (partial update semantics).
type amendInstrumentItem struct {
	ID      string  `json:"id"`
	Name    string  `json:"name,omitempty"`
	LotSize int     `json:"lot_size,omitempty"`
	TickSize float64 `json:"tick_size,omitempty"`
}

// BulkAmendResult summarises the outcome of a mass instrument amendment.
type BulkAmendResult struct {
	Total   int              `json:"total"`
	Updated int              `json:"updated"`
	Failed  int              `json:"failed"`
	Errors  []types.BulkError `json:"errors"`
}

// handleBulkInstrumentsAmend handles POST /api/v1/securities/bulk/instruments/amend.
//
// Accepts a JSON array of [{id, name?, lot_size?, tick_size?}].
// Each entry is applied independently; the endpoint always returns 200 with a BulkAmendResult.
func (s *Server) handleBulkInstrumentsAmend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		return
	}

	var items []amendInstrumentItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be a JSON array of amend objects", nil)
		return
	}

	result := BulkAmendResult{
		Total:  len(items),
		Errors: []types.BulkError{},
	}

	for i, item := range items {
		if item.ID == "" {
			result.Failed++
			result.Errors = append(result.Errors, types.BulkError{
				Index: i,
				Error: "id is required",
			})
			continue
		}

		update := store.InstrumentUpdate{
			Name:    item.Name,
			LotSize: item.LotSize,
			TickSize: item.TickSize,
		}

		if err := s.instrumentStore.Update(item.ID, update); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, types.BulkError{
				Index: i,
				Error: fmt.Sprintf("failed to update instrument %s: %s", item.ID, err.Error()),
			})
			continue
		}
		result.Updated++
	}

	s.writeJSON(w, http.StatusOK, result)
}
