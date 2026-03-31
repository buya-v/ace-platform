package collateral

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/margin-engine/internal/types"
)

// testPriceProvider returns a price provider that uses a fixed map.
func testPriceProvider(prices map[string]string) PriceProvider {
	return func(commodityID string) (types.Decimal, bool) {
		s, ok := prices[commodityID]
		if !ok {
			return types.DecimalZero(), false
		}
		d, err := types.ParseDecimal(s)
		if err != nil {
			return types.DecimalZero(), false
		}
		return d, true
	}
}

func TestCalculateReceiptCollateral_Empty(t *testing.T) {
	pp := testPriceProvider(nil)
	src := NewWarehouseCollateralSource("localhost:1", pp)

	result := src.CalculateReceiptCollateral(nil)
	if !result.IsZero() {
		t.Errorf("expected zero for nil receipts, got %s", result.String())
	}

	result = src.CalculateReceiptCollateral([]pledgedReceipt{})
	if !result.IsZero() {
		t.Errorf("expected zero for empty receipts, got %s", result.String())
	}
}

func TestCalculateReceiptCollateral_SingleReceipt(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	src := NewWarehouseCollateralSource("localhost:1", pp) // default 80% haircut

	receipts := []pledgedReceipt{
		{
			ReceiptID:   "R1",
			CommodityID: "WHEAT",
			Quantity:    "100",
			Status:      "PLEDGED",
		},
	}

	result := src.CalculateReceiptCollateral(receipts)
	// 100 * 1000 * 0.80 = 80000
	expected := types.DecimalFromInt(80000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateReceiptCollateral_MultipleReceipts(t *testing.T) {
	pp := testPriceProvider(map[string]string{
		"WHEAT":  "1000",
		"COFFEE": "2000",
	})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "50"},
		{ReceiptID: "R2", CommodityID: "COFFEE", Quantity: "30"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	// (50 * 1000 * 0.80) + (30 * 2000 * 0.80) = 40000 + 48000 = 88000
	expected := types.DecimalFromInt(88000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateReceiptCollateral_UnknownCommoditySkipped(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "500"})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "10"},
		{ReceiptID: "R2", CommodityID: "UNKNOWN", Quantity: "100"}, // no price
	}

	result := src.CalculateReceiptCollateral(receipts)
	// Only WHEAT: 10 * 500 * 0.80 = 4000
	expected := types.DecimalFromInt(4000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateReceiptCollateral_InvalidQuantitySkipped(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "500"})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "not-a-number"},
		{ReceiptID: "R2", CommodityID: "WHEAT", Quantity: "20"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	// Only R2: 20 * 500 * 0.80 = 8000
	expected := types.DecimalFromInt(8000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateReceiptCollateral_ZeroQuantitySkipped(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "500"})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "0"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	if !result.IsZero() {
		t.Errorf("expected zero for zero quantity, got %s", result.String())
	}
}

func TestCalculateReceiptCollateral_NegativeQuantitySkipped(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "500"})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "-10"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	if !result.IsZero() {
		t.Errorf("expected zero for negative quantity, got %s", result.String())
	}
}

func TestCalculateReceiptCollateral_CustomHaircut(t *testing.T) {
	pp := testPriceProvider(map[string]string{"SESAME": "2000"})
	src := NewWarehouseCollateralSource("localhost:1", pp, WithHaircut(0.60))

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "SESAME", Quantity: "100"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	// 100 * 2000 * 0.60 = 120000
	expected := types.DecimalFromInt(120000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateReceiptCollateral_InvalidHaircutIgnored(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})

	// Haircut of 0 should be ignored, keeping default 0.80
	src := NewWarehouseCollateralSource("localhost:1", pp, WithHaircut(0))

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "100"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	expected := types.DecimalFromInt(80000) // default 80%
	if !result.Equal(expected) {
		t.Errorf("expected %s (default haircut), got %s", expected.String(), result.String())
	}

	// Haircut > 1.0 should also be ignored
	src2 := NewWarehouseCollateralSource("localhost:1", pp, WithHaircut(1.5))
	result2 := src2.CalculateReceiptCollateral(receipts)
	if !result2.Equal(expected) {
		t.Errorf("expected %s (default haircut), got %s", expected.String(), result2.String())
	}
}

func TestWarehouseCollateralSource_HTTPSuccess(t *testing.T) {
	receipts := []pledgedReceipt{
		{ReceiptID: "R1", HolderID: "P1", CommodityID: "WHEAT", Quantity: "50", Status: "PLEDGED"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/receipts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		holderID := r.URL.Query().Get("holder_id")
		if holderID != "P1" {
			t.Errorf("expected holder_id=P1, got %s", holderID)
		}
		status := r.URL.Query().Get("status")
		if status != "PLEDGED" {
			t.Errorf("expected status=PLEDGED, got %s", status)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(warehouseReceiptsResponse{
			Receipts: receipts,
			Count:    len(receipts),
		})
	}))
	defer server.Close()

	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	addr := server.URL[7:] // strip "http://"
	src := NewWarehouseCollateralSource(addr, pp)

	result := src.GetCollateral("P1")
	// 50 * 1000 * 0.80 = 40000
	expected := types.DecimalFromInt(40000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestWarehouseCollateralSource_HTTPServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	addr := server.URL[7:]
	src := NewWarehouseCollateralSource(addr, pp)

	result := src.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero on server error, got %s", result.String())
	}
}

func TestWarehouseCollateralSource_HTTPUnreachable(t *testing.T) {
	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	src := NewWarehouseCollateralSource("localhost:1", pp) // unlikely to be listening

	result := src.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero when unreachable, got %s", result.String())
	}
}

func TestWarehouseCollateralSource_HTTPInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	addr := server.URL[7:]
	src := NewWarehouseCollateralSource(addr, pp)

	result := src.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero on invalid JSON, got %s", result.String())
	}
}

func TestWarehouseCollateralSource_HTTPEmptyReceipts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(warehouseReceiptsResponse{
			Receipts: []pledgedReceipt{},
			Count:    0,
		})
	}))
	defer server.Close()

	pp := testPriceProvider(map[string]string{"WHEAT": "1000"})
	addr := server.URL[7:]
	src := NewWarehouseCollateralSource(addr, pp)

	result := src.GetCollateral("P1")
	if !result.IsZero() {
		t.Errorf("expected zero for empty receipts, got %s", result.String())
	}
}

func TestWarehouseCollateralSource_HTTPMultipleReceipts(t *testing.T) {
	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "WHEAT", Quantity: "100"},
		{ReceiptID: "R2", CommodityID: "CORN", Quantity: "200"},
		{ReceiptID: "R3", CommodityID: "UNKNOWN_COMMODITY", Quantity: "500"}, // no price
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(warehouseReceiptsResponse{
			Receipts: receipts,
			Count:    len(receipts),
		})
	}))
	defer server.Close()

	pp := testPriceProvider(map[string]string{
		"WHEAT": "500",
		"CORN":  "300",
	})
	addr := server.URL[7:]
	src := NewWarehouseCollateralSource(addr, pp)

	result := src.GetCollateral("P1")
	// WHEAT: 100 * 500 * 0.80 = 40000
	// CORN: 200 * 300 * 0.80 = 48000
	// UNKNOWN: skipped
	// Total: 88000
	expected := types.DecimalFromInt(88000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestDefaultHaircut(t *testing.T) {
	if DefaultHaircut != 0.80 {
		t.Errorf("expected DefaultHaircut 0.80, got %f", DefaultHaircut)
	}
}

func TestWithHaircutOption(t *testing.T) {
	pp := testPriceProvider(nil)
	src := NewWarehouseCollateralSource("localhost:1", pp, WithHaircut(0.70))
	if src.haircut != 0.70 {
		t.Errorf("expected haircut 0.70, got %f", src.haircut)
	}
}

func TestFractionalQuantity(t *testing.T) {
	pp := testPriceProvider(map[string]string{"SESAME": "3000"})
	src := NewWarehouseCollateralSource("localhost:1", pp)

	receipts := []pledgedReceipt{
		{ReceiptID: "R1", CommodityID: "SESAME", Quantity: "10.5"},
	}

	result := src.CalculateReceiptCollateral(receipts)
	// 10.5 * 3000 * 0.80 = 25200
	expected := types.DecimalFromInt(25200)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}
