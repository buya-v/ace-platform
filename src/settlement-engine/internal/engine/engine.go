package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/payment"
	"github.com/garudax-platform/settlement-engine/internal/pnl"
	"github.com/garudax-platform/settlement-engine/internal/settlement"
	"github.com/garudax-platform/settlement-engine/internal/types"
	"github.com/garudax-platform/settlement-engine/internal/valuation"
)

// PositionSource provides positions from the clearing engine.
type PositionSource interface {
	GetAllPositions() []types.Position
}

// CycleHandler is called when a settlement cycle completes.
type CycleHandler func(cycle types.SettlementCycle)

// Engine orchestrates the daily mark-to-market settlement cycle.
type Engine struct {
	mu sync.Mutex

	priceStore    valuation.PriceStore
	pnlCalc       *pnl.Calculator
	generator     *settlement.Generator
	processor     *payment.Processor
	idGen         types.IDGenerator

	cycles   map[string]*types.SettlementCycle
	handler  CycleHandler
}

// NewEngine creates a new settlement engine.
func NewEngine(
	priceStore valuation.PriceStore,
	idGen types.IDGenerator,
	gateway payment.Gateway,
) *Engine {
	return &Engine{
		priceStore: priceStore,
		pnlCalc:    pnl.NewCalculator(priceStore),
		generator:  settlement.NewGenerator(idGen),
		processor:  payment.NewProcessor(gateway),
		idGen:      idGen,
		cycles:     make(map[string]*types.SettlementCycle),
	}
}

// SetCycleHandler sets a callback invoked when a settlement cycle completes.
func (e *Engine) SetCycleHandler(h CycleHandler) {
	e.handler = h
}

// RunSettlementCycle executes a full daily settlement cycle:
// 1. Fetch all positions
// 2. Value positions at settlement prices
// 3. Calculate daily P&L (variation margin)
// 4. Generate net settlement instructions per participant
// 5. Submit payments via the payment gateway
func (e *Engine) RunSettlementCycle(cycleID string, settleDate time.Time, positions []types.Position) (*types.SettlementCycle, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cycle := &types.SettlementCycle{
		CycleID:    cycleID,
		SettleDate: settleDate,
		Status:     types.CycleStatusValuing,
		StartedAt:  time.Now(),
	}
	e.cycles[cycleID] = cycle

	// Step 1: Calculate daily P&L for all positions
	cycle.Status = types.CycleStatusValuing
	pnlRecords, err := e.pnlCalc.CalculateBatch(positions, settleDate)
	if err != nil {
		cycle.Status = types.CycleStatusFailed
		cycle.Error = fmt.Sprintf("P&L calculation failed: %v", err)
		return cycle, err
	}
	cycle.PnLRecords = pnlRecords
	cycle.Status = types.CycleStatusCalculated

	// Step 2: Generate settlement instructions
	instructions := e.generator.Generate(cycleID, pnlRecords)
	payIn, payOut := settlement.Totals(instructions)
	cycle.TotalPayIn = payIn
	cycle.TotalPayOut = payOut

	// Step 3: Submit to payment gateway
	cycle.Status = types.CycleStatusSettling
	processed := e.processor.ProcessAll(instructions)
	cycle.Instructions = processed

	// Step 4: Determine final status
	allConfirmed := true
	for _, inst := range processed {
		if inst.Status != types.InstructionConfirmed {
			allConfirmed = false
			break
		}
	}

	if allConfirmed {
		cycle.Status = types.CycleStatusCompleted
	} else {
		cycle.Status = types.CycleStatusFailed
		cycle.Error = "one or more payment instructions failed"
	}
	cycle.CompletedAt = time.Now()

	if e.handler != nil {
		e.handler(*cycle)
	}

	return cycle, nil
}

// GetCycle returns a settlement cycle by ID.
func (e *Engine) GetCycle(cycleID string) (types.SettlementCycle, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	c, ok := e.cycles[cycleID]
	if !ok {
		return types.SettlementCycle{}, false
	}
	return *c, true
}

// GetAllCycles returns all settlement cycles.
func (e *Engine) GetAllCycles() []types.SettlementCycle {
	e.mu.Lock()
	defer e.mu.Unlock()

	result := make([]types.SettlementCycle, 0, len(e.cycles))
	for _, c := range e.cycles {
		result = append(result, *c)
	}
	return result
}

// GetPriceStore returns the price store for setting settlement prices.
func (e *Engine) GetPriceStore() valuation.PriceStore {
	return e.priceStore
}

// SetSettlementPrice is a convenience method to set the settlement price for an instrument.
func (e *Engine) SetSettlementPrice(instrumentID string, date time.Time, price types.Decimal) {
	e.priceStore.SetSettlementPrice(instrumentID, date, price)
}
