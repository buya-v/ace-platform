// Package server — HTTP handlers for fixed-income bond endpoints.
package server

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/garudax-platform/decimal"
	"github.com/garudax-platform/securities-service/internal/store"
	"github.com/garudax-platform/securities-service/internal/types"
)

// handleBonds dispatches GET /api/v1/securities/bonds (list)
// and POST /api/v1/securities/bonds (create).
func (s *Server) handleBonds(w http.ResponseWriter, r *http.Request) {
	if s.bondStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "bond store not configured", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListBonds(w, r)
	case http.MethodPost:
		s.handleCreateBond(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

// handleBond dispatches for /api/v1/securities/bonds/{id}
// and sub-resource /accrued-interest.
func (s *Server) handleBond(w http.ResponseWriter, r *http.Request) {
	if s.bondStore == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NOT_CONFIGURED", "bond store not configured", nil)
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if strings.HasSuffix(path, "/accrued-interest") {
		if r.Method == http.MethodGet {
			s.handleBondAccruedInterest(w, r)
		} else {
			s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetBond(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", nil)
	}
}

func (s *Server) handleListBonds(w http.ResponseWriter, r *http.Request) {
	bonds, err := s.bondStore.List()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  bonds,
		"total": len(bonds),
	})
}

func (s *Server) handleCreateBond(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                 string                   `json:"id"`
		ISIN               string                   `json:"isin"`
		Name               string                   `json:"name"`
		Issuer             string                   `json:"issuer"`
		MaturityDate       string                   `json:"maturity_date"`
		CouponRate         float64                  `json:"coupon_rate"`
		CouponFrequency    string                   `json:"coupon_frequency"`
		ParValue           types.Decimal            `json:"par_value"`
		DayCountConvention types.DayCountConvention `json:"day_count_convention"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON body", nil)
		return
	}
	if req.ID == "" || req.ISIN == "" || req.Issuer == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "id, isin, and issuer are required", nil)
		return
	}
	if !req.ParValue.IsPos() {
		s.writeError(w, http.StatusBadRequest, "INVALID_PAR_VALUE", "par_value must be positive", nil)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	bond := &types.Bond{
		ID:                 req.ID,
		ISIN:               req.ISIN,
		Name:               req.Name,
		Issuer:             req.Issuer,
		MaturityDate:       req.MaturityDate,
		CouponRate:         req.CouponRate,
		CouponFrequency:    req.CouponFrequency,
		ParValue:           req.ParValue,
		DayCountConvention: req.DayCountConvention,
		TradingStatus:      types.TradingStatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.bondStore.Create(bond); err != nil {
		s.writeError(w, http.StatusConflict, "CONFLICT", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusCreated, bond)
}

func (s *Server) handleGetBond(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/securities/bonds/")
	id = strings.TrimSuffix(id, "/")
	bond, err := s.bondStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "bond not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	s.writeJSON(w, http.StatusOK, bond)
}

// handleBondAccruedInterest computes accrued interest for a bond using the bond's
// day-count convention and a settlement_date query parameter.
//
// Accrued interest = coupon_rate * par_value * (days / basis)
//
// where basis is:
//   - ACT/360: actual days in period / 360
//   - ACT/365: actual days in period / 365
//   - 30/360:  months * 30 / 360 (standardised 30-day months)
func (s *Server) handleBondAccruedInterest(w http.ResponseWriter, r *http.Request) {
	// Extract bond ID.
	path := strings.TrimSuffix(r.URL.Path, "/")
	path = strings.TrimSuffix(path, "/accrued-interest")
	id := strings.TrimPrefix(path, "/api/v1/securities/bonds/")

	bond, err := s.bondStore.Get(id)
	if err == store.ErrNotFound {
		s.writeError(w, http.StatusNotFound, "NOT_FOUND", "bond not found", nil)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	// Parse last_coupon_date and settlement_date from query params.
	lastCouponStr := r.URL.Query().Get("last_coupon_date")
	settlementStr := r.URL.Query().Get("settlement_date")
	if lastCouponStr == "" || settlementStr == "" {
		s.writeError(w, http.StatusBadRequest, "MISSING_PARAMS",
			"last_coupon_date and settlement_date query parameters are required", nil)
		return
	}

	lastCoupon, err1 := time.Parse("2006-01-02", lastCouponStr)
	settlement, err2 := time.Parse("2006-01-02", settlementStr)
	if err1 != nil || err2 != nil {
		s.writeError(w, http.StatusBadRequest, "INVALID_DATE", "dates must be in YYYY-MM-DD format", nil)
		return
	}

	accrued := calcAccruedInterest(bond.CouponRate, bond.ParValue, bond.DayCountConvention, lastCoupon, settlement)

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"bond_id":              bond.ID,
		"last_coupon_date":     lastCouponStr,
		"settlement_date":      settlementStr,
		"coupon_rate":          bond.CouponRate,
		"par_value":            bond.ParValue,
		"day_count_convention": string(bond.DayCountConvention),
		"accrued_interest":     roundTo2DP(accrued.Float64()),
	})
}

// calcAccruedInterest computes accrued interest given the bond parameters and dates.
// The formula is: couponRate * parValue * dayFraction
// where dayFraction depends on the convention:
//   - ACT/360: actual days / 360
//   - ACT/365: actual days / 365
//   - 30/360:  standardised days (30-day months) / 360
func calcAccruedInterest(couponRate float64, parValue types.Decimal, conv types.DayCountConvention, lastCoupon, settlement time.Time) types.Decimal {
	var days, basis int64
	switch conv {
	case types.DayCountACT360:
		days = int64(math.Round(settlement.Sub(lastCoupon).Hours() / 24))
		basis = 360
	case types.DayCountACT365:
		days = int64(math.Round(settlement.Sub(lastCoupon).Hours() / 24))
		basis = 365
	case types.DayCount30360:
		// 30/360: each month is treated as 30 days.
		y1, m1, d1 := lastCoupon.Date()
		y2, m2, d2 := settlement.Date()
		// Clamp day values to 30.
		if d1 > 30 {
			d1 = 30
		}
		if d2 > 30 && d1 == 30 {
			d2 = 30
		}
		days = int64(360*(y2-y1) + 30*(int(m2)-int(m1)) + (d2 - d1))
		basis = 360
	default:
		// Fall back to ACT/365 for unknown conventions.
		days = int64(math.Round(settlement.Sub(lastCoupon).Hours() / 24))
		basis = 365
	}
	// Money-correct decomposition: accrued = parValue * couponRate * days / basis.
	// couponRate is a genuine ratio (e.g. 0.05) converted to a Decimal at the
	// boundary; days and basis are exact integers. Multiplying through and
	// dividing by the basis LAST keeps full fixed-point precision (folding the
	// day-fraction into one 4dp factor would lose too much for sub-1% fractions).
	couponFactor, err := decimal.NewFromFloat(couponRate)
	if err != nil {
		return types.Decimal{}
	}
	return parValue.MulDecimal(couponFactor).MulInt64(days).DivInt64(basis)
}

// roundTo2DP rounds a float64 to 2 decimal places.
func roundTo2DP(v float64) float64 {
	return math.Round(v*100) / 100
}
