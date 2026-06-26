package collateral

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/garudax-platform/decimal"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// DefaultHaircut is the default collateral haircut applied to warehouse receipts.
// 80% means 1000 MNT of grain provides 800 MNT of collateral value.
// This protects against price drops between margin calculation and liquidation.
const DefaultHaircut = 0.80

// pledgedReceipt mirrors the warehouse-service Receipt JSON response.
// Only the fields relevant for collateral valuation are included.
type pledgedReceipt struct {
	ReceiptID   string `json:"ReceiptID"`
	HolderID    string `json:"HolderID"`
	CommodityID string `json:"CommodityID"`
	Quantity    string `json:"Quantity"`
	Status      string `json:"Status"`
	PledgedTo   string `json:"PledgedTo"`
}

// warehouseReceiptsResponse matches the warehouse-service /receipts endpoint
// JSON envelope: {"receipts": [...], "count": N}.
type warehouseReceiptsResponse struct {
	Receipts []pledgedReceipt `json:"receipts"`
	Count    int              `json:"count"`
}

// PriceProvider returns the current settlement/mark price for a commodity.
// This allows the warehouse collateral source to value receipts at market price.
type PriceProvider func(commodityID string) (types.Decimal, bool)

// WarehouseCollateralSource fetches pledged warehouse receipts from the
// warehouse-service HTTP API and calculates their collateral value.
//
// Collateral value = sum(receipt.quantity * settlement_price * haircut)
//
// This is a key feature for agricultural commodity exchanges (ECX, EAX model):
// a farmer stores grain in a certified warehouse, gets a receipt, pledges it,
// and the margin requirement is reduced by the haircut-adjusted value.
type WarehouseCollateralSource struct {
	baseURL       string
	client        *http.Client
	haircut       float64
	priceProvider PriceProvider
}

// WarehouseOption configures a WarehouseCollateralSource.
type WarehouseOption func(*WarehouseCollateralSource)

// WithHaircut sets a custom haircut percentage (0.0 - 1.0).
func WithHaircut(h float64) WarehouseOption {
	return func(s *WarehouseCollateralSource) {
		if h > 0 && h <= 1.0 {
			s.haircut = h
		}
	}
}

// WithHTTPClient sets a custom HTTP client (useful for testing).
func WithHTTPClient(c *http.Client) WarehouseOption {
	return func(s *WarehouseCollateralSource) {
		s.client = c
	}
}

// NewWarehouseCollateralSource creates a collateral source that queries the
// warehouse-service for pledged receipts and values them using the price provider.
// addr should be host:port, e.g. "warehouse-service:8088".
func NewWarehouseCollateralSource(addr string, priceProvider PriceProvider, opts ...WarehouseOption) *WarehouseCollateralSource {
	s := &WarehouseCollateralSource{
		baseURL: fmt.Sprintf("http://%s", addr),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		haircut:       DefaultHaircut,
		priceProvider: priceProvider,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetCollateral returns the total collateral value of pledged warehouse receipts
// for the given participant. Returns zero on any error (best-effort).
func (s *WarehouseCollateralSource) GetCollateral(participantID string) types.Decimal {
	url := fmt.Sprintf("%s/receipts?holder_id=%s&status=PLEDGED", s.baseURL, participantID)

	resp, err := s.client.Get(url)
	if err != nil {
		log.Printf("collateral: warehouse-service unreachable for %s: %v", participantID, err)
		return types.DecimalZero()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("collateral: warehouse-service returned %d for %s", resp.StatusCode, participantID)
		return types.DecimalZero()
	}

	var envelope warehouseReceiptsResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		log.Printf("collateral: failed to decode warehouse receipts for %s: %v", participantID, err)
		return types.DecimalZero()
	}

	return s.calculateReceiptCollateral(envelope.Receipts)
}

// calculateReceiptCollateral computes collateral value from pledged receipts:
//
//	collateral = sum(quantity * settlement_price * haircut)
//
// Receipts for commodities with unknown prices are skipped (valued at zero).
func (s *WarehouseCollateralSource) calculateReceiptCollateral(receipts []pledgedReceipt) types.Decimal {
	total := types.DecimalZero()

	// haircut as Decimal: e.g. 0.80 -> 0.8000. Convert through the shared
	// decimal's half-even NewFromFloat rather than int64(haircut*10000), which
	// truncates toward zero (R006 money-path audit). haircut is bounded (0,1] by
	// WithHaircut/DefaultHaircut, so NewFromFloat cannot fail on a finite value;
	// fall back to a zero haircut (no collateral credit) if it ever does.
	haircutDecimal, err := decimal.NewFromFloat(s.haircut)
	if err != nil {
		log.Printf("collateral: invalid haircut %v: %v", s.haircut, err)
		return types.DecimalZero()
	}

	for _, r := range receipts {
		qty, err := types.ParseDecimal(r.Quantity)
		if err != nil || qty.IsZero() || qty.IsNeg() {
			continue
		}

		price, ok := s.priceProvider(r.CommodityID)
		if !ok || price.IsZero() {
			// No settlement price available for this commodity; skip
			log.Printf("collateral: no price for commodity %s (receipt %s), skipping", r.CommodityID, r.ReceiptID)
			continue
		}

		// value = quantity * price * haircut
		value := qty.MulDecimal(price).MulDecimal(haircutDecimal)
		total = total.Add(value)
	}

	return total
}

// CalculateReceiptCollateral is exported for testing. It computes collateral
// value for a slice of receipts using the source's price provider and haircut.
func (s *WarehouseCollateralSource) CalculateReceiptCollateral(receipts []pledgedReceipt) types.Decimal {
	return s.calculateReceiptCollateral(receipts)
}
