package collateral

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/garudax-platform/margin-engine/internal/types"
)

func TestCalculateCollateral_Empty(t *testing.T) {
	result := CalculateCollateral(nil)
	if !result.IsZero() {
		t.Errorf("expected zero for nil positions, got %s", result.String())
	}

	result = CalculateCollateral([]clearingPosition{})
	if !result.IsZero() {
		t.Errorf("expected zero for empty positions, got %s", result.String())
	}
}

func TestCalculateCollateral_SingleLong(t *testing.T) {
	positions := []clearingPosition{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   10,
			AvgEntryPrice: "450.0000",
		},
	}
	result := CalculateCollateral(positions)
	// 10 * 450 = 4500
	expected := types.DecimalFromInt(4500)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateCollateral_SingleShort(t *testing.T) {
	positions := []clearingPosition{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   -5,
			AvgEntryPrice: "600.0000",
		},
	}
	result := CalculateCollateral(positions)
	// |−5| * 600 = 3000
	expected := types.DecimalFromInt(3000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateCollateral_MultiplePositions(t *testing.T) {
	positions := []clearingPosition{
		{NetQuantity: 10, AvgEntryPrice: "100"},
		{NetQuantity: -20, AvgEntryPrice: "50"},
	}
	result := CalculateCollateral(positions)
	// 10*100 + 20*50 = 1000 + 1000 = 2000
	expected := types.DecimalFromInt(2000)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateCollateral_InvalidPrice(t *testing.T) {
	positions := []clearingPosition{
		{NetQuantity: 10, AvgEntryPrice: "invalid"},
		{NetQuantity: 5, AvgEntryPrice: "100"},
	}
	result := CalculateCollateral(positions)
	// Only the valid position counts: 5 * 100 = 500
	expected := types.DecimalFromInt(500)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestCalculateCollateral_ZeroQuantity(t *testing.T) {
	positions := []clearingPosition{
		{NetQuantity: 0, AvgEntryPrice: "100"},
	}
	result := CalculateCollateral(positions)
	if !result.IsZero() {
		t.Errorf("expected zero for zero quantity, got %s", result.String())
	}
}

func TestHTTPCollateralSource_Success(t *testing.T) {
	positions := []clearingPosition{
		{ParticipantID: "P1", InstrumentID: "CORN", NetQuantity: 10, AvgEntryPrice: "450"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/positions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		pid := r.URL.Query().Get("participant_id")
		if pid != "P1" {
			t.Errorf("unexpected participant_id: %s", pid)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(positions)
	}))
	defer server.Close()

	// Extract host:port from test server URL (strip "http://")
	addr := server.URL[7:]
	source := NewHTTPCollateralSource(addr)
	result := source.GetCollateral("P1")

	expected := types.DecimalFromInt(4500)
	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

func TestHTTPCollateralSource_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	addr := server.URL[7:]
	source := NewHTTPCollateralSource(addr)
	result := source.GetCollateral("P1")

	if !result.IsZero() {
		t.Errorf("expected zero on server error, got %s", result.String())
	}
}

func TestHTTPCollateralSource_Unreachable(t *testing.T) {
	source := NewHTTPCollateralSource("localhost:1") // unlikely to be listening
	result := source.GetCollateral("P1")

	if !result.IsZero() {
		t.Errorf("expected zero when unreachable, got %s", result.String())
	}
}

func TestHTTPCollateralSource_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	addr := server.URL[7:]
	source := NewHTTPCollateralSource(addr)
	result := source.GetCollateral("P1")

	if !result.IsZero() {
		t.Errorf("expected zero on invalid JSON, got %s", result.String())
	}
}

func TestHTTPCollateralSource_EmptyPositions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]clearingPosition{})
	}))
	defer server.Close()

	addr := server.URL[7:]
	source := NewHTTPCollateralSource(addr)
	result := source.GetCollateral("P1")

	if !result.IsZero() {
		t.Errorf("expected zero for empty positions, got %s", result.String())
	}
}
