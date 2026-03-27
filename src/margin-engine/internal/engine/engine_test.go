package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ace-platform/margin-engine/internal/params"
	"github.com/ace-platform/margin-engine/internal/types"
)

type testIDGen struct {
	counter uint64
}

func (g *testIDGen) NewID() string {
	n := atomic.AddUint64(&g.counter, 1)
	return fmt.Sprintf("test-mc-%d", n)
}

type testCollateral struct {
	mu         sync.RWMutex
	collateral map[string]types.Decimal
}

func newTestCollateral() *testCollateral {
	return &testCollateral{collateral: make(map[string]types.Decimal)}
}

func (c *testCollateral) Set(participantID string, amount types.Decimal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.collateral[participantID] = amount
}

func (c *testCollateral) GetCollateral(participantID string) types.Decimal {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if d, ok := c.collateral[participantID]; ok {
		return d
	}
	return types.DecimalZero()
}

func setupEngine() (*Engine, *testCollateral) {
	ps := params.NewStore()
	ps.Set(params.InstrumentParams{
		InstrumentID:   "CORN-2026-07",
		PriceScanRange: types.NewDecimal(3, 0),
		VolScanRange:   types.NewDecimal(0, 5000),
		SpotPrice:      types.NewDecimal(450, 0),
		ContractSize:   5000,
	})
	ps.Set(params.InstrumentParams{
		InstrumentID:    "WHEAT-2026-09",
		PriceScanRange:  types.NewDecimal(4, 0),
		VolScanRange:    types.NewDecimal(0, 5000),
		SpotPrice:       types.NewDecimal(600, 0),
		ContractSize:    5000,
		DeliveryCharge:  types.NewDecimal(0, 500),
		IsDeliveryMonth: true,
	})

	col := newTestCollateral()
	eng := NewEngine(ps, &testIDGen{}, col, 1*time.Hour)
	return eng, col
}

func TestCalculateMarginBasic(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000)) // $1M collateral

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   10,
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm.ParticipantID != "P1" {
		t.Errorf("expected P1, got %s", pm.ParticipantID)
	}
	if pm.TotalRequired.IsZero() {
		t.Error("total required should be non-zero")
	}
}

func TestCalculateMarginTriggersCall(t *testing.T) {
	eng, _ := setupEngine() // Zero collateral

	var receivedCall *types.MarginCall
	eng.SetMarginCallHandler(func(call types.MarginCall) {
		receivedCall = &call
	})

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   10,
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pm.ExcessDeficit.IsNeg() {
		t.Error("should have deficit with zero collateral")
	}
	if receivedCall == nil {
		t.Fatal("margin call should have been issued")
	}
	if receivedCall.ParticipantID != "P1" {
		t.Errorf("margin call for wrong participant: %s", receivedCall.ParticipantID)
	}
}

func TestCalculateMarginNoCallWithSufficientCollateral(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(100000000)) // $100M — plenty

	var callIssued bool
	eng.SetMarginCallHandler(func(call types.MarginCall) {
		callIssued = true
	})

	positions := []types.Position{
		{
			ParticipantID: "P1",
			InstrumentID:  "CORN-2026-07",
			NetQuantity:   1,
			AvgEntryPrice: types.NewDecimal(450, 0),
		},
	}

	_, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callIssued {
		t.Error("no margin call should be issued with sufficient collateral")
	}
}

func TestGetPortfolioMarginCached(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 5, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	eng.CalculateMargin("P1", positions)

	pm, ok := eng.GetPortfolioMargin("P1")
	if !ok {
		t.Fatal("should have cached portfolio margin")
	}
	if pm.ParticipantID != "P1" {
		t.Errorf("expected P1, got %s", pm.ParticipantID)
	}
}

func TestGetPortfolioMarginNotFound(t *testing.T) {
	eng, _ := setupEngine()
	_, ok := eng.GetPortfolioMargin("UNKNOWN")
	if ok {
		t.Error("should not find margin for unknown participant")
	}
}

func TestUpdateSpotPrice(t *testing.T) {
	eng, _ := setupEngine()

	newPrice := types.NewDecimal(500, 0)
	if err := eng.UpdateSpotPrice("CORN-2026-07", newPrice); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, _ := eng.GetParamStore().Get("CORN-2026-07")
	if !p.SpotPrice.Equal(newPrice) {
		t.Errorf("spot price not updated: expected %s, got %s", newPrice.String(), p.SpotPrice.String())
	}
}

func TestCheckDeadlines(t *testing.T) {
	eng, _ := setupEngine()

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions)

	// Before deadline
	breached := eng.CheckDeadlines(time.Now())
	if len(breached) != 0 {
		t.Errorf("no breaches expected before deadline, got %d", len(breached))
	}

	// After deadline
	breached = eng.CheckDeadlines(time.Now().Add(2 * time.Hour))
	if len(breached) != 1 {
		t.Errorf("expected 1 breach after deadline, got %d", len(breached))
	}
}

func TestConcurrentCalculation(t *testing.T) {
	eng, col := setupEngine()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pid := fmt.Sprintf("P%d", id)
			col.Set(pid, types.DecimalFromInt(int64(id*100000)))

			positions := []types.Position{
				{
					ParticipantID: pid,
					InstrumentID:  "CORN-2026-07",
					NetQuantity:   int64(id + 1),
					AvgEntryPrice: types.NewDecimal(450, 0),
				},
			}
			_, err := eng.CalculateMargin(pid, positions)
			if err != nil {
				t.Errorf("concurrent calc failed for %s: %v", pid, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all cached
	for i := 0; i < 20; i++ {
		pid := fmt.Sprintf("P%d", i)
		_, ok := eng.GetPortfolioMargin(pid)
		if !ok {
			t.Errorf("missing cached margin for %s", pid)
		}
	}
}

func TestMultiInstrumentPortfolio(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(5000000)) // $5M

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
		{ParticipantID: "P1", InstrumentID: "WHEAT-2026-09", NetQuantity: -5, AvgEntryPrice: types.NewDecimal(600, 0)},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pm.Requirements) != 2 {
		t.Errorf("expected 2 requirements, got %d", len(pm.Requirements))
	}
	if pm.TotalRequired.IsZero() {
		t.Error("total required should be non-zero for multi-instrument portfolio")
	}
}
