package collateral

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// clearingPosition mirrors the clearing-engine's Position JSON response.
// We only need the fields relevant for collateral calculation.
type clearingPosition struct {
	ParticipantID string `json:"ParticipantID"`
	InstrumentID  string `json:"InstrumentID"`
	NetQuantity   int64  `json:"NetQuantity"`
	AvgEntryPrice string `json:"AvgEntryPrice"`
}

// HTTPCollateralSource fetches positions from the clearing-engine HTTP API
// and calculates collateral as the sum of absolute position values.
// This is a simplified proxy: collateral = sum(|net_qty * avg_entry_price|).
// Falls back to zero if clearing-engine is unreachable.
type HTTPCollateralSource struct {
	baseURL string
	client  *http.Client
}

// NewHTTPCollateralSource creates a collateral source that queries the clearing-engine.
// addr should be host:port, e.g. "clearing-engine:8082".
func NewHTTPCollateralSource(addr string) *HTTPCollateralSource {
	return &HTTPCollateralSource{
		baseURL: fmt.Sprintf("http://%s", addr),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// GetCollateral returns the estimated collateral for a participant.
// It fetches positions from the clearing-engine and sums |net_qty * avg_price|.
// Returns zero on any error (best-effort).
func (s *HTTPCollateralSource) GetCollateral(participantID string) types.Decimal {
	url := fmt.Sprintf("%s/positions?participant_id=%s", s.baseURL, participantID)

	resp, err := s.client.Get(url)
	if err != nil {
		log.Printf("collateral: clearing-engine unreachable for %s: %v", participantID, err)
		return types.DecimalZero()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("collateral: clearing-engine returned %d for %s", resp.StatusCode, participantID)
		return types.DecimalZero()
	}

	var positions []clearingPosition
	if err := json.NewDecoder(resp.Body).Decode(&positions); err != nil {
		log.Printf("collateral: failed to decode positions for %s: %v", participantID, err)
		return types.DecimalZero()
	}

	return CalculateCollateral(positions)
}

// CalculateCollateral computes collateral as sum of |net_qty * avg_entry_price|.
// Exported for testing.
func CalculateCollateral(positions []clearingPosition) types.Decimal {
	total := types.DecimalZero()
	for _, pos := range positions {
		price, err := types.ParseDecimal(pos.AvgEntryPrice)
		if err != nil {
			continue
		}
		// position value = |net_qty| * avg_entry_price
		qty := pos.NetQuantity
		if qty < 0 {
			qty = -qty
		}
		posValue := price.MulInt64(qty)
		total = total.Add(posValue)
	}
	return total
}
