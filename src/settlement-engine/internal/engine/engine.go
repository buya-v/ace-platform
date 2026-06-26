package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/dvp"
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
	mu sync.RWMutex

	priceStore    valuation.PriceStore
	pnlCalc       *pnl.Calculator
	generator     *settlement.Generator
	processor     *payment.Processor
	idGen         types.IDGenerator
	dvpCoord      *dvp.DVPCoordinator

	instruments map[string]types.InstrumentConfig // instrumentID -> config
	cycles      map[string]*types.SettlementCycle
	handler     CycleHandler
}

// NewEngine creates a new settlement engine.
func NewEngine(
	priceStore valuation.PriceStore,
	idGen types.IDGenerator,
	gateway payment.Gateway,
) *Engine {
	return &Engine{
		priceStore:  priceStore,
		pnlCalc:    pnl.NewCalculator(priceStore),
		generator:  settlement.NewGenerator(idGen),
		processor:  payment.NewProcessor(gateway),
		idGen:      idGen,
		instruments: make(map[string]types.InstrumentConfig),
		cycles:     make(map[string]*types.SettlementCycle),
	}
}

// SetDVPCoordinator sets the DVP coordinator for physical delivery settlement.
func (e *Engine) SetDVPCoordinator(coord *dvp.DVPCoordinator) {
	e.dvpCoord = coord
}

// RegisterInstrument registers an instrument's settlement configuration.
// Instruments without explicit registration default to cash-settled.
func (e *Engine) RegisterInstrument(config types.InstrumentConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.instruments[config.InstrumentID] = config
}

// getInstrumentConfig returns the config for an instrument, defaulting to
// cash-settled. It takes the read lock so it is safe for concurrent use with
// RegisterInstrument (which writes e.instruments under the write lock).
func (e *Engine) getInstrumentConfig(instrumentID string) types.InstrumentConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lookupInstrumentConfig(instrumentID)
}

// lookupInstrumentConfig returns the config for an instrument, defaulting to
// cash-settled. The caller MUST hold e.mu (read or write); this exists so
// cycle methods that already hold the write lock can read configs without a
// non-reentrant RLock deadlock.
func (e *Engine) lookupInstrumentConfig(instrumentID string) types.InstrumentConfig {
	if cfg, ok := e.instruments[instrumentID]; ok {
		return cfg
	}
	return types.InstrumentConfig{
		InstrumentID: instrumentID,
		Type:         types.InstrumentCashSettled,
	}
}

// SetCycleHandler sets a callback invoked when a settlement cycle completes.
// The handler field is guarded by e.mu so it may be set concurrently with
// cycle execution without a data race.
func (e *Engine) SetCycleHandler(h CycleHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = h
}

// RunSettlementCycle executes a full daily settlement cycle:
// 1. Fetch all positions
// 2. Value positions at settlement prices
// 3. Calculate daily P&L (variation margin)
// 4. Generate net settlement instructions per participant
// 5. Submit payments via the payment gateway
func (e *Engine) RunSettlementCycle(cycleID string, settleDate time.Time, positions []types.Position) (*types.SettlementCycle, error) {
	cycle, snapshot, handler, err := e.runSettlementCycleLocked(cycleID, settleDate, positions)

	// Invoke the cycle handler OUTSIDE the critical section. handler is nil on
	// the early P&L-failure path (matching the previous behavior where the
	// handler only fired once a cycle reached completion).
	if handler != nil {
		handler(snapshot)
	}

	return cycle, err
}

func (e *Engine) runSettlementCycleLocked(cycleID string, settleDate time.Time, positions []types.Position) (*types.SettlementCycle, types.SettlementCycle, CycleHandler, error) {
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
		return cycle, types.SettlementCycle{}, nil, err
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

	return cycle, *cycle, e.handler, nil
}

// RunMultiInstrumentCycle executes a settlement cycle across ALL active instruments.
// For each instrument: fetches settlement price, calculates P&L per position,
// aggregates net settlement amounts per participant across all instruments,
// and uses DVPCoordinator for physical delivery instruments.
func (e *Engine) RunMultiInstrumentCycle(
	cycleID string,
	settleDate time.Time,
	positions []types.Position,
	deliveryReceipts map[string][]types.DeliveryReceipt, // instrumentID -> receipts
) (*types.SettlementCycle, *types.MultiInstrumentResult, error) {
	cycle, multiResult, snapshot, handler, err := e.runMultiInstrumentCycleLocked(cycleID, settleDate, positions, deliveryReceipts)

	// Invoke the cycle handler OUTSIDE the critical section.
	if handler != nil {
		handler(snapshot)
	}

	return cycle, multiResult, err
}

func (e *Engine) runMultiInstrumentCycleLocked(
	cycleID string,
	settleDate time.Time,
	positions []types.Position,
	deliveryReceipts map[string][]types.DeliveryReceipt,
) (*types.SettlementCycle, *types.MultiInstrumentResult, types.SettlementCycle, CycleHandler, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cycle := &types.SettlementCycle{
		CycleID:    cycleID,
		SettleDate: settleDate,
		Status:     types.CycleStatusValuing,
		StartedAt:  time.Now(),
	}
	e.cycles[cycleID] = cycle

	// Group positions by instrument
	positionsByInstrument := make(map[string][]types.Position)
	for _, pos := range positions {
		positionsByInstrument[pos.InstrumentID] = append(positionsByInstrument[pos.InstrumentID], pos)
	}

	multiResult := &types.MultiInstrumentResult{
		CycleID:              cycleID,
		InstrumentResults:    make(map[string]*types.InstrumentSettlementResult),
		NetParticipantAmounts: make(map[string]types.Decimal),
	}

	var allPnLRecords []types.PnLRecord
	hasError := false

	// Process each instrument
	for instrumentID, instrPositions := range positionsByInstrument {
		instrConfig := e.lookupInstrumentConfig(instrumentID)
		instrResult := &types.InstrumentSettlementResult{
			InstrumentID:   instrumentID,
			InstrumentType: instrConfig.Type,
		}

		// Calculate P&L for this instrument's positions
		pnlRecords, err := e.pnlCalc.CalculateBatch(instrPositions, settleDate)
		if err != nil {
			instrResult.Error = fmt.Sprintf("P&L calculation failed: %v", err)
			multiResult.InstrumentResults[instrumentID] = instrResult
			hasError = true
			continue
		}
		instrResult.PnLRecords = pnlRecords
		allPnLRecords = append(allPnLRecords, pnlRecords...)

		// Generate per-instrument instructions for DVP coordination
		instrInstructions := e.generator.Generate(cycleID, pnlRecords)
		payIn, payOut := settlement.Totals(instrInstructions)
		instrResult.PayIn = payIn
		instrResult.PayOut = payOut

		// Handle DVP for physical delivery instruments
		if instrConfig.Type == types.InstrumentPhysicalDelivery && e.dvpCoord != nil {
			receipts := deliveryReceipts[instrumentID]
			dvpResult := e.dvpCoord.CoordinateSettlement(cycleID, instrConfig, instrInstructions, receipts)
			instrResult.DVPResult = dvpResult
			if dvpResult.Status != types.DVPSucceeded {
				instrResult.Error = fmt.Sprintf("DVP failed: %s", dvpResult.Error)
				hasError = true
			}
		}

		multiResult.InstrumentResults[instrumentID] = instrResult
	}

	cycle.Status = types.CycleStatusCalculated
	cycle.PnLRecords = allPnLRecords

	// Aggregate net amounts per participant across ALL instruments
	for _, rec := range allPnLRecords {
		current := multiResult.NetParticipantAmounts[rec.ParticipantID]
		multiResult.NetParticipantAmounts[rec.ParticipantID] = current.Add(rec.VariationMargin)
	}

	// Generate aggregated settlement instructions across all instruments
	instructions := e.generator.Generate(cycleID, allPnLRecords)
	payIn, payOut := settlement.Totals(instructions)
	cycle.TotalPayIn = payIn
	cycle.TotalPayOut = payOut
	multiResult.AggregatedPayIn = payIn
	multiResult.AggregatedPayOut = payOut

	// Process payments
	cycle.Status = types.CycleStatusSettling
	processed := e.processor.ProcessAll(instructions)
	cycle.Instructions = processed

	// Determine final status
	allConfirmed := true
	for _, inst := range processed {
		if inst.Status != types.InstructionConfirmed {
			allConfirmed = false
			break
		}
	}

	if allConfirmed && !hasError {
		cycle.Status = types.CycleStatusCompleted
	} else {
		cycle.Status = types.CycleStatusFailed
		if hasError {
			cycle.Error = "one or more instruments had settlement errors"
		} else {
			cycle.Error = "one or more payment instructions failed"
		}
	}
	cycle.CompletedAt = time.Now()

	return cycle, multiResult, *cycle, e.handler, nil
}

// GetCycle returns a settlement cycle by ID.
func (e *Engine) GetCycle(cycleID string) (types.SettlementCycle, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	c, ok := e.cycles[cycleID]
	if !ok {
		return types.SettlementCycle{}, false
	}
	return *c, true
}

// GetAllCycles returns all settlement cycles.
func (e *Engine) GetAllCycles() []types.SettlementCycle {
	e.mu.RLock()
	defer e.mu.RUnlock()

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
