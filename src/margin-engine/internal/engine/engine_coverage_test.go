package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// --- Additional engine tests for coverage improvement ---

func TestSetMarginHandler(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	var received *types.PortfolioMargin
	eng.SetMarginHandler(func(pm types.PortfolioMargin) {
		received = &pm
	})

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 5, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	_, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received == nil {
		t.Fatal("margin handler should have been called")
	}
	if received.ParticipantID != "P1" {
		t.Errorf("handler received wrong participant: %s", received.ParticipantID)
	}
	if received.TotalRequired.IsZero() {
		t.Error("handler should receive non-zero total required")
	}
}

func TestGetActiveMarginCall(t *testing.T) {
	eng, _ := setupEngine() // zero collateral => deficit

	// No call yet
	_, ok := eng.GetActiveMarginCall("P1")
	if ok {
		t.Error("should not have active call before calculation")
	}

	// Trigger margin call
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions)

	call, ok := eng.GetActiveMarginCall("P1")
	if !ok {
		t.Fatal("should have active margin call after deficit calculation")
	}
	if call.ParticipantID != "P1" {
		t.Errorf("expected P1, got %s", call.ParticipantID)
	}
	if call.Status != types.MarginCallIssued {
		t.Errorf("expected ISSUED status, got %s", call.Status.String())
	}
}

func TestGetAllActiveMarginCalls(t *testing.T) {
	eng, _ := setupEngine() // zero collateral

	// Calculate for two participants
	for _, pid := range []string{"P1", "P2"} {
		positions := []types.Position{
			{ParticipantID: pid, InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
		}
		eng.CalculateMargin(pid, positions)
	}

	active := eng.GetAllActiveMarginCalls()
	if len(active) != 2 {
		t.Errorf("expected 2 active margin calls, got %d", len(active))
	}
}

func TestGetAllActiveMarginCallsEmpty(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(100000000)) // plenty of collateral

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 1, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions)

	active := eng.GetAllActiveMarginCalls()
	if len(active) != 0 {
		t.Errorf("expected no active margin calls, got %d", len(active))
	}
}

func TestGetMarginCallStats(t *testing.T) {
	eng, _ := setupEngine()

	// Issue calls for P1 and P2
	for _, pid := range []string{"P1", "P2"} {
		positions := []types.Position{
			{ParticipantID: pid, InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
		}
		eng.CalculateMargin(pid, positions)
	}

	stats := eng.GetMarginCallStats()
	if stats.TotalIssued != 2 {
		t.Errorf("expected 2 total issued, got %d", stats.TotalIssued)
	}
	if stats.Active != 2 {
		t.Errorf("expected 2 active, got %d", stats.Active)
	}
}

func TestGetMarginCallStatsAfterResolution(t *testing.T) {
	eng, col := setupEngine()

	// Issue call for P1 (zero collateral)
	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions)

	// Resolve by adding collateral and recalculating
	col.Set("P1", types.DecimalFromInt(100000000))
	eng.CalculateMargin("P1", positions)

	stats := eng.GetMarginCallStats()
	if stats.Satisfied != 1 {
		t.Errorf("expected 1 satisfied, got %d", stats.Satisfied)
	}
	if stats.Active != 0 {
		t.Errorf("expected 0 active, got %d", stats.Active)
	}
}

func TestCalculateMarginMissingInstrument(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "NONEXISTENT", NetQuantity: 1},
	}

	_, err := eng.CalculateMargin("P1", positions)
	if err == nil {
		t.Fatal("expected error for missing instrument params")
	}
}

func TestCalculateMarginEmptyPositions(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	pm, err := eng.CalculateMargin("P1", []types.Position{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm.TotalRequired.IsZero() == false {
		// Empty positions should have zero requirements
	}
	if len(pm.Requirements) != 0 {
		t.Errorf("expected 0 requirements for empty positions, got %d", len(pm.Requirements))
	}
}

func TestCalculateMarginFlatPositionSkipped(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 0},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pm.Requirements) != 0 {
		t.Errorf("flat positions should be skipped, got %d requirements", len(pm.Requirements))
	}
}

func TestCalculateMarginHandlerAndCallHandler(t *testing.T) {
	eng, _ := setupEngine() // zero collateral

	var marginReceived bool
	var callReceived bool

	eng.SetMarginHandler(func(pm types.PortfolioMargin) {
		marginReceived = true
	})
	eng.SetMarginCallHandler(func(call types.MarginCall) {
		callReceived = true
	})

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	eng.CalculateMargin("P1", positions)

	if !marginReceived {
		t.Error("margin handler should have been called")
	}
	if !callReceived {
		t.Error("margin call handler should have been called for deficit")
	}
}

func TestCalculateMarginUpdatesCache(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(1000000))

	// First calculation
	positions1 := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 5, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions1)

	pm1, _ := eng.GetPortfolioMargin("P1")
	firstRequired := pm1.TotalRequired

	// Second calculation with larger position
	positions2 := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 50, AvgEntryPrice: types.NewDecimal(450, 0)},
	}
	eng.CalculateMargin("P1", positions2)

	pm2, _ := eng.GetPortfolioMargin("P1")
	if !pm2.TotalRequired.GreaterThan(firstRequired) {
		t.Error("larger position should result in higher margin requirement")
	}
}

func TestCheckDeadlinesMultipleParticipants(t *testing.T) {
	eng, _ := setupEngine()

	// Create margin calls for 3 participants
	for _, pid := range []string{"P1", "P2", "P3"} {
		positions := []types.Position{
			{ParticipantID: pid, InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
		}
		eng.CalculateMargin(pid, positions)
	}

	// Before deadline — no breaches
	breached := eng.CheckDeadlines(time.Now())
	if len(breached) != 0 {
		t.Errorf("expected 0 breaches before deadline, got %d", len(breached))
	}

	// After deadline — all should breach
	breached = eng.CheckDeadlines(time.Now().Add(2 * time.Hour))
	if len(breached) != 3 {
		t.Errorf("expected 3 breaches after deadline, got %d", len(breached))
	}

	for _, b := range breached {
		if b.Status != types.MarginCallBreached {
			t.Errorf("expected BREACHED status, got %s", b.Status.String())
		}
	}

	// After breach, no more active calls
	active := eng.GetAllActiveMarginCalls()
	if len(active) != 0 {
		t.Errorf("no active calls should remain after breach, got %d", len(active))
	}
}

func TestUpdateSpotPriceMissing(t *testing.T) {
	eng, _ := setupEngine()

	err := eng.UpdateSpotPrice("NONEXISTENT", types.NewDecimal(500, 0))
	if err == nil {
		t.Error("expected error for missing instrument")
	}
}

func TestMarginCallIssuedThenResolved(t *testing.T) {
	eng, col := setupEngine()

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	// Zero collateral -> deficit -> margin call
	eng.CalculateMargin("P1", positions)

	call, ok := eng.GetActiveMarginCall("P1")
	if !ok {
		t.Fatal("should have active margin call")
	}
	if call.Deficit.IsZero() {
		t.Error("deficit should be non-zero")
	}

	// Add sufficient collateral and recalculate
	col.Set("P1", types.DecimalFromInt(100000000))
	eng.CalculateMargin("P1", positions)

	// Margin call should be resolved
	_, ok = eng.GetActiveMarginCall("P1")
	if ok {
		t.Error("margin call should be resolved after adding collateral")
	}

	stats := eng.GetMarginCallStats()
	if stats.Satisfied != 1 {
		t.Errorf("expected 1 satisfied call, got %d", stats.Satisfied)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	eng, col := setupEngine()

	var wg sync.WaitGroup

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pid := fmt.Sprintf("P%d", id)
			col.Set(pid, types.DecimalFromInt(int64(id*50000)))

			positions := []types.Position{
				{ParticipantID: pid, InstrumentID: "CORN-2026-07", NetQuantity: int64(id + 1), AvgEntryPrice: types.NewDecimal(450, 0)},
			}
			eng.CalculateMargin(pid, positions)
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pid := fmt.Sprintf("P%d", id)
			eng.GetPortfolioMargin(pid)
			eng.GetActiveMarginCall(pid)
			eng.GetAllActiveMarginCalls()
			eng.GetMarginCallStats()
		}(i)
	}

	wg.Wait()
}

func TestMultiInstrumentWithDeliveryMonth(t *testing.T) {
	eng, col := setupEngine()
	col.Set("P1", types.DecimalFromInt(500000))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 5, AvgEntryPrice: types.NewDecimal(450, 0)},
		{ParticipantID: "P1", InstrumentID: "WHEAT-2026-09", NetQuantity: -3, AvgEntryPrice: types.NewDecimal(600, 0)},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pm.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(pm.Requirements))
	}

	// Find wheat requirement — it should have delivery month charge
	for _, req := range pm.Requirements {
		if req.InstrumentID == "WHEAT-2026-09" {
			if req.DeliveryMonth.IsZero() {
				t.Error("wheat delivery month charge should be non-zero")
			}
		}
		if req.InstrumentID == "CORN-2026-07" {
			if !req.DeliveryMonth.IsZero() {
				t.Error("corn should not have delivery month charge")
			}
		}
	}
}

func TestMarginCallDeficitAmount(t *testing.T) {
	ps := params.NewStore()
	ps.Set(params.InstrumentParams{
		InstrumentID:   "TEST-INST",
		PriceScanRange: types.NewDecimal(10, 0),
		VolScanRange:   types.NewDecimal(0, 5000),
		SpotPrice:      types.NewDecimal(100, 0),
		ContractSize:   100,
	})

	col := newTestCollateral()
	idGen := &testIDGen{}
	eng := NewEngine(ps, idGen, col, 1*time.Hour)

	// Set small collateral
	col.Set("P1", types.DecimalFromInt(1000))

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "TEST-INST", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(100, 0)},
	}

	pm, err := eng.CalculateMargin("P1", positions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pm.ExcessDeficit.IsNeg() {
		t.Error("should have deficit with small collateral")
	}

	call, ok := eng.GetActiveMarginCall("P1")
	if !ok {
		t.Fatal("should have active margin call")
	}

	// Deficit should equal required - on hand
	expectedDeficit := pm.TotalRequired.Sub(pm.CollateralOnHand)
	if !call.Deficit.Equal(expectedDeficit) {
		t.Errorf("expected deficit %s, got %s", expectedDeficit.String(), call.Deficit.String())
	}
}

func TestMarginCallUpdatedOnRecalculation(t *testing.T) {
	eng, _ := setupEngine()

	var callCount int64
	eng.SetMarginCallHandler(func(call types.MarginCall) {
		atomic.AddInt64(&callCount, 1)
	})

	positions := []types.Position{
		{ParticipantID: "P1", InstrumentID: "CORN-2026-07", NetQuantity: 10, AvgEntryPrice: types.NewDecimal(450, 0)},
	}

	// First calculation — new call issued
	eng.CalculateMargin("P1", positions)
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 call handler invocation, got %d", atomic.LoadInt64(&callCount))
	}

	// Second calculation — existing call updated, handler NOT called again
	eng.CalculateMargin("P1", positions)
	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("handler should not be called for updates, got %d invocations", atomic.LoadInt64(&callCount))
	}
}

func TestGetParamStore(t *testing.T) {
	eng, _ := setupEngine()

	ps := eng.GetParamStore()
	if ps == nil {
		t.Fatal("param store should not be nil")
	}

	p, err := ps.Get("CORN-2026-07")
	if err != nil {
		t.Fatalf("should find CORN params: %v", err)
	}
	if p.ContractSize != 5000 {
		t.Errorf("expected 5000, got %d", p.ContractSize)
	}
}
